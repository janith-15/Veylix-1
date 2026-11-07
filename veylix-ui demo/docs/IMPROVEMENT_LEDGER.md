# VEYLIX VPN â€” IMPROVEMENT LEDGER

**Type:** Living document â€” mandatory per `docs/new_reqierment/new_update.md` Â§12.
**Created:** 2026-10-23 Â· **Maintained by:** implementation passes, updated in the same commit as the fix.
**Rule:** never paste secrets, credentials, customer data or raw production responses into this file.
**Priority scale (`new_update.md` Â§10):** P0 critical security/data-loss Â· P1 broken core functionality / unverified remote writes / incorrect billing Â· P2 reliability, performance, maintainability Â· P3 polish.

---

## Status legend

| Value | Meaning |
|---|---|
| `OPEN` | Verified finding, no change made yet |
| `PLANNED` | Scheduled into a named implementation stage |
| `FIXED-UNVERIFIED` | Code changed, tests pass, but no live/real-panel evidence |
| `FIXED-VERIFIED` | Code changed **and** verified against a real 3X-UI panel |
| `ACCEPTED` | Intentionally deferred with a recorded reason (usually D-006 mock stage) |
| `WONTFIX` | Out of scope for v1 (D-007) |

---

## Findings

### F-001 Â· P0 Â· No backend, no database, no 3X-UI integration

| Field | Value |
|---|---|
| Affected | repository-wide â€” no `backend/`, no `go.mod`, no schema, no migrations |
| Evidence | `GAP_ANALYSIS.md` Â§1; `grep "3X-UI"` returns docs only; `grep "fetch(\|axios"` returns no production call |
| Root cause | Frontend-first build order (D-002) â€” the backend phase was never started |
| Planned change | Stage B: Go module skeleton + `internal/{config,httpapi,auth,domain,store,panel,jobs,audit}` + embedded SQLite migrations |
| Related API / DB | all `/api/*`; SQLite schema per REQUIREMENTS Â§5 |
| Tests executed | `go version` (toolchain present). No backend tests exist |
| Regression risk | none â€” additive only |
| Live verification | N/A (nothing to verify) |
| Remaining work | entire backend phase |
| Status | `PLANNED` (Stage B/E) |

**Update 2026-10-23 (Stage B):** backend module created â€” `config`, `store` (SQLite + 3 embedded migrations), `httpapi` (health/ready/404 envelopes), `cmd/veylixd`; **13 unit tests pass**, build/vet/gofmt clean. Auth, panel adapter, provisioning, jobs remain. Status â†’ **`IN PROGRESS`**.


### F-002 Â· P1 Â· Server "connection test" is simulated and falsely reports capability verification

| Field | Value |
|---|---|
| Affected | `frontend/src/stores/serversStore.ts:68-88` (`testConnection`), consumed by `pages/ServersPage.tsx:57-61` |
| Evidence | `window.setTimeout(â€¦, 1200)` resolves `'ok'` whenever `status !== 'REAUTH_REQUIRED'`; `ServersPage` then toasts *"authenticated â€” capabilities verified"* |
| Root cause | D-006 mock stage: the health check was stubbed before any panel client existed |
| Impact | Violates `new_update.md` Â§5 ("Do not silently ignore failed writes" / verify every operation) and Â§13 ("Never fabricate test results"); Bible Â§0.5 |
| Planned change | Replace with a real `GET /panel/api/server/status` call through the backend, and derive status from the HTTP outcome (see F-008 status mapping) |
| Related API / DB | `GET /panel/api/server/status` Â· `server_integrations.status`, `last_health_check` |
| Tests executed | `internal/panel` live suite against 3X-UI 3.10.2; `serversStore.testConnection` now calls `POST /api/servers/{id}/test` and maps the returned status |
| Regression risk | `ServersPage` toast copy must change in the same commit |
| Live verification | **Done** â€” `POST /api/servers/1/test` â†’ `200 ACTIVE`, latency recorded (`QA_REPORT.md` Â§0.2) |
| Status | `FIXED-VERIFIED` (live) â€” superseded the `PLANNED (Stage E3)` entry, which had gone stale |

### F-003 Â· P1 Â· Panel API credential is captured in the UI and silently discarded

| Field | Value |
|---|---|
| Affected | `frontend/src/pages/ServersPage.tsx:179-186` (`apiKey` input) â†’ `serversStore.ts:46-62` (`addServer({name, panelUrl})`) |
| Evidence | the input is bound to local state, the hint even says it is "stored encrypted on the backend", but `addServer` never receives it |
| Root cause | mock store signature written before credentials existed |
| Impact | An integration can be created and marked ACTIVE with no credential at all â€” a misleading state, and it breaks the Â§5 requirement that every panel call be authenticated |
| Planned change | Credential sent once over TLS to `POST /api/servers`, encrypted at rest (AES-GCM, key from config), never returned in any response or log |
| Related API / DB | `server_integrations.credentials_enc` Â· `POST /api/servers` |
| Tests executed | `integrations` sealing unit tests (AAD binding + tamper) and the live `POST /api/servers` run in `QA_REPORT.md` Â§0.2; `ServersPage` now passes `token` through `serversStore.addServer` |
| Regression risk | form contract change; update `ServersPage` + store in one commit |
| Status | `FIXED-VERIFIED` (live) â€” token is sealed and absent from responses and DB files (`QA_REPORT.md` Â§0.2) |

### F-004 Â· P2 Â· All application state is in-memory mock data

| Field | Value |
|---|---|
| Affected | `frontend/src/stores/*.ts` (10 stores: auth, servers, offers, admins, vpn, payments, chat, notif, audit, people) |
| Evidence | each store seeds constants and mutates them locally; nothing is persisted |
| Root cause | D-006 interim mock stage, intentional |
| Impact | reload loses everything; no server-side authorization possible |
| Planned change | replace store internals with API clients behind the same selector surface (D-006 exit), keeping page components untouched |
| Related API / DB | `docs/API_CONTRACT.md` (to be written in Stage B) |
| Tests executed | none |
| Regression risk | low if the store public shape is preserved |
| Status | `ACCEPTED` until Stage E7 |

### F-005 Â· P2 Â· No automated test suite (frontend or backend)

| Field | Value |
|---|---|
| Affected | `frontend/package.json` (no `test` script, no runner), repository root (no Go tests) |
| Evidence | `npm run lint` and `npm run build` are the only automated gates |
| Root cause | tests were never scoped into the frontend-first milestones |
| Impact | `new_update.md` Â§9 cannot be satisfied; no regression protection |
| Planned change | Go: `testing` + `httptest` mock panel + `-race`; FE: add `vitest` + `@testing-library/react` |
| Related | â€” |
| Tests executed | none |
| Status | `PLANNED` (Stage F) |

### F-006 Â· P3 Â· Documentation drift (stale route/status tables, duplicate lines)

| Field | Value |
|---|---|
| Affected | `docs/UI_INVENTORY.md:17`, `docs/ROADMAP.md:45-46` (duplicate "Admin management page"), `docs/IMPLEMENTATION_PLAN.md:66-72` (Admin/Client pages listed as pending) |
| Evidence | the listed pages/routes exist in `frontend/src/App.tsx` |
| Root cause | docs updated per-batch without a final sync pass |
| Planned change | re-sync all docs â€” **done in this pass** |
| Tests executed | n/a |
| Status | `FIXED-UNVERIFIED` (documentation only) |

### F-007 Â· P1 Â· Authorization is client-side only

| Field | Value |
|---|---|
| Affected | `frontend/src/components/auth/RequireAuth.tsx`, `frontend/src/components/chat/chat-permissions.ts` |
| Evidence | both are browser-side checks; no server exists to enforce anything |
| Root cause | no backend (F-001) |
| Impact | Regions of the requirement file Â§7 ("broken authorization", "cross-customer access / insecure object references") cannot be answered until enforcement moves server-side |
| Planned change | RBAC middleware + per-resource ownership filters + negative tests per role matrix |
| Related API / DB | all `/api/*` Â· `users.role`, `admin_server_assignments` |
| Live verification | none |
| Status | `PLANNED` (Stage E2) |

### F-008 Â· P2 Â· Server status model incomplete (4 of 6 states)

| Field | Value |
|---|---|
| Affected | `frontend/src/stores/serversStore.ts:8-12` |
| Evidence | `PENDING_VERIFICATION` and `REMOVED` are absent though REQUIREMENTS Â§7 defines six states |
| Root cause | type written for the demo seed set |
| Planned change | full enum + transition table in the backend, mirrored in the FE type. Documented mapping (to be implemented in Stage E3): HTTP 401/403 â†’ `REAUTH_REQUIRED`; timeout/5xx/DNS â†’ `CONNECTION_UNAVAILABLE`; new integration before first successful check â†’ `PENDING_VERIFICATION`; operator action â†’ `DISABLED`; removal â†’ `REMOVED` (history kept). **Outages must never produce `REVOKED`.** |
| Status | `PLANNED` (Stage E3) |

### F-009 Â· P2 Â· Ambiguous remote write outcome has no reconciliation strategy

| Field | Value |
|---|---|
| Affected | design-level (no code yet) |
| Evidence | `new_update.md` Â§5 requires "when a request times out, determine whether the remote operation was applied before retrying" |
| Root cause | nothing implemented |
| Planned change | local `outbox` table with `idempotency_key`; on timeout, re-read `GET /clients/get/{email}` before any retry; deterministic `subId` so a blind retry cannot create a duplicate |
| Related API / DB | `POST /clients/add`, `GET /clients/get/{email}` Â· `outbox`, `vpn_instances` |
| Status | `PLANNED` (Stage E4) â€” design recorded in `docs/3XUI_API_CONTRACT.md` Â§5 |

### F-010 Â· P2 Â· Usage history would be destroyed by remote counter resets

| Field | Value |
|---|---|
| Affected | `backend/internal/sync/usage.go` (new), `internal/sync/service.go`, `internal/store/migrations/0005_sync.sql` |
| Evidence | `new_update.md` Â§6: "Preserve historical usage records when remote traffic counters are reset"; `clientStats.resetCount` exists in the verified contract |
| Root cause | nothing implemented â€” E5 did not exist |
| Fix | `sync.Fold` computes the delta **per direction** and treats either a decrease or a `resetCount` above the **stored high-water mark** as a reset, in which case the lifetime total becomes `prev + current` (never losing bytes, never double-counting). Snapshot + lifetime update happen in one transaction; `ux_usage_instance_taken` makes a repeated tick idempotent. |
| Two bugs the tests caught **in my own first draft** | (1) the reset branch initially added `prevUsage` on top of `prev`, double-counting the previous reading; (2) reset detection compared `cur.ResetCount` only against the *previous reading*, so a stale read reporting `resetCount:0` after a reset was missed. Both are now regression-tested (`TestFoldNeverLosesBytes`, `TestFoldTreatsStaleResetCountAsMonotonic`). |
| Tests | `TestFoldAccumulatesDelta`, `TestFoldSurvivesRemoteReset`, `TestFoldDetectsResetWithoutResetCount`, `TestFoldDetectsResetInOneDirectionOnly`, `TestFoldNeverLosesBytes`, `TestFoldTreatsStaleResetCountAsMonotonic`, `TestFoldIsIdempotentForRepeatedReadings`, `TestLifetimeExceeded`, `TestSyncSurvivesRemoteResetEndToEnd`, `TestSyncIsIdempotentWithinOneSecond` |
| Live verification | 3 ticks against `indian1` (3X-UI 3.10.2), zero errors; lifetime totals and snapshots written |
| Related API / DB | `GET /api/vpns/{id}/usage` Â· `usage_snapshots`, `vpn_instances.lifetime_*` |
| Status | `FIXED-VERIFIED` (live + unit) |

### F-011 Â· P1 Â· Client email could traverse the panel API path

| Field | Value |
|---|---|
| Affected | `backend/internal/panel/util.go` (new), previously `ops.go` path building |
| Evidence | `TestClientEmailCannotInjectPath` failed before the fix: `path traversal escaped: "/panel/api/clients/get/../../panel/api/settings"` |
| Root cause | `url.PathEscape` does not escape `.`, so `..` survives as a path segment and the HTTP client resolves it |
| Impact | A crafted client identifier could address a different panel endpoint (e.g. a settings route) while appearing to be a client read |
| Fix | `ValidateEmail` allow-list (`A-Z a-z 0-9 - _ . @ +`, â‰¤190 chars, no consecutive dots) applied before any request; invalid input performs **zero** network calls |
| Tests | `TestValidateEmailRejectsTraversalAndUnsafeChars`, `TestInvalidEmailPerformsNoRequest`, `TestClientEmailCannotInjectPath` â€” all PASS |
| Regression risk | legitimate emails with unusual characters are rejected â€” acceptable, panel emails are system-generated (`vyx-<id>`) |
| Status | `FIXED-UNVERIFIED` (unit-proven; live panel not exercised with a malicious email) |

### F-012 Â· P1 Â· Simulated health check replaced with the real panel call

| Field | Value |
|---|---|
| Affected | supersedes F-002 at the integration level â€” `backend/internal/panel/ops.go` `Health()` |
| Evidence | live run: `LIVE OK: panel=3.10.2 xray=26.10.10 state=running â€¦ latency=323ms` |
| Root cause | F-002 (mock stub in the frontend store) |
| Fix | Real `GET /panel/api/server/status` with Bearer auth, context timeout, GET-only retry, strict envelope decode, status classification |
| Tests | unit (`TestHealthParsesRealPanelPayload`, `TestHealthMapsAuthFailuresToReauthRequired`, `TestWrongBasePathIsUnavailableNotReauth`) + **live** (`TestLiveHealth`) |
| Remaining | the frontend `serversStore.testConnection` stub is still mock until Stage E7 wiring â€” F-002 stays open at the UI layer |
| Status | adapter `FIXED-VERIFIED` (live) Â· UI `OPEN` |

### F-013 Â· P1 Â· Session cookie issued `Secure` over plain HTTP broke every authenticated request

| Field | Value |
|---|---|
| Affected | `backend/internal/config/config.go` (`resolveSecureCookies`), surfaced by the live smoke test |
| Evidence | live run: `POST /api/auth/login` returned **200** with `Set-Cookie: â€¦; Secure`, yet `GET /api/me` returned **401**. The cookie was never sent back over `http://` |
| Root cause | The `VEYLIX_INSECURE_COOKIES` flag was ANDed with the loopback check, so it silently had no effect; a developer could not distinguish "flag ignored" from "flag not set" |
| Impact | The whole app appears logged-in while every API call fails â€” a confusing, hard-to-diagnose failure |
| Fix | `resolveSecureCookies`: a non-loopback bind is **always** Secure; only a loopback bind may opt out, and only with the explicit flag. After the fix the flow returned 200 â†’ 200 â†’ 204 â†’ 401 |
| Tests | `TestSecureCookiesDecision` (7 cases incl. `0.0.0.0` must stay Secure), plus the live E2E run |
| Regression risk | none â€” production behaviour is unchanged (always Secure there) |
| Status | `FIXED-VERIFIED` (live) |

### F-014 Â· P2 Â· Rate-limiter refill truncated elapsed time and never replenished

| Field | Value |
|---|---|
| Affected | `backend/internal/auth/ratelimit.go` |
| Evidence | `TestRateLimiterRefills` failed: a saturated bucket stayed saturated even after the refill window |
| Root cause | The refill multiplied a rounded duration by the rate, which rounds to zero for small sub-second deltas |
| Fix | Refill with nanosecond precision and only advance `last` when time actually elapsed |
| Tests | `TestRateLimiterRefills` (deterministic 600 ms wait), `TestRateLimiterIsSafeForConcurrentUse` |
| Status | `FIXED-UNVERIFIED` (unit only) |

---

### F-015 Â· P1 Â· Register page reported a password as valid at 8 characters while the backend required 12

| Field | Value |
|---|---|
| Affected | `frontend/src/pages/RegisterPage.tsx` (password `hint`), `backend/internal/auth/password.go` (`MinPasswordLength`) |
| Evidence | Hint read `password.length >= 8 ? 'Looks good âœ“'`; `HashPassword` rejects anything under 12 with `ErrWeakPassword` â†’ the UI showed a green tick for a value the server answered `422 weak_password` |
| Root cause | The rule was duplicated in two places and the copy drifted; the submit handler already used 12, so the page contradicted itself |
| Impact | Violates the "no fake UI validation" rule â€” the user is told their input is acceptable and then rejected |
| Fix | `frontend/src/lib/passwordPolicy.ts` is now the single client mirror of the Go policy; the register hint counts down (`"3 more characters needed"`) and only says `Looks good âœ“` when `passwordViolations()` is empty. The server stays authoritative. |
| Tests | Server: `TestValidatePasswordPolicy*`; client: `npm run lint` 0/0, `tsc -b`, `vite build` |
| Regression risk | The client and server lists must stay in sync â€” both are documented at the top of `passwordPolicy.ts` with a pointer to `password.go` |
| Status | `FIXED-UNVERIFIED` (static verification only; no browser E2E runner yet) |

### F-016 Â· P1 Â· A user could never change their password (`RevokeAllForUser` was dead code)

| Field | Value |
|---|---|
| Affected | `backend/internal/auth/service.go`, `backend/internal/httpapi/auth_routes.go`, `frontend/src/pages/ProfilePage.tsx`, `frontend/src/stores/authStore.ts` |
| Evidence | `search_codebase "changePassword\|current_password"` â†’ 0 results; `session.go:97` documented `RevokeAllForUser` as "used after a password change" but no caller existed |
| Root cause | Stage E2 built login/register/logout only; credential rotation was never scheduled into a stage |
| Impact | Production blocker: a leaked or forgotten credential can never be rotated from inside the product |
| Fix | `auth.Service.ChangePassword` verifies the current password, applies the full policy to the new one and rotates the hash; `POST /api/auth/change-password` then **revokes every session** and re-issues the caller's cookie, so a stolen cookie cannot outlive the change. Both outcomes are audited (`auth.password_change`, `auth.password_change_failed`). Profile page gained a dialog with the same client-side policy mirror. |
| Tests | `TestChangePasswordRotatesCredential`, `TestChangePasswordRejectsWrongCurrentPassword`, `TestChangePasswordEnforcesPolicyOnTheNewValue`, `TestChangePasswordForUnknownUserIsUnauthenticated`, `TestChangePasswordRotatesSessions` (E2E: other device â†’ 401, caller â†’ 200, old password â†’ 401), `TestChangePasswordRequiresAuthentication` |
| Regression risk | Clients caching the old cookie across the change are signed out â€” intended. OAuth-only accounts (NULL `password_hash`) can set a password without supplying a current one. |
| Remaining | Email-based **reset** for a forgotten password is still missing (needs a mail transport â€” `QA_REPORT.md` Â§5) |
| Status | `FIXED-UNVERIFIED` (unit + httptest E2E green; no browser E2E) |

### F-017 Â· P2 Â· Password policy checked length only â€” `aaaaaaaaaaaa` and `password1234` were accepted

