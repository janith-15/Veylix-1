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
