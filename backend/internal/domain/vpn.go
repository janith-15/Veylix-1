// Package domain holds business rules and invariants that must not live in
// handlers or repositories (docs/ARCHITECTURE.md §1).
package domain

import "errors"

// VPN lifecycle states (REQUIREMENTS §7). Kept as typed strings so a transition
// table — not scattered if-statements — decides what is legal.
type VPNStatus string

const (
	VPNPending            VPNStatus = "PENDING"
	VPNActive             VPNStatus = "ACTIVE"
	VPNTrafficDepleted    VPNStatus = "TRAFFIC_DEPLETED"
	VPNExpired            VPNStatus = "EXPIRED"
	VPNSuspended          VPNStatus = "SUSPENDED"
	VPNServerUnavailable  VPNStatus = "SERVER_UNAVAILABLE"
	VPNRevocationPending  VPNStatus = "REVOCATION_PENDING"
	VPNRevoked            VPNStatus = "REVOKED"
	VPNRemovedFromCatalog VPNStatus = "REMOVED_FROM_CATALOG"
	// VPNReleased is the 3-day undo window of D-027. It exists so a released VPN
	// stops being reported as SERVER_UNAVAILABLE: the panel client was disabled by
	// the release, sync then found it absent, and the owner was told "the server
	// failed" about a VPN they had given up on purpose. Released means "set aside,
	// restorable"; unavailable means "something broke".
	VPNReleased VPNStatus = "RELEASED"
)

// ErrIllegalTransition is returned when a state change is not permitted.
var ErrIllegalTransition = errors.New("domain: illegal status transition")

// transitions is the single source of truth for legal VPN state changes.
//
// Invariants encoded here (REQUIREMENTS §7, Bible §7):
//   - a remote outage may only produce SERVER_UNAVAILABLE — never REVOKED;
//   - REVOKED is reachable only from states where the client still exists;
//   - a revoked VPN can never become ACTIVE again (a new assignment is needed).
var transitions = map[VPNStatus][]VPNStatus{
	VPNPending:           {VPNActive, VPNServerUnavailable, VPNRevocationPending, VPNRevoked},
	VPNActive:            {VPNTrafficDepleted, VPNExpired, VPNSuspended, VPNServerUnavailable, VPNRevocationPending, VPNRevoked, VPNReleased},
	VPNTrafficDepleted:   {VPNActive, VPNExpired, VPNSuspended, VPNRevocationPending, VPNRevoked, VPNReleased},
	VPNExpired:           {VPNActive, VPNRevocationPending, VPNRevoked, VPNReleased},
	VPNSuspended:         {VPNActive, VPNExpired, VPNRevocationPending, VPNRevoked, VPNReleased},
	VPNServerUnavailable: {VPNActive, VPNExpired, VPNRevocationPending, VPNRevoked, VPNReleased},
	// RELEASED is a resting state with one way back (restore) and one way out
	// (revoke). It deliberately has NO edge to SERVER_UNAVAILABLE: sync must not
	// be able to relabel a deliberate release as a server fault, which is the bug
	// this state was added to stop.
	VPNReleased:           {VPNActive, VPNRevocationPending, VPNRevoked},
	VPNRevocationPending:  {VPNRevoked, VPNServerUnavailable},
	VPNRevoked:            {},
	VPNRemovedFromCatalog: {},
}

// CanTransition reports whether from → to is legal.
func CanTransition(from, to VPNStatus) bool {
	for _, allowed := range transitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

// MustTransition returns an error when the transition is illegal, so callers
// cannot silently write an inconsistent state.
func MustTransition(from, to VPNStatus) error {
	if CanTransition(from, to) {
		return nil
	}
	return ErrIllegalTransition
}

// IsTerminal reports whether no further transition is possible.
func (s VPNStatus) IsTerminal() bool { return len(transitions[s]) == 0 }

// SeverOutage maps a panel HTTP outcome to a VPN state without ever producing
// REVOKED (REQUIREMENTS §7: "Never map an API outage directly to REVOKED").
func SeverOutage(current VPNStatus) VPNStatus {
	switch current {
	case VPNRevoked, VPNRemovedFromCatalog:
		return current
	default:
		return VPNServerUnavailable
	}
}

// QuotaReleasingStatuses are the assignment states that do NOT consume a
// reseller slot (REQUIREMENTS §2.2: expiry/revocation releases the slot).
func QuotaReleasingStatuses() []string {
	return []string{"EXPIRED", "REVOKED", "CANCELLED", "REJECTED"}
}