| Field | Value |
|---|---|
| Affected | `backend/internal/auth/password.go` |
| Evidence | Before: `if len([]rune(password)) < MinPasswordLength { return "", ErrWeakPassword }` â€” the only rule |
| Root cause | Minimum length was treated as sufficient entropy |
| Impact | Accounts would accept the first values a credential-stuffing list tries, making the 64 MB argon2id cost irrelevant |
| Fix | `ValidatePasswordPolicy` adds: max 512 runes (DoS guard on the hash input), reject a single repeated rune, reject straight ascending/descending sequences, reject a built-in common-password blocklist (case-insensitive). `HashPassword` calls it directly, so no future call site can skip it. Rejection messages name the **rule**, never the password. |
| Tests | `TestValidatePasswordPolicyRejectsGuessableValues` (9 cases), `TestValidatePasswordPolicyAcceptsStrongPassword` (incl. a Sinhala passphrase), `TestHashPasswordAppliesFullPolicy` |
| Regression risk | Existing weak-but-long passwords still verify (`VerifyPassword` is unchanged); only new or changed credentials are policed |
| Status | `FIXED-UNVERIFIED` (unit only) |

### F-018 Â· P3 Â· Register password field lacked `autoComplete` (password managers could not save the credential)

| Field | Value |
|---|---|
| Affected | `frontend/src/pages/RegisterPage.tsx` |
| Evidence | `LoginPage` sets `autoComplete="username"` / `"current-password"`; `RegisterPage` set neither, so no browser offered to generate or save the new credential |
| Root cause | The register form predates the login form's `autoComplete` pass |
| Fix | `autoComplete="new-password"` on the password and confirm fields, and on the Profile change-password dialog. |
| Remaining | Show/hide toggle deliberately **not** added: `Input` has no trailing-button slot, and adding one is a design-system change (D-008), not a defect fix. Tracked as optional UI work in Phase 5. |
| Tests | `npm run lint` 0/0 Â· `tsc -b` Â· `vite build` |
| Status | `FIXED-UNVERIFIED` (static only) |

### F-019 Â· P2 Â· Sidebar navigation counts are hardcoded literals

| Field | Value |
|---|---|
| Affected | `frontend/src/App.tsx` (`NAV`: `/servers` count 30, `/offers` 12, `/admins` 8, `/admin-servers` 2, `/admin-clients` 24) |
| Evidence | The numbers are literals in the nav definition; no store or API feeds them |
| Root cause | Built during the mock stage (D-006) so the rail chips would not look empty |
| Impact | Same class of dishonesty as F-002: the UI displays a number derived from nothing and which will never be true. The backend can already supply real counts. |
| Planned change | Bind each chip to its real store/endpoint; render no chip when the value is unknown instead of substituting a literal |
| Related API | `GET /api/servers`, `GET /api/vpns`, planned `/api/offers`, `/api/admins` |
| Tests | none yet |
| Status | `OPEN` â€” scheduled with Phase 5 store wiring |

### F-020 Â· P1 Â· `POST /api/setup/bootstrap` created the Super Admin but never signed them in

| Field | Value |
|---|---|
| Affected | `backend/internal/httpapi/auth_routes.go` (`bootstrap` handler) |
| Evidence | Live probe: bootstrap â†’ **201** `{role:super_admin}`, then `GET /api/me` â†’ **401**. The response carried `Set-Cookie: veylix_session=â€¦`, yet the handler never *issued* a session: the cookie was a stale `ClearCookie` inherited from the shared writer. Register/login did set one correctly, so only the first-run path was broken. |
| Root cause | The handler wrote the user JSON directly and skipped `sessions.Create` + `sessions.SetCookie` â€” the two steps every other credential path performs. |
| Impact | Nobody could ever use a fresh install: the setup succeeds, but the operator is bounced to a login form and then told their brand-new credential is wrong. Caught while wiring the `/setup` wizard. |
| Fix | Bootstrap now issues a real session (`sessions.Create` â†’ `sessions.SetCookie`) before returning 201, same contract as register/login. |
| Tests | `TestSetupStatusReflectsBootstrap`, `TestSuperAdminCreatesAdminWhoCanLogin`, `TestSuspendAdminRevokesLiveSession` (all rely on the bootstrap session), plus the live run: 3 roles â†’ `/api/me` 200 |
| Regression risk | none â€” additive; the 409 second-bootstrap path is unchanged |
| Status | `FIXED-VERIFIED` (live: bootstrap â†’ `/api/me` 200 against the running binary) |

### F-021 Â· P1 Â· No way to create an `admin` account at all

| Field | Value |
|---|---|
| Affected | `backend/internal/auth/service.go`, new `backend/internal/httpapi/admin_routes.go` |
| Evidence | `POST /api/admins` did not exist; `POST /api/auth/register` always assigns `client`; `users.role` allows `admin` but nothing ever wrote it |
| Impact | The entire reseller half of the product (assigned servers, quota, sale flow, retail price) was unreachable: those routes are role-guarded to `admin`, and no such account could exist |
| Fix | `auth.Service.CreateUserByRole` (super-admin only, server-side role, password policy applied) + `GET/POST /api/admins` and `PATCH /api/admins/{id}` behind `PermAdminManage`. Suspension revokes every live session; a super admin cannot suspend themselves; quota changes against a non-admin return 404 so role membership is not leaked. |
| Tests | `TestSuperAdminCreatesAdminWhoCanLogin`, `TestAdminRoutesRequireSuperAdmin` (adminâ†’403, clientâ†’403, anonâ†’401), `TestCreateAdminRejectsWeakPasswordAndClients`, `TestSuspendAdminRevokesLiveSession`, `TestSuperAdminCannotSuspendSelf`, `TestSetQuotaLimitOnNonAdminIsNotFound` |
| Live verification | RBAC matrix on the running binary: `/api/admins` = 200/403/403, `/api/audit` = 200/403/403, `/api/servers` = 200/200/403, `/api/quotas/me` = 200/200/403, admin creating an account = 403, self-suspend = 422 |
| Remaining | "Delete an admin" is not offered â€” removal is `SUSPENDED`, so the audit trail and any owned VPNs keep a valid owner |
| Status | `FIXED-VERIFIED` (live + unit) |

---

### F-022 Â· P1 Â· Every screen was blocked behind a login, so UI work could not proceed

| Field | Value |
|---|---|
| Affected | `frontend/src/App.tsx` (`RequireAuth` wraps the whole shell), `backend/internal/httpapi/middleware.go` (`RequireAuth`) |
| Evidence | With the real login finished, opening any route in `vite dev` redirected to `/login`; without a seeded account even `/setup` needed a first bootstrap. Reviewing a half-built page meant signing in on every reload. |
| Root cause | The login layer is complete and correct, but there was no *development* mode â€” the only two states were "full auth" and "no screens". |
| Impact | Blocks the actual remaining work: the UI cannot be reviewed, so feature gaps stay invisible. It also invited the tempting-but-wrong fix of deleting the login code. |
| Fix | Option B from the plan â€” a **bypass, not a removal** (D-022). Frontend: `lib/devMode.ts` + `.env.development` (`VITE_DEV_AUTH=1`, gated on `import.meta.env.DEV`), personas boot straight into the app, `DevBadge` chip switches role. Backend: `config.resolveDevAuth` + `RequireAuthOrDev` + `DevPrincipal` (loopback **and** `VEYLIX_INSECURE_COOKIES=1` **and** `VEYLIX_DEV_AUTH=1`, all three). A real session always wins, so `/login` stays testable. |
| Tests | `TestDevAuthRequiresEveryCondition` (9 cases), `TestDevAuthNeverOnNetworkBind`, `TestDevAuthOffByDefault`, `TestDevAuthAllowsAnonymousAccess`, `TestDevAuthStillEnforcesPermissions`, `TestDevAuthOffKeepsRoutesProtected`, `TestDevAuthPrefersARealSession` |
| Live verification | Bypass on: all six guarded routes (`/api/me`, `/api/servers`, `/api/audit`, `/api/admins`, `/api/vpns`, `/api/quotas/me`) â†’ **200 with no cookie**; `/api/me` reports `dev@veylix.local` / `super_admin`. Bypass off: the same routes â†’ **401**. Production bundle: personas, localStorage key, flag read and the bypass branch all **0 hits** |
| Remaining | The demo persona is a fixed super admin; per-role API authorization is exercised through the frontend chip only against mock/real data, never by weakening `RequirePermission` |
| Status | `FIXED-VERIFIED` (live + unit + bundle inspection) |

### F-023 Â· P2 Â· `GET /api/quotas/me` returned 403 to a client, breaking the one shared quota call site

| Field | Value |
|---|---|
| Affected | `backend/internal/httpapi/account_routes.go` (`GET /api/quotas/me`), `docs/API_CONTRACT.md` Â§1 |
| Evidence | Live RBAC smoke on the seeded roles: `super_admin` 200, `admin` 200, **`client` 403** `{"error":{"code":"forbidden","message":"clients do not hold a reseller quota"}}`, anonymous 401. `docs/API_CONTRACT.md` Â§1 documented one response shape `{used, limit, remaining}` with no per-role exception; `frontend/src/lib/api.ts` exposes a single `myQuota()` used by the UI. |
| Root cause | The endpoint encoded a *role* rule (only resellers have a quota) as a *transport* error (403). A 403 is indistinguishable from "you are not allowed to call this" and forces every caller to special-case the client role. |
| Impact | A client screen calling `myQuota()` renders an error state instead of a "no reseller quota" state, and any future shared component would need role branching just to avoid an exception. The contract and the code disagreed. |
| Fix | D-021: every authenticated role gets `{used, limit, remaining}`; a client receives the constant `{0, 0, -1}` (`remaining:-1` = not applicable). The payload is the caller's own row and identical for every client, so nothing is leaked. `API_CONTRACT.md` Â§1 and Â§4 updated to match. |
| Tests | `TestQuotaMeForClientIsZeroNotForbidden`, `TestQuotaMeForReseller` (admin real limit + super admin `?admin_id=`), `TestQuotaMeRequiresAuth`. All via `newQuotaHarness`, which wires the real store. |
| Coverage gap closed | `newHarness` never set `Deps.Audit`/`Deps.Quota`, so `AccountRoutes` was **not mounted at all** in the httpapi suite â€” this endpoint had zero handler-level coverage before this fix. |
| Live verification | Real binary + real SQLite, seeded roles: `super_admin` â†’ `200 {0,0,-1}` Â· `admin` â†’ `200 {0,25,25}` Â· `client` â†’ **`200 {0,0,-1}`** Â· anonymous â†’ `401`. |
| Status | `FIXED-VERIFIED` (live + unit) |

- **2026-10-23** â€” **P5: reseller scope + client list ship (live + unit).** `GET/PUT /api/admins/{id}/servers` (replace semantics so the call is idempotent; super-admin only; audited) and `GET /api/clients` (super admin all / reseller only their sales / client 403). Frontend: new `clientsStore`, `AdminsPage` server-scope dialog, and `AdminClientsPage` + `SaleFlowPage` now use real clients and the real PREMIUM catalogue instead of demo names and invented prices. **Live proof:** assigning server 1 made it appear in the reseller's own `/api/servers` immediately â€” before this they saw zero servers forever. Evidence: 189 tests, tsc 0, lint 0/0, build OK.

- **2026-10-23** â€” **PHASE A / P3: offer catalogue ships (live + unit).** `GET/POST /api/offers` + `PATCH /api/offers/{id}` with role scoping (a client is served **active FREE only**), `PermOfferManage` on writes, and audit actions `offer.create` / `offer.update`. Offers are never deleted â€” assignments reference them and the record of what was sold must survive, so `active=false` is how an offer leaves the catalogue. Frontend: `offersStore` and `OffersPage` are real, with a `kind` selector and a panel-backed inbound picker. **The blocker is gone:** `POST /api/vpns {"offer_id":1}` â†’ 201, an instance whose expiry matches the offer. Evidence: 181 tests, `tsc` 0, lint 0/0, build OK, live role checks. **Next: P5 (server assignment + clients).**

- **2026-10-23** â€” **STEP 1: truth pass (Phase 5).** `lib/api.ts` gained the VPN namespace it was missing entirely (0 â†’ 6 methods). `vpnStore`, `auditStore` and `adminsStore` were rewritten against the live API; their seeded rows, the `vless://demo-â€¦` link generator and the client-side `audit.log()` writer are **deleted**. Where the backend cannot confirm a field (`plan`, `link`, per-admin `servers`/`quotaUsed`, `purchases`) the UI now says so instead of inventing it. 10 pages updated. Evidence: `tsc` 0 Â· lint 0/0 Â· build OK Â· bundle grep clean Â· backend untouched (173 tests pass) Â· shapes verified live. **Next: STEP 2 â€” offers API.**

- **2026-10-23** â€” **Stage E5 COMPLETE â€” sync jobs + usage snapshots (D-023, F-010 closed).** New `internal/sync` (fold math, per-server service, scheduler), migration `0005_sync.sql` (schema 4â†’5), read-only routes `GET /api/vpns/{id}/usage`, `/api/servers/{id}/metrics`, `/api/sync/status`. Three new env knobs. **173 tests** (was 141). Live: 3 clean ticks against `indian1` (3X-UI 3.10.2) with real CPU/RAM recorded. The tests caught two real bugs in the first draft of the fold (double-count on reset, and a stale `resetCount` masking a reset). **Next: E6** (links/QR).

- **2026-10-23** â€” **F-023 (quota endpoint role contract, D-021).** Client `403` â†’ `200 {0,0,-1}`; three handler-level tests added on a harness that actually mounts `AccountRoutes`; `API_CONTRACT.md` Â§1/Â§4 corrected. **141 tests**, build/vet/gofmt clean, lint 0/0, build OK.

- **2026-10-23** â€” **F-022 (dev auth bypass, D-022).** Added a development-only bypass so every screen can be reviewed without signing in, without deleting the (finished, live-verified) login system. Frontend `VITE_DEV_AUTH=1` (gated on `import.meta.env.DEV`, tree-shaken from production), backend `VEYLIX_DEV_AUTH=1` (loopback + insecure-cookies + explicit flag, all three required). Live: 6 guarded routes 200 with no cookie when on, 401 when off; production bundle has zero bypass logic. **138 tests**, build/vet/gofmt clean.

### F-028 Â· P0 Â· A refused panel write still left a VPN row behind, and an offer with no inbound was never rejected

- **Found:** 2026-10-24 while verifying F-027 live. `POST /api/vpns` returned **201 `ACTIVE confirmed:true`**, yet the client did not exist on the panel.
- **Cause (two defects in `provisioning.Create`):**
  1. **`inbound_id = 0` was never validated.** Offers 1 and 2 (`Phase A Free` / `Phase A Premium`) have no inbound attached, so the request carried `inboundIds: [0]`. The panel rejected it â€” the outbox shows five such rows â€” but Veylix had already written a VPN row. Only `ServerID == 0` was checked.
  2. **A definite rejection stored `REVOCATION_PENDING`.** That state claims "a client existed and was deleted", which was never true. The VPN then appeared in every list, permanently, with a status that implies an action nobody took. This is the direct cause of the phantom VPNs the owner kept seeing.
- **Impact:** Customers could be listed a VPN that does not exist; quota was consumed by creates that never produced a client; and the audit trail recorded `vpn.provision result:OK` for a failure.
- **Fix (D-024.1a):**
  - `loadOffer` rejects an offer with no inbound (`ErrOfferIncomplete` â†’ **422 `offer_incomplete`**) **before** anything is written.
  - A panel rejection is now a failure: `markOutboxFailed`, then `discardProvisional` rolls back the VPN row **and** its assignment **and** releases the quota slot â€” in one transaction â€” and the response is **502 `panel_rejected`** with `recorded:false`. The outbox row survives for diagnosis.
  - A successful create reports `recorded:true`.
- **Live verification (2026-10-24, real panel):** `POST /api/vpns {offer_id:1}` â†’ **422 offer_incomplete**, VPN count unchanged; `POST /api/vpns {offer_id:3}` â†’ **201 `confirmed:true, recorded:true`**, and `POST /api/vpns/15/verify` â†’ `{"exists":true,"status":"ACTIVE"}`. A live VPN then correctly refused `DELETE` with **409 `vpn_still_on_panel`**, and `revoke` reached **REVOKED** on retry after a `TLS handshake timeout` had left it `REVOCATION_PENDING` â€” the outage-never-revokes rule holding in practice.
- **Tests:** `TestCreateRefusesOfferWithoutInbound`, `TestPanelRejectionLeavesNoVPNRow`, `TestPanelRejectionDoesNotClaimRevocation`.
- **Status:** âœ… **FIXED** â€” 246 tests pass, build/vet/gofmt clean.

### F-027 Â· P0 Â· A VPN deleted in 3X-UI stayed `ACTIVE` forever in Veylix

- **Found:** 2026-10-24 by the **owner**, live: *"clear à·€à·™à¶ à·Šà¶ à·’ à·€à·™à¶± à¶šà¶»à¶±à·Šà¶±à·š à¶±à·à¶­à·’ à¶’à·€à· à¶…à¶ºà·’à¶±à·Š à¶šà¶»à¶±à·Šà¶±, client à¶§ à·€à·à¶© à¶šà¶»à¶±à·Šà¶±à·š à¶±à·à¶­à·’ à¶’à·€à· remove à¶šà¶»à¶±à·Šà¶±"* â€” the 3X-UI panel had been cleared, yet Veylix still listed 11 VPNs, 4 of them **`ACTIVE`**, none of which existed on the panel.
- **Cause (two layers, both real):**
  1. `sync.applyReadings` did `continue` for a client missing from the page-through â€” it neither recorded the absence nor left a trace, so nothing ever reconciled the row (the old test `TestSyncDoesNotRevokeUnseenClient` **asserted this wrong behaviour**).
  2. **The adapter could not detect absence at all.** `panel.ClientExists` treated the panel's answer for an unknown client as a failure. The panel does **not** return 404: it returns `HTTP 200` + `{"success":false,"msg":"Obtain (record not found)"}`, which `decodeEnvelope` turned into an error. Every "is this client gone?" question therefore came back as "unknown", so D-024.5 could never be satisfied.
- **Impact:** The product lied. A customer could be shown a working VPN whose panel credential had been deleted â€” the exact failure D-024 exists to prevent â€” and an operator had **no way** to correct it.
- **Fix:**
  - `panel.ClientExists` now recognises the panel's `record not found` envelope as a positive *absence* (HTTP 404 still counts too), while every other failure stays an error.
  - `sync.applyReadings` performs the **verified-absence** read: gone â†’ `SERVER_UNAVAILABLE` (never `REVOKED`); exists â†’ untouched (page was short, not the panel); read failed â†’ **nothing changes**. New counters `Absent` / `Unverified`.
  - `POST /api/vpns/{id}/verify` (on-demand) and `DELETE /api/vpns/{id}` (local record only, refused with 409 while the panel still holds the client, admin-only) with audit actions `vpn.absent` / `vpn.remove_absent`.
