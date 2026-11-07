package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Every method below corresponds to an endpoint listed in
// docs/3XUI_API_CONTRACT.md with a verified method + path. Nothing outside that
// list may be called (new_update.md §2).

// HealthResult is the outcome of an authenticated health check.
type HealthResult struct {
	Status       Status
	PanelVersion string
	XrayVersion  string
	XrayState    string
	CPUPct       float64
	MemCurrent   int64
	MemTotal     int64
	LatencyMS    int64
	// NetUpBytes / NetDownBytes are the panel's CUMULATIVE counters since the
	// interface came up (`netIO.up` / `netIO.down`). They are not a rate: a
	// caller that wants a rate must difference two readings, which is what
	// sync does (M6-12). Storing them as a rate would draw a lie.
	NetUpBytes   int64
	NetDownBytes int64
	// Load1 is the 1-minute load average when the panel reports one.
	Load1 float64
	// Err is set when the check could not complete. It never contains secrets.
	Err error
}

// Health performs the authenticated health check used to activate an
// integration (GET /panel/api/server/status). This replaces the simulated
// testConnection stub entirely (ledger F-002).
func (c *Client) Health(ctx context.Context) HealthResult {
	started := time.Now()
	raw, code, err := c.do(ctx, http.MethodGet, "/panel/api/server/status", nil)
	result := HealthResult{LatencyMS: time.Since(started).Milliseconds()}

	if err != nil {
		result.Status = Classify(0, err)
		result.Err = err
		return result
	}

	result.Status = Classify(code, nil)
	if code != http.StatusOK {
		result.Err = &StatusError{HTTPStatus: code, Endpoint: "/panel/api/server/status"}
		return result
	}

	obj, decErr := decodeEnvelope(raw)
	if decErr != nil {
		result.Status = StatusConnectionUnavailable
		result.Err = decErr
		return result
	}

	var st ServerStatus
	if err := json.Unmarshal(obj, &st); err != nil {
		result.Status = StatusConnectionUnavailable
		result.Err = fmt.Errorf("panel: decode server status: %w", err)
		return result
	}

	result.PanelVersion = st.PanelVersion
	result.XrayVersion = st.Xray.Version
	result.XrayState = st.Xray.State
	result.CPUPct = st.CPU
	result.MemCurrent = st.Mem.Current
	result.MemTotal = st.Mem.Total
	// The counters the panel already sent and Veylix used to drop (M6-12), so a
	// network chart has real numbers behind it instead of a zero line.
	result.NetUpBytes = st.NetIO.Up
	result.NetDownBytes = st.NetIO.Down
	if len(st.Loads) > 0 {
		result.Load1 = st.Loads[0]
	}
	return result
}

// Inbounds lists selectable inbounds via the lightweight picker projection
// (GET /panel/api/inbounds/options) — it omits settings/streamSettings/stats,
// so it stays cheap on large panels.
func (c *Client) Inbounds(ctx context.Context) ([]InboundOption, error) {
	raw, code, err := c.do(ctx, http.MethodGet, "/panel/api/inbounds/options", nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, &StatusError{HTTPStatus: code, Endpoint: "/panel/api/inbounds/options"}
	}
	obj, err := decodeEnvelope(raw)
	if err != nil {
		return nil, err
	}
	var out []InboundOption
	if err := json.Unmarshal(obj, &out); err != nil {
		return nil, fmt.Errorf("panel: decode inbounds: %w", err)
	}
	return out, nil
}

// ListClients reads one page of clients (GET /panel/api/clients/list/paged).
// Server-side filtering/pagination keeps the payload small (D-012).
func (c *Client) ListClients(ctx context.Context, page, pageSize int) (*ClientPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 200
	}
	path := "/panel/api/clients/list/paged?page=" + strconv.Itoa(page) + "&pageSize=" + strconv.Itoa(pageSize)

	raw, code, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, &StatusError{HTTPStatus: code, Endpoint: "/panel/api/clients/list/paged"}
	}
	obj, err := decodeEnvelope(raw)
	if err != nil {
		return nil, err
	}
	var out ClientPage
	if err := json.Unmarshal(obj, &out); err != nil {
		return nil, fmt.Errorf("panel: decode client page: %w", err)
	}
	return &out, nil
}

// ErrClientNotFound is the panel's way of saying a client does not exist.
//
// 3X-UI answers GET /clients/get/{email} for an unknown client with HTTP 200 and
// {"success":false,"msg":"Obtain (record not found)"} — NOT a 404. Treating that
// as a generic failure made "the client is gone" indistinguishable from "the
// panel is broken", so verified absence (D-024.5) could never succeed.
var ErrClientNotFound = errors.New("panel: client not found")

