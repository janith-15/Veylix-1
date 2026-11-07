# VEYLIX — LIVE PREVIEW (local dev)

**Purpose:** one command starts the real backend and the Vite dev server, and prints
a link you can click. No production build, no `vite preview`, no manual `npm run dev`
in the right order — the order matters and forgetting it is the single most common
reason the app "looks broken".

> Building a **single binary** (UI embedded, API and app on one port) is a different,
> one-command job: `.\dev.ps1 -Package`. See section 8.

---

## 1. One command

```powershell
cd "h:\my apps\Veylix-1"
.\dev.ps1
```

Then open (the script prints these, and only after it has verified they answer):

| What | URL |
|---|---|
| **App** | http://localhost:5173/dashboard |
| Sign in / register | http://localhost:5173/login |
| First-run wizard | http://localhost:5173/setup |
| Backend health | http://127.0.0.1:8080/api/health |
| Backend "who am I" | http://127.0.0.1:8080/api/me |

Stop everything:

```powershell
.\dev.ps1 -Stop
```

Check what is running (and whether the dev bypass is really on):

```powershell
.\dev.ps1 -Status
```

### Variants

| Command | Effect |
|---|---|
| `.\dev.ps1 -Log .\var\logs` | Both processes log to files, windows stay hidden |
| `.\dev.ps1 -NoBrowser` | Start without opening a browser tab |
| `.\dev.ps1 -Lan` | Also bind Vite to the LAN so a phone can open `http://<your-ip>:5173/dashboard` |
| `.\dev.ps1 -Backend` / `-Frontend` | Start only one side |
| `.\dev.ps1 -Stop` | Kill whatever holds :8080 / :5173 |

---

## 2. Why the script exists (what it actually does)

1. **Loads `.veylix-secrets\dev-env.ps1` into the launching shell**, then starts
   `go run ./cmd/veylixd` from that shell so the child inherits
   `VEYLIX_DEV_AUTH=1`, `VEYLIX_INSECURE_COOKIES=1` and the DB keys.
   Starting `go run` from a fresh terminal silently enforces real auth and every
   screen answers `401 authentication required` (D-022).
2. **Waits for `/api/health`** — it does not poll `HasExited`, because with
   redirected output PowerShell keeps the reader handles open and that flag can
   stay `false` forever (this hung the script during the 2026-10-24 run).
3. **Proves the dev bypass** by calling `/api/me` and printing the identity it got
   (`dev@veylix.local · super_admin`). A 401 here is reported explicitly instead of
   being left for you to discover in the UI.
4. **Starts Vite and waits for the first 200** so the printed link is one that
   answers, not a connection-refused page.

### Port discipline

`frontend/vite.config.ts` pins **port 5173 with `strictPort: true`**. Vite's default
is to hop to 5174 when 5173 is taken, which yields a page that loads fine while
every state-changing request answers `403 cross_origin` (the backend's CSRF origin
check and the dev cookie are bound to 5173). A hard failure is better than that.

---

## 3. What you see when it works

- The topbar shows a **DEV · auto-login** chip. Click it to switch persona
  (super admin / reseller / client) without touching the login form. Switching
  navigates to that persona's home (`/dashboard`, `/admin`, `/client`).
- **Sign in for real**: open `/login` and use an account from
  `.veylix-secrets/seed.json` (or Google, once configured). The old DEV chip is
  gone — the bypass was removed on 2026-10-28, see `docs/AUTH.md`.
- Screens read real data: `/dashboard` shows the panels from
  `GET /api/servers`, their CPU from `GET /api/servers/{id}/metrics`, freshness from
  `GET /api/sync/status`, clients from `GET /api/clients`.
- Editing a `.tsx`/`.css` file hot-reloads instantly (Vite HMR).

---

## 4. Verification, in order

