---
name: veylix-architecture
description: Use when deciding where Veylix backend or frontend code belongs, when adding a package, layer, route or cross-boundary feature, when a change might create an import cycle, or when verifying that a change respects the cmd -> httpapi -> domain -> {store, panel} dependency direction and the 1 vCPU / 500 MB budget. Covers the twelve internal Go packages, the cmd tool inventory, the frontend build boundary, and the reason ./veylix-ui demo/ is out of scope. Load this before creating a new file or package.
---

# VEYLIX — ARCHITECTURE

Read `./docs/ARCHITECTURE.md` for the full argument. This skill is the applied
form of Rule 02.

## The layer map (verified against `./backend/internal/`)

```
cmd/veylixd            process wiring + dependency injection
  └─ internal/httpapi  transport only: routing, decode, envelopes, status codes
       └─ internal/domain        business rules, state machines, invariants
            ├─ internal/store    SQLite, migrations, transactions  (only SQL)
            └─ internal/panel    outbound 3X-UI HTTP               (only panel I/O)
```

Single-purpose support packages:

| Package | Responsibility |
|---|---|
| `config` | configuration loading; read by `cmd` only |
| `auth` | argon2id, sessions, RBAC, ownership |
| `audit` | append-only privileged-action log |
| `integrations` | panel add/verify/inbounds/disable/remove, sealed tokens |
| `provisioning` | quota + outbox + reconciler |
| `sync` | panel sync, usage snapshots, metrics (read-only) |
| `releases` | local release and grace handling |
| `payments` | payment records and confirmation |
| `mail` | outbound mail |

## `cmd/` tools (verified — these are probes/utilities, not product surface)

`veylixd` (the server) · `accounttool` · `dbcheck` · `dbexec` · `limitprobe` ·
`ob` · `outboxprobe` · `panelprobe` · `seedacct` · `seeddemo` · `vpnprobe`.

A probe is **not** an API. Do not wire product behaviour through `cmd/`.
**`dbexec` and the seeding tools mutate data — treat them as dangerous (see
`veylix-testing-review`).**

## Placement decision tree

1. **Is it a business rule, quota rule, state transition or invariant?**
   → `internal/domain`. Never in `httpapi`.
2. **Is it a SQL statement, migration or transaction?**
   → `internal/store`.
3. **Is it an outbound 3X-UI call, or does it touch a panel token?**
   → `internal/panel` (or `integrations` for integration lifecycle).
4. **Is it decoding, routing, a status code or an envelope?**
   → `internal/httpapi`. Nothing more than that.
5. **Is it React, a store, a page, a theme token?**
   → `frontend/src`. It talks to the backend only through
   `frontend/src/lib/api.ts`.
6. **Does it span two of the above?** It is a cross-boundary change: it needs a
   row in `./docs/API_CONTRACT.md` and both surfaces updated in the same change.

## The frontend boundary

`frontend/src/lib/api.ts` is the **only** HTTP path. `frontend/src/stores/`
holds one file per domain. Every store is **API-backed** — a mock, a constant or
a hard-coded array in a store is a defect, and `npm run audit` exists to catch it.

The frontend:
- never holds a panel token, UUID or limit source of truth,
- never derives VPN limits locally (the panel owns them),
- never imports Go, and is never imported by Go (the build output is embedded).

## Cycle check

If `httpapi` needs to import `store`, or `domain` needs to import `httpapi`, the
design is wrong. Move the responsibility down, or introduce an interface in
`domain` that `cmd` satisfies. Never "just this once".

## Out of scope

`./veylix-ui demo/` is a **different application** (own `package.json`, `src/`,
`.env`, and a duplicate `docs/` tree). Do not read it, edit it, or cite
`./veylix-ui demo/docs/…` as Veylix evidence. `H:\my apps\Veylix.worktrees\` is
likewise outside this workspace.

## Architecture findings (R1) — historical only

Prior investigations concluded that the layer split is sound and should be
**kept**, not rewritten: handlers already hold no business rules, and remote
integration is already isolated in `panel`. **Re-check the current source before
acting on this.** Do not re-derive it from this paragraph.

## Checklist before you add a file

- [ ] I named the owning layer.
- [ ] I found and followed an existing precedent in that layer.
- [ ] No new import points upstream.
- [ ] If cross-boundary: `./docs/API_CONTRACT.md` updated in the same change.
- [ ] If new dependency: justified against the 1 vCPU / 500 MB ceiling.
