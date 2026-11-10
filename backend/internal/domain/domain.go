// Package domain will hold business rules and invariants: quota consumption,
// status transition tables, provisioning orchestration and ownership checks.
//
// Stage B ships the package boundary only — no behaviour is implemented here
// yet, so nothing can be claimed as working (`new_update.md` §13).
//
// Planned contents (see docs/IMPLEMENTATION_PLAN.md):
//   - Stage E2: authorization helpers (role + ownership), audit recording
//   - Stage E3: server integration state machine (map panel HTTP outcome → status)
//   - Stage E4: provisioning + quota ledger transaction, outbox scheduling
//   - Stage E5: usage delta accounting that survives remote counter resets
package domain

// Version is the domain layer contract version, bumped when an invariant changes.
const Version = 1
