# VEYLIX — ARCHITECTURE

**Status:** authoritative · **Established:** 2026-11-08 · **Baseline:** `1b09553`

**Evidence rule.** Every claim is marked **C** (confirmed by reading imports and
code), **A** (assumed), or **U** (unknown). An unmarked statement is a **rule the
architecture must obey** — and where the code currently breaks it, that is stated
plainly in §6 rather than hidden.

---

## 1. The shape

One Go process. `cmd/veylixd` wires every dependency and starts an
`http.ServeMux`. The React bundle is compiled into that same binary
(`go:embed`), so there is no separate web server and no Node process in
production.

```
                    ┌──────────────────────────────────────┐
   browser ──HTTP──▶│  cmd/veylixd  (wiring + workers)      │
                    │    │                                  │
                    │    ├─▶ internal/httpapi  (transport)  │
                    │    │        │                         │
                    │    ├─▶ internal/provisioning          │
                    │    ├─▶ internal/integrations          │
                    │    ├─▶ internal/sync     (worker)     │
                    │    └─▶ internal/settings / mail / ... │
                    │                 │                     │
                    │                 ▼                     │
                    │       internal/{store, panel}         │
                    └──────────┬───────────────┬───────────┘
                               ▼               ▼
                          SQLite file     3X-UI panel
```

---

## 2. The real dependency graph (C — read from imports, not documentation)

Each row is what the package **actually imports** from `veylix/backend/internal/*`:

| Package | Imports |
|---|---|
| `audit` | — |
| `config` | — |
| `domain` | — |
| `mail` | — |
| `panel` | — |
| `versioning` | — |
| `store` | `domain` |
| `auth` | `audit`, `store` |
| `releases` | `audit`, `auth`, `store` |
| `provisioning` | `audit`, `auth`, `domain`, `panel`, `store` |
| `sync` | `domain`, `panel`, `store` |
| `integrations` | `audit`, `auth`, `panel`, `store` |
| `payments` | `auth`, `provisioning`, `store` |
| `settings` | `audit`, `auth`, `config`, `store` |
| `httpapi` | `audit`, `auth`, `domain`, `integrations`, `panel`, `payments`, `provisioning`, `releases`, `settings`, `store`, `sync` |

**C — no import cycle exists.** Verified by `go build ./...` succeeding and by
reading every edge above.

---

## 3. What each package owns

| Package | Owns | Rule |
|---|---|---|
| `cmd/veylixd` | process wiring, dependency injection, worker startup | must hold **no** business rule |
| `httpapi` | routing, decoding, JSON envelopes, status codes | must hold **no** business rule and **no** SQL |
| `domain` | state machines, status enums, provisioning rules | must not import `httpapi` |
| `store` | **all** SQL, migrations, transactions | the only package that may own SQL |
| `panel` | **all** outbound 3X-UI HTTP: SSRF guard, timeouts, sealing | the only package that may call the panel |
| `auth` | argon2id, sessions, RBAC, the two portals | |
| `audit` | append-only trail, inbox notification seam | |
| `integrations` | server registration, health, inbounds, panel clients | |
| `provisioning` | VPN create/lifecycle across DB **and** panel | |
| `sync` | read-only panel polling, usage snapshots | |
| `releases` | release lifecycle (D-027) | |
| `payments` | order settlement | |
| `settings` | operator-editable settings, sealed secrets | |
| `versioning` | update feed | |
| `mail` | SMTP + outbox | |
| `config` | environment reading | read by `cmd` only |

---

## 4. The five hard boundaries

1. **No import cycles.** If a change needs one, the design is wrong.
2. **Only `store` writes SQL.** A handler or service holding SQL is a defect.
3. **Only `panel` performs outbound panel requests.** SSRF guard, timeouts and
   sealing live there.
4. **No global mutable state.** Dependencies are injected from `cmd/veylixd`.
5. **A local transaction never spans a remote call.** Remote effects reconcile
   through `outbox` + a fresh read.

---

## 5. The frontend boundary (C)

`frontend/` is a **separate build**. It talks to the backend only over HTTP via a
single module, `frontend/src/lib/api.ts`. It never imports Go, never holds a panel
token, and never derives VPN limits locally.

```
frontend/src
├── pages/       37 route components
├── components/  ui/, layout/, client/, charts/
├── stores/      zustand: auth, vpn, notif, ...
├── lib/api.ts   the ONLY outbound HTTP surface
├── hooks/
├── theme/       presets + sub-themes
└── styles/
```

**C:** `npm run build` produces `dist/`, which `.\\dev.ps1 -Package` embeds into
the binary. Node never runs in production.

