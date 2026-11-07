# Veylix — frontend

React 19 + TypeScript + Vite 8 (Tailwind 4, zustand). **Start it from the repository
root, not from here:** `.\dev.ps1` loads the dev secrets, starts the Go backend,
verifies the dev auth bypass and then starts Vite — see
[`../docs/LIVE_PREVIEW.md`](../docs/LIVE_PREVIEW.md).

## Scripts

| Script | What it does |
|---|---|
| `npm run dev` | Vite dev server on **port 5173** (`strictPort`; the backend's origin check depends on it) |
| `npm run dev:quiet` | Same, without the browser sheet (used by `.\dev.ps1 -Log`) |
| `npm run build` | `tsc -b && vite build` |
| `npm run lint` | oxlint, expected **0 warnings / 0 errors** |
| `npm run audit` | **UI truth gate** — fails if a screen claims a capability the backend contradicts or ships invented business numbers |
| `npm run audit:selftest` | Proves the audit rules still fire (6 positive + 4 negative cases) |
| `npm run verify:ui -- /path` | Renders a page in headless Chrome through CDP and reports its real content; fails on any console error. `VEYLIX_DEV_ROLE=admin\|client` reviews role-only screens |
| `npm run preview` | Serves `dist/` (same `/api` proxy) |

> **Single binary (E7 / D-029):** `.\dev.ps1 -Package` from the repository root builds this
> app, copies `dist/` into `backend/internal/httpapi/dist/`, and compiles it into
> `backend/bin/veylixd.exe` — one artifact that serves the UI **and** the API on one port.
> The copy is generated, so it is git-ignored (only `dist/.gitkeep` is committed).

## Environment files

- `.env.development` → `VITE_DEV_AUTH=1` auto-login for UI review (never read by `vite build`).
- Vite server switches read from the shell by `dev.ps1`: `VEYLIX_DEV_PORT` (default 5173),
  `VEYLIX_DEV_HOST=1` (bind LAN), `VEYLIX_API_TARGET` (proxy target, default `http://127.0.0.1:8080`).

## Structure

```
src/
  pages/        role screens (dashboard, servers, offers, admins, payments, chat, …)
  components/   design system (ui/), charts/, layout/, theme-studio/, effects/, chat/, palette/
  stores/       zustand stores — all 11 call the real API through lib/api.ts (0 mocks)
  lib/          api.ts (typed client + ApiError), passwordPolicy.ts, qr.ts
  theme/        presets, sub-themes, ThemeProvider
  styles/       tokens, glass, themes, style-layer (everything consumes CSS custom properties)
scripts/        audit-ui-truth.mjs, verify-dashboard.mjs, check-root.mjs, smoke-*.ps1
```

