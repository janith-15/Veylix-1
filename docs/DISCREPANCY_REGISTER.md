# VEYLIX — DISCREPANCY REGISTER

**Created:** 2026-11-04 · **Baseline commit:** `95869b1` (`records/2026-11-04-baseline.md`)
**Type:** Living register — every known gap between *what the product claims* and *what the code does*, one row each, with the evidence that proved it and the item ID that closes it.

**The rule this register follows** (`docs/new_reqierment/new_update.md` §1, §13):
a feature is never `✅` because a function, a table or a button exists. It is
`✅` only when a command in §"Verification" passes **and** the fix refuses to
pass when reverted.

---

## 0. Status legend

| Symbol | Meaning |
|---|---|
| ✅ **CLOSED** | Fixed, test written that fails when reverted, gates green, evidence recorded |
| 🟡 **OPEN** | Confirmed real, not yet fixed |
| ⛔ **BLOCKED** | Cannot be closed without owner action or an environment we do not have |
| ⚪ **WITHDRAWN** | Investigated and found *not* to be a defect — the reason is recorded |

---

## 1. The register

| ID | Sev | System | Discrepancy | Evidence (verified 2026-11-04) | Status |
|---|---|---|---|---|---|
| **G0** | P1 | Foundation | 299 uncommitted changes; no restore point | `git status --porcelain` → 299; `main` at `c559034` | ✅ **CLOSED** — `95869b1` |
| **G0b** | P3 | Foundation | Root cluttered with 6 scratch files; the 5 dated diagnostics had no date in the name | `Get-ChildItem -File` root listing | ✅ **CLOSED** — `records/` + `.gitignore:63` |
| **G0c** | P2 | Foundation | No recorded gate baseline, so "still passing" was unprovable | — | ✅ **CLOSED** — `records/2026-11-04-baseline.md`, 10/10 gates |
| **D1** | **P1** | S1 Identity | **`GET /api/me` omits `display_name`, but the UI reads it.** After every reload a user's name silently reverts to the e-mail prefix | `auth_routes.go:224-226` returned `{id,email,role}` · `authStore.ts` `toUser()` reads `u.display_name` | ✅ **CLOSED** |
| **D2** | P2 | S1 Identity | `/api/me` omits `created_at`, so no screen can say when an account was made | same handler; `users.created_at` exists | ✅ **CLOSED** |
| **D3** | P1 | S1 Identity | No way to end *other* sessions; `RevokeAllForUser` exists but only a password change reaches it | `session.go:98`; callers = password change only | ✅ **CLOSED** |
| **D4** | P1 | Docs | `docs/SYSTEM_MAP.md` describes the mock era as current: "2 done / 4 partial / 2 missing", "7 of 11 stores real", "0 frontend tests" | file: baseline 2026-10-23; actual 17 stores, 122 routes | ✅ **CLOSED 2026-11-07** — `SYSTEM_MAP.md` re-baselined: 8 modules with per-module status, **17/17 stores API-backed**, 125 routes, 21 migrations, 518 tests. See §2. |
| **D5** | P2 | Docs | `docs/GAP_ANALYSIS.md` §1–§12 say "no database, no schema, no migrations, no backend" | §5 table lists every table as *All Missing* | ✅ **CLOSED 2026-11-07** — §1–12 kept as the historical baseline (they are the record of what the audit found), and the file now opens with an explicit banner plus a re-baselined §12 tally. A reader cannot mistake them for current. See §2. |
| **D6** | P2 | Docs | `docs/DOC_INDEX.md` states 22 docs / API 72 routes / 17 migrations | actual: 21 migrations, 122 routes, 23+ docs | ✅ **CLOSED 2026-11-07** — now **30 documents · 125 routes · 21 migrations · D-079**, all counted from the tree. `npm run docs:verify` gates the numbers that appear in `README.md`. See §2. |
| **D7** | P2 | Docs | `docs/FINAL_REPORT.md` §1 says "7 migrations", "72 routes" | actual 21 / 122 | ✅ **CLOSED 2026-11-07** — corrected to **21 migrations (schema v21)** and **125 routes**; §1 now says "as of 2026-11-07". See §2. |
| **D8** | **P1** | S2 Integrations | **HWID routes missing.** The adapter lists and revokes hardware devices; no HTTP route exposed either, so the capability was unreachable from the product | `ops.go:386` `Hwids`, `DeleteHwid` (+ 3 adapter tests); zero `mux.Handle` matches on `hwid` | ✅ **CLOSED** |
| **D9** | P2 | S2 Integrations | **Panel tagging has no screen.** `PUT /api/panel-clients/{s}/{e}/tag` exists and is tested; no page calls it | `integration_routes.go:434`; `panel_tag_routes_test.go` | ✅ **CLOSED 2026-11-07** — `VpnsPage.tsx:184` and `:230` call `api.setPanelClientTag(...)`. The register was stale. See §2. |
| **D10** | P2 | S8 Delivery | Zero frontend unit tests; no runner installed | `package.json` — no `vitest`; `scripts` full list | 🟡 **OPEN** |
| **D11** | P2 | S8 Delivery | CI workflows exist but have never been executed here | `.github/workflows/{ci,release}.yml` present | 🟡 **OPEN** |
| **D12** | P1 | S3 Provisioning | suspend / resume / revoke never executed against a real panel | `docs/FINAL_REPORT.md` §13 "⛔ not run (D-018)" | ⛔ **BLOCKED** — needs owner + non-production inbound |
| **D13** | P2 | S1 Identity | Google sign-in verified only against a fake, never a live Google tenant | `google_identity_fake_test.go` | ⛔ **BLOCKED** — needs client ID/secret |
| **D14** | P3 | S8 Delivery | `go test -race` never executed | no C compiler on this machine | ⛔ **BLOCKED** — environment |
| **D15** | P3 | S8 Delivery | No one-line installer; install is a manual runbook | `deploy/` had Caddyfile/systemd/backup only | ✅ **CLOSED 2026-11-07** — `install.sh` + `backend/cmd/vpn-platformctl` built (D-079); guards tested by `scripts/install.selftest.sh`. **Not yet run on a clean Linux VM** — `INSTALL.md` §F says so. See §2. |
| **D16** | P3 | S3 Provisioning | No per-client FREE monthly cap column (policy default of 5 applies to all) | `docs/FINAL_REPORT.md` §7 | ✅ **CLOSED 2026-11-07** — `store/claims.go:39` `MonthlyLimit`, `:50` `FreeUsage(..., monthlyLimit int)`, `:28` `ErrFreeMonthlyLimit`; `ClientSetFreeLimit` + `regression_d076_test.go`. See §2. |

