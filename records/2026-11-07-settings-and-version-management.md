# 2026-11-07 · Version management + operator-editable Google settings

**Scope:** two owner-requested capabilities, modelled on 3X-UI (`x-ui -v`, its
update menu, and a settings screen): (1) see and manage the running version
including a safe downgrade; (2) edit the Google OAuth client in the app, sealed,
applied live. Decisions **D-080** (settings) and **D-081** (versions).

## Version management (D-081)

- `/api/ready` now returns `version`; `GET /api/system/version` returns version,
  `schema_version`, `ui`, `go_version`, `platform` — read from the RUNNING
  process, never a file on disk.
- `internal/versioning`: reads the release feed, numeric version compare
  (`v1.10.0 > v1.9.0`), 1-hour TTL cache, and an **explicit error** (never "up to
  date") when the feed cannot be read.
- `GET /api/system/releases` and `GET /api/system/update-check` (Super Admin).
- `vpn-platformctl versions` (CLI) + `install.sh --list-versions`.
- **Safe downgrade:** `update --version <older>` needs `--allow-downgrade` **and**
  `--restore-backup <dir>`; the binary is swapped, then the matching backup is
  restored **before** the older binary starts. Without the flag it is a usage
  error. `--dry-run` is propagated into the restore it triggers.

## Operator-editable Google settings (D-080)

- Migration `0022_app_settings.sql`: `app_settings(key, value_ciphertext,
  is_secret, updated_at, updated_by)`.
- `internal/store/app_settings.go` owns the SQL (only `store` writes SQL);
  `internal/settings` seals (AES-256-GCM, AAD `app_settings:<key>`), merges
  DB-first over the environment, audits `settings.update`, and fires
  `OnGoogleChange` so a save is applied live without a restart.
- `GET/PUT /api/super/settings/google` (Super Admin only). The secret is
  write-only: the read returns `has_secret`, never the value; an empty
  `client_secret` keeps the stored one.
- Frontend: `SuperSettingsPage.tsx` (+ `super-settings.css`) at `/settings`,
  reachable from the Super Admin nav. Shows the version, an update notice with the
  exact `sudo vpn-platformctl update` command to copy (the browser cannot install
  — that needs root), and the Google client form with a write-only secret.

## Gates actually run (output seen)

```
go build ./...            exit 0
go vet ./...              exit 0
gofmt -l .                clean
go test ./... -count=1    15/15 packages ok
npm run lint              0 errors (1 advisory)
npm run build             exit 0
npm run audit             OK
npm run docs:verify       OK — 22 migrations · 133 routes · 560 tests in 15 packages
scripts/install.selftest.sh   exit 0, all guard checks passed (incl. new T8)
bash -n install.sh        exit 0
```

## Not proved, not claimed

- Live Google OAuth round-trip against a real tenant — **NOT PERFORMED** (no
  credentials here).
- A real downgrade over live data — **NOT PERFORMED** (no live host here).
- `go test -race` — **BLOCKED** (no C compiler).
- HTTPS, clean Linux VM, 1 vCPU / 500 MB benchmark — **BLOCKED** (no domain / VM).

## Files

Backend: `internal/store/migrations/0022_app_settings.sql`,
`internal/store/app_settings.go`, `internal/settings/{settings.go,settings_test.go}`,
`internal/httpapi/{settings_routes.go,settings_routes_test.go}`,
`internal/versioning/*` (prior pass), `internal/httpapi/system_routes*.go`,
`cmd/vpn-platformctl/{versions.go,update.go,main.go,main_test.go,support.go}`,
`cmd/veylixd/main.go`, `internal/audit/audit.go`, `internal/config/config_test.go`.
Frontend: `src/lib/api.ts`, `src/pages/SuperSettingsPage.tsx`,
`src/pages/super-settings.css`, `src/App.tsx`. Installer: `install.sh`,
`scripts/install.selftest.sh`. Docs: `DECISIONS.md` (D-080, D-081),
`API_CONTRACT.md`, `IMPROVEMENT_LEDGER.md`, `QA_REPORT.md`, `README.md`.