# VEYLIX — QA PLAN

**Version:** 1.0 · **Created:** 2026-10-23 · **Decision:** D-015
**Scope:** what is tested, with which tool, and what counts as pass.
**Results** are recorded separately in `docs/QA_REPORT.md` (created when the first real run happens — Bible §17.11). Until then the report states only what actually ran.

> **Hard rule (`new_update.md` §9 / §13):** mocked tests are *not* proof of live compatibility, and a test result may never be invented or approximated.

---

## 1. Test layers

| Layer | Tool | Command | Status today |
|---|---|---|---|
| Go build | `go build` | `go build ./...` | ✅ exit 0 (8 packages) |
| Go static | `go vet` | `go vet ./...` | ✅ 0 issues |
| Go format | `gofmt` | `gofmt -l .` | ✅ no output |
| Go unit | `testing` | `go test ./...` | ✅ **189 tests pass** (9 test packages) |
| Go race | `-race` | `go test -race ./...` | ⛔ **blocked** — no C compiler |
| Go integration | temp-dir SQLite | `go test ./internal/store/...` | ✅ passing |
| Contract | `httptest` fake panel replaying verified examples | `go test ./internal/panel/...` | ✅ passing |
| FE typecheck | `tsc -b` | `npm run build` | ✅ 0 errors |
| FE lint | `oxlint` | `npm run lint` | ✅ 0/0 (96 files) |
| FE build | `vite build` | `npm run build` | ✅ passes |
| FE unit | `vitest` + `@testing-library/react` | `npm test` | ⬜ **not installed** (Stage F) |
| FE E2E (dev) | puppeteer harness pattern in `frontend/scripts/*.mjs` | `npm run test:e2e` | ⬜ not written |
| Live panel | real 3X-UI + API token | manual + scripted | ✅ reads/CRUD verified · ⛔ client provisioning blocked (D-018) |

---

## 2. Functional test matrix (per requirement area)

| # | Area | Test | Expected |
|---|---|---|---|
| T-01 | Auth | valid/invalid login | 200 / 401 with generic message |
| T-02 | Auth | role escalation attempt (client → admin route) | 403 |
| T-03 | Server integration | add with bad token | `REAUTH_REQUIRED`, no ACTIVE claim |
| T-04 | Server integration | panel unreachable | `CONNECTION_UNAVAILABLE` (never `REVOKED`) |
| T-05 | Server integration | SSRF URL table (§4) | every private/metadata target blocked |
| T-06 | Client provisioning | create Free for two clients | two distinct panel identities |
| T-07 | Client provisioning | retry after timeout | **exactly one** client (idempotency) |
| T-08 | Quota | 2 concurrent provisions at quota−1 | 1 success, 1 rejected |
| T-09 | Revocation | panel online | `REVOKED` only after verified delete |
| T-10 | Revocation | panel offline | `REVOCATION_PENDING`, no fake success |
| T-11 | Usage | counters reset on panel | lifetime totals preserved |
| T-12 | Links/QR | inactive client | no link/QR surfaced |
| T-13 | Links/QR | active client | link matches `GET /clients/links/{email}` |
| T-14 | Ownership | admin requests another admin's client | 404 |
| T-15 | Ownership | client requests another client's VPN | 404 |
| T-16 | Chat | non-participant posts | 403 |
| T-17 | Payments | manual confirm | audited, never auto-confirmed |
| T-18 | History | server removed | history readable, marked unavailable |
| T-19 | Migration | upgrade an existing DB | data intact |
| T-20 | Backup | dump → restore → read | identical rows |
| T-21 | Password policy | 12-char minimum, repeated rune, sequence, blocklist, > 512 | 422 `weak_password`, never a pass |
| T-22 | Password change | wrong current password | 422 `password_mismatch`, credential unchanged |
| T-23 | Password change | success | old credential dead, other sessions 401, caller still 200 |
| T-24 | First run | fresh DB → `/setup` → Super Admin | 201 + session; `/api/me` 200; second bootstrap 409 |
| T-25 | Account creation | super admin creates an `admin` | 201, the new admin can log in and lands on `/admin` |
| T-26 | Privilege escalation | admin/client/anonymous hit `/api/admins` | 403 / 403 / 401 |
| T-27 | Suspension | super admin suspends an admin | 200; that admin's live session → 401; login → 403 |
| T-28 | Lockout prevention | super admin suspends self | 422, own session survives |

### 2.1 Tests required by D-024 … D-028 (2026-10-24) — **must exist before those stages are "done"**