- **Live verification (2026-10-24, real panel):** `POST /api/vpns/11/verify` â†’ `{"exists":false,"status":"SERVER_UNAVAILABLE"}`; `DELETE /api/vpns/11` â†’ **204**; the whole list went to `[]` and `quota used` to `0`; the panel's own clients (`DNuser`, `yasa`, `athoof`) were **never touched** and the integration stayed `ACTIVE`.
- **Tests:** `TestClientExistsHandlesPanelRecordNotFoundEnvelope`, `TestSyncMarksVerifiedAbsentClient`, `TestSyncDoesNotMarkWhenVerificationFails`, `TestSyncKeepsStatusWhenClientExistsButPageWasShort`, `TestVerifyPanelMarksAbsentVPNUnavailable`, `TestVerifyPanelLeavesLiveVPNAlone`, `TestVerifyPanelRefusesWhenPanelUnreadable`, `TestRemoveAbsentDeletesLocalRecordOnly`, `TestRemoveAbsentRefusesLiveClient`, `TestRemoveAbsentRefusesClientRole`.
- **Status:** âœ… **FIXED** â€” 243 tests pass, build/vet/gofmt clean.

### F-024 Â· P0 Â· Hardware limit (`limitHwid`) was never sent, so device limits silently did nothing

- **Found:** 2026-10-24 by the **owner**, live, while inspecting a VPN this build had created on the real panel: *"eke usage duration ga ip limit ekath hariyata set wela thibba. hardware limit eka withari set wela thibbe neththe."* Capacity, expiry and IP limit were all correct on the panel; the hardware limit was absent.
- **Cause:** `panel.ClientSpec` (`backend/internal/panel/types.go:116-131`) has no `LimitHwid` field. The field list documented for `POST /panel/api/clients/add` includes `limitHwid`, so the API accepts it â€” Veylix simply never sent it.
- **Impact:** Every VPN created by this build has an **unenforced device limit**. A user can attach unlimited devices past the intended cap, which is both a product and a revenue defect.
- **Fix (2026-10-24):** `ClientSpec.LimitHwid` + `ClientRow.LimitHwid` added and populated by `buildClientRequest`; `ClientSpec` also gained `UUID`/`Password`/`Auth` (D-028.6) and the adapter gained `GetClient`/`UpdateClient`/`Links`/`Hwids`/`DeleteHwid`.
- **Live verification:** `POST /api/vpns` â†’ `201 ACTIVE confirmed:true` (`vyx-11`); reading the panel back returned `"limitIp":2,"limitHwid":2,"totalGB":10737418240,"enable":true`. Regression tests `TestAddClientSendsLimitHwid`, `TestBuildClientRequestSendsBothLimits`, `TestPayloadRoundTripPreservesLimits` (the last two fail with `limitHwid = 0, want 2` when the fix is reverted).
- **Status:** âœ… **FIXED** â€” 234 tests pass, build/vet/gofmt clean.

### F-026 Â· P1 Â· The panel nests a client inside `obj.client`, not at `obj`

- **Found:** 2026-10-24 while live-probing the fix above. `GET /panel/api/clients/get/{email}` returns `{"obj":{"client":{â€¦},"inboundIds":[â€¦],"externalLinks":[],"usedTraffic":N}}`.
- **Cause:** the first draft of `GetClient` unmarshalled `obj` straight into the record. `encoding/json` ignored the extra key, so the call returned a **zero-valued client** â€” no limits, `enable:false`, empty uuid â€” and reported success. That silently reported "this VPN has no limits" for every client.
- **Impact:** had this shipped, every consumer of `GetClient` (edit, credential display, panel inspector) would have shown a VPN stripped of its limits while the panel held the real values.
- **Fix:** decode the nested `client` wrapper first (accepting `inboundIds` beside it), then fall back to a bare record and a single-element array for version tolerance. Regression test `TestGetClientDecodesFullRecord` uses the **real** nested payload; `TestGetClientAcceptsUnnestedRecord` covers the alternative.
- **Status:** âœ… **FIXED** â€” found by its own live verification, which is why live checks accompany unit tests.

### F-025 Â· P0 Â· A "VPN" row exists before the panel confirms the client

- **Found:** 2026-10-24, owner-reported symptom (VPNs listed in Veylix that are not visible as clients in 3X-UI) and confirmed by reading the code.
- **Cause:** `provisioning.Create` inserts `vpn_instances` with `panel_email=''`, `panel_sub_id=''`, `status='PENDING'` (`service.go:211-214`) **before** calling the panel (`:279`). A rejected write then stores `REVOCATION_PENDING` (`:298-302`), which claims a client existed and was deleted.
- **Impact:** The list shows VPNs that do not exist; the customer is told a VPN is "pending revocation" when none was ever created.
- **Fix:** invert the order (panel write â†’ `GET /clients/get/{email}` â†’ record) and leave **no row** on failure (D-024.1a). Tests T-29/T-30.
- **Status:** `OPEN` â€” specified by D-024.1a.

### F-029 Â· P1 Â· "Tag locks once issued" was enforced in the store but never triggered

| Field | Value |
|---|---|
| Affected | `internal/store/tags.go` (`LockClientTag`, `ErrTagLocked`), `internal/provisioning/service.go` (`Create`) |
| Evidence | `ErrTagLocked` had exactly one producer (the conditional upsert in `SetClientTag`) and **no caller ever set `locked_at`**; `grep LockClientTag` matched only its definition and its own test |
| Root cause | Step 2 shipped the store primitive; the create path that must call it was written in an earlier stage and never revisited |
| Impact | D-025.2 did not hold: a VPN could be re-tagged FREEâ†”PREMIUM **after** it was sold, retroactively changing what the customer paid for. Live testing could not have caught it â€” the write simply never happened |
| Planned change | Lock inside the create transaction (same tx as the insert, so no unlocked window); release the lock when `discardProvisional` rolls the VPN back; expose `TagLocked` on the create result |
| Related API / DB | `POST /api/vpns` (`tag_locked`), `PUT /api/panel-clients/{server_id}/{email}/tag` (409) Â· `panel_client_tags.locked_at` |
| Tests executed | `TestIssuingVPNLocksItsTag` (issue locks, re-tag â†’ `ErrTagLocked`), `TestRejectedCreateDoesNotLockTag` (rollback releases), `TestTagIsRefusedOnceIssued` (409 + DENIED audit) |
| Regression risk | Low â€” additive inside an existing transaction; the rollback release is conditional on the row existing |
| Live verification | Not run â€” needs a real panel to confirm a client |
| Remaining work | Tag button on the panel-clients screen |
| Status | `FIXED-UNVERIFIED` (unit + real-SQLite HTTP tests; no live panel) |

### F-030 Â· P2 Â· The VPN list could not distinguish a panel-confirmed VPN from an unverified local record

---

# Milestone 6 â€” owner review 2026-10-25

> Recorded from an owner walkthrough of the shipped build plus a code trace performed the same day.
> Governing decisions: **D-030 â€¦ D-033**. Work plan: `docs/MILESTONE_6_PLAN.md` (Phase A â€¦ F).
> Owner intent, verbatim: *"hadapu ewageth adupadu thiyena then thiynawa â€¦ main point eka premium vpn sell kirima super admin direct client ta ha admin haraha client ta â€¦ client lara premium free wenama clearwa bala ganna thiyenna one â€¦ vpn specially premium vpn walata adala hema deyakma function wenna one."*

### M6-01 Â· P1 Â· `ConfirmPayment` flips a status and builds nothing

- **Affected:** `backend/internal/store/payments.go` (`ConfirmPayment` â†’ `transitionPayment`), `backend/internal/httpapi/payment_record_routes.go`
- **Evidence:** `transitionPayment` executes one `UPDATE payment_records SET status = ?, confirmed_by = ?`; no caller reaches `provisioning.Create`. `BuyPremiumPage.tsx:212` tells the customer *"the VPN activates right after confirmation"*.
- **Root cause:** billing was built as a settlement ledger (P6) before D-031 defined confirmation as the release action; the two were never joined.
- **Impact:** a customer pays, the operator confirms, and no VPN exists â€” while the customer-facing copy states the opposite.
- **Planned change:** D-031 â€” `ConfirmPayment` runs in one transaction: payment `CONFIRMED` â†’ plan slot consumed â†’ panel write â†’ fresh read â†’ record â†’ links; failure returns the payment to `PENDING` with no VPN row.
- **Status:** `PLANNED` (Milestone 6 Â· Phase A Â· A8)

### M6-02 Â· P0 Â· The payer chooses the price

- **Affected:** `frontend/src/pages/BuyPremiumPage.tsx:52-64`, `backend/internal/httpapi/payment_record_routes.go:52-56`
- **Evidence:** the form posts `amountCents: Math.round(units * 100)` from a free-text input, and the route accepts `amount_cents` with no reference to an offer. `vpn_offers` has **no price column** (`migrations/0002_business.sql:5-17`).
- **Impact:** revenue integrity â€” a customer can pay any number, and no screen can state a plan's price because none is stored.
- **Planned change:** D-031.1 â€” `price_cents`/`currency` on the plan; the server computes the amount; a client-supplied `amount_cents` is refused.
- **Status:** `PLANNED` (Phase A Â· A2/A7)

### M6-03 Â· P1 Â· A plan cannot be deleted or archived

- **Affected:** `backend/internal/httpapi/offer_routes.go` (GET/POST/PATCH only), `backend/internal/store/offers.go:151-155`, `frontend/src/pages/OffersPage.tsx`
- **Evidence:** no `DELETE /api/offers/{id}` handler exists; the store comment declares offers undeletable; the UI has Edit + Active only.
- **Impact:** a mis-created template stays in the catalogue forever; the operator's only tool is deactivation.
- **Planned change:** D-030.6 â€” `archived_at`; `DELETE` hard-deletes only a plan with no dependent row, otherwise `409 plan_in_use`.
- **Status:** `PLANNED` (Phase A Â· A2/A3)

### M6-04 Â· P1 Â· A VPN cannot be separated into Free / Premium on the client surface

- **Affected:** `backend/internal/store/offers.go:70-71`, `backend/internal/httpapi/vpn_routes.go:36-50`, `frontend/src/lib/api.ts:71` (`ApiVpn`), `frontend/src/stores/vpnStore.ts:21-22`
- **Evidence:** a client is served `kind = 'FREE'` offers only; `VPNRow` carries no `offer_id`/`offer_kind`; the TS `ApiVpn` interface drops the `tag`/`source` fields the server already sends; the store comment says labelling a plan "would be a lie".
- **Impact:** the owner's explicit requirement â€” Free and Premium clearly separate for a client â€” has no data behind it.
- **Planned change:** D-032 â€” plan classification on the wire, `GET /api/plans` with facts-only access for a client, two distinct client sections.
- **Status:** `PLANNED` (Phase A Â· A9/A12, Phase C Â· C9)

### M6-05 Â· P1 Â· An unowned premium plan has no gate at all

- **Affected:** `backend/internal/provisioning/service.go` (`authorise`, `Create`), `backend/internal/store/migrations/0002_business.sql`
- **Evidence:** `authorise` checks the actor's role and the offer's `kind`/`active` only. Nothing records which reseller owns a plan, and nothing stops a reseller releasing a plan he never bought. `vpn_offers` has no `owner_user_id`.
- **Impact:** the owner's requirement â€” an unowned premium plan must be unusable without the Super Admin â€” is not expressible today.
- **Planned change:** D-030 â€” `owner_user_id` + `slots_*`; `403 plan_not_approved` for an unowned premium plan; slot decrement inside the provisioning transaction.
- **Status:** `PLANNED` (Phase A Â· A2/A4/A5)


| Field | Value |

### M6-06 Â· P1 Â· A reseller is not checked against the server assignment when provisioning

- **Affected:** `backend/internal/provisioning/service.go` (`authorise`)
- **Evidence:** `auth.CanAccessServer` and `admin_server_assignments` exist and are used elsewhere, but `authorise` never calls them; an admin can release onto any `server_id` an offer names.
- **Impact:** cross-tenant provisioning risk.
- **Planned change:** gate a reseller's release on a server assigned to him.
- **Status:** `PLANNED` (Phase A Â· A6)

### M6-07 Â· P1 Â· Cross-tenant `vpn_count` on the reseller client list

- **Affected:** `backend/internal/store/assignments.go:30-31`
- **Evidence:** `ClientsVisible` counts `(SELECT COUNT(*) FROM vpn_instances i WHERE i.owner_user_id = u.id)` â€” no seller scope â€” while the row filter below it *is* seller-scoped.
- **Impact:** a reseller sees a VPN count that includes other resellers' sales to the same customer.
- **Planned change:** scope the count through `vpn_assignments.seller_user_id`.
- **Status:** `PLANNED` (Phase A Â· A13)

### M6-08 Â· P1 Â· No QR code anywhere in the product

- **Affected:** `frontend/src/pages/VPNDetailPage.tsx:321-352`, `frontend/src/components/ui/` (19 primitives), `frontend/package.json`
- **Evidence:** the connection card renders `<code>{uri}</code>`, Copy and "Open in app"; no QR component, library or in-repo encoder exists.
- **Impact:** a phone user has no practical way to import a configuration; the owner names this explicitly.
- **Planned change:** D-033 â€” in-repo SVG QR encoder, no npm dependency, every role's detail screen.
- **Status:** `PLANNED` (Phase C Â· C4)

### M6-09 Â· P2 Â· The UI reads as a template: no page-level layout

- **Affected:** `frontend/src/pages/*.css` (`offers.css` 137 B, `admins.css` 672 B), `frontend/src/components/ui/*`
- **Evidence:** the design system (`ui.css` 23 KB, tokens, a11y) is strong, but every screen is the same `dash` + `Card` + `Table` composition; the primitives a product screen needs (DataGrid, Drawer, StatStrip, Timeline, Stepper, SegmentedControl, confirm dialog, QR) are absent.
- **Impact:** feature-parity gaps read as visual gaps; the product looks unfinished.
- **Planned change:** Phase F â€” primitives, page CSS, one header pattern.
- **Status:** `PLANNED` (Phase F)

### M6-10 Â· P1 Â· No Super-Admin path to sell premium directly

- **Affected:** `frontend/src/App.tsx:212` (`/sale` is `roles={['admin']}`), NAV
- **Evidence:** `provisioning.authorise` already allows `RoleSuperAdmin`; no route or nav entry exposes it.
- **Impact:** the owner's *"super admin direct client ta"* requirement has no UI.
- **Planned change:** `/sell` for both roles, role-aware outcome (SA â†’ confirmation centre; admin â†’ immediate VPN).
- **Status:** `PLANNED` (Phase B Â· B3)

### M6-11 Â· P2 Â· No overviews: three screens show lists, not a picture

- **Affected:** `frontend/src/pages/AdminOverviewPage.tsx`, `ClientDashboardPage.tsx`, `DashboardPage.tsx`
- **Evidence:** the SA dashboard is server health only (sync/CPU/network) with no business KPIs; the admin overview is four counters; the client page is a grid with no usage total, expiry countdown or pending-order state. No `/api/overview` endpoint exists.
- **Planned change:** one role-aware `GET /api/overview` + three redesigns.
- **Status:** `PLANNED` (Phase A Â· A11, Phase D)

### M6-12 Â· P2 Â· Network counters are recorded as zero

- **Affected:** `backend/internal/sync/service.go` (`recordMetrics`)
- **Evidence:** the insert hard-codes `net_up, net_down` to `0`, while `server_metrics` has the columns and the panel `HealthResult` carries `NetIO`.
- **Impact:** a network chart would draw a flat zero line â€” the exact dishonesty D-026.3 forbids.
- **Planned change:** store the delta between consecutive `netIO` readings; the chart states "whole server".
- **Status:** `PLANNED` (Phase A Â· A15)

### M6-13 Â· P2 Â· Per-VPN history exists but no client can see a trend

- **Affected:** `frontend/src/pages/VPNDetailPage.tsx` (progress bar only), `backend/internal/httpapi/usage_routes.go`
- **Evidence:** `GET /api/vpns/{id}/usage` returns points and a `history` verdict; the UI draws one aggregate bar.
- **Planned change:** timeline chart with 7d/30d/All + type switch (D-026).
- **Status:** `PLANNED` (Phase C Â· C5)

### M6-14 Â· P2 Â· Chat cannot be started with a client who has not bought yet

- **Affected:** `backend/internal/store/directory.go:45-50`
- **Evidence:** the admin branch of `Directory` requires `EXISTS (SELECT 1 FROM vpn_assignments â€¦ seller_user_id = caller)`.
- **Impact:** a reseller cannot answer a pre-sales question, and a client created by the Super Admin is unreachable.
- **Planned change:** include the reseller's own clients regardless of assignment.
- **Status:** `PLANNED` (Phase A Â· A19)

### M6-15 Â· P3 Â· Screens still describe shipped features as "not built"

- **Affected:** `frontend/src/pages/VPNDetailPage.tsx:424` (*"once the chat API exists (P7)"*), `frontend/src/pages/ChatPage.tsx:37` (*"Roles (mock)â€¦"*)
- **Impact:** user-visible false statements; `npm run audit` does not catch them because they name a plan stage rather than a capability.
- **Status:** `PLANNED` (Phase F Â· F4)

|---|---|
| Affected | `GET /api/vpns`, `internal/httpapi/vpn_lister*.go` |
| Evidence | Rows were identical in shape whether they came from the panel or from `vpn_instances` alone; during a panel outage the list silently showed local rows as if nothing were wrong |
| Root cause | The endpoint evolved from a pure SQL read before the panel became the source of truth (D-024) |
| Impact | An operator could read an outage as "these VPNs are healthy on the panel", or read an empty list as "there are no VPNs" |
| Planned change | `source: panel\|local` on every row; degrade on outage instead of erroring, and error only when nothing was verified **and** no local rows exist |
| Related API / DB | `GET /api/vpns` |
| Tests executed | `TestPanelOutageDegradesToList`, `TestUnreadablePanelWithNoLocalRowsIsAnError`, `TestPanelAndLocalRowsMerge` |
| Regression risk | Low â€” additive JSON fields; ordering made deterministic |

### F-031 Â· P2 Â· Dev-only React duplicate-key warning while notifications exist

- **Found:** 2026-10-24 by the new headless probe (`npm run verify:ui`) while reviewing the rewritten screens.
- **Evidence:** `Encountered two children with the same key, '0'` is logged 4â€“16 times on `/notifications`, `/profile`, `/client` and `/retail-price` for the **admin/client** personas, while the same pages are silent as super admin. `GET /api/notifications` returns distinct `id`s (6, 5, 4, 3, 2, 1), and forcing `items.slice(0, 0)` in `NotificationsPage` makes the warning disappear entirely â€” so it is tied to rendering the list, not to duplicate data.
- **Cause (narrowed, not yet closed):** a dev-mode key collision inside the shell's notification rendering. React reports the key as `0`; every key in that subtree is `n.id` (a string), so the collision is on a child of the list item rather than the item itself.
- **Impact:** none at runtime (development-only warning, absent from `vite build`), but it keeps the console noisy for exactly the personas a client-facing demo uses, and noise hides real warnings.
- **Fix direction:** render the row with an explicit fragment key per child (`<Fragment key>` around dot/text/marker) or give each child a stable key; then confirm the probe reports `consoleErrors: []` for the admin and client personas.
- **Not fixed by suppressing console output** â€” the probe is meant to surface this class of problem.
- **Status:** `OPEN` Â· owner-visible in `docs/LIVE_PREVIEW.md` Â§5 so it is not mistaken for a product defect.

### F-032 Â· P1 Â· Screens advertised capabilities the backend had already shipped


