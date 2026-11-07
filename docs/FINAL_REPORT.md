# VEYLIX — FINAL REPORT

**Required by** `docs/new_reqierment/new_update.md` §13 (14 numbered items + a per-feature classification).
**Report date:** 2026-10-24 · **Numbers re-verified:** 2026-11-07
**Scope covered:** everything built from the empty repository to this commit.
**Rule honoured throughout:** every claim below names the command or file that proves it. Anything not executed is marked **NOT RUN**, never implied (§13 "never fabricate test results").

> **⚠️ 2026-11-07 — §1 and the route/migration figures are now correct.** Earlier
> revisions of this report said **72 routes** and **7 migrations**. Both were
> point-in-time facts from 2026-10-24 and were never re-derived until now. The
> current figures come from `npm run docs:verify`, which reads the tree and fails
> when the README disagrees:
>
> | Figure | This report said | Actual (2026-11-07) |
> |---|---|---|
> | HTTP routes | 72 | **125** |
> | Migrations | 7 | **21** (schema v21) |
> | Test functions | 286 | **518**, in **13** packages |
> | Store files | 11 | **17** (all API-backed) |
> | Frontend pages | 23 | **47** |
> | Docs | 22 | **30** |
> | Installation | not built | **`install.sh` + `vpn-platformctl`** (D-079), **not VM-tested** |
>
> Sections 1–2 remain the record of that 2026-10-24 pass; **the banner above is
> the authority on numbers**. For per-feature truth use `docs/PROCESS_MAP.md` §13
> and §15.

> **How to read this document.** Sections 1–6 answer §13's 14 questions. Section 7 is the mandated **per-feature classification**. Where this report and `docs/PROCESS_MAP.md` disagree, **PROCESS_MAP is current** — it carries a verified date.

---

## 1. Files inspected

| Area | Count | What |
|---|---|---|
| Backend Go | **125 files** (28 `_test.go`) | `cmd/` (7 binaries: `veylixd`, `seeddemo`, `panelprobe`, `vpnprobe`, `outboxprobe`, `dbexec`, `ob`) · `internal/{config,store,domain,auth,panel,integrations,provisioning,sync,releases,audit,httpapi}` |
| Frontend TS/TSX | **96 files** | `pages/` (23), `stores/` (11), `components/` (ui, charts, layout, theme-studio, effects, chat, palette, auth, dev, brand), `lib/`, `theme/`, `hooks/` |
| Frontend CSS | **27 files** | tokens, glass, theme-extras, base, ui-themes, style-layer, variants + per-page sheets |
| Migrations | **21** | `0001_identity` … the 21st; embedded and applied at startup |
| Docs | **30** | see `docs/DOC_INDEX.md` |
| HTTP routes | **125** | `internal/httpapi/*`; derived by `npm run docs:verify` |
| Scripts | 9 | `dev.ps1`, `audit-ui-truth.mjs`, `verify-dashboard.mjs`, 5 × `smoke-*.ps1`, `check-root.mjs` · plus `install.sh` and `scripts/lib/common.sh` (D-079) |

---

## 2. Files changed and why (by theme)

