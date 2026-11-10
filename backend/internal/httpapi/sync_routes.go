package httpapi

import (
	"context"
	"net/http"
	"time"

	"veylix/backend/internal/auth"
	"veylix/backend/internal/sync"
)

// SyncRunner is the on-demand sync the Super Admin can trigger.
//
// The background scheduler runs on an interval (5 minutes by default), and
// `GET /api/sync/status` reports how old the last pass is — but there was no way
// to ask for a pass NOW. An operator who has just changed something in 3X-UI had
// to wait, and could not tell whether waiting was working. RunOnce reports the
// same Result the scheduler records, so the answer says what actually happened
// rather than "started".
type SyncRunner interface {
	RunOnce(ctx context.Context, serverIDs []int64) sync.Result
}

// syncRunResponse is the wire shape of a manual sync pass.
//
// `sync.Result` is an internal type with no JSON tags, so encoding it directly
// produced {"Servers":[{"ServerID":11,...}]} — PascalCase keys that no client
// could read with the conventions the rest of the API uses. The response is
// therefore mapped explicitly, and the counts are named the way the console
// labels them ("absent", "unverified", …) rather than mirroring Go field names.
type syncRunResponse struct {
	Servers []syncServerResult `json:"servers"`
	// DurationMs, not a Go duration string: every other duration on the wire is
	// milliseconds, so a client never has to parse "1.234s".
	DurationMs int64 `json:"duration_ms"`
}

type syncServerResult struct {
	ServerID   int64 `json:"server_id"`
	Clients    int   `json:"clients"`
	Snapshots  int   `json:"snapshots"`
	Resets     int   `json:"resets"`
	Depleted   int   `json:"depleted"`
	Expired    int   `json:"expired"`
	Absent     int   `json:"absent"`
	Unverified int   `json:"unverified"`
	Mirrored   int   `json:"mirrored"`
	// Error says why THIS server failed while the others succeeded: a pass is not
	// all-or-nothing, and reporting only the totals would hide a broken panel.
	Error string `json:"error,omitempty"`
}

// toWire maps an internal Result onto the public shape.
func toWire(res sync.Result) syncRunResponse {
	out := syncRunResponse{
		Servers:    make([]syncServerResult, 0, len(res.Servers)),
		DurationMs: res.Duration.Milliseconds(),
	}
	for _, s := range res.Servers {
		row := syncServerResult{
			ServerID: s.ServerID, Clients: s.Clients, Snapshots: s.Snapshots,
			Resets: s.Resets, Depleted: s.Depleted, Expired: s.Expired,
			Absent: s.Absent, Unverified: s.Unverified, Mirrored: s.Mirrored,
		}
		if s.Err != nil {
			row.Error = s.Err.Error()
		}
		out.Servers = append(out.Servers, row)
	}
	return out
}

// SyncRoutes registers the manual sync trigger (D-024 Stage C: reads only, never
// writes to the panel).
//
//	super_admin only — a reseller's scope is one server, and a shared trigger
//	would let them drive the whole fleet's panel reads.
func SyncRoutes(mux *http.ServeMux, runner SyncRunner, authed func(http.Handler) http.Handler) {
	if runner == nil {
		return
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// An optional {"server_ids": [..]} narrows the pass. An empty body means
		// "every active server", which is what the button in the console sends.
		var in struct {
			ServerIDs []int64 `json:"server_ids"`
		}
		if r.ContentLength > 0 {
			if err := DecodeJSON(r, &in); err != nil {
				WriteError(w, http.StatusBadRequest, "invalid_body", "request body is not valid JSON")
				return
			}
		}

		// A sync pass reads every panel; it must not be cut off at the default
		// request timeout while a slow server is still answering.
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()

		res := runner.RunOnce(ctx, in.ServerIDs)
		WriteJSON(w, http.StatusOK, toWire(res))
	})

	mux.Handle("POST /api/sync/run", authed(RequirePermission(auth.PermServerViewAll)(handler)))
}