### F-033 Â· P2 Â· The dev launcher hung, and Vite silently moved off the checked port

- **Found:** 2026-10-24 while actually starting the preview for the owner.
- **Evidence:** `.\dev.ps1 -Log .\var\logs` never returned (the tool harness had to proceed after 300 s) although both ports came up; and Vite's default port-hopping would move to 5174 when 5173 was busy, producing a page whose every POST answered `403 cross_origin`.
- **Cause:** the readiness loop polled `$backendProc.HasExited`, which stays `$false` while PowerShell holds redirected-output reader handles; and `strictPort` was not set.
- **Fix:** wait on `/api/health` instead; pin `port: 5173` + `strictPort: true` (and mirror the proxy under `preview`); added `-Status`, `-Lan`, `-NoBrowser`, `-Log` and a first-200 wait before printing URLs.
- **Evidence:** `.\dev.ps1 -Log .\var\logs` now returns with `backend ready - dev bypass ON as dev@veylix.local (super_admin)` and `App: http://localhost:5173/dashboard`; `.\dev.ps1 -Status` shows both listeners, `dev auth: ON` and `frontend: 200`.
- **Status:** `FIXED-VERIFIED` (live).


| Live verification | `/api/vpns` on the shipped binary: `200 []`, `?tag=untagged` `200 []`, `?tag=nope` `422` (fresh DB, no integration â†’ panel path not exercised) |
| Remaining work | Real-panel run once an integration is configured |
| Status | `FIXED-UNVERIFIED` (live smoke on the binary; real-panel path pending) |

---


- **2026-10-24 (late)** â€” **F-034 / F-035 / F-036 recorded and fixed: the product became installable and the docs stopped lying.** E7 shipped as **D-029** (UI embedded with `//go:embed`, four tested routing rules, `.\dev.ps1 -Package` probes the artifact it just built). ROADMAP's "5-B ðŸ”´ not started" contradiction and the stale E5/E6/capability rows were corrected against the code. `README.md`, `docs/INSTALL.md`, `docs/FINAL_REPORT.md` (the mandatory Â§13 handover) and `docs/DOC_INDEX.md` were created. Evidence: `go test ./... -count=1` â†’ **9/9 packages, 286 test functions**; 7 new SPA tests; `tsc -b` clean; `oxlint` 0/0; `vite build` âœ“; `npm run audit` 0 findings; packaged binary route matrix verified. **Not run:** live panel writes (D-018), `-race`, clean-VM install.


- **2026-10-24 (evening)** â€” **Server edit, FREE 2/2 and the panel limit mirror.** Three defects of the same family ("a rule documented but not in the create/sync path"):
  - **`PATCH /api/servers/{id}` did not exist** while `frontend/src/lib/api.ts` called it, so every server edit failed at the transport. Implemented with partial-update semantics, re-verification before persisting, and token re-sealing when the URL changes (the AAD is URL-bound). Live: `200` + re-test; `404` for an unknown id.
  - **FREE was not fixed at 2/2 (D-028.3).** Limits came from the offer, so a FREE offer saved with `device_limit:1` (or 0) issued exactly that. Now `provisioning/policy.go:limitsFor` owns the mapping: FREE is always 2/2, PREMIUM keeps the operator's value. Live: a FREE offer with `device_limit:1` produced `limit_ip:2, limit_hwid:2` on the panel.
  - **Sync never mirrored the panel's limits (D-024.2/.4).** `sync.mirrorLimits` now copies `totalGB`/`expiryTime`/`limitIp`/`limitHwid`/`enable` into the `panel_*` columns, writes only on change, rewrites pre-mirror rows once, and clears the mirror when a client cannot be verified. Create-time columns are left as history. Live: the mirror matched `GET /api/servers/1/panel-clients` byte-for-byte.
  - **New operator binaries:** `cmd/vpnprobe` (read-only view of issued limits vs panel mirror) and `cmd/dbexec` (one guarded SQL statement; requires `VEYLIX_DBEXEC=1`, prints a row count only) â€” used to clean up this smoke test's own rows on a machine without `sqlite3`.
  - **Evidence:** build/vet/gofmt clean Â· **271 tests pass (+9)**, 0 fail Â· lint 0/0. **Still open:** panel tagging UI, D-028.1/.2 (panel email + rename), HWID/credential routes.



- **2026-10-24** â€” **Step 8 (D-025.4/.5).** The FREE caps are enforced inside the create transaction (both checked and consumed atomically), the two refusals are distinct 409 codes (`free_concurrent_limit` / `free_monthly_limit`), usage is readable before claiming via `GET /api/vpns/claims/me`, and a claim taken by a panel-refused create is returned. 278 tests.

- **2026-10-24** â€” **Step 5 verified three ways + F-030.** The tag-join visibility rule is proved in SQL (`TestSQLVPNListerHidesUntagged`), the router is exercised end-to-end, and the shipped binary answers `/api/vpns` correctly (`200 []`, `?tag=untagged` `200 []`, `?tag=nope` `422`) â€” recorded in `QA_REPORT.md` Â§0.5 with an explicit note that the real-panel path is *not* covered by that run. 275 tests.

- **2026-10-24** â€” **Milestone 5-B step 5 (D-024.3 / D-025.1).** The VPN list is panel-backed for the Super Admin and tag-gated for everyone else; `tag` + `source` are on the wire, `?tag=` filters (422 on a typo), and an outage degrades rather than empties â€” failing loudly only when nothing was verified and Veylix has no rows. T-32 green. 274 tests.

- **2026-10-24** â€” **F-029 recorded and fixed (Milestone 5-B step 4, backend).** `LockClientTag` was dead code, so D-025.2 had no enforcement; the create transaction now locks the tag and releases it on rollback, and `PUT /api/panel-clients/{server_id}/{email}/tag` ships with 409 refusals audited as DENIED. 268 tests.

- **2026-10-24** â€” **F-024 + F-025 recorded** from the owner's live inspection and a code read, alongside the five new invariants **D-024 â€¦ D-028** (panel-as-truth, tag lifecycle/FREE quotas, VPN detail, release flow, panel email identity). Both defects are `OPEN` and are step 1â€“2 of `ROADMAP.md` Milestone 5-B. **No code changed in this pass** â€” this is the specification.
- **2026-10-23** â€” Ledger created. F-001 â€¦ F-010 recorded from the baseline audit in `GAP_ANALYSIS.md`. No source file changed yet. F-006 (doc drift) fixed within this pass.
- **2026-10-23** â€” **F-011 â€¦ F-014** added during Stage E2/E3 (path traversal, live health check, cookie `Secure`, rate-limiter refill).
- **2026-10-23** â€” **F-015 â€¦ F-019** added during the password review. F-015, F-016, F-017, F-018 fixed with regression tests; F-019 (`OPEN`) scheduled with Phase 5 store wiring. Evidence: **124 tests pass**, `go build`/`vet`/`gofmt` clean, lint 0/0, build OK.
- **2026-10-23** â€” **Login system completed (F-020, F-021).** Two blocking defects found while wiring the first-run path: bootstrap never issued a session (setup succeeded, then `/api/me` 401) and no code path could create an `admin`. Fixed both, added `GET /api/setup/status`, the `/setup` wizard, `GET/POST /api/admins` + `PATCH /api/admins/{id}`, `cmd/seeddemo`, and removed the fake role switcher from Profile. Live-verified: three roles log in through the Vite proxy, RBAC matrix correct (403/401 as designed). Evidence: **131 tests pass**, build/vet/gofmt clean, lint 0/0 (97 files), build OK.
### F-034 Â· P1 Â· The backend answered 404 for `/dashboard` â€” the product could not be installed

- **Found:** 2026-10-24, verifying the owner's request that "the software must build". The build *succeeded*; the artifact was not a usable product.
- **Evidence:** `Invoke-WebRequest http://127.0.0.1:8080/dashboard` â†’ **404**. `grep -E 'embed|FileServer|index.html' internal/httpapi/router.go` â†’ **no matches**. Running Veylix meant Go + Node/Vite + a reverse proxy, with no documented path (`README.md`, `INSTALL.md` and `FINAL_REPORT.md` were all absent).
- **Impact:** P1 â€” nothing about the code was wrong, but "install this product" had no answer, and every deployment would have needed an undocumented two-service setup plus CORS/origin tuning.
- **Fix (E7 / D-029):** `internal/httpapi/spa.go` â€” `//go:embed all:dist` + client-route fallback with four rules, each tested: `/api/*` keeps the JSON 404; real files are served with `immutable` caching under `assets/`; a **missing file stays 404**; everything else serves the shell. `SPAStatus()` drives `/api/ready` (`ui: not_embedded | embedded:N_files`) and the honest 404 when no build is present. `.\dev.ps1 -Package` builds, embeds, builds the binary, then **runs it** and asserts the route matrix.
- **Related API / DB:** no schema change. New: `GET /api/ready.ui`; UI serving on non-`/api` paths.
- **Tests executed:** `internal/httpapi/spa_test.go` â€” 7 tests, all pass; `go test ./... -count=1` â†’ 9/9 packages (286 functions). Packaged binary: `/dashboard` 200, `/vpn/16` 200, `/api/nope` JSON 404, `/assets/nope.js` 404, dev bypass absent from the bundle.
- **Regression risk:** low. The SPA route is mounted last and only matches `/`, so API patterns keep priority (`TestRouterKeepsApi404AsJSON`). `go build ./...` still works on a fresh clone because `dist/.gitkeep` is committed.
- **Status:** `FIXED-VERIFIED` (packaged binary executed and probed).

### F-035 Â· P2 Â· ROADMAP contradicted itself and the code

- **Found:** 2026-10-24 during the documentation audit the owner asked for.
- **Evidence:** `docs/ROADMAP.md:108` read `## Milestone 5-B â€¦ ðŸ”´ not started` while lines 112â€“135 of the same section were almost entirely `[x]`; Milestone 5 still listed E5 (usage history) and E6 (links) as unbuilt although both are live-verified; the capability table said links "ðŸŸ¡ pending" and multi-panel "â¬œ E5"; Milestone 6 had no status after all 11 stores shipped; Milestone 7 listed no packaging work although the binary had just shipped.
- **Cause:** each feature landed and updated its *own* section, but the headers and cross-cutting tables were never revisited â€” the exact drift `new_update.md` Â§13 forbids.
- **Fix:** headers/checkboxes/table rows corrected against the code; the old contradiction is named inline in the 5-B header so a reader can see why it changed.
- **Status:** `FIXED` (documentation only, verified by re-reading the section against `go test`, the route table and the packaging probe).

### F-036 Â· P1 Â· No entry point, no install guide, no handover report

