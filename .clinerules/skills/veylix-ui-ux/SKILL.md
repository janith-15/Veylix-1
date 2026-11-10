---
name: veylix-ui-ux
description: Use for any change under frontend/src - components, pages, Zustand stores, API calls, theme tokens, Theme Studio, charts, layout or accessibility. Covers the rule that every store is API-backed with no mocks, that frontend/src/lib/api.ts is the only HTTP path, that status colours come from theme tokens and never hard-coded values, the theme architecture and Theme Studio requirements, the oxlint rules in use, and the UI truth and browser verification gates that must pass. Load before editing any React or CSS file.
---

# VEYLIX — UI / UX

## Non-negotiable frontend rules

1. **`frontend/src/lib/api.ts` is the only HTTP path.** No component and no store
   calls `fetch` directly.
2. **Every store is API-backed. Zero mocks.** A constant array, a fake delay or
   a stub in `frontend/src/stores/` is a defect. `npm run audit` exists to catch
   exactly this — and a passing build does not exempt you from it.
3. **Status colours come from theme tokens.** Never a hard-coded hex for a
   success/warning/danger state. Hard-coded colour is why the theme system leaks.
4. **The UI never owns a truth the server owns.** No locally derived VPN limits,
   no locally computed quota, no locally guessed expiry. Ask the server.
5. **An empty state says "no data".** Never draw a zero line to fill a chart.
6. **A VPN list shows the VPN's identity, not just its package name** (D-091.4).
7. **A non-active VPN state is explained, not merely coloured** (D-091.5).
8. **One layout hierarchy per screen, and no inline styles** (D-091, D-059).

## Showing a VPN: identity beside the package (D-091.4)

The panel email is the VPN's *name* — `<client email>_<kind>_<n>`, e.g.
`nimal@gmail.com_paid_2`. A customer who cannot see it cannot tell support which
client they hold, so every support thread starts with an operator guessing.

Rules for any client-facing list or card:

- Show **both** facts: the human label from `vpnDisplayName` (the package it came
  from) **and** the panel identity from `vpn.panelEmail`. Never one without the other.
- Render the identity so it is **readable**, not a raw `_paid_1` blob alone — pair
  the local part with what the suffix means (e.g. `nimal@gmail.com` + a `Paid · #2`
  chip). The **copy control must copy the exact panel email**, because that is the
  string an operator will search for on the panel.
- It is **not** a secret: it is the customer's own identity on their own server. Do
  not hide it, and do not substitute a package name for it.
- Never invent a label. If `panelEmail` is empty the row is malformed — say so rather
  than rendering a plausible-looking name.
- The same rule applies to any future screen that lists an owned VPN. A list without
  it is incomplete, which is a defect rather than a design preference.

## Explaining a non-active VPN (D-091.5)

`ACTIVE` needs no sentence. Every other status does, **on the row as well as on the
detail page**, because a grey chip is not an explanation:

| Status | The sentence must convey |
|---|---|
| `SUSPENDED` | paused by the seller; nothing deleted; it can be resumed |
| `TRAFFIC_DEPLETED` | the allowance is used up |
| `EXPIRED` | expired; a new package is needed |
| `SERVER_UNAVAILABLE` | **Veylix cannot currently see this VPN on its server** — this is why it is shown, not hidden |
| `REVOCATION_PENDING` | a removal is waiting for the server to confirm |
| `REVOKED` | revoked |
| `RELEASED` | released by the customer; undo window before Veylix drops its record |
| `REMOVED_FROM_CATALOG` | the package is gone but the VPN still exists |
| `PENDING` | still being set up on the server |

- Give an **unknown** status the generic sentence. Never invent an explanation —
  `VPNDetailPage.tsx` already follows this and is the reference implementation.
- `SERVER_UNAVAILABLE` is the case this rule exists for: it is the difference between
  "your VPN is gone" and "Veylix cannot see it right now", and the customer must be
  able to tell those apart.

## Layout discipline (D-091, D-059)

