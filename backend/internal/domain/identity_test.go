package domain

import "testing"

// D-034.1: the panel e-mail names the customer, not an opaque row id.
func TestPanelEmailIsClientAndKind(t *testing.T) {
	cases := []struct {
		email, kind, want string
	}{
		{"nimal@gmail.com", "free", "nimal@gmail.com_free"},
		{"Nimal@Gmail.COM", "paid", "nimal@gmail.com_paid"},
		{"  kamal@example.com ", "paid", "kamal@example.com_paid"},
		{"a b@example.com", "free", "a-b@example.com_free"},
		{"", "paid", "client_paid"},
		{"!!@example.com", "paid", "@example.com_paid"},
	}
	for _, tc := range cases {
		if got := PanelBase(tc.email, tc.kind); got != tc.want {
			t.Errorf("PanelBase(%q, %q) = %q, want %q", tc.email, tc.kind, got, tc.want)
		}
	}
}

// D-034.3: the idempotency pair survives the readable name, because the subId is
// still derived from the local row id.
func TestPanelSubIdStaysDeterministic(t *testing.T) {
	if PanelSubID(21) != "vyx-21" {
		t.Errorf("PanelSubID(21) = %q, want vyx-21", PanelSubID(21))
	}
}

// D-034.2: slots only move forward — a revoked slot is never re-issued.
func TestPanelSlotsOnlyMoveForward(t *testing.T) {
	if got := PanelSlotNext("nimal@gmail.com_paid_1"); got != 2 {
		t.Errorf("next after _1 = %d, want 2", got)
	}
	if got := PanelSlotNext("nimal@gmail.com_paid_7"); got != 8 {
		t.Errorf("next after _7 = %d, want 8", got)
	}
	// A name with no slot starts at 1 rather than erroring.
	for _, name := range []string{"", "vyx-21", "nimal@gmail.com_paid", "nimal@gmail.com_paid_"} {
		if got := PanelSlotNext(name); got != 1 {
			t.Errorf("next after %q = %d, want 1", name, got)
		}
	}
}

// The next free slot skips names the panel or Veylix already holds, and refuses
// to invent one it could not verify.
func TestPanelFindNextFreeSlot(t *testing.T) {
	taken := map[string]bool{
		"nimal@gmail.com_paid_1": true,
		"nimal@gmail.com_paid_2": true,
		"nimal@gmail.com_paid_4": true,
	}
	got, err := PanelFindNextFreeSlot("nimal@gmail.com_paid", 1, func(n string) bool { return taken[n] })
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != "nimal@gmail.com_paid_3" {
		t.Errorf("found %q, want nimal@gmail.com_paid_3 (the first gap)", got)
	}

	// Nothing free in range ⇒ an error, never a guessed name.
	always := func(string) bool { return true }
	if _, err := PanelFindNextFreeSlot("x_paid", 1, always); err == nil {
		t.Error("an exhausted range must return an error, not a duplicate name")
	}
}