---

## 2. Closed items — evidence

Every entry below names the test that **fails when the fix is reverted**. A fix
with no failing-on-revert test is not closed; it is untested.

### ✅ D1 / D2 · `GET /api/me` omitted the display name and creation date

**Symptom.** A signed-in user's name was right immediately after sign-in and
wrong after every reload. `toUser()` does
`name: u.display_name || u.email.split('@')[0]`, and `GET /api/me` answered
`{id, email, role}` — so `display_name` was `undefined` on every hydrate and the
fallback ran. A user named **Nimal Perera** became **nimal**. Nothing threw and
nothing logged: a wrong name still looks like a name, which is why no existing
test and no screen noticed for as long as the route has existed.

**Fix.**
- `store.UserRow` gained `CreatedAt`; `UserByID` now selects `created_at`.
- New narrow `httpapi.AccountReader` interface (one method, no hash, no session),
  wired through `Deps.Accounts` and set from `main.go`.
- `GET /api/me` reads the account row and adds `display_name`, `created_at` and
  `email_verified`. **A failed read is not an auth failure**: the caller is
  demonstrably signed in, so the route still answers `200` with the session's own
  facts plus `profile_read_error: true`. With no reader wired the two keys are
  **absent** — never guessed.

**Tests.** `TestMeReturnsDisplayNameAndCreatedAt` (asserts the exact name, the
e-mail, the role, a plausible `created_at`, and that a successful read is not
flagged) and `TestMeStaysUsableWhenTheAccountCannotBeRead` (the nil-reader branch:
`200`, e-mail present, `display_name` **absent**).

**Catches the bug?** **Yes.** The handler was temporarily changed so it published
the name under `wrong_name` instead of `display_name` (simulating the pre-D1 body
without breaking compilation). Result:

```
--- FAIL: TestMeReturnsDisplayNameAndCreatedAt (0.18s)
    me_routes_test.go:75: display_name = <nil>, want "Nimal Perera"
        (the UI falls back to "nimal" when this is absent)
FAIL
```

Exit 1. The change was reverted immediately and the suite re-run to green.

**Live proof** (`records/2026-11-04-d1d2d3-live-proof.txt`, real `veylixd` binary,
throwaway DB):
```
{"created_at":1793809797,"display_name":"Nimal Perera","email":"nimal@e2e.test",
 "email_verified":true,"id":1,"role":"super_admin"}
```