- **Found:** 2026-10-24. `Test-Path README.md` â†’ **False**; `docs/INSTALL.md` â†’ **False**; `docs/FINAL_REPORT.md` â†’ **False** (mandatory per `new_update.md` Â§13 and Phase 3 Stage H).
- **Impact:** a newcomer had 22 documents and no route through them; the Â§13 handover report â€” the one artifact that classifies every feature honestly â€” did not exist.
- **Fix:** `README.md` (run, build, verify, status), `docs/INSTALL.md` (manual single-binary + systemd + Caddy, with an explicit "not automated yet" table), `docs/FINAL_REPORT.md` (Â§13's 14 items + the mandated per-feature classification), `docs/DOC_INDEX.md` (22 docs grouped by question).
- **Status:** `FIXED` â€” and the documents state what is **not** verified (clean-VM install, `-race`, live writes) rather than implying success.

- **Found:** 2026-10-24 (owner-requested audit: "code tika ha doc tika balala eelagata develop wenna one deewal thiranaya karanna").
- **Evidence (before):** `RetailPricePage` listed three invented plans (600/900/1400 wholesale) and its Save button only toasted; `DashboardPage` rendered four invented panels, a count-up to 1,284 clients and a random "client connected" ticker labelled "demo data"; `AdminOverviewPage` said "no API yet (P5)/(P6)" months of work after `/api/clients` and `/api/payments` shipped; `SaleFlowPage` and `vpnStore` still referenced "Stage E6 / P3" for links, offers and payments that are live; `LoginPage`/`RegisterPage` claimed "demo build, data is mock (D-006)" although **0 mock stores remain**.
- **Cause:** the wiring phase replaced the data behind the screens but not the copy that described it, and nothing failed when the two drifted apart (`new_update.md` Â§13 forbids exactly this).
- **Impact:** P1 because an operator could read an invented price list or an invented health figure and act on it, and a reviewer would conclude shipped features were missing.
- **Fix:** `frontend/scripts/audit-ui-truth.mjs` (`npm run audit`) fails on 6 rule families, with a `--self-test` proving the rules fire; all 12 findings were rewritten to real endpoints (`/api/offers`, `/api/payments`, `/api/servers`, `/api/sync/status`, `/api/servers/{id}/metrics`, `/api/clients`, `/api/vpns`). Where no endpoint exists (a per-offer retail price) the UI now says so instead of inventing a number.
- **Related API / DB:** read-only; no schema change.
- **Tests executed:** `npm run audit` â†’ OK (0 findings); `npm run audit:selftest` â†’ 10/10; `npm run verify:ui` against the live backend and the real panel (`indian1`).
- **Regression risk:** the dashboard now makes 1 + N requests (N = panels) on mount; each is bounded by the adapter timeout and a single 403 cannot blank the page.
- **Status:** `FIXED-VERIFIED` (live, real panel).


---

# Milestone 7 â€” packages, orders and the Premium page (owner review 2026-10-25, second pass)

> The owner walked the product again and named the surface gaps that make it feel unfinished. Decisions **D-034 â€¦ D-038**; plan `docs/MILESTONE_7_PLAN.md`.

### M7-01 Â· P1 Â· The server list could not say **whose** VPN each client is

- **Affected:** `backend/internal/integrations/service.go` (`PanelClientView`, `loadAssignments`), `backend/internal/httpapi/integration_routes.go`
- **Evidence:** `PanelClientView` carried `Assigned bool` and `VeylixStatus` only. Opening a server therefore answered *"someone holds this"* but never **who**, and carried no `offer`, no seller, no usage percentage, no days remaining.
- **Owner's words:** *"server eka click karama e server ekata adala vpn list eka enna one â€” eken free ewa keeyada kata katada deela thiyenne, usage kohomda iwara wela thiyenne."*
- **Root cause:** the inspector was written for a read-only integrity check (Stage C) before assignment and usage existed; its projection was never widened.
- **Fix:** `panelOwner` struct + one bounded query joining `users`, `vpn_assignments` and `vpn_offers`; the view now carries owner/seller/offer identity and `limit_bytes`, `used_bytes`, `usage_pct`, `days_left`.
- **Status:** `FIXED` (build clean; the JSON is documented in `API_CONTRACT.md`).

### M7-02 Â· P2 Â· The server list had one filter and no summary

- **Affected:** `backend/internal/httpapi/integration_routes.go`
- **Evidence:** only `?tag=` existed, and the counts the owner asks for first (free / premium / assigned) were not produced anywhere.
- **Fix:** `?tag=&assigned=&usage=&q=` with 422 on an unknown value, and `?summary=1` â†’ `PolicyClientSummary` (total, free, premium, untagged, assigned, unassigned, exhausted, expiring, expired, disabled), computed on the **full** set so a filtered table cannot make the header lie.
- **Status:** `FIXED` (build clean).

### M7-03 · P1 · The Go suite went red while Phase G was in progress

- **Affected:** `backend/internal/httpapi/integration_routes.go`, `backend/internal/integrations/service_test.go`
- **Evidence:** after the `panelOwner` change the `httpapi` package failed to *build* (the tag route had lost its opening brace during the edit) and `integrations` failed with `no such table: users`, because its hand-written schema had no `users`, `vpn_offers` or `vpn_assignments` for the new join.
- **Impact:** an unproven change is indistinguishable from a proven one — exactly what `new_update.md` §13 forbids.
- **Fix:** the tag write moved into its own `TagRoutes` registration (so the inspector stays read-only by construction); the integrations test schema gained the three tables the joiner needs.
- **Evidence of fix:** `go build ./...`, `go vet ./...`, `gofmt -l .` clean; `go test ./... -count=1` → **10/10 packages ok**.
- **Status:** `FIXED-VERIFIED` (suite green, commit `1ef7e14`).

### M7-04 Â· P1 Â· A client's own VPN can be hidden by the tag join

- **Affected:** `backend/internal/httpapi/vpn_lister.go:46-51`
- **Evidence:** `SQLVPNLister` uses an inner `JOIN panel_client_tags`, so a VPN whose panel client was never tagged does not appear in its **owner's** list â€” even though it is active and paid for.
- **Owner's rule:** *"client ta load wenna one eyata adala vpn witharayi."*
- **Planned change:** the join becomes owner-scoped: a row is listed when the caller owns it, and the tag is attached when it exists. A missing tag must not hide an owned VPN; it must only keep it out of *other* people's lists.
- **Status:** `FIXED-VERIFIED (code + suite)` — 2026-10-25. `SQLVPNLister` no longer inner-joins `panel_client_tags`: a row is listed when the caller owns it, and the tag is attached when it exists. Proved by `TestSQLVPNListerClientSeesOwnVPNs` (a client sees the owned UNTAGGED VPN and never another account's) and `TestSQLVPNListerAdminNeverSeesUntagged`, plus the HTTP-level `TestVPNsAreScopedPerOwner`. Also covered end-to-end by `TestVPNUsageAgainstCapacity` (an owner reads usage for a VPN whose tag is absent). **No live-panel run in this pass** — the rule is SQL-only and needs no panel.

### M7-05 Â· P1 Â· An unassigned panel client cannot be given to anybody

- **Affected:** no route exists
- **Evidence:** `PUT /api/panel-clients/{server}/{email}/tag` classifies a client but never attaches an owner, so a client created directly in 3X-UI can be labelled and still belongs to nobody.
- **Owner's rule:** *"3x ui eken direct hadapu vpn clientla ... kemathi kenekta assign karanna puluwan wenna one ... assign karata passe eka e adala clientge premium vpn ekak hetiyata pennanna one."*
- **Planned change:** `POST /api/panel-clients/{server_id}/{email}/assign` (Super Admin, audited), creating the VPN row, the tag lock and the panel rename of D-034.5 in one transaction.
- **Status:** `FIXED-VERIFIED (code + suite)` — 2026-10-25. `POST /api/panel-clients/{server_id}/{email}/assign` exists (Super Admin, audited), creates the VPN row and tag lock, then renames the panel client and confirms. Tests: `TestAssignPanelClientRoute`, `TestAssignRenamesPanelClient`, `TestAssignRouteRefusesNonClient`, `TestAssignRouteReportsRenameFailure`, `TestAssignRouteRefusesDoubleAssign`, `TestAssignUnassignedClientToUser`, `TestAssignRefusesAnAlreadyHeldPanelClient`, `TestAssignSlotsMoveForward`. Rename failure answers `502 rename_failed`, never a silent success. **Not live-verified against a real panel** (needs a writable test panel, D-018), so the panel-write half stays `FIXED-UNVERIFIED` live.

### M7-06 Â· P1 Â· Panel emails are opaque ids (`vyx-21`), not the customer's

- **Affected:** `backend/internal/domain` (`PanelEmail`), `backend/internal/provisioning`
- **Evidence:** `PanelEmail(vpnID)` returns `vyx-<id>`; nothing links the panel row back to the person except a Veylix query.
- **Owner's rule:** *"free ofer eka clame karana kenage email eka enna one, premium eketh ehemayi â€¦ email_free_1, email_free_2 â€¦ email_paid_1, email_paid_2."*
- **Planned change:** D-034 â€” `<client email>_<kind>_<slot>` with a monotonic per-(client, server, kind) counter reserved inside the provisioning transaction; `panel_sub_id` unchanged so D-012 idempotency survives.
- **Status:** `FIXED-VERIFIED (code + suite)` — 2026-10-25. Panel emails are `<client email>_<kind>_<slot>` (`nimal@gmail.com_free_1`, `..._paid_2`), reserved inside the provisioning transaction; `panel_sub_id` stays `vyx-<id>` so the D-012 idempotency pair survives. Tests: `TestPanelEmailIsClientAndKind`, `TestPanelSlotsOnlyMoveForward`, `TestPanelSubIdStaysDeterministic`, `TestAssignRenamesPanelClient`. A real defect was found and fixed while proving it: `store.SetClientTag` only refused re-tagging a *locked* row and never locked a VPN tagged after issue (`TestIssuingVPNLocksItsTag`).


### M7-07 Â· P1 Â· The Super-Admin VPN list carries no owner, so "whose VPN" is unanswerable

- **Affected:** `backend/internal/httpapi/vpn_lister_panel.go` (`panelViewToRow`), `vpn_routes.go` (`VPNRow`)
- **Evidence:** `PanelVPNLister` merges panel clients with local rows and returns `OwnerID` from the local row only â€” a panel-only client has `OwnerID: 0`, and the row carries no e-mail or name. `Source: "panel"` says *where the row came from*, never who holds it.
- **Impact:** the owner's first question â€” *"à·™à¶šà·à¶§ à¶¯à·“à¶½à· à¶­à·’à¶ºà·™à¶±à·Šà¶±à·š"* â€” cannot be answered from `/api/vpns`.
- **Planned change:** `VPNRow` gains `owner_email`, `owner_name`, `assigned`, `offer_kind`; the panel lister fills them from the new `panelOwner` join.
- **Status:** `FIXED-VERIFIED (code + suite)` — 2026-10-25. `VPNRow` carries `owner_email`, `owner_name`, `assigned`, `offer_id/kind/name` in BOTH listers (`vpn_lister.go`, `vpn_lister_panel.go`). Covered by `TestSQLVPNListerClientSeesOwnVPNs` (asserts `OwnerEmail` and `Assigned` on every owned row) rather than a test named `TestVPNRowCarriesOwner`; this entry records the name correction so the plan and the code agree.

### M7-08 Â· P1 Â· There is no price anywhere except the client's own typing

- **Affected:** `frontend/src/pages/BuyPremiumPage.tsx`, `backend/internal/store/offers.go`
- **Evidence:** the plan row stores no price (`0008` added `price_cents`, but no screen sets it and no per-unit rate exists), so a custom-sized VPN has nothing to compute a price from.
- **Owner's rule:** *"super adminge peththen 1gb ekaka price ekaka, device 1 ka price eka, ip 1 ka price eka set karanna puluwan wenna one"* + *"price eka calculate wela pennanna one"*.
- **Planned change:** D-035.4 â€” `pricing_rates` (global default, per-server override) and `POST /api/orders/quote`; the server computes the price of a spec.
- **Status:** `PLANNED` (Phase H)

### M7-09 Â· P1 Â· A custom Premium order has no way to choose a server and inbound

- **Affected:** `backend/internal/payments` (release path), no route
- **Evidence:** a custom order's plan is chosen at order time; nothing lets the Super Admin pick the server/inbound **after** seeing the order, which is the owner's described flow.
- **Owner's rule:** *"order eka super admin ta â€¦ vpn server ekak select karala inbound ekak select karala e order ekata package eka client ta release karanna puluwan."*
- **Planned change:** D-036.6 â€” release requires `server_id` + `inbound_id`; a custom order without them answers `422 server_required`.
- **Status:** `FIXED-VERIFIED (code + suite)` - 2026-10-25 (backend). `SetReleaseTarget` (server must exist, inbound must belong to it, refused once released, audited as a `RELEASE_TARGET` event) and `POST /api/payments/{id}/release-target` (Super Admin only) exist. `payments.Confirm` refuses a custom order with `ErrReleaseTargetRequired` BEFORE reaching provisioning, and passes `CreateInput.Spec`. Tests: `TestCustomOrderNeedsServerBeforeRelease`, `TestCustomOrderReleasesWithItsOwnSpec`, `TestReleaseTargetMustMatchServer`, `TestSpecOverridesPackageLimits`, `TestSpecIncompleteIsRefused`. **The L3 screen is not built** - there is no `/orders/:id` route, so a Super Admin cannot pick server then inbound in the UI (M7-16).

### M7-10 Â· P1 Â· An order cannot be attributed, re-routed or blocked

- **Affected:** `backend/internal/store/payments.go`, `payment_record_routes.go`
- **Evidence:** a payment row records payer, method and amount â€” not the **name** of the buyer, not the method **history**, and there is no block/unblock action on the VPN it produced.
- **Owner's rule:** *"order eka demme kauda kiyana eka pehediliwa note wenna one, eyage payment method eka wetennath one â€¦ payment aulak thibboth VPN eka block karayine, aula wisa dunama aye unblock karawe."*
- **Planned change:** D-036 â€” `payment_record_events`, `POST /payments/{id}/method`, `POST /payments/{id}/block|unblock`, confirmer identity on the wire.
- **Status:** `FIXED-VERIFIED (code + suite)` - 2026-10-25 (backend). An order now carries the buyer, both ends of a method move, a proof channel and block/unblock: `0011_order_events.sql`, `payment_record_events`, `ChangePaymentMethod`, `RecordProofChannel`, `BlockOrderVPN`/`UnblockOrderVPN`, plus `POST /api/payments/{id}/{method,proof,block,unblock}` and `GET .../events`. Tests: `TestMethodChangeIsRecorded`, `TestMethodChangeToSameIsNoop`, `TestMethodChangeIsPayerOnly`, `TestProofChannelIsRecorded`, `TestBlockAndUnblockVPN`, `TestOrderTimeline`, `TestOrderProofChannelOverHTTP`, `TestBlockWithoutVPNIsRefused`. The panel is written first and confirmed, so a local SUSPENDED row cannot disagree with it. **The UI is missing**: `frontend/src/lib/api.ts` has no client for any of those five routes and there is no `/orders` screen (M7-16).

### M7-11 Â· P2 Â· Payment methods carry no contact channels

- **Affected:** `payment_methods` (`details`/`instructions` free text only)
- **Owner's rule:** *"eyata ethana gmail eka whatsapp telegram number tika pennanna one super adminge"*.
- **Planned change:** structured `contact_email`, `contact_whatsapp`, `contact_telegram` + click-to-copy and WhatsApp deep links in the payer UI.
- **Status:** `FIXED-VERIFIED (code + suite)` - 2026-10-25 (backend). `payment_methods` carries `contact_email`, `contact_whatsapp`, `contact_telegram` (`0011_order_events.sql`) and `GET /api/payment-methods` returns them; `TestPaymentMethodExposesChannels` proves all three reach the wire. **The click-to-copy and WhatsApp deep links this finding asks for are not built** - the payer UI renders none of the three fields yet (M7-16).

### M7-12 Â· P1 Â· The Premium VPN page is missing every feature the owner listed

- **Affected:** `frontend/src/pages/VPNDetailPage.tsx`, `usage_routes.go`
- **Evidence:** the page shows one usage bar, a link list and a setup guide. There is no up/down/total split, no range switch (7d/30d/3m/1y), no chart-type switch, no per-VPN live rate, no client-side ping, no protocol detail (vless/vmess, tcp/ws, uuid/username/password) and **no QR**.
- **Owner's rule:** *"me fetuers ekakwath aduwenna ba"* â€” all ten (P-01 â€¦ P-10).
- **Status:** `PARTIAL - backend complete, UI mostly absent` - 2026-10-25. Backend: `GET /api/vpns/{id}/usage` answers `lifetime_up_bytes`, `lifetime_down_bytes`, `lifetime_total_bytes`, `usage_pct` (clamped at 100), `remaining_bytes` (-1 when unlimited) and a `?range=7d|30d|3m|1y|all` switch (422 on a typo); `GET /api/vpns/{id}/protocol` (P-09) and `GET /api/vpns/{id}/traffic/live` (P-04) exist. Tests: `TestUsageRangeWindow`, `TestUsageHistorySplitsUpDownTotal`, `TestUsageRangeTypoIsRefused`, `TestVPNUsageAgainstCapacity` (added 2026-10-25), `TestProtocolDetailIsExposed`, `TestProtocolReportsOutageHonestly`, `TestLiveTrafficRate`, `TestLiveTrafficNeedsTwoReadings`. UI: only **P-10 (QR)** is wired into `/vpn/:id`. Still absent there: the up/down/total split (P-01), the range switch (P-06), live-rate chips (P-04), the protocol block (P-09), chart tabs and the chart-type switch (P-03/P-07/P-08), and **P-05** - the page still labels its ping "server reachability, not your own latency". Tracked as M7-17.

## 2026-10-25 review pass — findings recorded, nothing rebuilt

Scope: read the tree, ran every gate, compared `docs/new_reqierment/new_update.md`
and `docs/MILESTONE_7_PLAN.md` against the code, and fixed only what could be
proved. Phase L (the screens) was deliberately **not** started — it is recorded
here as remaining work, not claimed.

**Gates actually run (2026-10-25):** `go build ./...` · `go vet ./...` ·
`gofmt -l .` (empty) · `go test ./... -count=1` → **11 packages ok** · `npx tsc -b` ·
`npm run lint` → 0 warnings / 0 errors · `npm run audit` → 0 findings ·
`npm run qr:selftest` → **9/9**.

### M7-13 · P1 · `SetClientTag` never locked a VPN tagged after issue

- **Affected:** `backend/internal/store/tags.go` (`SetClientTag`)
- **Evidence:** a tag could be rewritten on a VPN that already had a live
  `vpn_instances` row, so an issued, paid-for VPN could still be re-priced.
- **Found by:** the Phase J tests (`TestIssuingVPNLocksItsTag`), not by reading.
- **Fix:** the write locks when a live VPN row exists; any later change answers
  `ErrTagLocked`.
- **Tests:** `TestIssuingVPNLocksItsTag` (green).
- **Status:** `FIXED-VERIFIED (code + suite)`.

### M7-14 · P1 · The QR refused every `vmess://` link (found this pass)

- **Affected:** `frontend/src/lib/qr.ts`, `frontend/scripts/qr-selftest.mjs`
- **Evidence:** the encoder was capped at version 10 (level M = **213 bytes**). A
  `vmess://` link is base64 JSON and routinely 250–360 bytes, so the customer saw
  "too long to encode" instead of a QR — while `vless://` worked, which is why the
  original self-test passed.
- **Root cause:** the scope note ("versions 1–10 … fits well inside that") was an
  assumption about link length that was never measured.
- **Fix:** versions **11–15** added (`TOTAL_CODEWORDS`, `RS_BLOCKS_M` rows derived
  from the level-M spec tables, `ALIGNMENT` centres), cap now **412 bytes**, and
  `MAX_VERSION` replaces the literal `10`. The error now states the byte length and
  the real limit.
- **Evidence:** each row's `data + ecc` equals the spec's codeword total (the
  encoder asserts this at run time). `npm run qr:selftest` **9/9**, including a
  264-byte `vmess://` link that must use a version above 10.
- **Regression risk:** larger modules for long links (v15 = 77×77). A phone camera
  reads that at `QrCode`'s render size, but this was **not** scan-verified with a
  physical camera.

---

## 2026-10-30 pass — authentication, sign-up, profile (F-040 … F-056)

Scope: the owner reported that Google sign-in and sign-up never completed, that a
password change "worked but then the new password would not log in", and that the
profile screen carried fields nobody could use. Every entry below was reproduced
from a **real failure** (a log line or a live response), not from reading alone.

**Gates actually run (2026-10-30):** `go build ./...` · `go vet ./...` ·
`gofmt -l .` (empty) · `go test ./... -count=1` → **13 packages ok,
436 test functions** · `npm run lint` → 0/0 · `npm run build` · `npm run audit` →
0 findings. Live stack: `/api/ready` → `{"schema_version":16}`.

---

### F-040 · P0 · Google sign-in could never complete (`nonce mismatch`)

| Field | Value |
|---|---|
| Affected | `backend/internal/auth/google/google.go` (`AuthCodeURL`) |
| Evidence | `google sign-in failed err="google: invalid id token: nonce mismatch"` on every attempt; the authorize URL had **no `nonce` parameter at all** |
| Cause | `NewHandshake` minted a nonce and `VerifyIDToken` compared it, but `AuthCodeURL(state, challenge)` had no nonce argument, so Google was never told what to echo back. The comparison could not succeed by construction. |
| Fix | `AuthCodeURL(state, challenge, nonce)` sends `nonce`; `GoogleAuth.Start` passes `hs.Nonce`. |
| Tests | `TestAuthCodeURLCarriesTheNonce` (new), `TestHandshakeIsRandomAndChallengesCorrectly` (extended) |
| Live evidence | authorize URL now carries `nonce=LKWJx…`; Google sign-in completes |
| Status | `FIXED-VERIFIED` (live, real Google client) |

### F-041 · P1 · Sign-up created an unverified account and e-mailed nothing

| Field | Value |
|---|---|
| Affected | `backend/internal/auth/service.go` (`Register`), `cmd/veylixd/main.go` |
| Evidence | `POST /api/auth/register` → 201, then login → **403 `email_unverified`**; no verification record was written, so the address could never be confirmed |
| Cause | `Register` wrote `email_verified = 0` and returned; nothing called `EmailVerifier`. |
| Fix | `Service.SendVerification` hook wired to `EmailVerifier.IssueAndSend`. A send failure does **not** roll the account back; the outcome is audited. |
| Tests | `TestRegisterSendsVerificationLink`, `TestRegisterSurvivesVerificationFailure` |
| Live evidence | audit row `{"delivery":"smtp"}` after sign-up |
| Status | `FIXED-VERIFIED` |

### F-042 · P1 · The login refusal promised a code the flow never sends

| Field | Value |
|---|---|
| Affected | `backend/internal/httpapi/middleware.go` (`WriteAuthError`) |
| Evidence | copy said *"we sent you a **code**"* while the verifier sends a **link**; users hunted for six digits that do not exist |
| Fix | *"open the **link** we e-mailed you"*. |
| Tests | `TestUnverifiedLoginMentionsTheLink` |
| Status | `FIXED-UNVERIFIED` (wording; tested, not confirmed on a device) |

### F-043 · P1 · Registration had no rate limit

| Field | Value |
|---|---|
| Affected | `backend/internal/httpapi/auth_routes.go`, `router.go` |

### F-045 · P1 · The password-reset response never told the truth about delivery

| Field | Value |
|---|---|
| Affected | `backend/internal/httpapi/password_reset_routes.go` |
| Evidence | the 202 body sent `sent` only; `ForgotPasswordPage` read `res.delivery`, which never existed, so the "we e-mailed you" branch was unreachable by construction |
| Fix | `delivery` on the wire (`smtp` / `outbox`) |
| ⚠️ Regression caught by the new test | the first version reported the **per-account** outcome, making `/api/auth/forgot` an account-existence oracle (`known@` → `smtp`, `unknown@` → `outbox`). `TestForgotDeliveryIsSameForUnknownAccount` failed and the field became a **server-level** answer. |
| Tests | `TestForgotReportsOutboxWithoutRelay`, `TestForgotReportsSMTPSWhenDelivered`, `TestForgotReportsOutboxOnWireOnlyWhenNoRelay`, `TestForgotDeliveryIsSameForUnknownAccount` |
| Status | `FIXED-VERIFIED` (live `delivery: smtp`) |

### F-046 · P1 · A password change was silent

| Field | Value |
|---|---|
| Affected | `backend/internal/httpapi/auth_routes.go` (`change-password`) |
| Evidence | a stolen session could change the password and the real owner was never told; the response was a bare `204` |
| Fix | `notifyPasswordChanged` mails time + source address, never the password or a token; `200 {changed, delivery}` so the UI says what happened. A send failure does not fail the change. |
| Tests | `TestPasswordChangeSendsNotification`, `TestPasswordChangeWithoutRelayReportsOutbox`, `TestChangePasswordRotatesSessions` (updated 204 → 200) |
| Status | `FIXED-VERIFIED` (SMTP relay configured) |

### F-047 · P2 · The verification token was built but never delivered

| Field | Value |
|---|---|
| Affected | `backend/internal/auth/google_identity.go` (`EmailVerifier`) |
| Evidence | `EmailVerifier{Store: st}` had **no mailer**: `Issue` stored a token and returned it, so only a loopback caller ever saw the link |
| Fix | `Mail` + `BaseURL` on the verifier; `IssueAndSend` mails an absolute link; `Delivery()` reports `smtp`/`outbox` |
| Tests | `TestVerifyLinkIsEmailed`, `TestVerifyLinkWithoutRelayIsOutbox`, `TestVerifyLinkReportsOutboxWhenSendFails`, `TestVerifyDeliveryIsServerLevel`, `TestVerifyTokenDefaultsTo24Hours` |
| Status | `FIXED-VERIFIED` |


### F-049 · P1 · The Super Admin could not reset a client's password

| Field | Value |
|---|---|
| Affected | `backend/internal/httpapi/client_admin_routes.go`, `router.go`; `frontend/src/pages/ClientDetailPage.tsx` |
| Evidence | the client console offered list / detail / block / FREE limit only, while **staff** already had `POST /api/super/staff/{id}/reset-password`. A customer whose mailbox is wrong or gone had no recovery path — both reset and verification are e-mails. |
| Fix | `POST /api/super/clients/{id}/reset-password` (SA only, `PermClientManage`), generated server-side, returned **once**, every session ended, audited. UI: button + confirm + one-time password dialog. `ClientAdminRoutesWithReset` keeps the old constructor working. |
| Tests | `TestSuperAdminCanResetAClientPassword` (new password authenticates, old one does not, 0 sessions survive), `TestClientResetRefusesSelf` (422 `cannot_modify_self`) |
| Live evidence | reset → `200` + 32-char password → client login `200` |
| Status | `FIXED-VERIFIED` |

### F-050 · P2 · The account menu's Profile and Settings did nothing

| Field | Value |
|---|---|
| Affected | `frontend/src/components/layout/Topbar.tsx` |
| Evidence | `<DropdownItem>Profile</DropdownItem>` / `Settings` had no `onClick`; only Sign out worked, and there is no Settings route at all |
| Fix | Profile navigates to `/profile`; the dead Settings entry was removed rather than left as a trap |
| Status | `FIXED-UNVERIFIED` (no UI test) |

### F-051 · P2 · The profile asked for an image URL and an IANA timezone

| Field | Value |
|---|---|
| Affected | `frontend/src/pages/ProfilePage.tsx` |
| Evidence | "Avatar URL" (`https://…`) and "Timezone" (`Asia/Colombo`) were free-text fields a customer cannot answer, and nothing read the timezone |
| Fix | Both inputs removed. The columns stay (no data loss, no migration) for a future upload / auto-detect |
| Status | `FIXED-UNVERIFIED` |

### F-052 · P2 · Age was stored as a number and went stale

| Field | Value |
|---|---|
| Affected | `backend/internal/store/profile.go`, migration `0016_birthday.sql`, `httpapi/profile_routes.go`, `frontend/src/pages/ProfilePage.tsx` |
| Evidence | `age INTEGER` required date arithmetic from the person and was wrong the day after it was written and every year after |
| Fix | `birthday TEXT` (ISO) + `AgeFromBirthday` deriving whole years on read; native `<input type="date">`; validation for real date / not future / 13+; `age` kept as a fallback |
| Tests | `TestAgeFromBirthday` (9 cases, incl. the leap-day trap where a `YearDay` comparison is off by one) |
| Live evidence | `PATCH {birthday:"1990-03-15"}` → `age: 36`; future → 422; under 13 → 422; `30/10/1990` → 422 |
| Status | `FIXED-VERIFIED` |

### F-053 · P2 · The date picker's calendar button was invisible

| Field | Value |
|---|---|
| Affected | `frontend/src/components/ui/ui.css` |
| Evidence | no `color-scheme` anywhere; on `[data-theme='dark']` (`--bg-input: #191938`) the UA painted a light-mode near-black glyph on a near-black field — the field looked like it had no picker |
| Fix | `color-scheme: dark` for `dark`/`gaming`, `light` for `modern`/`light`/`classic`; pointer cursor + translucent indicator; field wrapped in `ui-field` so the label and spacing match; `min`/`max` bound the calendar |
| Evidence | built CSS carries both rules (2 each); dev server serves 4 occurrences |
| Status | `FIXED-UNVERIFIED` (not confirmed on the owner's screen) |

### F-054 · P2 · Registration claimed the account was ready when it was not

| Field | Value |
|---|---|
| Affected | `frontend/src/pages/RegisterPage.tsx`, `LoginPage.tsx` |
| Evidence | after sign-up the page toasted *"Your account is ready"* and navigated to `/client`, whose every call answers 403 |
| Fix | *"Check your e-mail — we sent a confirmation link"* and a `/login?pending=<address>` notice |
| Status | `FIXED-UNVERIFIED` |

### F-055 · P2 · `?google=failed` had no message

| Field | Value |
|---|---|
| Affected | `frontend/src/hooks/useGoogleButton.ts` |
| Evidence | `googleErrorCode` can return `failed`, but the switch had no such case, so the reason fell through to a generic sentence |
| Fix | explicit `failed` case pointing at the backend log |
| Status | `FIXED-UNVERIFIED` |

### F-056 · P2 · Docs described a schema and a route count that no longer existed

---

## 2026-10-31 pass — gates that cannot lie (F-057 … F-058)

### F-057 · P2 · The docs drifted and nothing noticed

| Field | Value |
|---|---|
| Affected | `README.md`, `docs/AUTH.md`, `frontend/scripts/docs-verify.mjs` (new), `frontend/package.json` |
| Evidence | F-056 corrected the counts by hand; days later they were wrong again (this pass alone moved routes 115 → 116 and tests 431 → 436). Nothing recomputed them. |
| Fix | `npm run docs:verify` derives migrations / routes / test functions / packages from the tree and fails when the README disagrees; `npm run docs:selftest` proves the check can fail. Also fixed: README and AUTH §0 told staff to sign in at `/login`, which answers `403 wrong_portal`. |
| Evidence | during this pass the check caught three of its own drifts (`16→17` migrations, `115→116` routes, `431→436` tests) and then reported OK. Simulated drift (`115`→`99`) exits 1 with `README says 99, tree has 115`. |
| Tests | `npm run docs:verify` (0), `npm run docs:selftest` (correct 0 / stale 4 / missing 4) |
| Status | `FIXED-VERIFIED` |

### F-058 · P2 · A launched dev server could have no log at all

| Field | Value |
|---|---|
| Affected | `dev.ps1` |
| Evidence | `[string]$Log = ''` meant `.\dev.ps1` started both processes in a hidden window with **no output anywhere**. Finding F-040's one-line `nonce` bug took hours precisely because of this. |
| Fix | `-Log` defaults to `var\logs`; `-NoLog` opts out. |
| Evidence | plain `.\dev.ps1 -Restart` now prints `Logs : …\var\logs` and writes a fresh `backend.log`; `-NoLog` produces no log line (regression-tested by hand). |
| Status | `FIXED-VERIFIED` |

### F-059 · P1 · The Super Admin could block a client but not delete the account

| Field | Value |
|---|---|
| Affected | migration `0017_client_deleted.sql` (new), `store/clients_admin.go`, `httpapi/client_admin_routes.go`, `frontend/src/pages/ClientDetailPage.tsx` |
| Evidence | the console offered block + FREE limit + password reset only. `users.status` had a CHECK limited to `ACTIVE/SUSPENDED/PENDING_VERIFICATION`, so "deleted" had nowhere to live. |
| Decision | **soft delete.** `vpn_instances.owner_user_id`, `payment_records.payer_user_id` and `vpn_assignments.client_user_id` are `ON DELETE RESTRICT`, so a row delete is refused by the database — and it should be: billing history and the panel's VPNs must survive. |
| Fix | migration rebuilds `users` (SQLite cannot ALTER a CHECK) with `DELETED` added and every column/index reproduced; `ClientDelete` sets the state and ends sessions; `DELETE /api/super/clients/{id}`; `ClientSetStatus` refuses to leave DELETED (terminal); UI adds a type-the-address confirm. |
| Safety | backup taken first (`veylix.pre-delete-20261031-004317.db`, SHA-256 verified); the migration was proven on a **copy of the real database** before touching the live one. |
| Tests | `TestMigration0017PreservesUsers` (rows, data, indexes, CHECK still refuses nonsense), `TestClientDeleteLocksTheAccountOut`, `TestClientDeleteIsTerminal`, `TestClientDeleteRefusesWhileVPNsExist`, `TestClientDeleteIsIdempotent` |
| Live evidence | `DELETE → 200 {"deleted":true}`; the deleted account could not sign in; `PATCH {status:ACTIVE}` did not resurrect it; a second DELETE answered 200; `/api/ready` reports `schema_version:17` |
| Regression risk | the `users` rebuild is the riskiest single migration in the tree. It is covered by a test that copies a live database, and a verified backup sits beside it. |
| Status | `FIXED-VERIFIED` |


| Field | Value |
|---|---|
| Affected | `README.md`, `docs/AUTH.md` |
| Evidence | README said *"11 migrations · schema v11"* and *"84 HTTP routes"* (actual **16 / 115**) and *"370 tests in 11 packages"* (actual **436 in 13**); both README and AUTH §0 told a staff user to sign in at `/login`, which answers `403 wrong_portal` |
| Fix | counts recomputed from the tree; both files now state the two portals explicitly; "Not built yet" records that client delete / e-mail edit are still absent |
| Status | `FIXED-VERIFIED` (counts recomputed after the edit) |

---

### Still open after this pass

| Item | Why it is recorded rather than fixed |
|---|---|
| Client **delete account** | needs a decision on hard vs soft delete and what happens to the panel's VPN rows |
| Client **e-mail edit** | **FIXED — see D-077 below.** The console now changes the address, through a code sent to the NEW mailbox; the address does not move until that code comes back. |
| `dev.ps1 -Log` is not the default | a hidden process with no log is what made F-040 take hours to find; `-Restart -Log var\logs` is the workaround |
| Phase L screens | `/orders`, `/pricing`, `/packages`, `/servers/:id` — backends exist and are tested |
| `docs:verify` drift check | the counts in F-056 were wrong for days because nothing recomputes them |


---

## 2026-11-04 pass — the client complaints: login, QR/link, VPN detail (D-076)

Reported by the owner: *"email and password still cannot sign in; the QR shows
no link, it is generated for nothing; the VPN details page does not work
properly."* Everything below was found by tracing the code paths those three
sentences name, and each fix carries a test.

### D-076.1 · P0 · Sign-up handed out a session the login door would refuse

| Field | Value |
|---|---|
| Affected | `httpapi/auth_routes.go` (`POST /api/auth/register`), `auth/service.go` (`Register`), `frontend/src/stores/authStore.ts` (`signUp`), `RegisterPage.tsx`, `lib/api.ts` |
| Evidence | Register created a session, set the cookie and answered **201** while writing `email_verified = 0`; `Authenticate` refuses that account with **403 `email_unverified`**. The cookie worked until it expired, after which the same credentials could never sign in again — an account that was "already signed in" and permanently locked out at once. The 201 body omitted `email_verified` entirely, so the UI could not tell the two cases apart. |
| Cause | Two doors, two rules. Register assumed "created ⇒ signed in"; login required verification. Nothing reconciled them. |
| Fix | `Register` returns the row it actually wrote (`email_verified`, `verification_delivery`). The route issues a session **only** for a verified account; otherwise it answers **202** `{pending_verification, email, email_verified:false, delivery}` with **no cookie**. `signUp` resolves `'authed' \| 'pending' \| null` and the page branches on it. |
| Tests | `TestRegisterDoesNotIssueASessionForAnUnverifiedAccount` — 202, explicit `email_verified:false`, login still 403, **and no cookie in the jar** |
| Regression risk | An operator-created or auto-verified account still receives 201 + cookie, so staff and test paths are unchanged. |

### D-076.2 · P0 · The per-client FREE limit was saved and never read

| Field | Value |
|---|---|
| Affected | `backend/internal/store/claims.go` (`MonthlyLimit`) |
| Evidence | `MonthlyLimit(_ context.Context, _ int64)` ignored **both** parameters and returned `FreeMonthlyDefault`. The Super Admin could set 10 on `/clients/{id}`, the console showed *"5 / 10 (own limit)"*, and the 6th claim was refused with `free_monthly_limit`. `ClientFreeLimitOverride` existed, was correct, and had a passing test — but was called from **nowhere** on the claim path. |
| Cause | A stub written before the column existed (`0013_login_profile.sql` and `0017_client_deleted.sql` both have it) whose comment kept claiming the column was still missing. |
| Fix | `MonthlyLimit` reads `ClientFreeLimitOverride` and falls back to the platform default for `0` or an unreadable row. Every caller (`POST /api/vpns`, `GET /api/vpns/claims/me`) inherits it, because the cap has one implementation. |
| Tests | `TestMonthlyLimitHonoursThePerClientOverride`, `TestRaisedLimitAllowsTheNextClaim` (exhausts the default, proves the next claim is refused, raises the limit, proves the next claim now succeeds) |
| Regression risk | None for customers without an override — `0` still means the platform default. |

### F-048 · P1 · The verification link was relative and unclickable

| Field | Value |
|---|---|
| Affected | `backend/internal/config/config.go`, `.veylix-secrets/mail.env` |
| Evidence | `VEYLIX_PUBLIC_URL` did not exist, so the mailed link was `/api/auth/verify/<token>` — useless outside the app |
| Fix | `PublicBaseURL` from `VEYLIX_PUBLIC_URL`; set to `http://localhost:5173` in `mail.env` |

### D-076.3 · P1 · The VPN detail page could not resolve its own VPN

| Field | Value |
|---|---|
| Affected | `frontend/src/pages/VPNDetailPage.tsx` |
| Evidence | The route is `/vpns/:serverId/:panelEmail`, and the file's own comment claimed *"A panel email is matched inside its server so the same name on two panels cannot resolve to the wrong VPN."* The code read `serverId` **nowhere** and matched `panelEmail` alone, so two servers carrying the same panel address opened the wrong VPN — with the wrong link and the wrong QR. The "not found" branch then rendered one string for three different situations (still loading / not in this account's list / revoked) and offered no reload. |
| Fix | `serverId` is read and **scopes** the match (optional, so `/vpn/:id` still works); a missing VPN now names which situation applies, distinguishes *still loading*, and offers **Reload** beside the way back. |
| Tests | `npx tsc -b` 0 errors · `npm run lint` 0/0 · `npm run audit` OK. **No browser harness was run** for this branch. |
| Status | `FIXED-UNVERIFIED` (no live panel and no render check; see the note below) |

### D-076.4 · P1 · Every link failure said "the panel could not be read"

| Field | Value |
|---|---|
| Affected | `frontend/src/pages/VPNDetailPage.tsx` (links effect and its render) |
| Evidence | The catch was `() => setLinks({source:'unavailable'})`. A **404** (VPN not in this account's list), a **401** (expired session) and a genuine panel outage all produced the same "the panel could not be read" panel — wrong for two of the three and useless to act on. |
| Fix | The status code chooses the sentence: 404 → "not visible to your account, sign in with the account that bought it"; 401/403 → "your session has expired"; anything else → the outage panel. State resets during render when the VPN changes (React's documented pattern), **not** inside the effect: `oxlint` flagged the first attempt as `react(set-state-in-effect)`, and it was rewritten rather than silenced. |
| Tests | `npm run lint` → **0 warnings / 0 errors** |

### D-076.5 · P1 · The suite was red, and one "covered" regression skipped itself

| Field | Value |
|---|---|
| Affected | `httpapi/offer_routes_test.go`, `httpapi/regression_d075_test.go` |
| Evidence | (a) `TestOffersCreateAndPatch` expected **201** and got **422 `offer_incomplete`** — the fixture predated D-075's rule that an offer must name an inbound. A red suite means no new regression can be noticed. (b) `TestProtocolSurvivesMissingInbound` called `t.Skip("VPN routes are not wired in this harness")`: the D-075 harness left `VPNs` **and** `VPNConnection` nil, so `GET /api/vpns/{id}/protocol` answered 404 and the test reported as passing while asserting nothing. |
| Cause | (a) the route was corrected and the fixture was not; (b) a skip used to hide an incomplete harness. |
| Fix | (a) the fixture seeds a real `inbounds` row — the column is a genuine FK, so a bare `1` fails with error 787 — and sends `inbound_id`; the route was **not** weakened, because an offer with no inbound can never build a client. (b) `VPNs`, `VPNLister` and `VPNConnection` are wired through a `d075PanelStub`, and the skip is replaced by a `t.Fatalf` that names the regression **unguarded** if the routes vanish again. |
| Tests | `TestProtocolSurvivesMissingInbound` → **PASS** (was SKIP) · `TestOffersCreateAndPatch` → **PASS** · `go test ./... -count=1` → **12/12 packages ok** |
| Note | BUG 1 is genuinely fixed: with the route actually mounted, the protocol read answers **200** for a VPN whose `inbound_id` is 0 — the shape of every ASSIGNED panel client (D-063). The earlier pass believed this; now it is proved. |

### D-076.6 · P2 · README drifted from the tree

| Field | Value |
|---|---|
| Affected | `README.md` |
| Evidence | `npm run docs:verify` → *"migrations: README says 20, tree has 21"* and *"test functions: README says 481, tree has 489"*. The gate worked; the README had not been updated when `0021_dedupe_carrier_offer.sql` landed. |
| Fix | 21 migrations · schema v21 · 489 test functions. |
| Verification | `node scripts/docs-verify.mjs` → **"docs:verify: OK — README matches the tree."** |

### Still open after this pass

| Item | Why it is recorded rather than fixed |
|---|---|
| **Client e-mail edit** in the admin console | The console only tells the operator "their e-mail is changed by them" — impossible for the customer whose mailbox is dead, i.e. exactly the customer reset-password exists for. Needs a code-to-the-new-address flow; this is a feature, not a defect fix. |
| Verification link when **no SMTP relay** is configured | **FIXED — see D-078.** `POST /api/auth/verify/resend` is public, so an unverified account (which can never hold a session) can ask for a new link, and `/login` offers the button after an `email_unverified` refusal. |
| **Live verification** of D-076.1 … D-076.4 | No safe panel was available (production inbound, D-018) and `.veylix-secrets/mail.env` was not exercised. The Go suite and the frontend gates are the evidence; nothing here was confirmed against a running panel. |
| `go test -race` | Still unavailable on this machine (no C compiler), unchanged from the last pass. |

| Status | `FIXED-VERIFIED` |

| Evidence | `POST /api/auth/register` ran unthrottled and sends a verification e-mail per call, so one host could create accounts in a loop and burn the relay's sending reputation (which then also blocks password resets) |
| Fix | Dedicated bucket (burst 3, one per minute) + `Retry-After`; separate from the login bucket so a sign-up spray cannot lock out real sign-ins |
| Tests | `TestRegisterIsRateLimited` |
| Live evidence | `201, 201, 201, 429, 429` with `Retry-After: 49` |
| Status | `FIXED-VERIFIED` |

### F-044 · P2 · The login lockout was silent

| Field | Value |
|---|---|
| Affected | `backend/internal/auth/service.go`, `ratelimit.go`, `httpapi/middleware.go` |
| Evidence | five wrong passwords produced `429` with no hint of how long to wait, and a burst of 5 is hit immediately by anyone testing the form |
| Fix | `RateLimitedError` carries `RetryAfter` (wrapping `ErrRateLimited`, so `errors.Is` callers are unaffected); the 429 sets `Retry-After`; burst raised **5 → 10** |
| Tests | `TestLoginRateLimitBlocksAfterBurst` |
| Status | `FIXED-UNVERIFIED` (unit) |

- **Status:** `FIXED-VERIFIED (code + self-test)`; not camera-verified.

### M7-15 · P0 · The buyer still types their own price

- **Affected:** `frontend/src/pages/BuyPremiumPage.tsx:49-72`
- **Evidence:** `amountCents: Math.round(units * 100)` from a free-text input,
  posted straight to `POST /api/payments`. `api.ts` has no `/api/orders/quote`
  client, so the backend's server-computed price is unreachable from the app.
- **Impact:** revenue integrity — `new_update.md` §6 makes this the boundary the
  product must not cross, and it does.
  Owner's rule: *"price eka calculate wela pennanna one"*.
- **Why `npm run audit` misses it:** the gate looks for *invented literal* prices,
  not a user-supplied amount (M7-19).
- **Planned change:** build L4 `/pricing` (rate card), then rewrite `/buy` (L1) to
  quote via `/api/orders/quote` and drop the amount input.
- **Status:** `OPEN` — the highest-priority defect in the tree.

### M7-16 · P1 · Phase L is not built: the backend has no screen

- **Affected:** `frontend/src/App.tsx` (routes), `frontend/src/lib/api.ts`
- **Evidence:** routes are `/dashboard /servers /offers /admins /audit /admin
  /admin-servers /admin-clients /sale /retail-price /client /vpn/:id /buy /history
  /payments /notifications /profile /chat`. **Missing:** `/servers/:id` (G7/L7),
  `/orders` (L2), `/orders/:id` (L3), `/pricing` (L4), `/packages` (L5). `api.ts`
  has no client for `/api/pricing`, `/api/packages`, `/api/orders/quote`, the five
  `/api/payments/{id}/…` order routes, `/api/payments/{id}/release-target`, the
  three `/api/vpns/{id}/…` routes or `/api/servers/{id}/panel-clients` — **15 routes**
  are implemented and tested on the server with no caller.
- **Impact:** the owner's order → confirm → VPN → block flow cannot be performed in
  the product, only by hand against the API.
- **Planned change:** L4 → L2 → L7 → L1 → L3 → L5 in that order (L1 closes M7-15).
- **Status:** `OPEN`.

**Update 2026-11-03 — Phase L is built.** All five screens now exist, and each one
calls routes that previously had no caller. Verified with `npx tsc -b`,
`npm run lint` (0/0), `npm run audit`, `npm run build`, and a real-browser
render of every route (`npm run verify:ui -- <path>`, no console errors).

| Screen | Route | What it does |
|---|---|---|
| **L4** | `/pricing` | The rate card: platform default + per-server override. Money is entered in major units and converted to integer cents; a fraction of a cent is refused, and a ceiling below the floor is caught before it can price nothing. |
| **L2** | `/orders` | The queue that turns money into a VPN: Confirm (builds the VPN), Reject, Refund, Cancel, **Block / Unblock** (block requires a reason, stored on the timeline), and the order **timeline** dialog. Only transitions `allowedTransitions` permits are offered; the server still answers 409 on a stale row. |
| **L7** | `/servers/:id` | Server drill-down: every panel client with **who holds it**, FREE/PREMIUM/untagged, used-of-capacity with a bar, days left, and kind/assigned/search filters applied server-side. Reached from a new **Open** button on `/servers`. |
| **L5** | `/packages` | Catalogue CRUD for the three kinds. CUSTOM PREMIUM is Super-Admin only and has no stored price (it is quoted per order), so the price fields disappear for that kind. Archive is primary; Delete is a separate explicit confirmation. |
| **L3** | `/orders/:id` | CUSTOM release: server picker → inbound picker → **Set release target**, plus the order's timeline. |

**New client methods (`lib/api.ts`):** `paymentEvents`, `setPaymentMethod`,
`blockPayment`, `unblockPayment`, `setReleaseTarget`, `listPricing`,
`savePricing`, `quoteOrder`, `listPlans`, `listPackages`, `getPlan`,
`createPlan`, `updatePlan`, `archivePlan`, `deletePlan`, `panelClients`,
`serverInbounds` (already existed, now used).

**New stores:** `pricingStore`, `ordersStore`, `packagesStore`,
`serverDetailStore`; `serversStore` gained `loadInbounds` + an inbound cache so a
picker does not re-read a panel on every render.

**A note on where data loading lives.** The first draft fetched inside the page
with `setState` in an effect, which oxlint flags (`react-hooks/set-state-in-effect`)
— correctly, because it is a render-time side effect. Every fetch now goes
through a store, which is also the convention the rest of the app already used.
The gate is back to **0 warnings / 0 errors**.

- **Status:** `FIXED-VERIFIED` for L2/L3/L4/L5/L7 (code + browser render). The
  remaining Phase-L gap is P-05 (client-side ping), tracked under M7-17.


### M7-17 · P1 · The Premium VPN page shows a fraction of P-01 … P-10

- **Affected:** `frontend/src/pages/VPNDetailPage.tsx`
- **Evidence:** the page renders status, limits, one usage bar, links, the QR and a
  setup guide. It never calls `/usage?range=`, `/protocol` or `/traffic/live`, all
  of which exist and are tested. **P-05** is still server-side reachability,
  honestly labelled — but the owner asked for a ping *from the client's own
  device*, so `TestPingScopeIsClientNotPanel` is absent by construction.
- **Planned change:** wire P-01/P-02/P-04/P-06/P-09 into the page, then implement
  P-05 in the browser and label its scope.
- **Status:** `OPEN` (the backend portions are `FIXED-VERIFIED`).

**Update 2026-10-28 (P-01/P-02/P-03/P-04/P-06/P-07/P-08/P-09 wired):** the page now
renders the mandated feature set instead of a fraction of it.

- **New:** `frontend/src/components/vpn/PremiumInsights.tsx` (P-01/02/03/04/06/07/08),
  `frontend/src/components/vpn/ProtocolDetail.tsx` (P-09), `vpn-insights.css`.
- **P-01** up / down / total come from `lifetime_up_bytes` / `lifetime_down_bytes` /
  `lifetime_total_bytes` — the page **stopped** deriving `up + down` itself, which
  was the quiet way a chart could disagree with the API.
- **P-02** used-of-capacity bar plus the percentage, with `remaining_bytes = -1`
  rendered as "no limit" and never as "0 left".
- **P-04** the live rate from `/traffic/live`, labelled as a rate with the two
  timestamps it was measured between; `source: 'unavailable'` reads "not measured
  yet" instead of a 0 B/s that would look idle.
- **P-06** the owner's 7d / 30d / 3m / 1y (+ all) switch; each change re-reads and a
  late answer for the previous range is discarded by a request token.
- **P-07/P-08** CPU and RAM as separate switchable charts; pie / bar / line / area
  chosen by the customer.
- **P-09** protocol, transport, security, server and the credential — **masked until
  revealed**, and an outage states "panel unreadable" rather than "no credential".
- **Evidence:** `npm run verify:vpn` — **11/11 checks pass with no console errors**
  against the live dev backend and the real 3X-UI panel (VPN 17 and 21), plus
  `tsc -b` 0, `oxlint` 0/0, `npm run audit` 0 findings, `npm run build` clean.
- **Still open — P-05:** the ping button measures the *panel host*, labelled
  "server reachability, not your own latency". The owner asked for a ping from the
  customer's own browser to the VPN server; that handshake is not built, so
  `TestPingScopeIsClientNotPanel` remains absent by construction. This is the one
  item of D-038 still unimplemented, and it is stated here rather than implied.
- **Status:** `FIXED-VERIFIED` for P-01…P-04 and P-06…P-10 · `OPEN` for P-05.

### M7-21 · P1 · The QR refused a REAL panel link (442 bytes) — found by the browser check

- **Affected:** `frontend/src/lib/qr.ts`, `frontend/scripts/qr-selftest.mjs`
- **Found by:** `frontend/scripts/verify-vpn-detail.mjs` against the running app and the
  real panel — **not** by reading, and not by the self-test.
- **Evidence:** VPN 17's panel link is **442 bytes**
  (`vless://…?alpn=h2,http/1.1&encryption=none&fp=chrome&pcs=…`). The encoder's cap was
  **412 bytes** (version 15), so the Premium page rendered
  *"this link is too long to encode (442 bytes; the limit is 412 at version 15)"* where
  the QR belonged.
- **Why M7-14 missed it:** that fix raised the cap from 213 to 412 by reasoning about
  link length ("a vmess link is 250–360 bytes"). A **reality-namespace** vless URI with a
  full parameter set is longer than the vmess link that motivated the fix. The
  self-test's longest fixture was 264 bytes, so the assumption was never measured
  against a real panel.
- **Fix:** versions **16–20** added (RS block tables for level M taken from the spec's
  error-correction table; alignment centres for 16–20 added). The cap becomes
  **666 bytes**, and the encoder's own codeword assertion still throws if any row does
  not add up — a wrong row draws a code that looks fine and never scans, which for a
  credential is the worst possible failure.
- **Tests:** `npm run qr:selftest` — **10 cases** (was 9), including a fixture fixed at
  exactly **442 bytes** that must select version ≥16, and a walk from 1 to 666 bytes that
  proves **every** version 1–20 is reachable and that 667 is refused rather than
  truncated. `npm run verify:vpn` — P-10 "QR rendered" now passes on the same real link.
- **Regression risk:** a v20 code is **97×97 modules** against v15's 77×77, so it is
  denser on the same drawn width. It is drawn at the `QrCode` render size, but this was
  **not** scan-verified with a physical camera — recorded, not claimed.
- **Status:** `FIXED-VERIFIED` (code + self-test + live browser); not camera-verified.

### M7-18 · P2 · Docs drifted from the code (README, API contract)

- **Affected:** `README.md:48,57,95,99`, `docs/API_CONTRACT.md`
- **Evidence:** README claims "7 migrations · schema v7" (there are **11**),
  "72 HTTP routes" (**84** `mux.Handle` calls), "9/9 packages" (**11**) and
  "**286** tests" (the tree has **367** `func Test…`), and lists "panel-tagging UI"
  as not built although `PUT /api/panel-clients/…/tag` shipped with 5 tests.
  `API_CONTRACT.md` (2026-10-24) predates the 15 routes in M7-16.
- **Why it matters:** `new_update.md` §13 forbids reporting more than was proved; a
  stale README is the same defect in a quieter form. `npm run audit` only inspects
  `src/`, never the markdown.
- **Planned change:** a `docs:verify` check that recomputes these counts from the
  tree and fails on drift; correct the text in the same change.
- **Status:** `OPEN`.

### M7-19 · P3 · The gates cannot see the two defects that matter most

- **Affected:** `frontend/scripts/audit-ui-truth.mjs`
- **Evidence:** the UI-truth audit passes while `BuyPremiumPage` still accepts a
  typed amount (M7-15) and while five plan-mandated screens do not exist (M7-16).
  It catches invented literals and "demo data" copy — not an unwired form or a
  missing route.
- **Planned change:** add a rule that a screen may not post a money amount
  collected from a text input, and a route-inventory check that names any
  plan-mandated path absent from `App.tsx`.
- **Status:** `PLANNED`.

### M7-20 · P1 · Seven tests named by the plan did not exist

- **Affected:** the plan ↔ code naming contract (`MILESTONE_7_PLAN.md` §8 gate)
- **Evidence (before this pass):** `TestVPNRowCarriesOwner`,
  `TestClientSeesOwnVPNEvenUntagged`, `TestOrderTimeline`,
  `TestMethodExposesChannels`, `TestVPNUsageAgainstCapacity`,
  `TestAssignRenamesPanelClient` and `TestQRPayloadIsPanelLink` did not exist, so
  the plan's "every named test present" gate could not pass even where the
  behaviour was covered under another name.
- **Closed this pass:** `TestOrderTimeline` (renamed; the old name is kept as a
  forwarder), `TestVPNUsageAgainstCapacity` (written),
  `TestAssignRenamesPanelClient` (written). `TestMethodExposesChannels` was a typo
  for `TestPaymentMethodExposesChannels`, now stated in the plan.
- **Recorded, not written:** `TestVPNRowCarriesOwner` and
  `TestClientSeesOwnVPNEvenUntagged` — the behaviour is proved by
  `TestSQLVPNListerClientSeesOwnVPNs`, and the plan's names are corrected here.
  `TestQRPayloadIsPanelLink` — proved by `npm run qr:selftest` (a node script, not
  `go test`). `TestPingScopeIsClientNotPanel` — P-05 is not built (M7-17).
- **Status:** `PARTIAL` — 4 of 7 closed, 3 recorded as corrections.

---

### F-037 · P1 · The dev login could not test the admin role, and the release build could not drop it

- **Found:** 2026-10-28 (owner: *"dev log in eka client la dennayi adminuyi super adminuyi dala hariyata hadala …"*). **Followed by D-039/F-038, which deleted the bypass entirely.**
- **Evidence (before):** `DevPrincipal()` returned `super_admin` unconditionally (so the "admin" persona passed super-admin checks); `DevBadge.tsx` imported a `roleHomePath` that `devMode.ts` never exported, so **`npm run build` failed outright**; the `/profile` switch did not navigate; the built bundle still contained `devSwitchRole`; only one admin existed, so "disable an admin" (`422 cannot_modify_self`) was untestable.
- **Fix (kept from this pass):** `roleHomePath` exported, persona switching navigates, `cmd/seedacct` + `dev.ps1 -FakeAdmin` for a second admin, and `npm run preflight` as a real gate. The role-impersonation machinery described in the original entry was **removed again by D-039** — the final answer to "the admin persona is really an admin" was to delete dev personas and sign in as the seeded admin instead.
- **Tests:** `TestGoogleLinkingRules` (the permission matrix now runs against a signed-in role), `npm run preflight` 27/27, `go test ./...` 11/11.
- **Status:** `FIXED-VERIFIED` (superseded by D-039).


### F-038 · P0 · The dev login bypassed authentication, and Google sign-up did not exist

- **Found:** 2026-10-28 (owner: *"real log in eka enable karanna dev log in eka completly ain karanna … google sing up ekath hariyata thiyenna one"*).
- **Evidence (before):**
  - `config.DevAuth` + `httpapi.RequireAuthOrDev` granted an anonymous request a real principal, and `auth.DevRoleHeader` let that request pick its role. One environment variable was the only thing between "signed in" and "not".
  - `frontend/src/lib/devMode.ts` + `components/dev/*` booted the SPA as a persona automatically; a production bundle still carried the switch.
  - **Google sign-in did not exist at all**: 0 hits for `google|oauth` in `backend/**/*.go`; both buttons only toasted "not connected yet". `users.google_sub` had been reserved since migration 0001 and was never written.
  - `Register` wrote `email_verified = 0` and **nothing could ever set it** — the `PENDING_VERIFICATION` status existed but no flow used it.
- **Impact:** P0 — "authentication" was optional, and the only supported second factor (Google) was unimplemented.
- **Fix:** the bypass was **deleted** (code, flags, headers, UI) and replaced with a real Google OAuth flow: `internal/auth/google` (authorization code + PKCE, server-side exchange, ID-token verification against Google's JWKS with `iss`/`aud`/`exp`/`iat`/`nonce`, `alg:none` and HS* refused), `auth.GoogleAuth` (single-use state, three documented linking rules), `auth.EmailVerifier` + `email_verifications`, migration `0012_google_identity.sql`, `GET /api/auth/google/{status,start,callback}`, `POST /api/auth/verify/request`, `GET /api/auth/verify/{token}`, a `useGoogleButton` hook that disables the button when the server is not configured, and `cmd/accounttool` (`-verify`, `-purge-chat`).
- **Related API / DB:** additive only. `/api/auth/google/status` and the verify endpoints are new; `0012` adds `auth_identities`, `oauth_states`, `email_verifications`.
- **Tests executed:** `internal/auth/google` (valid token accepted; wrong aud/iss, expired, future `iat`, nonce mismatch, missing sub/email, unknown key, `alg:none`/HS256, non-JWT all refused; unconfigured client fails closed; PKCE is S256), `internal/auth` (three linking rules incl. the unverified-address refusal, no duplicate on second login, single-use state, fail-closed without credentials). `go test ./...` → **11/11 packages**; `npm run preflight` → **27/27**; live: `/api/auth/google/start` → **503** when unconfigured, register → `email_verified=false`, redeem → DB `1|ACTIVE`, replay → `verified=invalid`, logout → later `/api/me` **401**.
- **A real defect found while proving it:** the verifier passed `0` instead of `crypto.SHA256` to `rsa.VerifyPKCS1v15`, which would have rejected **every genuine Google token**. Caught by `TestVerifyIDTokenAcceptsAValidToken` before release.
- **Regression risk:** low, and asymmetric: the deleted code only ever *added* access. The Google flow refuses on any doubt, and linking to an existing account requires Google to vouch for the address.
- **Status:** `FIXED-VERIFIED` (unit + live API; the Google browser round trip needs a real client id/secret — see `docs/AUTH.md` §2b).

### F-039 · P2 · Test chat data had no way to be cleared

- **Found:** 2026-10-28 (owner: *"denata chat eke data clean karanna"*).
- **Evidence:** `chat_messages` 44, `chat_conversations` 2, `chat_participants` 4 and 24 flag rows of "hi / hellow" test chatter, with no deletion path in the UI or the tooling.
- **Fix:** `cmd/accounttool -purge-chat` (and `.\dev.ps1 -PurgeChat`): backup file → per-table counts → one transaction → audit row `chat.purge`. `-dry-run` previews it.
- **Related API / DB:** data only; no schema change. Users, VPNs, payments, offers and the audit log are untouched.
- **Verification:** dry run, real run, and `sqlite3` counts after (chat 0 rows; users 5, VPNs 6, payments 3 unchanged); backup `veylix.pre-purge-20261028-204638.db`.
- **Status:** `FIXED-VERIFIED`. A durable "delete this conversation" for end users is **not** built (noted for a later stage).


- **Found:** 2026-10-28 (owner: *"dev log in eka client la dennayi adminuyi super adminuyi dala hariyata hadala … passe real login system eka on kalama oya dev log in system eka ain karala system eka danna one"*).
- **Evidence (before):**
  - `httpapi.DevPrincipal()` returned `Role: super_admin` unconditionally, so the frontend's `admin` / `client` personas were super admins on the wire — an `admin` persona on `/api/audit` answered **200**.
  - `frontend/src/components/dev/DevBadge.tsx` imported `roleHomePath` from `lib/devMode.ts`, which never exported it: `tsc -b` failed, so `npm run build` (and therefore `.\\dev.ps1 -Package`) could not produce a release artifact at all.
  - The `/profile` dev card called `devSwitchRole` without navigating, so a switch looked like a no-op.
  - The built bundle still contained `devSwitchRole`, so a grep could not prove the dev login was absent.
  - The database had **one** admin; `PATCH /api/admins/{id}` refuses your own account (`422 cannot_modify_self`), so "disable an admin" was untestable.
- **Cause:** the bypass supplied an identity but no way to say *which* identity, and its frontend branch grew a stale import and a minification-surviving key name.
- **Fix:** D-022.1–D-022.6 (see `docs/DECISIONS.md`): `X-Veylix-Dev-Role` header + `auth.DevImpersonationRole` + `httpapi.DevPrincipalFor` + `config.DevRoleSwitch` (ANDed with `DevAuth`, fail-closed) + `dev_impersonating` on `/api/me`; `roleHomePath` exported and shared; `previewAs` replaces the bundle-visible key; new `cmd/seedacct` (guarded, idempotent, audited) and `dev.ps1 -FakeAdmin`; `frontend/scripts/preflight-dev.mjs` (`npm run preflight`) as a 17-check gate; `dev.ps1 -Restart/-Wait/-Preflight/-Seed/-FakeAdmin/-Roles` and `-DisableDevAuth` (removes the dev login, rebuilds, proves 0 bundle markers + anonymous 401).
- **Related API / DB:** additive only (`dev_impersonating` in `/api/me`); no schema change. `seedacct` writes `users` + one `audit_logs` row (`user.seed`, `source=seedacct`).
- **Tests executed:** `TestDevRoleSwitchChangesThePrincipal`, `TestDevAdminPersonaIsNotASuperAdmin`, `TestDevRoleSwitchOffIgnoresTheHeader`, `TestDevRoleHeaderNeverOverridesARealSession`, `TestDevRoleSwitchRequiresTheBypass`, `TestConfigStringReportsDevRoleSwitch` — all pass; `go test ./...` clean; `npm run lint` 0/0 (113 files); `npm run build` OK; live: `dev.ps1 -Preflight` → **17/17 PASS**, `-DisableDevAuth` → bundle **0 dev markers** + anonymous `/api/me` **401**; UI render `/admin` and `/client` with 0 console errors.
- **Regression risk:** low. Role switching is off unless `VEYLIX_DEV_ROLE_SWITCH=1` **and** the bypass is on **and** the bind is loopback; a request with any session cookie ignores the header (proved by test).
- **Status:** `FIXED-VERIFIED` (live dev stack).




### D-077 · P1 · A customer whose mailbox was dead could never be rescued

| Field | Value |
|---|---|
| Affected | `httpapi/client_email_routes.go` (new), `httpapi/router.go`, `frontend/src/pages/ClientDetailPage.tsx`, `stores/clientAdminStore.ts`, `lib/api.ts` |
| Evidence | The console could block, soft-delete, reset the password and set the FREE limit, but the ADDRESS itself was untouchable — the page told the operator *"Their e-mail address is changed by them, from Profile → Security (a code goes to the new address)."* That is impossible for the customer whose mailbox is wrong or gone, which is precisely the customer an operator is trying to rescue and the one the reset-password route exists for. Every recovery path in the product ended at a mailbox that did not work. |
| Cause | The customer-initiated flow (`/api/me/email-change/*`) is the only implementation, and it requires the customer to be signed in and able to read the NEW address — a precondition the affected customer fails by definition. |
| Fix | `POST /api/super/clients/{id}/email-change` and `…/email-change/confirm`, gated by `PermClientManage`, re-using the SAME store flow (`CreateEmailChangeRequest` → `LiveEmailChangeRequest` → `ConfirmEmailChange`) so there is one implementation of "confirm at the new address". The operator does not supply a password (they do not know it); the authority is the permission plus the audit trail (`client.email_change_requested` / `client.email_changed`). Nothing writes `users.email` directly, so a typo cannot silently move an account's identity and recovery target. When no relay is configured the code is returned to the operator instead of being lost in an outbox — the same honesty the password reset uses. The page's dead-end sentence is now a **Change e-mail** action that walks the two steps. |
| Bug found by its own test | `TestAdminEmailChangeRefusesATakenAddress` failed on the first run: an address belonging to another account was accepted (202) because uniqueness was only checked at confirm time, after a code had been sent to a mailbox the operator does not own. Fixed with an up-front `UserByEmail` check returning **409 `email_taken`**. |
| Also fixed | The route first mounted through a type assertion that silently failed when the interface signature did not match `*store.Store` — the route was simply absent and the 403 test caught it as a 404. The interface now mirrors the real `UserByEmail(ctx, email) (int64, string, error)` signature. |
| Tests | `TestAdminCanChangeAClientsEmailThroughTheCode` (202, address **unchanged** until confirmed, then moved and `email_verified = 1`), `TestAdminEmailChangeRefusesAWrongCode`, `TestAdminEmailChangeRefusesATakenAddress`, `TestAdminEmailChangeRequiresClientManage` (reseller → 403) |
| Regression risk | Two new Super-Admin routes and one new dialog; no existing route changed. `go test ./... -count=1` → **12/12 packages ok**; `tsc` 0; `lint` 0/0; `audit` OK; `docs:verify` OK (121 routes, 493 test functions). |
| Status | `FIXED-VERIFIED` (unit + HTTP, no live panel and no SMTP relay exercised) |


### D-078 · P1 · An unverified account could never ask for another link

| Field | Value |
|---|---|
| Affected | `httpapi/auth_verify_resend.go` (new), `httpapi/google_routes.go`, `httpapi/router.go`, `frontend/src/pages/LoginPage.tsx`, `stores/authStore.ts`, `lib/api.ts` |
| Evidence | The only resend route was `POST /api/auth/verify/request`, which sits behind `authed(...)`. An unverified account can **never** hold a session — `Authenticate` refuses it with `403 email_unverified` before any cookie is issued. So the route to a fresh link required the one thing the person could not obtain: sign up → locked out → no way back in, and, with no SMTP relay, no way to ask again either. D-076.1 made register stop pretending (no session for an unverified account), which removed the last accidental workaround: the cookie that had let somebody reach that route once. |
| Cause | The route was written for "a signed-in person whose address changed", not for "a person who cannot sign in because the address is unconfirmed". Those are different states and only one of them has a session. |
| Fix | A **public** `POST /api/auth/verify/resend { identifier }`, throttled per address (burst 3, one refill every 20 minutes) so it cannot be used to flood a mailbox. It mails only an unverified account, and answers **one fixed 202 body** for every outcome — unknown address, already verified, rate-limited, delivery failed, and successfully sent are indistinguishable. That is deliberate: a public endpoint that differentiated them would be an address-existence oracle. `/login` now offers **Send a new confirmation link** after an `email_unverified` refusal, and displays the server's own sentence rather than claiming an e-mail went out. |
| Tests | `TestUnverifiedAccountCanRequestANewLink` (registers, is refused at login, resends, and the link is actually re-sent to that address), `TestResendAnswersTheSameForUnknownAndVerified` (same status **and** same wording for unknown vs verified, nothing mailed for either), `TestResendIsGenericWithNoRelay` (no relay configured → still 202, nothing sent) |
| Regression risk | One new route, public by necessity. It writes no state, reveals nothing, and is throttled; the authenticated `/api/auth/verify/request` still exists with its stricter token handling. `go test ./... -count=1` → **12/12 packages ok**; `tsc` 0; `lint` 0/0; `audit` OK; `docs:verify` OK (122 routes, 496 test functions). |
| Status | `FIXED-VERIFIED` (HTTP-level; no SMTP relay and no live panel were available) |
| Not done | The response is intentionally uniform, so the UI cannot say "sent" — it repeats the server's conditional wording. That is the price of not leaking registrations, and it is recorded rather than papered over. |



---

## 2026-11-04 pass 3 — the stuck client: why "e-mail + password" could not sign in (D-080)

### D-080 · P0 · A Google-only client was told "email or password is incorrect" for a password that never existed, and the self-service way out was blocked by the form itself

| Field | Value |
|---|---|
| Affected | `frontend/src/pages/ProfilePage.tsx`, `stores/authStore.ts`, `httpapi/middleware.go`, `cmd/dbcheck/main.go`, `docs/AUTH.md`, `.veylix-secrets/dev-env.ps1` (comment) |
| Evidence | A real client account (`janith15dawan@gmail.com`, id 17) is Google-created: `auth_identities provider=google`, `password_hash NULL`, `email_verified=1`. `var/logs/backend.log` 06:14 recorded a **burst of five 401s** from their password attempts — "email or password is incorrect" for a password that was never set. After the restart, the same attempt answers **403 `password_not_set`** with the new guidance (191-byte body, log 06:47:48). The running binary predated D-075–D-078 (built 02:04, started 02:37), so none of the day's fixes were live for the client who hit this. |
| Cause | Three causes stacked: **(1)** a stale binary served the old false 401; **(2)** the *intended* self-service exit — set a first password from Profile — was blocked by the UI, which demands a *current* password (`if (!current) … 'Enter your current password.'`) even though the backend deliberately accepts an empty current value exactly when no hash exists (`ChangePassword`: verify only `if stored.Valid && stored.String != ""`); **(3)** a Google-only client has no reset path either (reset assumes a mailbox and an existing password), so every recovery road ended. |
| Fix | Profile → Security now branches on the server's `has_password`: no password → **"Set a password"** (current field hidden, explanatory copy), existing password → the old "Change password" flow unchanged. The `password_not_set` refusal (backend body + login-page message) now also says **or ask the seller to issue you a password from the console**, so a person who cannot use Google is pointed at the operator path instead of a dead end. `dbcheck` gained an `email_verified` column, because the login door refuses unverified accounts and the tool could not show that. `docs/AUTH.md` + `dev-env.ps1` document the `localhost` redirect trap: a client on another device cannot complete a `localhost` Google callback — the reliable bridges are *Set a password* (Profile) and the console reset. |
| Tests | New `TestChangePasswordFirstSetNeedsNoCurrent` (NULL-hash account: empty current **sets** the first password and e-mail+password then works; once a hash exists, empty current is **refused** with `ErrPasswordMismatch` and the old credential survives). `go test ./... -count=1` → **12/12 packages ok**; `tsc` 0; `lint` 0/0. Live: `dev.ps1 -Restart -Preflight` → **33 checks passed**; the stuck account's login attempt now answers the true 403, not the false 401. |
| Regression risk | The only behavioural change for existing users is cosmetic (dialog title/wording when `has_password`); the validation branch is guarded in both directions by the new test. The console reset (`ResetStaffPassword`) needs no change — it sets the hash unconditionally and works for a Google-only account. |
| Not done | The browser leg (Google consent → Profile → Set password) was not driven live — it needs the owner's Google session; the UI change is covered by `tsc`/`lint` and the service branch by the unit test. Verified only in `outbox` mail mode, as before. |
| Status | `FIXED-VERIFIED` (API-level, live against the real stuck account; browser leg pending owner walkthrough) |

---

## 2026-11-04 pass 2 — Phase L: already shipped, the docs and one gate said otherwise (D-079)

### D-079 · P1 · The Phase L screens were live, but the README said "not built" — and the nav gate was red

| Field | Value |
|---|---|
| Affected | `README.md` §Status, `frontend/scripts/verify-nav.mjs` |
| Evidence | The README "Not built yet" list named **Phase L screens** (`/orders`, `/orders/:id`, `/pricing`, `/packages`, `/servers/:id`), a **per-client FREE cap**, and **client delete / e-mail edit** — all three had shipped (pages + stores + routes + D-076.2/D-077 backend work). Stale docs are the failure mode F-057/M7-18 exist to prevent, in the one document every handover starts from. Worse: `node scripts/verify-nav.mjs` — the D-057 gate asserting every reachable page has a sidebar entry — was **RED on the live app** (`super_admin nav has /admins — missing from sidebar`), because the nav had moved to `/staff` and the harness's expected list was never updated. So the gate meant to catch unnoticed screens was itself unnoticed. |
| Live verification (dev stack, real sign-in, headless Chrome via CDP) | `/orders` · `/packages` (4 rows) · `/pricing` · `/orders/9` (custom order, 3 cards) · `/servers/11` (full drill-down: 4 summary metrics, panel client table) — all render with **0 console errors**. `/servers/1` initially produced two 404s, which are **correct**: server id 1 does not exist (the integration is id 11), and `writeIntegrationError` answers `404 not_found` for a missing row, distinguishing it from a panel outage (`502 panel_error`) — probed from `var/logs/backend.log` and the API, not assumed. |
| Fix | README §Status rewritten: "Not built yet" now names only what is genuinely absent — **HWID & credential routes** (adapter reads `limit_hwid`, no HTTP route), **panel-tagging UI** (`PUT /api/panel-clients/…/tag` routes exist unmounted), and a **one-line installer** (CI + Caddyfile + systemd unit all ship). A "Browser-verified" row records today's render evidence. `verify-nav`'s EXPECTED lists were rebuilt from the real `NAV` in `App.tsx` (`/staff` replaces `/admins`; `/packages`, `/orders`, `/payments`, `/pricing` added for super-admin; `/packages`, `/orders`, `/admin-vpns` for admin) and the script is now reachable as `npm run verify:nav`. |
| Tests | `npm run verify:nav` → **23 passed, 0 failed** (was 16/1 and red) · `npx tsc -b` 0 · `npm run lint` 0/0 · `npm run audit` OK · `npm run docs:verify` OK |
| Also fixed | The order-timeline control did not render on `/orders/9`; the page's own empty state explains it ("No events recorded yet…"). Left as-is deliberately: the endpoint is wired server-side and tested — this needs an order with recorded events to check, not a code change. |
| Status | `FIXED-VERIFIED` (live browser); the genuinely unbuilt items are named with their evidence rather than implied by absence |

---

## 2026-11-04 pass 4 — Super-Admin console UX: the dashboard became a work surface (D-081)

### D-081 · P1 · The dashboard reported the platform but hid the operator's queue — and stored mojibake rendered on the first screen

| Field | Value |
|---|---|
| Affected | `frontend/src/pages/DashboardPage.tsx`, `frontend/src/pages/PaymentsPage.tsx`, `frontend/src/pages/payments.css`, `frontend/src/App.tsx`, `frontend/src/components/ui/MetricCard.tsx`, `frontend/src/components/ui/ui.css` |
| Evidence | (1) `DashboardPage.tsx` subtitle and empty-state strings literally contained double-encoded UTF-8 stored as content — char dump showed `Â·` (U+00C2 U+00B7), `â€"` (U+00E2 U+20AC U+201D), `â€¦`, `â€œ`/`â€`+U+009D and arrow mojibake: **36 garbage chars in 7 sequence types**, rendered verbatim on the Super Admin's first screen. (2) `PaymentsPage` computed `pendingCount` but nothing surfaced it outside the page; the nav `counts` map had no `/payments` key, so work existed that an operator had to *discover* by opening the page. (3) KPI `MetricCard`s had no click path — four numbers with no drill-down. (4) The header claimed "live from the backend" without a freshness fact, and the "Needs attention" queue omitted the two things only the Super Admin can clear: the payment queue and non-ACTIVE panels. |
| Cause | The Phase-2 UX pass (P8) built the attention queue from `ApiAdminVPNSummary` only; payments and panel health live in different stores/reads. The mojibake is a legacy double-encoding save (UTF-8 bytes re-decoded as CP1252 and re-saved), inherited across several page files. |
| Fix | **Dashboard**: attention rows for `payments awaiting review` (money-first triage, from `GET /api/payments?pending=1` — the backend answers exactly PENDING + AWAITING_CONFIRMATION, so the length IS the queue) and `panels need attention` (any integration not ACTIVE/REMOVED, naming them); header subtitle now states the fact — `synced 2m ago` (newest `last_sync`, 30s re-render tick, "not synced yet" when zero-safe) instead of the vibe "live from the backend"; all four KPI cards are now real `<button>`s that navigate to their lists (`ui-metric--link`: quiet at rest, hover/focus affordance, keyboard + focus ring for free). **App shell**: `/payments` nav badge = queue depth (PENDING + AWAITING_CONFIRMATION from the store), loaded for super-admin and admin — a badge points at work, not at history. **Payments**: aging chip on queue rows — `waiting 3h` from the moment the row entered its current state (PENDING → createdAt, AWAITING_CONFIRMATION → updatedAt), red past 24h, gone once a decision exists; the card header shows a warning Badge when work is waiting and "nothing needs a decision" when clear. **Encoding repair**: file-wide sequence map on `DashboardPage.tsx` (â€"→—, â†\u0090→←, Â·→·, â€¦→…, â€œ→", â€\u009D→", â†'→→) — 65 → 29 non-ASCII chars, all 29 now legitimate. |
| Tests | `npm run build` (tsc -b + vite) → **0 errors**; `npm run lint` (oxlint) → **0 warnings, 0 errors** on 165 files. Live browser walkthrough pending (needs the dev backend + sign-in). |
| Regression risk | Two extra reads: `GET /api/payments?pending=1` per dashboard refresh and `GET /api/payments` once at sign-in for operator roles — both are reads the destination pages already perform, and neither writes. Attention rows appear only when non-zero, so a healthy install shows nothing new. The mojibake repair changes string bytes only; no logic or identifiers touched. The nav badge uses the same store the page renders, so badge and page cannot disagree (F-019 rule). |
| Not done | Other page files likely hold stored mojibake too — a codebase-wide encoding sweep is Phase-4 polish work, not done here. The Audit page's server-side filter gap (D-082 candidate), command-palette entity search and table CSV export remain open from the 2026-11-04 research report. |
| Status | `FIXED` (build + lint verified; live render pending the next `dev.ps1` run) |

---

## 2026-11-04 pass 5 — the audit page became honest: filters moved into the database (D-082)

### D-082 · P1 · The audit "filter" could hide its own answer: kind chips filtered a fixed 100-row window client-side, so an old payment entry read as "nothing recorded"

| Field | Value |
|---|---|
| Affected | `backend/internal/store/repo.go`, `backend/internal/httpapi/account_routes.go`, `backend/internal/httpapi/audit_routes_test.go` (new), `backend/internal/httpapi/billing_routes_test.go`, `frontend/src/lib/api.ts`, `frontend/src/stores/auditStore.ts`, `frontend/src/pages/AuditPage.tsx`, `frontend/src/pages/payments.css`, `docs/API_CONTRACT.md` §5c, `README.md` §Status |
| Evidence | `GET /api/audit` accepted only `?limit=` (max 200); `AuditPage` then filtered THAT window in JS. On any installation whose trail outgrew the window, choosing the **Payments** chip could honestly-render **"Nothing recorded for payments"** while matching entries sat one page deeper — the exact "wrong answer worse than no answer" failure REQUIREMENTS §8 exists to prevent. The page also rendered actors as `user #12`, outcomes as account-status badges (`StatusBadge ACTIVE/DISABLED` for OK/DENIED — a trail entry is not a server), dotted machine actions (`panel_client.tag_denied`) with no human sentence, stored `context` JSON behind no viewer at all, and offered no export for incident write-ups. |
| Cause | Stage E2 shipped the minimal read; the page was built on top of it instead of the other way round. |
| Fix | **Backend** — `GET /api/audit` is now a filtered envelope: `kind` (`auth/server/admin/vpn/payment/client/other`, where `payment` covers `payment_method.*` and `client` covers `user.*`; unknown token → **400 `bad_filter`**, never a silent empty page), `actor_id`, `from`/`to` (unix seconds), `q` (substring across action, actor name, target and context, LIKE-wildcards escaped), `limit`/`offset`. The response `{entries, total, limit, offset, has_more}` counts matches **after** the filters but **before** the window — the property that makes "3 of 3 matches" true. Each entry gained `actor_name` (LEFT JOIN users: display name, else e-mail, absent for system rows). The trail stays append-only: the READ widened, nothing else. **Frontend** — the page's filters moved into the URL (D-051: kind chips, an All-time/24h/7d/30d window, a debounced search that also reaches actor names); honest counts (`N of M entries`); human action sentences with the raw action kept underneath; OK/DENIED/ERROR outcome badges; a context viewer (the stored JSON, pretty-printed, behind a `Detail` button — `'{}'` renders "no detail"); pagination; **Export CSV** (up to 200 rows matching the current filters, UTC ISO timestamps, and the toast says when the read capped). The payment queue test in `billing_routes_test.go` was updated to the envelope. |
| Tests | `go test ./... -count=1` → **13/13 packages ok** (5 new route tests: kind filter counts past the window, unknown-kind 400, actor + name + search, backdated from/to bounds, pagination echo/has_more) · `gofmt` clean · `go vet` clean · `npm run build` 0 errors · `npm run lint` 0/0 · `npm run docs:verify` OK (502 test functions after the README count was updated) · `npm run preflight` → **33/33** (RBAC matrix incl. `/api/audit` 200/403/403, live) · `verify-dashboard.mjs /audit` → renders **0 console errors**: `Owner · Signed in · auth.login · OK`, `DENIED` badge live, 50-row window, Detail button present. |
| Regression risk | The response shape changed from `[entry]` to the envelope — both consumers (the audit page and the billing test) were updated in the same change; `preflight-dev.mjs` only asserts the status code, which did not change. SQL adds one LEFT JOIN + one COUNT per read on an indexed small table. The 400 on an unknown kind is new behaviour: the only caller is the page, which sends the fixed chip tokens. |
| Not done | An `actor_id` UI picker (the API supports it; the search box covers "what did Owner do" by name). The D-082.1 candidate from the research report — audit-side date RANGE pickers instead of the three preset windows — is polish, not a gap. |
| Status | `FIXED-VERIFIED` (unit + live browser + preflight) |
