# 02 · ARCHITECTURE BOUNDARIES

Always loaded. Full detail: Skill `veylix-architecture`.

## The dependency rule

One direction only. Nothing points back upstream.

```
cmd/veylixd  →  internal/httpapi  →  internal/domain  →  internal/{store, panel}
```

| Layer | Owns | Must never |
|---|---|---|
| `cmd/` | process wiring, dependency injection | hold business rules |
| `internal/httpapi` | routing, decoding, JSON envelopes, status codes | hold business rules, touch the SQL driver |
| `internal/domain` | state machines, quota, provisioning rules | import `httpapi` |
| `internal/store` | **all** SQL, migrations, transactions | be imported by `httpapi` |
| `internal/panel` | **all** outbound 3X-UI HTTP calls | be called from `httpapi` directly |

Support packages stay single-purpose: `config` (read by `cmd` only), `auth`,
`audit`, `integrations`, `provisioning`, `sync`, `releases`, `payments`, `mail`.

## Hard boundaries

1. **No import cycles.** If a change needs one, the design is wrong — fix the
   design, not the rule.
2. **Only `store` writes SQL.** A handler that imports the DB driver is a defect.
3. **Only `panel` performs outbound panel requests.** SSRF guard, timeouts and
   sealing live there.
4. **No global mutable state.** Dependencies are injected from `cmd/veylixd`.
5. **A local transaction never spans a remote call.** Remote effects are
   reconciled through `outbox` + fresh reads. This is the atomicity statement
   in `./docs/ARCHITECTURE.md` §4.
6. **`frontend/` is a separate build.** It talks to the backend only over HTTP
   via `frontend/src/lib/api.ts`. It never imports Go, never holds a panel
   token, and never derives VPN limits locally.

## Out of scope

`./veylix-ui demo/` is a **separate application** with its own `package.json`,
`src/`, `.env` and a duplicate `docs/` tree. It is **not** Veylix. Do not read it
for Veylix behaviour, do not edit it, and never cite
`./veylix-ui demo/docs/…` as a Veylix source.

## Before adding code, answer these in writing

1. Which layer owns this responsibility?
2. Which existing package is the closest precedent? Follow it.
3. Does anything upstream now need to import it? That is a cycle — redesign.
4. Does it cross the backend/frontend boundary? Then it needs an
   `./docs/API_CONTRACT.md` entry.

## Resource ceiling is architectural

1 vCPU / 500 MB is a design constraint, not a tuning note. No Redis, no
PostgreSQL, no Docker, no persistent Node server, no extra agent — unless the
owner explicitly approves the trade-off with evidence.