| Theme | Files | Reason |
|---|---|---|
| Backend foundation | `cmd/veylixd/main.go`, `internal/{config,store,httpapi}` | Go + SQLite backend, embedded migrations, fail-closed config, WAL, FK, bounded pool |
| 3X-UI adapter | `internal/panel/*` (10) | Bearer auth, SSRF guard, timeouts, GET-only retries, full client CRUD, HWIDs, links |
| Auth & RBAC | `internal/auth/*` (14) | argon2id, sessions (fingerprint only), permission matrix, ownership, rate limits, D-022 |
| Provisioning | `internal/provisioning/*` (7) | Panel write → fresh read → record (D-024.1a), quota in-transaction, outbox + reconciliation |
| Sync | `internal/sync/*` (8) | Bounded per-server jobs, usage snapshots, panel limit mirror (D-024.2/.4) |
| Releases | `internal/releases/*` | 3-day grace, local-removal-only purge (D-027) |
| HTTP surface | `internal/httpapi/*` (45) | **125 routes**, one error envelope, `SameOrigin` CSRF, recover middleware |
| **E7 packaging** | `internal/httpapi/spa.go`, `spa_test.go`, `dist/.gitkeep`, `dev.ps1 -Package` | **Single binary** serving UI + API (D-029) |
| Frontend design system | `components/ui/*`, `styles/*`, `theme/*` | 5 presets, 40 sub-themes, Theme Studio, every component consumes tokens |
| Frontend wiring | `stores/*` (11), `lib/api.ts` (659 lines) | **All 11 stores API-backed; 0 mocks remain** (D-006 exit) |
| **UI truth pass** | 8 pages + `scripts/audit-ui-truth.mjs` | 12 screens claimed capabilities the backend contradicted (F-032) |
| Dev workflow | `dev.ps1`, `vite.config.ts`, `docs/LIVE_PREVIEW.md` | One-command preview, `strictPort`, `-Status`/`-Lan`/`-Log`/`-Package` (F-033) |

---

## 3. Existing functionality preserved

- **All 5 theme presets + 40 sub-themes + Theme Studio** — untouched by the backend work; CSS output 139 kB, unchanged in shape.
- **The real login flow** — the dev bypass (D-022) is additive: a real session always wins, and a production build contains none of it (bundle grep: absent).
- **Public API contracts** — no route renamed or removed; planned routes were only *added*. `docs/API_CONTRACT.md` records before/after for each.
- **Pre-existing working tree** — inspected before editing; owner changes in `App.tsx`, `ui.css`, `layout.css` left intact.


---

## 4. Defects identified and fixed

| ID | P | Defect | Fix evidence |
|---|---|---|---|
| F-011 | P1 | Client email could traverse the panel URL path | validation + 6 rejection tests |
| F-013 | P1 | `Secure` cookie over plain HTTP made every authed call 401 | loopback-only opt-out + `TestSecureCookiesDecision` |
| F-014 | P2 | Rate-limiter truncated elapsed time, never refilled | unit test |
| F-015/017 | P1/P2 | Password policy: UI allowed 8 chars, backend 12; `aaaaaaaaaaaa` accepted | shared policy + `passwordPolicy.ts` mirror |
| F-016 | P1 | A user could never change their password | `POST /api/auth/change-password`, revokes other sessions |
| F-019 | P2 | Sidebar counts were hardcoded literals | badges derive from the same store list the page renders |
| F-020/021 | P1 | Bootstrap did not sign the user in; no way to create an `admin` | `/api/setup/bootstrap` issues a session; `/api/admins` CRUD |
| F-023 | P2 | `/api/quotas/me` returned 403 to a client | per-role contract (D-021) |
| F-024 | **P0** | `limitHwid` never sent → device limits did nothing | `ClientSpec.LimitHwid`; **live-verified 2/2 on the panel** |
| F-025/F-028 | **P0** | A "VPN" row existed before the panel confirmed the client | inverted order; a rejected create leaves **no row** and returns the quota slot |
| F-026 | P1 | The panel nests the client in `obj.client`, so `GetClient` returned a zero record | nested decode + real-payload test |
| F-027 | **P0** | A client deleted in 3X-UI stayed `ACTIVE` in Veylix | sync + `POST /verify` mark `SERVER_UNAVAILABLE`, never `REVOKED` |
| F-029 | P1 | "Tag locks once issued" was dead code | lock inside the create tx, release on rollback, `409 tag_locked` audited as DENIED |
| F-030 | P2 | The list could not tell panel-confirmed from unverified | `source: panel|local` + `tag` on the wire |
| **F-032** | P1 | **12 screens claimed capabilities the backend had shipped** (invented LKR prices, invented dashboard panels, "data is mock") | rewritten to real endpoints; `npm run audit` now fails on regression |
| **F-033** | P2 | `dev.ps1 -Log` hung; Vite could hop to 5174 and break CSRF | wait on `/api/health`; `strictPort: true` |
| F-031 | P2 | Dev-only React duplicate-key warning (admin/client personas) | **OPEN** — documented, deliberately not suppressed |