// ClientExists performs the fresh read used to resolve an ambiguous write
// (new_update.md §5) and to verify absence (D-024.5).
//
// Returns (true, nil) only when the panel positively reports the client.
// Returns (false, nil) only when the panel positively says it does not exist —
// either an HTTP 404 or the panel's own "record not found" envelope. Any other
// failure returns an error, so the caller keeps the operation unresolved rather
// than assuming either outcome.
func (c *Client) ClientExists(ctx context.Context, email string) (bool, error) {
	seg, err := emailPathSegment(email)
	if err != nil {
		return false, err
	}
	raw, code, err := c.do(ctx, http.MethodGet, "/panel/api/clients/get/"+seg, nil)
	if err != nil {
		return false, err
	}
	switch code {
	case http.StatusOK:
		obj, decErr := decodeEnvelope(raw)
		if decErr != nil {
			// The panel wraps "not found" in a failure envelope with HTTP 200.
			if isRecordNotFound(raw) {
				return false, nil
			}
			return false, decErr
		}
		return len(obj) > 0 && string(obj) != "null", nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, &StatusError{HTTPStatus: code, Endpoint: "/panel/api/clients/get"}
	}
}

// isRecordNotFound reports whether a 200 response is the panel's "no such
// client" answer. Matched on the message because the envelope carries no code;
// the check is deliberately narrow (an explicit "not found"/"record not found")
// so a genuine failure is never mistaken for absence.
func isRecordNotFound(raw []byte) bool {
	var env envelope[json.RawMessage]
	if err := json.Unmarshal(raw, &env); err != nil {
		return false
	}
	if env.Success {
		return false
	}
	msg := strings.ToLower(env.Msg)
	return strings.Contains(msg, "record not found") || strings.Contains(msg, "not found")
}

// AddClient creates a client and attaches it to inbounds
// (POST /panel/api/clients/add — body {client, inboundIds}).
//
// This is a NON-idempotent write, so it is never retried here. The caller owns
// reconciliation: on a transport error it must call ClientExists before any
// retry (D-012).
func (c *Client) AddClient(ctx context.Context, req AddClientRequest) error {
	raw, code, err := c.do(ctx, http.MethodPost, "/panel/api/clients/add", req)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return &StatusError{HTTPStatus: code, Endpoint: "/panel/api/clients/add"}
	}
	_, err = decodeEnvelope(raw)
	return err
}

// DeleteClient removes a client (POST /panel/api/clients/del/{email}).
// Only a verified success may move a VPN to REVOKED (REQUIREMENTS §7).
func (c *Client) DeleteClient(ctx context.Context, email string) error {
	seg, err := emailPathSegment(email)
	if err != nil {
		return err
	}
	raw, code, err := c.do(ctx, http.MethodPost, "/panel/api/clients/del/"+seg, nil)
	if err != nil {
		return err
	}
	switch code {
	case http.StatusOK:
		_, err := decodeEnvelope(raw)
		return err
	case http.StatusNotFound:
		// Already absent — the desired end state already holds.
		return nil
	default:
		return &StatusError{HTTPStatus: code, Endpoint: "/panel/api/clients/del"}
	}
}

// SetClientEnabled suspends or resumes a client without deleting it
// (POST /panel/api/clients/bulkEnable | bulkDisable) — "suspend" must always be
// reversible.
func (c *Client) SetClientEnabled(ctx context.Context, email string, enable bool) error {
	path := "/panel/api/clients/bulkDisable"
	if enable {
		path = "/panel/api/clients/bulkEnable"
	}
	body := map[string][]string{"emails": {email}}

	raw, code, err := c.do(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return &StatusError{HTTPStatus: code, Endpoint: path}
	}
	_, err = decodeEnvelope(raw)
	return err
}

// GetClient reads one client's full record (GET /panel/api/clients/get/{email}).
//
// Needed because clients/update REPLACES the row instead of patching it: every
// edit is a read → modify → write → re-read cycle. Returns (nil, nil) when the
// panel reports the client absent, so an absent client is never confused with
// a transport failure.
func (c *Client) GetClient(ctx context.Context, email string) (*PanelClient, error) {
	seg, err := emailPathSegment(email)
	if err != nil {
		return nil, err
	}
	raw, code, err := c.do(ctx, http.MethodGet, "/panel/api/clients/get/"+seg, nil)
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, nil
	}
	if code != http.StatusOK {
		return nil, &StatusError{HTTPStatus: code, Endpoint: "/panel/api/clients/get"}
	}
	obj, err := decodeEnvelope(raw)
	if err != nil {
		return nil, err
	}
	if len(obj) == 0 || string(obj) == "null" {
		return nil, nil
	}

	// The panel nests the record: {"obj":{"client":{...},"inboundIds":[...],
	// "externalLinks":[],"usedTraffic":N}}. Accept the nested shape, the bare
	// record, and a single-element array, because the wrapper has changed
	// between panel versions — decoding an empty struct would silently report a
	// client with no limits.
	var wrapped struct {
		Client     *PanelClient `json:"client"`
		InboundIDs []int        `json:"inboundIds"`
	}
	if err := json.Unmarshal(obj, &wrapped); err == nil && wrapped.Client != nil {
		rec := *wrapped.Client
		rec.InboundIDs = wrapped.InboundIDs
		if rec.Email == "" {
			rec.Email = email
		}
		return &rec, nil
	}

	var rec PanelClient
	if err := json.Unmarshal(obj, &rec); err != nil {
		var arr []PanelClient
		if err2 := json.Unmarshal(obj, &arr); err2 != nil || len(arr) == 0 {
			return nil, fmt.Errorf("panel: decode client: %w", err)
		}
		rec = arr[0]
	}
	if rec.Email == "" {
		rec.Email = email
	}
	return &rec, nil
}

