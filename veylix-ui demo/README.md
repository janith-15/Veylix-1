<div align="center">
<img width="1200" height="475" alt="GHBanner" src="https://ai.google.dev/static/site-assets/images/share-ais-513315318.png" />
</div>

# Run and deploy your AI Studio app

This contains everything you need to run your app locally.

View your app in AI Studio: https://ai.studio/apps/850480f6-ead0-49dd-8cc4-22b752f1ed4a

## Run Locally

**Prerequisites:** Node.js **20.19+** (or 22.12+) — required by Vite 8.

### One command (Windows)

```powershell
.\dev.ps1              # installs deps if missing, starts Vite, opens http://localhost:3000
.\dev.ps1 -Status      # is it running, does it answer?
.\dev.ps1 -Restart     # replace whatever holds the port
.\dev.ps1 -Stop        # kill it
.\dev.ps1 -Log         # run hidden, output in .\logs\dev.log
.\dev.ps1 -Port 3001   # serve on another port
```

### Manual

1. Install dependencies:
   `npm install`
2. (Optional) Create `.env` from `.env.example` — **not required today**, no module
   reads `GEMINI_API_KEY` yet. Set `APP_URL="http://localhost:3000"` for your own use.
3. Run the app:
   `npm run dev` → open **http://localhost:3000**

Other scripts: `npm run lint` (tsc --noEmit) · `npm run build` (→ `dist/`) · `npm run preview`.

### Notes for this repository

- This is the **frontend-only** app (React 19 + Vite 8 + Tailwind 4). There is **no
  backend and no database**: `src/store/useVeylixStore.ts` persists everything to
  `localStorage` under `veylix_platform_state_v2`, seeded from `src/data/initialData.ts`.
- Blank screen or unexpected data → run `localStorage.removeItem('veylix_platform_state_v2')`
  in the browser console and reload (old saved state from an incompatible schema).
- The `docs/` folder describes the full Go product (`dev.ps1` on :8080 + :5173, 27-step
  auth preflight, 3X-UI API). **Those docs are for the `Veylix-1` repository**, whose
  backend code is not part of this checkout — the `dev.ps1` here is the frontend-only one.

View your app in AI Studio: https://ai.studio/apps/850480f6-ead0-49dd-8cc4-22b752f1ed4a

