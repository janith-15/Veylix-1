# VEYLIX - ACTIVE CONTEXT

**This file is a current-session summary.** It is rewritten at the end of each
session, not appended to. History belongs in `./docs/cline/progress.md`.

**Current session summary - rewritten 2026-11-08 (S5: Section 02 My VPNs).**
Cline 4.1.26 has no Memory Bank, so this file plus `progress.md` are the memory.
Read both at the start of a session.

---

## INTERRUPTION RECOVERY - read this first if resuming

| Field | Value |
|---|---|
| **Task in flight** | **Client Side, section by section.** Section 01 Dashboard **done**. **Section 02 My VPNs: DONE this pass.** Sections 03-09 **NOT STARTED**. |
| **Last completed** | **S5 / Section 02 My VPNs.** `MyVpnsPage.tsx` rewritten (cards + compact list, overview, three category chips, collapsible filters, three sorts, paging). New `pages/client/vpns.css`. **D-088 + D-088.1 recorded.** |
| **Verified this pass** | `tsc` **0** - `lint` **0 errors / 2 pre-existing** - `audit` **OK** - `audit:selftest` **8+6** - `docs:verify` **23/133/615/15** - `build` **exit 0** (13.66 kB JS + 7.28 kB CSS, route-split) - `verify:nav` **74/74** - `verify:ui` **`consoleErrors: []`** - `/client/vpns` **`consoleErrors: []`** - responsive **0 overflow at 360/390/640/768/1024/1440/1920** - light + dark tokens resolve - `inlineColourStyles: 0` |
| **Known problems** | **Populated My VPNs NOT PROVEN** (client user 15 owns 0 VPNs) - **`POST /api/offers` 500** (pre-existing, blocks creating a FREE offer) - **S3-1 / S3-3 / S3-4 / S3-5 OPEN** - `go test -race` **BLOCKED** - custom package specs not displayable (not on the list row) - 16 legacy notifications - U18 / U21 / I-34 / I-35 unchanged |
| **Next recommended action** | **Owner review of Section 02.** Do not start Section 03 until approved. Unblocking item: `POST /api/offers` 500, which is what makes the client catalogue empty and most populated-state verification impossible. |

---

## Section 02 My VPNs - what was built (D-088)

| Area | Behaviour |
|---|---|
| Categories | `All` / `Free` / `Premium` - **three**. Custom Premium is Premium. |
| Overview | Total / Active / Needs attention over the **whole** inventory, hidden at 0. |
| Search | Name, server, package name. Debounced, URL-backed. |
| Advanced filters | Collapsible: Status (only statuses present, with counts), Server, Expiry. |
| Active-filter tags | Always visible, individually removable, plus Clear filters. |
| Sort | Newest first, Name A-Z, Expiry date (no-expiry **last**). |
| Views | Cards and Compact List over the **same** slice; `view` in the URL; no extra request. |
| Pagination | Nine per page, Previous/Next + numbers, `Showing X-Y of Z`, reset on filter change. |
| Detail | `/vpn/:id` only. |

### The rules a future edit must not break

1. **Three categories, never four** (D-088). Custom Premium is a Premium VPN.
2. **Overview counts are the whole inventory**, never `visible.length`.
3. **No per-card usage request.** S3 fired one `GET /api/vpns/{id}/usage` per
   card; that N+1 is gone and must not come back.
4. **No usage number is rendered on this screen** (D-088.1). `GET /api/vpns`
   sends no usage field; `used_bytes` belongs to `ApiAdminVPNRow`.
5. **Card and list render the same `visible` slice.** Switching view must never
   fetch.
6. **Every `.mv-*` rule stays scoped under `.mv`**, and `vpns.css` is imported
   only by `MyVpnsPage.tsx`.
7. **No hardcoded colour.** Tokens and `color-mix()` only. The error token is
   **`--error`**; there is no `--danger`.

---

## The decision records this pass touched

- **D-088** - recorded on owner authorisation, because it did not exist
  (`DECISIONS.md` jumped D-087 to D-090). Three categories, one list, two views,
  page size nine.
- **D-088.1** - a correction of a factual error inside D-088. D-088 claimed
  `used_bytes` / `limit_bytes` / `usage_pct` were on every `GET /api/vpns` row;
  they are on `ApiAdminVPNRow` (`/api/super/vpns`) instead. `tsc` proved it with
  six `TS2339` errors. D-088 keeps its original text; D-088.1 governs.
- **D-089 was not created.**

---

## File map for the Client Side

| Path | Role |
|---|---|
| `frontend/src/pages/ClientDashboardPage.tsx` | **Section 01 dashboard** (S4) |
| `frontend/src/pages/client.css` | Shared client CSS; `SECTION 01` block at the end |
| `frontend/src/pages/client/MyVpnsPage.tsx` | **Section 02 My VPNs** (this pass) |
| `frontend/src/pages/client/vpns.css` | **New** - Section 02 CSS, `.mv`-scoped, route-split |
| `frontend/src/components/client/ClientPage.tsx` | `ClientPage` + `ClientSection` frame (S3) |
| `frontend/src/stores/vpnStore.ts` | `ClientVpn`, `toKind`, `vpnDisplayName`, `vpnServerLabel` |
| `frontend/src/hooks/useListQuery.ts` | URL-as-truth list state, debounce, page reset |
| `frontend/src/pages/client/Client*Page.tsx` | `/client/offers|billing|usage|guides|support|notifications` shells |
| `frontend/src/components/layout/navModel.ts` | `CLIENT_NAV`, 11 destinations in 4 groups |
| `frontend/src/App.tsx` | Client route block, lines ~390-415 |

---

## Open / not done

- **Section 02 populated state NOT PROVEN** - client user 15 owns 0 VPNs. Needs a
  client-owned VPN, which needs either a FREE offer (blocked by the 500 below) or
  a successful `assign` (refused `409 panel_client_taken` for the one free row).
- **`POST /api/offers` returns 500** - pre-existing backend defect, logged as
  `status:500` on `/api/offers`. This is the single biggest blocker to verifying
  any client surface with real data.
- **Custom package specifications are not displayed** - `GET /api/vpns` sends no
  `is_custom` / `traffic_gb` / `duration_days` / `device_limit`. Backend change,
  out of scope for Section 02.
- **`auth-login.mjs:28-30`** lets `VEYLIX_EMAIL` override every role. Unset it
  before any role-parameterised script or `verify:nav` fails on the admin case.
- **S3-1 / S3-3 / S3-4 / S3-5** - unchanged, still open.
- **U18** focus ring unverified; **U21** no frontend unit runner.
- **I-34 / I-35** `ON DELETE CASCADE` (P3).
- **16 legacy notifications** in the dev database.
- `go test -race` **BLOCKED** - no C compiler.
- 1 vCPU / 500 MB target only **ESTIMATED**.
