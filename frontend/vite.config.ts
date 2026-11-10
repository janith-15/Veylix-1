import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// The real backend runs on loopback (default :8080). Proxying keeps the dev
// server same-origin, which the HttpOnly session cookie and the backend's
// origin check both require. Override with VEYLIX_API_TARGET if needed.
const apiTarget = process.env.VEYLIX_API_TARGET ?? 'http://127.0.0.1:8080'

// Port 5173 is pinned because the backend's CSRF origin check and the then-
// current dev cookie are both bound to the origin the browser used. Vite's
// default behaviour is to hop to 5174 when 5173 is taken, which produces a
// working page whose every POST answers 403 cross_origin — a failure that looks
// like a broken login. `strictPort` turns that into an immediate, readable
// error instead.
const devPort = Number(process.env.VEYLIX_DEV_PORT ?? 5173)

// `--host` equivalent for phone/LAN review of the preview. Opt-in only:
//   VEYLIX_DEV_HOST=1        `npm run dev`        (LAN access, same port)
//   VEYLIX_DEV_QUIET=1       `npm run dev:quiet`  (log file, no browser sheet)
// The dev auth bypass is loopback-only on the backend, so a LAN host can never
// widen the dev principal beyond this machine's panels (D-022).
const lanHost = process.env.VEYLIX_DEV_HOST === '1'

/*
 * The public source repository, shown as a link at the foot of the sidebar
 * (D-101).
 *
 * WHY IT IS DEFINED HERE RATHER THAN ONLY IN `.env`
 * `.env` is git-ignored in this repository (it holds secrets in other projects),
 * so a value that lives only there reaches one machine and silently vanishes
 * everywhere else — the footer would appear for the developer and disappear in
 * every deployment built from the repo. A build-time default belongs in the
 * build config, where it is committed and reviewed.
 *
 * It is still overridable per build (`VITE_REPO_URL=… npm run build`) and per
 * environment (a `.env.local` or a shell variable wins), so a fork can point the
 * link at itself without editing code.
 *
 * It is a build property, not a server setting: where the code lives cannot
 * change at runtime, needs no request to display, and is not a secret.
 */
const DEFAULT_REPO_URL = 'https://github.com/janith-15/Veylix-1'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  define: {
    // `import.meta.env.VITE_REPO_URL` resolves to a real env/.env value when one
    // is set, and to this committed default otherwise. Declared as a literal so
    // the `?.`/`||` in the component still compile to a plain string.
    __VEYLIX_REPO_URL__: JSON.stringify(process.env.VITE_REPO_URL || DEFAULT_REPO_URL),
  },
  server: {
    port: devPort,
    strictPort: true,
    host: lanHost ? true : undefined,
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: false,
      },
    },
  },
  // `vite preview` (the production build server) gets the same proxy so a
  // `dist/` review can talk to the same backend.
  preview: {
    port: devPort,
    strictPort: false,
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: false,
      },
    },
  },
})

