---
name: veylix-database-engineering
description: Use when adding or editing a SQLite migration, changing the schema, adding a store method, writing a transaction, or reasoning about a data invariant in Veylix. Covers the migration file convention and numbering, the idempotent transactional migration rules, the unique constraints and isolation model, the pure-Go modernc.org/sqlite driver, WAL and busy_timeout and maxOpenConns settings, the rule that a local transaction never spans a remote panel call, and the backward-compatibility discipline that protects existing user data. Load before touching backend/internal/store.
---

# VEYLIX — DATABASE ENGINEERING

## Driver and settings

`modernc.org/sqlite` — **pure Go, no CGO**. This is why `go test -race` cannot
run here (race needs CGO and there is no C compiler); do not try to fix that by
swapping the driver.

Connection discipline (see `./docs/ARCHITECTURE.md` §2 and §5):

- WAL journal mode
- foreign keys **on**
- `busy_timeout = 5000`
- `maxOpenConns = 4` — SQLite serialises writes; more connections do not help
  on 1 vCPU and only create lock contention.

## Migrations

Location: `./backend/internal/store/migrations/`, named `NNNN_snake_name.sql`
(4-digit, zero-padded, monotonic).

Rules:

1. **Append only.** Never edit a migration that has shipped. A correction is a
   **new** migration.
2. **Idempotent and transactional per file.** Re-running the set must be a no-op.
3. **Backward compatible.** Existing user data survives. Never drop a column or
   table to make a test pass, and never reset production data.
4. Update the schema version expectation and any test that asserts it, in the
   same change.
5. `npm run docs:verify` recomputes the migration count from the tree and fails
   if `./README.md` disagrees. If you add a migration, expect that gate to
   demand a README correction — report it, do not silently edit the README.

## Data invariants — these are contracts, not preferences

| Invariant | Why it exists |
|---|---|
| `vpn_instances UNIQUE (server_id, panel_email)` | one panel client is one VPN; a retried provision cannot duplicate |
| `outbox UNIQUE (server_id, op, idempotency_key)` | an ambiguous remote write is recorded exactly once |
| `server_integrations.status` CHECK constraint | the six-state integration model |
| `token_ciphertext` is a BLOB | a plaintext token has no column to live in |
| lifetime usage columns live on `vpn_instances` | a remote counter reset cannot erase history |
| `free_claims` holds the FREE caps | caps are never inferred by scanning `vpn_instances` |
| `client_storage_limits` + `chat_usage` | chat storage is budgeted separately from VPN quota |

Any change that weakens one of these is a **design change** requiring an
owner decision in `./docs/DECISIONS.md`, not a migration.

## Transactions

- **A local transaction never spans a remote panel call.** Remote effects are
  reconciled through `outbox` + fresh reads.
- Quota consumption is counted **in-transaction** so two concurrent requests
  cannot both consume the last slot.
- Bulk operations run **sequentially**, not as N parallel writes — fifty parallel
  writes against SQLite on 1 vCPU is an outage.
- Keep transactions short; do no I/O inside one.

## Adding a store method

1. One file per domain in `./backend/internal/store/` (follow the existing
   `*.go` + `*_test.go` pairing).
2. Return domain types or interfaces, not driver rows, upward.
3. `store` is the **only** package that speaks SQL (Rule 02).
4. Cover: success, not-found, conflict/duplicate, and unauthorized-scope.

## Data safety

Relevant runtime paths are git-ignored and must be treated as **live data**:
`./var/` (local SQLite + logs), `./.veylix-secrets/`, `*.db*`. Tests and probes
use an **isolated test database and isolated data directory** — never these.
See `./VPN_FIX_LEDGER.md` for the recorded incident where a verification script
deleted live data twice.

## Findings (R4) — historical only

Prior database investigation is **historical context**. Re-read
`./backend/internal/store/` and the current migration set before relying on any
conclusion; never assume an old schema version number is still current — count
the files.

## Checklist

- [ ] Migration is a new file, correctly numbered, idempotent, transactional.
- [ ] No shipped migration was edited.
- [ ] Existing data survives.
- [ ] Affected invariant still holds, and a test says so.
- [ ] No local transaction wraps a remote call.
- [ ] Migration count in `./README.md` reconciled (report, do not silently edit).