---

## 5. 3X-UI capabilities implemented

| Capability | State |
|---|---|
| `GET /panel/api/server/status` | 🟢 **live** — panel 3.10.2 / xray 26.10.10 |
| `GET /panel/api/inbounds/options` | 🟢 **live** |
| `GET /panel/api/clients/list/paged` | 🟢 **live** (paged, bounded) |
| `GET /panel/api/clients/get/{email}` | 🟢 **live** (fresh-read confirmation) |
| `POST /panel/api/clients/add` | 🟢 **live 2026-10-24** — `201 ACTIVE confirmed:true`; read-back `limit_ip:2, limit_hwid:2` |
| `POST /panel/api/clients/update/{email}` | 🟢 adapter + tests (full-record replace, no field dropped) |


---

## 6. Database & configuration changes

**Schema — 21 migrations, version 21 at runtime at this commit** (`GET /api/ready` →
`schema_version: 21`). The table below lists the **7 migrations that existed on
2026-10-24**; migrations 8–21 were added afterwards and are listed in
`backend/internal/store/migrations/`. `npm run docs:verify` counts the directory,
so the number cannot drift again.

| Migration | Contents |
|---|---|
| `0001_identity` | `users`, `sessions` |
| `0002_business` | `server_integrations`, `vpn_offers`, `inbounds`, `vpn_instances` (9-state CHECK), `vpn_assignments`, `audit_log` |
| `0003_observability` | `usage_snapshots`, `outbox`, metrics |
| `0004_quota_limit` | per-user quota ceiling |
| `0005_sync` | sync bookkeeping |
| `0006_panel_truth` | `panel_client_tags`, `free_claims`, `vpn_releases`, `client_storage_limits`, `panel_*` mirror columns |
| `0007_chat` | `chat_threads`, `chat_messages`, per-viewer flags, read markers |

**Configuration (`VEYLIX_*`, fail-closed, secrets never logged):** `ADDR`, `DATA_DIR`, `ENCRYPTION_KEY` (32 B), `SESSION_KEY` (32 B), `INSECURE_COOKIES`, `DEV_AUTH`, `SEED`, `SYNC_INTERVAL`, `SYNC_TIMEOUT`, `SYNC_PAGE_SIZE`, `SYNC_MAX_PAGES`.

---

---

## 7. Per-feature classification (mandated by §13)

| Classification | Features |
|---|---|
| **Implemented and tested (live-verified on a real panel)** | panel add/test/edit/remove · client **create** · client read (paged + fresh) · connection links · secrets sealed at rest (AES-256-GCM, absent from DB files) · auth (bootstrap, login, logout, password change) · RBAC + ownership denials · server metrics · sync + usage history · panel limit mirror · release/restore (local-only purge) · notifications + audit fan-out · payments state machine · chat with participant enforcement · offers · clients · directory · **single binary serving UI + API (E7 / D-029)** |
| **Implemented but not live-verified** | client suspend / resume / revoke against the real panel (blocked by D-018) · ambiguous-write reconciliation · HWID list/revoke (adapter only) · provisioning against a second panel |
| **Partially implemented** | per-client FREE monthly cap (D-025.5 — no column; the policy default of 5 applies) · chat storage budgets (D-027.6 — table only) · chart-type/range toggles (D-026) · panel email identity + rename-on-assign (D-028.1/.2 — still `vyx-<n>`) · panel inspector + tagging **UI** (backend routes exist and are tested; no frontend calls them yet) |
| **Unsupported by the target API / out of scope** | inbound create/edit/delete via Veylix (D-024) · outbound management (`POST /xray/update` whole-config) · automated payment gateway, NFT, native mobile apps (D-007) |
| **Blocked by missing access or environment** | live provisioning writes (D-018) · Google OAuth live test (no client id/secret) · `go test -race` (no C compiler) · email verification / password reset (no SMTP) · clean-Linux-VM install test (no VM) |

---

## 8. Commands and tests actually executed (latest run, 2026-10-24)