- **One hierarchy per screen.** Either every content group is an `<h2>` section, or
  every group is a titled `Card` — not both stacked in one column. A page that mixes
  them has no legible outline on screen or in a screen reader.
- **A heading never repeats at two levels.** `<h2>Connection</h2>` directly above
  `<Card title="Connection">` reads as a rendering fault.
- **Shared layout is a class, not a `style={{ … }}`.** Inline styles are what make two
  screens that should match drift apart; `D-059` already converted most of them, and
  a regression back to inline is a defect. If a layout needs to repeat (a warning
  block, a row of chips), it gets one class in that screen's stylesheet.
- Status colours, spacing and radii come from tokens. A layout that only works in one
  preset is broken.


## Store convention

`frontend/src/stores/` holds one file per domain (verified: `authStore`,
`serversStore`, `serverDetailStore`, `vpnStore`, `clientsStore`,
`clientAdminStore`, `adminsStore`, `offersStore`, `ordersStore`,
`packagesStore`, `pricingStore`, `paymentsStore`, `chatStore`, `notifStore`,
`peopleStore`, `profileStore`, `auditStore`).

Follow the existing shape — loading / error / data, explicit failure state, and
a real call through `lib/api.ts`. Do not add a new store for something an
existing store already owns.

## Theme architecture

`frontend/src/theme/` — `themes.ts`, `palettes.ts`, `subthemes.ts`,
`themeStore.ts`, `ThemeProvider.tsx`.
`frontend/src/styles/` — `tokens.css`, `tailwind.css`, `base.css`,
`ui-themes.css`, `variants.css`, `theme-extras.css`, `glass.css`,
`mobile.css`, `touch.css`, `style-layer.css`.

Theme Studio is a **required product feature**, not decoration: main presets,
editable sub-theme colours, typography settings, 3D/effect controls, reduced
motion, persistence across sign-in and reload, and a **Low-Resource mode**.
Settings must actually affect the whole app. Verify across pages, not on one.

Tailwind v4 declares tokens as custom properties; `.vscode/settings.json`
teaches the editor its at-rules. That config is intentional — do not change it
to silence an editor warning.

## Charts

Every chart **names its source and its window**. A 7d/30d/All-time selector
means three different queries. If the range has no data, say so. Server-wide
CPU/RAM/network charts are visually distinct from per-VPN usage and are labelled
as server-wide.

## Lint and accessibility

`frontend/.oxlintrc.json` enables `react`, `typescript` and `oxc` plugins with
`react/rules-of-hooks` as **error**. `oxlint` — not ESLint — is the linter here.

A cascading-render warning is a design signal: derive the value during render
(or `useMemo`) instead of writing state in an effect. Do not suppress it.

Form controls need labels, focus states, and correct disabled/loading semantics.
Tables need a real empty state, a real error state and a real loading state.

## Gates

```
npm run lint        # oxlint — target 0 warnings, 0 errors
npm run build       # tsc -b && vite build
npm run audit       # UI truth gate
npm run verify:ui -- /route     # real browser render of one route
npm run verify:nav              # navigation gate
```

Report the actual output. A green build is **not** evidence the page works.

## Findings (R2) — historical only

Prior frontend investigation is **historical context**. Re-read
`frontend/src/` before relying on it, and never assume a previously recorded
page list, store count or mock-free claim is still true today — verify it.

## Checklist

- [ ] All data goes through `frontend/src/lib/api.ts`.
- [ ] No mock, constant or hard-coded sample in a store.
- [ ] Colours and status come from tokens.
- [ ] Loading, empty, error and refused states all rendered honestly.
- [ ] `npm run lint` and `npm run build` run, output shown.
- [ ] `npm run audit` run, output shown.
- [ ] Every VPN the list renders shows `panelEmail` beside the package name, copyable.
- [ ] Every non-`ACTIVE` status carries an explanatory sentence on the row.
- [ ] One layout hierarchy (all `<h2>` or all `Card`), no repeated heading word.
- [ ] No new `style={{ … }}` for layout; the class lives in that screen's stylesheet.
