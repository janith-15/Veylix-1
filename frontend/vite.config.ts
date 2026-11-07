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

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
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