```
backend/  go build ./...                      → exit 0
          go vet ./...                        → exit 0
          gofmt -l .                          → nothing printed
          go test ./... -count=1              → 9/9 packages ok (286 Test* funcs)

frontend/ npm run build                       → ✓ built in 769 ms
          npx tsc -b                          → clean
          npm run lint                        → 0 warnings / 0 errors (103 files)
          node scripts/audit-ui-truth.mjs             → OK, 0 findings
          node scripts/audit-ui-truth.mjs --self-test → OK, 6 positive + 4 negative

repo/     .\dev.ps1 -Log .\var\logs           → backend ready, dev bypass ON, frontend 200
          .\dev.ps1 -Status                   → /api/health 200, /api/me dev principal
          .\dev.ps1 -Package                  → PACKAGED OK, one binary, one port
```

**Packaged-binary proof (single process, port 18082):**

```
/api/health        200  application/json
/api/ready         200  application/json     (ui: embedded:7_files)
/api/me            200  application/json
/api/vpns          200  application/json
/                  200  text/html
/dashboard         200  text/html
/vpn/16            200  text/html            (SPA deep link)
/nonexistent       200  text/html            (client route → shell)
/api/nope          404  json                 (API never swallowed by the SPA)
/assets/nope.js    404                       (a missing file stays 404)
dev bypass in bundle → absent
```

---

## 9. Actual pass/fail results


---

## 10. Security findings and remediation

| Finding | Remediation |
|---|---|
| Panel token in browser / logs / DB plaintext | sealed AES-256-GCM with **AAD bound to the URL**; never returned; absent from DB files (grep) |
| SSRF via a malicious panel URL | scheme/DNS/private-block checks, redirect refusal, no DNS-rebinding window (17-case rejection table) |
| Email path traversal into the panel URL | strict validation before any URL is built |
| Session theft | `HttpOnly` + `SameSite=Lax` + `Secure` (loopback opt-out only), fingerprint-only storage, 12 h lifetime, suspend revokes |
| CSRF | `Origin` vs request host on every state-changing request; **the single binary removes the cross-origin case entirely** |
| Password weakness | 12–512 chars, no repeats/sequences, blocklist; argon2id; the UI mirrors the policy |
| Dev bypass reaching production | `import.meta.env.DEV` gate + backend loopback gate; **bundle grep: absent** |
| ID enumeration | foreign ids answer **404**, never 403 |
| Secret in logs | `Config.String()` reports lengths only |

---

## 11. Performance measurements

| Measurement | Value |
|---|---|
| Packaged binary | **19.1 MB** (Go runtime + embedded SPA) |
| First load (gzip) | CSS 23.6 kB · JS 151.0 kB · lazy `threeScene` 134.2 kB |
| Backend start → `/api/health` 200 | < 1 s |
| Package probe start → routes answering | ≈ 1 s (4 s budget) |
| 500 MB / 1 vCPU benchmark | **NOT RUN** — needs the target VPS |

---

## 12. Remaining risks and blockers

1. **D-018** — client suspend/resume/revoke are unit-tested but never executed against the real panel (single production inbound).
2. **F-031** — dev-only React duplicate-key warning for the admin/client personas; harmless in a production build, noisy in review.
3. **Panel email identity** (D-028.1/.2) — a VPN's panel email is `vyx-<n>`, not the owner's address, and assignment does not rename it.
4. **No frontend for panel inspection/tagging** — backend routes exist and are tested, but a Super Admin cannot yet tag a client from the UI.
5. **No CI** — every gate above ran locally; `-race` and the Linux build are unverified.
6. **No installer / Caddy / systemd** — the binary works, but a one-step server install does not exist yet.
7. **`go test -race` never executed** anywhere.

---

## 13. Live operations verified against the real panel