**Gate.** build 0 · vet 0 · `go test ./...` **12/12** · tsc 0 · lint 0/0.

### ✅ D3 · No way to end other sessions

**Symptom.** `RevokeAllForUser` existed but was reachable only by changing the
password. A user who had left a session open on a lost laptop could only evict it
by rotating their credential — and the Profile screen's own copy
("Changing your password signs out every other device") described that as the only
route.

**Fix.**
- `SessionStore.RevokeOthersForUser(ctx, userID, keepToken) (int64, error)` —
  `DELETE … WHERE user_id = ? AND id <> ?` on the stored hash; an **empty**
  `keepToken` deletes nothing rather than everything.
- `POST /api/auth/logout-others` → `200 {ended}`; the token comes from the
  **cookie**, never the body.
- `authStore.signOutOthers()` returns `{ended}` or `{error}` — the screen can never
  report success on a failed call.
- New **"Signed-in devices"** card on `/profile` → Security, with the count in the
  toast ("2 other devices signed out" / "No other devices").

**Tests.** `TestLogoutOthersKeepsThisSession` (three sessions; `ended == 2`; the
caller still `200`; the other two `401`) and
`TestLogoutOthersRequiresASession` (anonymous → `401`).

**Catches the bug?** **Yes.** With the route wired to `RevokeAllForUser` (the
naive implementation), the run logs three `200` before and three `401` after:
`ended = 0, want 2` → `FAIL` (exit 1). That log is the defect itself — the caller
was signed out too. Restored.

**Live proof.**
```
ended = 2
caller  : 200 still signed in
device2 : 401 ended
device3 : 401 ended
anonymous : 401 (want 401)
```

**Gate.** build 0 · vet 0 · `go test ./...` **12/12** · tsc 0 · lint 0/0 ·
`npm run audit` OK · `npm run build` ✓ · `npm run docs:verify` OK
(21 migrations · **123** routes · **508** tests).

> `docs:verify` **failed first** on this change (README said 122 routes / 504
> tests; the tree had 123 / 508) and was fixed by updating the README numbers —
> which is the gate doing exactly its job.

### ✅ D8 · The adapter could free a device slot, and nothing could ask it to

**Symptom.** This was not a wrong answer. `panel.Client.Hwids` and
`panel.Client.DeleteHwid` were implemented, unit-tested against the real 3X-UI
paths (`/panel/api/clients/hwids/{email}`, `…/{email}/{id}`), and reachable from
**no HTTP route at all**. A Super Admin could hit a customer's device cap and had
no way to see which devices held it or to free one. Nothing failed loudly,
because nothing was ever called.

**Fix.**
- `integrations.Service.Devices` / `.RevokeDevice`, behind a new
  `panelHwidInspector` interface — kept separate from `panelClient` (read-only)
  and from `panelRenamer`, so the read-only inspector never gains a write.
- `RevokeDevice` is **write → fresh read → confirm**: a delete the panel
  acknowledged but did not perform is an error, not a success.
- An already-absent device returns success **without** calling the panel: the
  desired end state already holds.
- `httpapi.HwidRoutes`:
  `GET /api/panel-clients/{s}/{e}/hwids` (200, empty list is honest) and
  `DELETE /api/panel-clients/{s}/{e}/hwids/{id}` (204). A nil service answers
  **503**, never an empty list.
- `audit.ActionPanelClientHwidRevoked` records the fingerprint **read before the
  delete** (it no longer exists afterwards), so "which device was freed?" is
  answerable later.
- UI: `components/vpn/DeviceList.tsx` on the VPN detail page (staff only) — the
  panel's truth, or "could not be read"; removal drops the row only after the
  server confirms.

**Tests (9 new).**
`TestDevicesReadsThePanel`, `TestDevicesEmptyEmailIsRejected`,
`TestRevokeDeviceConfirmsTheRemoval`, `TestRevokeDeviceRefusesUnconfirmedRemoval`,
`TestRevokeAlreadyAbsentDeviceIsSuccess` (integrations) ·
`TestHwidRoutesAreRegistered`, `TestHwidRoutesRejectBadIDs`,
`TestHwidRoutesNilServiceIs503`, `TestHwidRoutesRequireAPrincipal` (httpapi).

**Catches the bug?** **Yes.** With the confirmation check disabled
(`if false && containsHwid(after, hwidID)`):
`a delete the panel did not confirm must be an error, not a success` → `FAIL`
(exit 1). Restored.

