package domain

import (
	"errors"
	"testing"
)

// An outage may only produce SERVER_UNAVAILABLE — never REVOKED
// (REQUIREMENTS §7, Bible §7).
func TestSeverOutageNeverProducesRevoked(t *testing.T) {
	for _, from := range []VPNStatus{
		VPNPending, VPNActive, VPNTrafficDepleted, VPNExpired, VPNSuspended, VPNServerUnavailable,
	} {
		if got := SeverOutage(from); got == VPNRevoked {
			t.Errorf("SeverOutage(%s) = REVOKED, must never happen", from)
		}
	}
}

// Terminal states stay terminal.
func TestSeverOutageKeepsTerminalStates(t *testing.T) {
	for _, from := range []VPNStatus{VPNRevoked, VPNRemovedFromCatalog} {
		if got := SeverOutage(from); got != from {
			t.Errorf("SeverOutage(%s) = %s, want unchanged", from, got)
		}
	}
}

func TestTransitions(t *testing.T) {
	cases := []struct {
		from, to VPNStatus
		ok       bool
	}{
		{VPNPending, VPNActive, true},
		{VPNActive, VPNSuspended, true},
		{VPNSuspended, VPNActive, true},
		{VPNActive, VPNRevocationPending, true},
		{VPNRevocationPending, VPNRevoked, true},
		// Revoked is final: re-activation needs a new assignment.
		{VPNRevoked, VPNActive, false},
		{VPNRevoked, VPNSuspended, false},
		// Pending cannot skip verification straight to suspended.
		{VPNPending, VPNSuspended, false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.ok {
			t.Errorf("CanTransition(%s → %s) = %v, want %v", c.from, c.to, got, c.ok)
		}
	}
}

func TestMustTransitionReturnsSentinel(t *testing.T) {
	if err := MustTransition(VPNRevoked, VPNActive); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("expected ErrIllegalTransition, got %v", err)
	}
	if err := MustTransition(VPNPending, VPNActive); err != nil {
		t.Fatalf("legal transition returned %v", err)
	}
}

func TestIsTerminal(t *testing.T) {
	if !VPNRevoked.IsTerminal() {
		t.Error("REVOKED must be terminal")
	}
	if VPNPending.IsTerminal() {
		t.Error("PENDING must not be terminal")
	}
}

// The deterministic identity is what makes retries safe (D-012).
func TestPanelIdentityIsDeterministic(t *testing.T) {
	if PanelEmail(42) != "vyx-42" {
		t.Errorf("PanelEmail = %q", PanelEmail(42))
	}
	if PanelEmail(42) != PanelEmail(42) {
		t.Error("PanelEmail must be stable")
	}
	if PanelEmail(42) == PanelEmail(43) {
		t.Error("different VPNs must have different panel identities")
	}
	if PanelSubID(42) != "vyx-42" {
		t.Errorf("PanelSubID = %q", PanelSubID(42))
	}
}

func TestOutboxKeyShape(t *testing.T) {
	got := NewOutboxKey("CREATE_CLIENT", 7, "vyx-3")
	if got != "CREATE_CLIENT:7:vyx-3" {
		t.Fatalf("key = %q", got)
	}
}

func TestQuotaReleasingStatuses(t *testing.T) {
	got := QuotaReleasingStatuses()
	want := map[string]bool{"EXPIRED": true, "REVOKED": true}
	for _, s := range got {
		if want[s] {
			delete(want, s)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing releasing statuses: %v", want)
	}
}
