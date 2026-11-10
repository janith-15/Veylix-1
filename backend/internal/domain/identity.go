// Package domain holds business rules and invariants that must not live in
// handlers or repositories (docs/ARCHITECTURE.md §1).
package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// PanelEmail returns the panel-side client identity for a local VPN instance.
//
// D-012: the pair (panel_email, panel_sub_id) is the idempotency key. It is
// derived deterministically from the local id, so a retried provisioning can
// never create a second panel client — the database UNIQUE constraint and the
// panel's own duplicate check both reject it.
//
// D-034 replaces this with a human-readable name derived from the customer
// (`<client email>_<kind>_<slot>`) for every NEW VPN; this helper stays as the
// legacy form so rows created before the change keep working.
func PanelEmail(localID int64) string {
	return fmt.Sprintf("vyx-%d", localID)
}

// PanelSubID mirrors PanelEmail for the subscription identifier.
func PanelSubID(localID int64) string {
	return fmt.Sprintf("vyx-%d", localID)
}

// PanelBase is the stable prefix of a slot-based panel name (D-034.1).
//
//	PanelBase("nimal@gmail.com", "paid") == "nimal@gmail.com_paid"
//
// The panel's email column is unique, so the `_<n>` suffix is what allows one
// customer to hold several VPNs of the same kind on one server.
func PanelBase(clientEmail, kind string) string {
	email := PanelNameSlug(clientEmail)
	return email + "_" + kind
}

// PanelNameSlug makes an arbitrary client e-mail safe as the prefix of a panel
// e-mail: trimmed, lower-cased, spaces to dashes, and any character the panel
// or a URL path would choke on folded to a dash.
//
// An empty result falls back to "client" so a name is never just "_paid".
func PanelNameSlug(email string) string {
	lower := strings.ToLower(strings.TrimSpace(email))
	if lower == "" {
		return "client"
	}
	var b strings.Builder
	b.Grow(len(lower))
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '@', r == '.', r == '-', r == '_', r == '+':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "client"
	}
	return out
}

// PanelSlotNext is the number that follows a slot name like
// "nimal@gmail.com_paid_2" (→ 3). It returns 1 when the name carries no slot.
//
// D-034.2: slots only move forward. A revoked `_paid_2` is never re-issued, so
// the next paid VPN a customer buys is `_paid_3` — which is what makes a panel
// e-mail a stable audit trail rather than a recycled label.
func PanelSlotNext(name string) int {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return 1
	}
	idx := strings.LastIndex(trimmed, "_")
	if idx < 0 || idx == len(trimmed)-1 {
		return 1
	}
	n, err := strconv.Atoi(trimmed[idx+1:])
	if err != nil || n < 0 {
		return 1
	}
	return n + 1
}

// PanelSlotsRange is how far PanelFindNextFreeSlot looks for a free slot.
//
// A customer with thousands of historical VPNs would otherwise make this
// unbounded work on a 1 vCPU host; 5000 slots is far beyond any real account
// and still bounded.
const PanelSlotsRange = 5000

// HasSlotPrefix reports whether name begins with `<base>_` — i.e. it is already
// one of this customer/kind's slots rather than somebody else's name.
func HasSlotPrefix(name, base string) bool {
	return strings.HasPrefix(strings.TrimSpace(name), base+"_")
}

// PanelFindNextFreeSlot returns the first free `<base>_<n>` at or after `from`,
// using `taken` to skip names the panel or Veylix already holds.
//
// Returning an error instead of silently looping is deliberate: a caller must
// never fall back to a name it did not check, because that name is the only
// thing tying a VPN row to a panel client.
func PanelFindNextFreeSlot(base string, from int, taken func(string) bool) (string, error) {
	if from < 1 {
		from = 1
	}
	for n := from; n < from+PanelSlotsRange; n++ {
		name := fmt.Sprintf("%s_%d", base, n)
		if !taken(name) {
			return name, nil
		}
	}
	return "", fmt.Errorf("no free panel slot for %s after %d", base, from)
}

// NewOutboxKey builds the idempotency key for a remote operation.
func NewOutboxKey(op string, serverID int64, panelEmail string) string {
	return fmt.Sprintf("%s:%d:%s", op, serverID, panelEmail)
}

// RandomToken returns a hex token of n random bytes, used for opaque handles
// such as temporary transaction ids in logs.
func RandomToken(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return strings.Repeat("0", n*2)
	}
	return hex.EncodeToString(buf)
}