**Gate.** build 0 · vet 0 · gofmt 0 · `go test ./...` **12/12** (517 funcs) ·
tsc 0 · lint 0/0 · `npm run audit` OK · `npm run build` ✓ · `docs:verify` OK
(125 routes).

**Live proof** (`records/2026-11-04-d8-live-proof.txt`, real `veylixd` binary, no
panel registered so every panel call must fail honestly):
```
--- 1. hwids for a server that does not exist ---
   status = 503  (want 5xx, never 200 with an empty list)
--- 2. delete a device on a missing server ---
   status = 503  (want 5xx, never 204)
--- 3. anonymous access ---
   status = 401  (want 401)
```

**Not claimed:** this was **not** exercised against a live panel. The adapter
paths it calls are already live-verified (client read/create/links), but no
production panel was asked to list or revoke a device in this pass — D-018 still
applies. The route, the confirmation logic and the failure mapping are proven by
tests and by the honest-failure run above, not by a real 3X-UI response.

---

### ✅ D4–D7, D9, D15, D16 · The register and four documents went stale (2026-11-07)

**What kind of defect this was.** Not a wrong answer from the product — a wrong
answer from the **documentation**. Every one of these rows described code that
had since been written. A stale register is dangerous for exactly one reason: it
tells a reader (or an agent) that work is outstanding when it is done, so the
real gap gets lost among the imaginary ones.

**Method.** Each row was re-checked by reading the current tree, not by trusting
an earlier document. The evidence for each is a `file:line` that exists today.

| Row | Claimed | Actual, verified 2026-11-07 |
|---|---|---|
| **D4** | `SYSTEM_MAP.md` describes the mock era ("2 done / 4 partial / 2 missing", "7 of 11 stores real", "0 frontend tests") | Re-baselined. `frontend/src/stores/` holds **17** store files, all API-backed (D-006 exit). Counts now come from `docs:verify`: 21 migrations · 125 routes · 518 test functions · 13 packages |
| **D5** | `GAP_ANALYSIS.md` §1–12 say "no database, no schema, no migrations, no backend" | §1–12 are kept **because they are the historical audit record**, and the file now opens with an explicit banner saying so, plus a re-baselined §12 tally. The distinction the register asked for was "do not read this as current"; the banner states it |
| **D6** | `DOC_INDEX.md` says 22 docs / 72 routes / 17 migrations | Now counts **30** `.md` files under `docs/`, **125** routes, **21** migrations, and cites **D-079** |
| **D7** | `FINAL_REPORT.md` §1 says "7 migrations", "72 routes" | Corrected to **21 migrations (schema v21)** and **125 routes**, dated 2026-11-07 |
| **D9** | Panel tagging has no screen | `frontend/src/pages/VpnsPage.tsx:184` and `:230` both call `api.setPanelClientTag(v.server_id, v.panel_email, tag)`. The route is `integration_routes.go:434` |
| **D15** | No one-line installer | `install.sh` and `backend/cmd/vpn-platformctl` exist (D-079). Guards are exercised by `scripts/install.selftest.sh` — **31 checks**, 1 SKIPPED on Windows. **Still not run on a clean Linux VM**, and `docs/INSTALL.md` §F keeps that `❌ NOT PERFORMED` |
| **D16** | No per-client FREE monthly cap column | `backend/internal/store/claims.go:39` declares `MonthlyLimit`, `:50` takes `monthlyLimit int`, `:28` declares `ErrFreeMonthlyLimit`; `ClientSetFreeLimit` and `regression_d076_test.go` cover it |

**What did NOT change.** No code. This pass touched documentation only — no
route, no migration, no business rule, no frontend file. The rows closed above
were already satisfied when the pass started.

**The tempting error, avoided.** D15 and D16 could each have been marked
`CLOSED` from the file's existence alone. D16 is closed on a **`file:line` plus a
test**, and D15 is closed on "built + guard-tested" while its **VM verification
stays open** in `INSTALL.md` §F. A close is only as good as the evidence beside
it.

**Gate.** `npm run docs:verify` PASS — 21 migrations · 125 routes · 518 test
functions in 13 packages. `npm run audit` PASS.

---

### ✅ D9 · The tagging screen (evidence detail)

The route and its test existed; the **screen** did not. Closed because
`frontend/src/pages/VpnsPage.tsx` calls `api.setPanelClientTag(...)` at **:184**
and **:230**, and `api.ts` talks to `PUT /api/panel-clients/{s}/{e}/tag`
(`integration_routes.go:434`). Nothing in the product code changed in this pass —
the screen had already been built.