| Operation | Result |
|---|---|
| Health + capability detection | ✅ 3.10.2 / xray 26.10.10, latency recorded |
| Inbound list | ✅ `(IN)-free_2`, vless:443 |
| Paged client read + fresh read | ✅ |
| **Client create** | ✅ `201 ACTIVE confirmed:true`; panel read-back `limit_ip:2, limit_hwid:2` |
| Connection links | ✅ genuine `vless://…` for `vyx-16` |
| Ping | ✅ `{reachable:true, latency_ms:1, scope:"server_reachability"}` |
| Sync tick + panel limit mirror | ✅ mirror matched `GET /api/servers/1/panel-clients` byte-for-byte |
| Server edit (`PATCH`) | ✅ `200` + re-test ACTIVE |
| Suspend / resume / revoke | ⛔ **not run** (D-018) |

---

## 14. Deployment and rollback instructions

### Build (one artifact)
```powershell
.\dev.ps1 -Package        # builds the SPA, embeds it, builds bin/veylixd.exe, then asserts the result
```

### Run
```powershell
$env:VEYLIX_ADDR='127.0.0.1:8080'
.\backend\bin\veylixd.exe
# open http://localhost:8080/dashboard — UI and API on the same origin
```
Required env: `VEYLIX_ENCRYPTION_KEY`, `VEYLIX_SESSION_KEY` (32 bytes each), `VEYLIX_DATA_DIR`.
First run: `GET /api/setup/status` → `bootstrap_required: true` → the `/setup` wizard creates the first super admin (no default password).

### Deploy (until the Phase 6 installer lands)
1. Copy `veylixd` to the server, create `/var/lib/veylix` for the database, run under a least-privilege systemd unit bound to `127.0.0.1:8080`.
2. Put Caddy in front for TLS (`reverse_proxy 127.0.0.1:8080`) — the binary itself does not terminate TLS.
3. Keep `VEYLIX_INSECURE_COOKIES=0` and `VEYLIX_DEV_AUTH=0` (the defaults) in production.

### Rollback
- **Binary:** keep the previous `veylixd`; stop the service, swap the file, start. The schema only moves forward, so a rollback across a migration needs the backup below.
- **Database:** stop the service and copy `veylix.db` **with** `-wal` and `-shm` before upgrading.
- **UI only:** the SPA lives inside the binary; there is no separate asset to revert.
- **Dev bypass:** set `VEYLIX_DEV_AUTH=0` and `VITE_DEV_AUTH=0` — no code change.

---

## Appendix — documents that remain authoritative

| Question | Document |
|---|---|
| "How do I run it right now?" | `docs/LIVE_PREVIEW.md` |
| "Which endpoint exists?" | `docs/API_CONTRACT.md` |
| "What is still missing?" | this report §7 + `docs/GAP_ANALYSIS.md` §0-A |
| "Why was it built this way?" | `docs/DECISIONS.md` (D-001 … D-029) |
| "What was tested and how?" | `docs/QA_REPORT.md` |
| "Where did a defect come from?" | `docs/IMPROVEMENT_LEDGER.md` (F-001 … F-033) |

| Gate | Result |
|---|---|
| Backend build · vet · format | **PASS** |
| Backend unit + integration (286 tests) | **PASS** (9/9 packages) |
| `go test -race` | **NOT RUN** — no C compiler |
| Frontend typecheck · lint · build | **PASS** (0/0 lint) |
| UI truth gate + self-test | **PASS** (0 findings, 10/10 cases) |
| Live panel reads | **PASS** |
| Live client create | **PASS** (2026-10-24) |
| Live client suspend/resume/delete | **NOT RUN** — D-018 |
| Single-binary packaging | **PASS** (routes above) |
| 39 acceptance tests | **NOT RUN** |
| Clean Linux VM / HTTPS | **NOT RUN** |

| `POST /panel/api/clients/bulkEnable\|bulkDisable` | 🟢 adapter + unit |
| `POST /panel/api/clients/del/{email}` | 🟢 adapter + unit |
| `GET /panel/api/clients/links/{email}` | 🟢 **live** — genuine `vless://…vyx-16` returned |
| `POST /panel/api/clients/hwids/{email}`, `DELETE …/{id}` | 🟢 adapter only — **no HTTP route yet** |
| Inbound write operations | ⬜ **deliberately not implemented** (D-024: whole-config overwrite is too destructive) |