```powershell
# 1. is everything up, and is the backend the dev one? (prints each persona)
.\dev.ps1 -Status

# 2. the gate: personas + permission matrix + accounts + proxy (17 checks)
cd frontend ; npm run preflight
#    or, in one command from the root:  .\dev.ps1 -Restart -Preflight

# 3. the proxy path the browser uses (must answer 200, same as direct :8080)
Invoke-WebRequest http://localhost:5173/api/me -UseBasicParsing -Headers @{ 'X-Veylix-Dev-Role' = 'admin' }

# 4. UI truth gate — no "demo data" / "no API yet" claims, no invented prices
npm run audit
npm run audit:selftest

# 5. real browser render of a page (headless Chrome, reports metric cards + console errors)
npm run verify:ui /dashboard
$env:VEYLIX_DEV_ROLE='admin'; npm run verify:ui /admin; Remove-Item Env:VEYLIX_DEV_ROLE

# 6. gates
npm run lint ; npm run build
cd ..\backend ; go build ./... ; go vet ./... ; go test ./... -count=1
```

---

## 5. Known dev-only noise (not a product defect)

`Encountered two children with the same key, 0` can appear in the browser console
while an account has several notifications. It is React's **development-only**
reconciler warning, it does not occur in a production build, and it reproduces on
untouched screens (`/notifications`, `/profile`) for the reseller and client
personas. The notification rows the server returns have distinct ids, so it is a
dev-mode collision inside the shell rather than duplicate data. Tracked in
`docs/IMPROVEMENT_LEDGER.md` (F-031) — it must not be "fixed" by suppressing console
errors, and it must not mask a real key bug in a page.

---

## 6. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `401 authentication required` everywhere | backend started without the dev env | `.\dev.ps1 -Restart` (never `go run` in a bare shell) |
| DEV chip says *admin* but super-admin screens still open | backend role switching is off (stale process, or `-Roles:$false`) | `.\dev.ps1 -Restart`; `.\dev.ps1 -Status` prints what each persona answers |
| `Port 8080 is already in use` / `Port 5173 is already in use` | a previous run is still alive | `.\dev.ps1 -Restart` (or `-Stop`) |
| `403 cross_origin` on POST | the page is on another port/origin (e.g. 5174) | use 5173 exactly; `strictPort` now prevents the hop |
| App blank, console `Cannot reach the server` | backend not up | `.\dev.ps1 -Status` |
| Login page loads but every screen is empty | you are signed in as a role that owns nothing yet | click the **DEV** chip, or create offers/clients first |
| Can't test "disable an admin" | the action refuses your own account (`422 cannot_modify_self`) | `.\dev.ps1 -Restart -FakeAdmin` (adds a second admin) |
| `go: command not found` | Go not on PATH | install Go 1.27+ (`go version`) |

---

## 7. Data location

The dev database is `H:\my apps\Veylix-1\.veylix-secrets\data\veylix.db`
(git-ignored). `cmd/seeddemo` and every probe binary write there too, so demo
accounts and test offers appear in the same DB the UI reads. To start from an
empty install, stop the server, move that folder aside, and re-run `.\dev.ps1`
(the app then offers `/setup` for the first super admin).

---

## 8. Build and run the single binary (E7 / D-029)

```powershell
.\dev.ps1 -Package
```

What it does, in order, and why each step exists:

1. `npm run build` in `frontend/` — judged by exit code **and** a real `dist/index.html`
   (Vite writes its chunk-size hint to stderr, which PowerShell would otherwise treat as
   a failure).
2. Wipes `backend/internal/httpapi/dist/` except `.gitkeep` and copies the new build there.
   The wipe matters: a stale `index-<hash>.js` left behind is a file a browser could still
   request long after it stopped matching the code.
3. `go build -o bin/veylixd.exe ./cmd/veylixd` — the embed happens at compile time.
4. **Starts that binary on a throwaway port and asserts it** (this is the step that makes
   the command trustworthy):

```
PACKAGED OK - one binary, one port (18080)
  /api/ready  -> ready  (ui: embedded:7_files)
  /dashboard  -> 200 text/html
  /vpn/16     -> 200 (SPA deep link)
  /assets/nope.js -> 404 (missing file stays 404)
  dev-bypass in bundle -> absent (ok)
```

Run the packaged app:

```powershell
$env:VEYLIX_ADDR='127.0.0.1:8080'
.\backend\bin\veylixd.exe        # http://localhost:8080/dashboard
```

If `/api/ready` says `"ui": "not_embedded"`, the binary was built **without** step 1–2 —
the API works, the interface is absent, and every UI path answers a JSON 404 that says so.
Server installation: [`INSTALL.md`](INSTALL.md).