// UpdateClient replaces a client's record (POST /panel/api/clients/update/{email}).
//
// DANGER: this is a REPLACE, not a patch — the panel overwrites every editable
// field with what is sent, so the caller must pass the complete record (read it
// with GetClient first and use SpecFromPanelClient). The panel also applies the
// change to each attached inbound concurrently and independently, so a
// success:false answer can still have partially applied. Callers must confirm
// with a fresh GetClient rather than trusting the response (new_update.md §5).
func (c *Client) UpdateClient(ctx context.Context, email string, spec ClientSpec) error {
	seg, err := emailPathSegment(email)
	if err != nil {
		return err
	}
	// The path identifies the row; the body must agree with it.
	spec.Email = email

	raw, code, err := c.do(ctx, http.MethodPost, "/panel/api/clients/update/"+seg, spec)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return &StatusError{HTTPStatus: code, Endpoint: "/panel/api/clients/update"}
	}
	_, err = decodeEnvelope(raw)
	return err
}

// Links returns the connection URLs the panel issued for a client
// (GET /panel/api/clients/links/{email}).
//
// This is the ONLY permitted source of connection material
// (REQUIREMENTS §3.5): links are never synthesized, because a link that looks
// valid but cannot connect is worse than no link at all. An inactive or absent
// client yields an empty slice.
func (c *Client) Links(ctx context.Context, email string) ([]string, error) {
	seg, err := emailPathSegment(email)
	if err != nil {
		return nil, err
	}
	raw, code, err := c.do(ctx, http.MethodGet, "/panel/api/clients/links/"+seg, nil)
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, nil
	}
	if code != http.StatusOK {
		return nil, &StatusError{HTTPStatus: code, Endpoint: "/panel/api/clients/links"}
	}
	obj, err := decodeEnvelope(raw)
	if err != nil {
		return nil, err
	}
	if len(obj) == 0 || string(obj) == "null" {
		return nil, nil
	}
	var links []string
	if err := json.Unmarshal(obj, &links); err != nil {
		return nil, fmt.Errorf("panel: decode links: %w", err)
	}
	return links, nil
}

// Hwids lists the devices registered for a client
// (POST /panel/api/clients/hwids/{email} — a POST that only reads).
//
// The panel exposes a short fingerprint only, never the full hash.
func (c *Client) Hwids(ctx context.Context, email string) ([]Hwid, error) {
	seg, err := emailPathSegment(email)
	if err != nil {
		return nil, err
	}
	raw, code, err := c.do(ctx, http.MethodPost, "/panel/api/clients/hwids/"+seg, map[string]any{})
	if err != nil {
		return nil, err
	}
	if code == http.StatusNotFound {
		return nil, nil
	}
	if code != http.StatusOK {
		return nil, &StatusError{HTTPStatus: code, Endpoint: "/panel/api/clients/hwids"}
	}
	obj, err := decodeEnvelope(raw)
	if err != nil {
		return nil, err
	}
	if len(obj) == 0 || string(obj) == "null" {
		return nil, nil
	}
	var devices []Hwid
	if err := json.Unmarshal(obj, &devices); err != nil {
		return nil, fmt.Errorf("panel: decode hwids: %w", err)
	}
	return devices, nil
}

// DeleteHwid frees one device slot (DELETE /panel/api/clients/hwids/{email}/{id}).
//
// A 404 means the device is already gone, which is the desired end state.
func (c *Client) DeleteHwid(ctx context.Context, email string, id int64) error {
	seg, err := emailPathSegment(email)
	if err != nil {
		return err
	}
	path := "/panel/api/clients/hwids/" + seg + "/" + strconv.FormatInt(id, 10)
	raw, code, err := c.do(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	switch code {
	case http.StatusOK:
		_, err := decodeEnvelope(raw)
		return err
	case http.StatusNotFound:
		return nil
	default:
		return &StatusError{HTTPStatus: code, Endpoint: "/panel/api/clients/hwids"}
	}
}