| ID | Area | Case | Expected |
|---|---|---|---|
| T-29 | D-024.1a | panel rejects `clients/add` | **zero** `vpn_instances` rows; outbox `FAILED`; audited |
| T-30 | D-024.1a | panel write times out, client is absent | no VPN row; outbox `RETRY`; never `ACTIVE` |
| T-31 | D-024.2 | panel reports `totalGB=20` while the offer said 10 | mirror shows **20** (panel wins) |
| T-32 | D-024.3 | panel client with no tag | visible to super admin, **invisible** to admin and client |
| T-33 | D-024.5 | client deleted on the panel, then a full sync | `SERVER_UNAVAILABLE` — never `REVOKED` |
| T-34 | D-025.2 | re-tag an **issued** VPN | 409; an unissued one → 200 |
| T-35 | D-025.4 | claim a 4th FREE VPN on the same server | 409 (cap 3) |
| T-36 | D-025.5 | 6th FREE claim within 30 days | 409; the rule is shown before claiming |
| T-37 | D-025.5 | release a FREE VPN, then claim again the same month | still counted — release does not refund |
| T-38 | D-027.2 | release, then restore on day 2 | restored; auto-removal cancelled |
| T-39 | D-027.3 | release, wait past the deadline | local row gone, **panel client still present**, audit entry written |
| T-40 | D-027.6 | chat exceeds the client's budget | sending blocked; VPN use unaffected; deleting messages frees space |
| T-41 | D-028.1 | second claim by the same client | panel email `…-2`, monotonic, never reused |
| T-42 | D-028.2 | assign a VPN created as `foo` to `kasun@gmail.com` | panel client renamed and confirmed by a fresh read |
| T-43 | D-028.3 | create a FREE VPN with `limitIp=5` | refused (FREE is fixed at 2/2) |
| T-44 | D-028.5 | `clients/add` body | contains `limitHwid` |
| T-45 | D-026.1 | ping endpoint | on-demand only; no request is made unless the button is pressed |
| T-46 | D-026 | usage range/chart type | server returns the requested range; no data ⇒ explicit "no data", never a zero line |
| T-47 | D-028.6 | client changes their `uuid`/password | confirmed by a fresh read; audited; a foreign VPN → 404 |


---

## 3. Security checks (evidence required)

| Check | Method | Pass criterion |
|---|---|---|
| Token leak | grep built bundle + all log output + response bodies for the seeded token | 0 hits |
| IDOR | object-id swap for every resource route | 403/404, never 200 |
| SSRF | URL fuzz table | all blocked |
| TLS | self-signed panel without explicit trust config | connection refused |
| Rate limit | burst of N sign-ins | throttled, logged |
| Secret at rest | inspect DB file bytes for the token | absent (encrypted) |
| CSRF/origin | cross-origin state-changing request | rejected |

### SSRF fuzz table (must all be blocked)
`http://` scheme · `127.0.0.1` · `localhost` · `10.0.0.1` · `172.16.0.1` · `192.168.0.1` · `169.254.169.254` · `[::1]` · `file://` · `gopher://` · public host that 302-redirects to `127.0.0.1` · hostname resolving to a private IP (DNS rebinding) — unless the Super Admin has explicitly enabled the documented private-network policy.

---

## 4. UI / UX checks (per changed screen)

- 360 / 768 / 1280 viewport: no horizontal scroll, no clipped controls
- loading / empty / error / stale states reachable and accurate
- keyboard: tab order, focus ring, Esc on dialogs
- status badges never communicate by colour alone
- theme sweep on at least the 5 presets (not all 40 for every run)
- `prefers-reduced-motion` honoured; low-resource mode kills animation/3D

---

## 5. Regression checklist (acceptance tests)

Bible §9 (18 tests) + §18 (21 tests) = **39 acceptance tests**.
Tracked as a PASS / FAIL / UNTESTED / BLOCKED table in `docs/QA_REPORT.md`. Only PASS may be claimed as done.

---

## 6. CI gate

```
backend:  gofmt -l . | fail if output
          go vet ./...
          go build ./...
          go test ./...
          go test -race ./...            (stage F+)

frontend: npm ci
          npm run lint
          npm run build
          npm test                        (after vitest lands)
```

Release builds only from a green CI run. No production server ever runs a build (`new_update.md` §8 / Bible §0.11).

---

## 7. What is currently true (2026-10-23, re-baselined after Stage E4 + password change)

- FE lint/build/typecheck: **passing** · 0 warnings / 0 errors · 96 files
- Backend: **stages B, E2, E3, E4, E5 implemented** — `go test ./...` green (**189 tests**), `go vet` clean, `gofmt` clean
- Login system: **complete and live-verified** — `/setup` first-run wizard, three-role RBAC matrix confirmed against the running binary, suspension revokes live sessions, lockout prevention (T-24…T-28)
- Password handling: argon2id (64 MB / t=2 / p=2), 16-byte salt, constant-time compare, hash-upgrade-on-login, 12-char + entropy + blocklist policy, change endpoint with session rotation — **all unit/e2e tested**
- Panel adapter: **live-verified** against 3X-UI 3.10.2 (health, inbounds, re-test)
- **Not live-verified:** client provisioning writes (production inbound holds live users — D-018)
- **Not implemented:** Stage E5 sync jobs / usage snapshots, E6 links+QR, E7 SPA embed, payments, chat, notifications, offers, admin CRUD
- **Not executable on this machine:** `go test -race` (no C compiler), FE unit tests (no runner)

Nothing in this document may be read as a claim that behaviour works beyond the evidence in `docs/QA_REPORT.md`.
