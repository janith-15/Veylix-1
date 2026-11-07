# VEYLIX — DEVELOPMENT BASELINE

**Status:** authoritative · **Established:** 2026-11-08 · **Baseline commit:** `1b09553`

This is the "how do I work on this repository" document. It is the replacement for
the operational half of `./docs/LIVE_PREVIEW.md`, `./dev.ps1` documentation and the
`CONTRIBUTING` content that never existed.

---

## 1. Prerequisites (A — confirmed on this machine)

| Tool | Version used | Check |
|---|---|---|
| Go | 1.27 | `go version` |
| Node | 24.19.0 | `node -v` |
| npm | ships with Node | `npm -v` |
| PowerShell | 5.1+ | `$PSVersionTable.PSVersion` |
| Git | 2.56.0.windows.4 | `git --version` |

**Not required:** Docker, a C compiler, Redis, PostgreSQL. The SQLite driver is pure
Go; the frontend is a build step, not a server.

---

## 2. First run

```powershell
# from H:\my apps\Veylix-2
cd frontend
npm install
cd ..
.\dev.ps1 -Status       # what is running, what is configured
.\dev.ps1 -Preflight     # build + test + lint + audit, in order
```

`.\\dev.ps1 -Package` builds the frontend, embeds it into the binary and produces the
shippable artifact. **`dev.ps1` is the single source of truth for run and verify** —
call it, never reimplement its steps.

---

## 3. The gates — run these, do not invent replacements

### Backend (from `./backend/`)

```powershell
go build ./...
go vet ./...
gofmt -l internal\ cmd\        # must print nothing
go test ./... -count=1
```

**`go build` and `go vet` are the only authority on Go diagnostics.** The workspace
editor config deliberately disables gopls staticcheck, so an editor squiggle is not
evidence of anything.

`go test -race` is **BLOCKED** on this machine (no C compiler). Report it as
`BLOCKED` — never `PASS`, never `FAIL`.

### Frontend (from `./frontend/`)

```powershell
npm run lint          # oxlint — this repo does NOT use ESLint
npm run build         # tsc -b && vite build
npm run audit         # UI truth gate: no mocks, no unbacked claims
npm run audit:selftest
npm run docs:verify   # documentation drift gate
```

`npm run audit` is the gate that actually protects honesty. A passing `build` proves
only that the code compiles.

### Browser (committed, deterministic)

```powershell
npm run verify:nav      # every route resolves for every role
npm run verify:ui       # real DOM assertions over headless-Chrome CDP
npm run verify:mobile   # responsive check
npm run verify:client-vpns  # My VPNs against a populated account (refuses if it owns none)
```

These are the **only** browser regression proof. An agent driving Playwright by hand
produces no reproducible assertion and can never be reported as `PASS` for browser
coverage. Use Playwright to **find** a problem; commit a `verify-*.mjs` to **prove**
it is fixed.

---

## 4. Data safety — mandatory, from a recorded incident

A verification script once deleted live data, **twice**. Therefore:

1. **Never execute a verification script against live user data.**
2. **Never delete, reset, truncate or overwrite existing runtime data.**
3. A test touching persistent data uses an **isolated test database and an isolated
   data directory** — never `./var/`, never `./.veylix-secrets/`.
4. Prefer read-only verification. If a check must write, it writes to a temporary
   location and says so.
5. Never point a test at a production deployment or the seeded development panel.

Before any destructive command: take a backup, run a dry run, read the dry-run row
count, then apply.

---

## 5. Where things live

```
H:\my apps\Veylix-2\
├── backend\                 Go module: veylix/backend
│   ├── cmd\veylixd\         the server
│   ├── cmd\pwreset\         dev tool: set a password (guarded, refuses super_admin
│   │                        unless VEYLIX_DBEXEC=1)
│   ├── cmd\panelprobe\      dev tool: ask 3X-UI whether a client exists
│   └── internal\            audit auth config domain httpapi integrations mail
│                            panel payments provisioning releases settings store
│                            sync versioning
├── frontend\                React + TS + Vite (37 pages)
├── docs\
│   ├── new\                 ← THIS SET, authoritative
│   ├── restart\             Phase 2 audit, known losses, unknowns
│   └── cline\               activeContext.md, progress.md
├── scripts\                 archive + verify helpers
├── var\                     local runtime output          (git-ignored)
├── .veylix-secrets\         local DB + keys                (git-ignored, P0)
├── dev.ps1                  run and verify — the one entry point
└── MASTER_PRODUCT_BIBLE.md  SUPERSEDED — historical reference, kept per ADR-003
```

`./veylix-ui demo/` is a **separate application**. Not Veylix. Do not edit it, do not
cite it.

---

## 6. Secrets

P0 credentials live in exactly two places: `./.veylix-secrets/` (git-ignored) or the
sealed column `server_integrations.token_ciphertext`. They must never appear in
source, docs, records, logs, error messages, API responses, the frontend bundle,
exports, or a command line.

**Before calling security work finished:** grep the changed surface for the secret
shapes; confirm no new log line can emit a token, key, UUID or password; confirm
every changed state-changing route is covered by the audit log.

---

## 7. The session protocol

1. Read `docs/cline/activeContext.md` and the tail of `docs/cline/progress.md`.
2. State the change scope **in writing, before editing**.
3. Load the matching skill before touching that layer.
4. Run the gates.
5. Append to `docs/cline/progress.md`, rewrite `activeContext.md`, add a
   `DEVELOPMENT_LEDGER.md` entry if a defect was touched, and **commit**.

---

## 8. Git workflow

```powershell
git status
git add -A
git commit -m "<area>: <what changed and why>"
```

- **Never** commit `.veylix-secrets/`, `var/`, `*.db`, `node_modules/`, `dist/`.
  `.gitignore` already excludes them; verify before the first commit of a session.
- One coherent change per commit.
- A commit is part of "done". Work that is not committed is work that can be lost —
  which is exactly what `KNOWN_LOSSES` `L-01` records.
