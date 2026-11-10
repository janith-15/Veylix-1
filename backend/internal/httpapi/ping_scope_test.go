package httpapi_test

import (
	"testing"

	"veylix/backend/internal/httpapi"
)

// TestPingScopeFromQuery pins the `source` filter that D-095 added to
// `/ping/history`.
//
// The rule that matters is the REFUSAL case. A filter that silently returned
// everything after a typo would tell a reader "this source has data" when in
// truth the value they typed does not exist — the same class of lie as a range
// that quietly falls back to 30 days (D-026.2).
func TestPingScopeFromQuery(t *testing.T) {
	cases := []struct {
		raw     string
		want    string
		wantOK  bool
		because string
	}{
		{"", "", true, "no filter means every source, which is the pre-D-095 behaviour"},
		{"all", "", true, "an explicit 'all' is the same as no filter"},
		{"ALL", "", true, "the value is case-insensitive, like every other query param"},
		{" all ", "", true, "surrounding whitespace is trimmed"},
		{"server_reachability", "server_reachability", true,
			"the one source the product actually records"},
		{"SERVER_REACHABILITY", "server_reachability", true, "case-insensitive"},
		{"device", "", false, "a client-side probe is not something Veylix records yet"},
		{"srever_reachability", "", false, "a typo must be refused, never ignored"},
		{"*", "", false, "there is no wildcard syntax; inventing one would be a silent match-all"},
	}
	for _, c := range cases {
		got, ok := httpapi.PingScopeFromQuery(c.raw)
		if ok != c.wantOK || got != c.want {
			t.Errorf("PingScopeFromQuery(%q) = (%q, %v), want (%q, %v) — %s",
				c.raw, got, ok, c.want, c.wantOK, c.because)
		}
	}
}