---

## 6. Where the code does NOT yet match the boundaries

This section exists because the audit found real violations. They are recorded
with counts rather than described as "clean", so Phase 5 can address them
deliberately.

### 6.1 SQL outside `store/` — 112 statements (C, measured)

Boundary 2 says only `store` owns SQL. It does not hold today:

| File | SQL statements | Note |
|---|---|---|
| `provisioning/service.go` | 35 | holds both `s.db` **and** `s.st` |
| `auth/service.go` | 30 | domain package with its own SQL |
| `integrations/service.go` + `purge.go` | 14 | |
| **`httpapi/usage_store.go`** | **7** | 🔴 **transport layer holding SQL** |
| `sync/service.go` + `scheduler.go` | 10 | |
| `auth/session.go` | 6 | |
| **`httpapi/vpn_lister.go`** | **2** | 🔴 transport layer |
| **`httpapi/vpn_lister_panel.go`** | **2** | 🔴 transport layer |
| **`httpapi/panel_cache.go`** | **1** | 🔴 transport layer |
| `provisioning/reconcile.go` | 1 | |
| `audit/audit.go`, `releases/service.go` | 2 | |
| `panel/client.go` | 1 | a false positive: the word appears in a comment |

**Assessment.** This is **not** a correctness bug — the code works and 617 tests
pass. It is a **maintainability** finding: the "only store writes SQL" rule is
aspirational, and a future contributor reading the rule will be misled about the
codebase. Two honest options exist and the owner must choose:

- **Option A — tighten the code.** Move the 12 `httpapi` queries into `store`.
  Bounded, low risk, no behaviour change.
- **Option B — tighten the rule.** Rewrite boundary 2 to describe what is actually
  true (SQL lives in `store` plus a small number of named exceptions), and record
  those exceptions in this file.

**Recommendation:** Option A for `httpapi` (it is the layer with the clearest
prohibition), Option B for the service packages (they legitimately query within
their own transaction, and extracting that into `store` would be a large,
behaviour-risking refactor for no functional gain).

**Status:** OPEN — recorded, not fixed. See `UNKNOWNS.md` Q-13.

### 6.2 Outbound HTTP outside `panel/` — **none** (C, corrected)

A first draft of this section claimed `versioning/service.go` called the panel.
**That was wrong.** Verified by reading the file: `versioning/service.go` only
mentions `/panel/api/server/update-check` **inside a comment** explaining the
design idea. It issues no request.

The complete list of packages that construct an outbound HTTP client (**C**,
measured with `http.Client{`, `http.Get(`, `http.Post(`, `http.NewRequest`):

| File | Calls | Destination | Boundary |
|---|---|---|---|
| `panel/client.go` | 2 | the operator's 3X-UI panel | ✅ correct home |
| `versioning/github.go` | 3 | `https://api.github.com` (release feed) | ✅ not the panel |
| `auth/google/google.go` | 3 | Google OAuth endpoints | ✅ not the panel |

**Boundary 3 holds.** No package besides `panel` talks to a 3X-UI panel.

> This correction is recorded rather than silently replaced, because the first
> version of this file asserted a defect that did not exist — and a fabricated
> finding is worse than a missing one.


### 6.3 `httpapi` imports `store` (C — 25 files)

`httpapi` references `store.OfferRow`, `store.ErrFreeConcurrentLimit` and similar
**types and sentinel errors**. That is interface coupling, not SQL, and it is
acceptable: the transport must map store errors to status codes. It becomes a
violation only when `httpapi` itself runs a query, which §6.1 lists.

---

## 7. Resource ceiling is architectural

**1 vCPU / 500 MB** is a design constraint, not a tuning note (**C**: stated in
the spec and reflected in the dependency list — two direct Go dependencies, one
embedded SQLite file, one process).

Consequences that must not be quietly reversed:

- No Redis, PostgreSQL, Docker, or message broker.
- No persistent Node server.
- No background agent beyond the in-process reconciler and sync workers.
- Pagination is bounded server-side (200 × 50 caps), not unbounded.

Adding any of the above requires an owner decision **with measured evidence**,
recorded in `DECISION_LOG.md`.

---

## 8. Rules a future edit must not break

1. No import cycle. `go build ./...` is the proof.
2. `httpapi` must not gain **new** SQL. The 12 existing statements are a
   recorded debt (§6.1), not a precedent.
3. A new outbound panel call goes in `panel`, never in a service.
4. A handler never trusts a client-supplied id for ownership; it resolves the
   principal from the session.
5. A local transaction never spans a remote call.
6. The frontend never derives a VPN limit, a price, or a quota locally.

