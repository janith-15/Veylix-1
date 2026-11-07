package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"veylix/backend/internal/integrations"
)

func tagPtr(s string) *string { return &s }

// The drill-down header answers the four questions the owner asked first: how
// many free, how many premium, who holds them, and what is left.
func TestServerClientSummary(t *testing.T) {
	rows := []integrations.PanelClientView{
		{PanelEmail: "a_free_1", Tag: tagPtr("FREE"), Assigned: true, UsagePct: 20, DaysLeft: 40, Enable: true},
		{PanelEmail: "b_free_2", Tag: tagPtr("FREE"), Assigned: true, UsagePct: 100, DaysLeft: 10, Enable: true},
		{PanelEmail: "c_paid_1", Tag: tagPtr("PREMIUM"), Assigned: true, UsagePct: 5, DaysLeft: 3, Enable: true},
		{PanelEmail: "d_paid_1", Tag: tagPtr("PREMIUM"), Assigned: false, UsagePct: 0, DaysLeft: 90, Enable: false},
		{PanelEmail: "e_untagged", Tag: nil, Assigned: false, UsagePct: 0, ExpiryTime: 1, DaysLeft: 0, Enable: true},
	}
	got := summarisePanelClients(rows)

	if got.Total != 5 {
		t.Errorf("total = %d, want 5", got.Total)
	}
	if got.Free != 2 || got.Premium != 2 || got.Untagged != 1 {
		t.Errorf("free/premium/untagged = %d/%d/%d, want 2/2/1", got.Free, got.Premium, got.Untagged)
	}
	if got.Assigned != 3 || got.Unassigned != 2 {
		t.Errorf("assigned/unassigned = %d/%d, want 3/2", got.Assigned, got.Unassigned)
	}
	if got.Exhausted != 1 {
		t.Errorf("exhausted = %d, want 1 (b_free_2 is at 100%%)", got.Exhausted)
	}
	if got.ExpiringSoon != 1 {
		t.Errorf("expiring_soon = %d, want 1 (c_paid_1 has 3 days)", got.ExpiringSoon)
	}
	if got.Expired != 1 {
		t.Errorf("expired = %d, want 1 (e_untagged)", got.Expired)
	}
	if got.Disabled != 1 {
		t.Errorf("disabled = %d, want 1", got.Disabled)
	}
}

// Every filter the owner asked for, and a 422 for a typo — so a misspelled
// filter can never look like "nothing matched".
func TestServerClientFilters(t *testing.T) {
	rows := []integrations.PanelClientView{
		{PanelEmail: "a_free_1", Tag: tagPtr("FREE"), Assigned: true, UsagePct: 10, DaysLeft: 30,
			ExpiryTime: 1, OwnerName: "Nimal", OwnerEmail: "nimal@gmail.com", OfferName: "Free 10"},
		{PanelEmail: "b_paid_1", Tag: tagPtr("PREMIUM"), Assigned: true, UsagePct: 100, DaysLeft: 2,
			ExpiryTime: 1, OwnerName: "Kamal", OwnerEmail: "kamal@gmail.com", OfferName: "Premium 50"},
		{PanelEmail: "c_untagged"},
	}

	cases := []struct {
		name  string
		query string
		want  []string
		bad   bool
	}{
		{"no filter", "", []string{"a_free_1", "b_paid_1", "c_untagged"}, false},
		{"free", "tag=free", []string{"a_free_1"}, false},
		{"premium", "tag=premium", []string{"b_paid_1"}, false},
		{"untagged", "tag=untagged", []string{"c_untagged"}, false},
		{"assigned", "assigned=1", []string{"a_free_1", "b_paid_1"}, false},
		{"unassigned", "assigned=0", []string{"c_untagged"}, false},
		{"exhausted", "usage=exhausted", []string{"b_paid_1"}, false},
		{"expiring", "usage=expiring", []string{"b_paid_1"}, false},
		{"search by owner name", "q=nimal", []string{"a_free_1"}, false},
		{"search by email", "q=kamal@", []string{"b_paid_1"}, false},
		{"search by offer", "q=premium%2050", []string{"b_paid_1"}, false},
		{"bad tag", "tag=gold", nil, true},
		{"bad assigned", "assigned=maybe", nil, true},
		{"bad usage", "usage=full", nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/api/servers/1/panel-clients?"+tc.query, nil)
			kept, ok := filterPanelClients(rec, r, rows)

			if tc.bad {
				if ok {
					t.Fatal("an unknown value must be refused, not silently ignored")
				}
				if rec.Code != http.StatusUnprocessableEntity {
					t.Errorf("status = %d, want 422", rec.Code)
				}
				return
			}
			if !ok {
				t.Fatalf("filter %q was refused (status %d)", tc.query, rec.Code)
			}
			if len(kept) != len(tc.want) {
				t.Fatalf("kept %d rows, want %d: %+v", len(kept), len(tc.want), kept)
			}
			for i, email := range tc.want {
				if kept[i].PanelEmail != email {
					t.Errorf("row %d = %q, want %q", i, kept[i].PanelEmail, email)
				}
			}
		})
	}
}
