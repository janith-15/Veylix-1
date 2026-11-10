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
| **D4** | P1 | Docs | `docs/SYSTEM_MAP.md` describes the mock era as current: "2 done / 4 partial / 2 missing", "7 of 11 stores real", "0 frontend tests" | file: baseline 2026-10-23; actual 17 stores, 122 routes | ✅ **CLOSED** (2026-11-05) |
| **D5** | P2 | Docs | `docs/GAP_ANALYSIS.md` §1–§12 say "no database, no schema, no migrations, no backend" | §5 table lists every table as *All Missing* | ✅ **CLOSED** (2026-11-05) |
| **D6** | P2 | Docs | `docs/DOC_INDEX.md` states 22 docs / API 72 routes / 17 migrations | actual: 21 migrations, 122 routes, 23+ docs | ✅ **CLOSED** (2026-11-05) |
| **D7** | P2 | Docs | `docs/FINAL_REPORT.md` §1 says "7 migrations", "72 routes" | actual 21 / 122 | ✅ **CLOSED** (2026-11-05) |
| **D8** | **P1** | S2 Integrations | **HWID routes missing.** The adapter lists and revokes hardware devices; no HTTP route exposed either, so the capability was unreachable from the product | `ops.go:386` `Hwids`, `DeleteHwid` (+ 3 adapter tests); zero `mux.Handle` matches on `hwid` | ✅ **CLOSED** |

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


| **D9** | P2 | S2 Integrations | **Panel tagging has no screen.** `PUT /api/panel-clients/{s}/{e}/tag` exists and is tested; no page calls it | `integration_routes.go:434`; `panel_tag_routes_test.go` | 🟡 **OPEN** |
| **D10** | P2 | S8 Delivery | Zero frontend unit tests; no runner installed | `package.json` — no `vitest`; `scripts` full list | 🟡 **OPEN** |
| **D11** | P2 | S8 Delivery | CI workflows exist but have never been executed here | `.github/workflows/{ci,release}.yml` present | 🟡 **OPEN** |
| **D12** | P1 | S3 Provisioning | suspend / resume / revoke never executed against a real panel | `docs/FINAL_REPORT.md` §13 "⛔ not run (D-018)" | ⛔ **BLOCKED** — needs owner + non-production inbound |
| **D13** | P2 | S1 Identity | Google sign-in verified only against a fake, never a live Google tenant | `google_identity_fake_test.go` | ⛔ **BLOCKED** — needs client ID/secret |
| **D14** | P3 | S8 Delivery | `go test -race` never executed | no C compiler on this machine | ⛔ **BLOCKED** — environment |
| **D15** | P3 | S8 Delivery | No one-line installer; install is a manual runbook. **Also missing the installation-awareness Bible §14.0/§14.3 already requires** (detect an existing installation, offer upgrade/reconfigure, generate secrets only when absent, never overwrite a live DB) — there is no script to carry the check | `deploy/` has Caddyfile/systemd/backup only; no `scripts/` directory in `git ls-files` | 🟡 **OPEN** — see D17/D18; **do not mark complete until a clean-VM install AND a second-run refusal are both proven** |
| **D16** | P3 | S3 Provisioning | No per-client FREE monthly cap column (policy default of 5 applies to all) | `docs/FINAL_REPORT.md` §7 | 🟡 **OPEN** |
| **D17** | P2 | S8 Delivery | **No installation identity and no restore provenance.** Nothing lets an installation prove that a database or backup belongs to it, so `veylix-backup.sh restore` will install another installation's backup after an integrity check that passes. The two restore intents (move the same installation / create a separate installation from another's data) are not distinguished, and no confirmation or audit row exists | no `installation_id` in any of `backend/internal/store/migrations/0001`…`0022`; `deploy/veylix-backup.sh:108-143` verifies `PRAGMA integrity_check` only | 🟡 **OPEN** — planned as Bible §14.7/§14.8, D-082, ROADMAP M7-B. Acceptance: Bible §18 test 47 |
| **D18** | P3 | S8 Delivery | **No per-install upgrade/ops tooling.** `status`, `health`, `logs`, `upgrade`, `rollback`, `domain set`, `uninstall --preserve-data` are runbook prose, so an independent installation cannot be operated or upgraded by command, and nothing enforces "upgrade only this instance" | `deploy/veylixd.service` + `docs/UPGRADE.md` are manual; no CLI binary ships | 🟡 **OPEN** — planned as Bible §14.0a/§14.5, D-081, ROADMAP M7-B |
