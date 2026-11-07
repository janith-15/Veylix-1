# RECORD · 2026-11-07 · Installation system build (D-079)

**Scope:** the installation surface only. Files created:
`docs/INSTALLATION_DESIGN.md`, `scripts/lib/common.sh`,
`scripts/lib/common.selftest.sh`, `scripts/install.selftest.sh`, `install.sh`,
`backend/cmd/vpn-platformctl/{main,support,status,lifecycle,update}.go` +
`main_test.go`. Files edited: `.github/workflows/release.yml`,
`docs/INSTALL.md`, `docs/UPGRADE.md`, `docs/DECISIONS.md`,
`docs/IMPROVEMENT_LEDGER.md`.

**Not touched:** `backend/internal/**` (no route, no migration, no business
rule), `frontend/**`, `var/`, `records/`, `.veylix-secrets/`. **No Git operation**
was performed — there is no repository here.

---

## 1. Gates actually run, with real output

| Gate | Command | Verdict | Output |
|---|---|---|---|
| Backend build | `go build ./...` | **PASS** | exit 0 |
| Backend vet | `go vet ./...` | **PASS** | no output, exit 0 |
| Formatting | `gofmt -l .` | **PASS** | empty (clean) |
| Backend tests | `go test ./... -count=1` | **PASS** | **13/13 packages ok** (new pkg is the 13th) |
| Race detector | `go test -race ./...` | **BLOCKED** | no C compiler on this machine |
| Frontend lint | `npm run lint` | **PASS** | 0 warnings, 0 errors, 175 files, 116 rules |
| UI truth gate | `npm run audit` | **PASS** | "no unwired/demo claims and no invented money literals" |
| UI truth self-test | `npm run audit:selftest` | **PASS** | 8 positive + 6 negative cases |
| Docs drift | `npm run docs:verify` | **PASS** | 21 migrations · 125 routes · 518 test functions in 13 packages |
| Workflow syntax | `npm run workflows:check` | **PASS** | `ci.yml` ok, `release.yml` ok — 2 files |
| CI CLI tests | `go test ./cmd/vpn-platformctl -count=1` | **PASS** | `ok veylix/backend/cmd/vpn-platformctl` |
| Linux cross-build | `GOOS=linux GOARCH=amd64 go build ./cmd/vpn-platformctl` | **PASS** | exit 0 |
| `bash -n` install.sh | `bash -n install.sh` | **PASS** | exit 0 |
| `bash -n` common.sh | `bash -n scripts/lib/common.sh` | **PASS** | exit 0 |
| `bash -n` self-tests | both selftest scripts | **PASS** | exit 0 |
| `shellcheck` | `shellcheck -x install.sh` | **NOT RUN** | shellcheck is not installed on this machine |
| `common.sh` self-test | `bash scripts/lib/common.selftest.sh` | **PASS** | **31 PASS**, 1 SKIPPED, exit 0 |
| `install.sh` guard test | `bash scripts/install.selftest.sh` | **PASS** | **31 PASS**, 1 SKIPPED, exit 0 |
| Clean-VM install | — | **BLOCKED** | no Linux VM and no public domain |
| HTTPS issuance | — | **BLOCKED** | needs a domain and ports 80/443 |
| Backup/restore rehearsal | — | **BLOCKED** | needs a clean VM |
| 1 vCPU / 500 MB benchmark | — | **NOT RUN** | — |

## 2. The two SKIPPED checks, named

| Script | Check | Why it is skipped |
|---|---|---|
| `common.selftest.sh` | "created file is mode 600" | Git Bash on Windows mounts with `noacl`; POSIX modes are emulated and `chmod 600` reads back as `644`. The assertion runs on any filesystem that can represent the mode. |
| `install.selftest.sh` | "T4 dry-run reaches every phase" | This host is Windows: `detect_os` finds no `/etc/os-release`, so `preflight` **correctly refuses** with `unsupported OS 'unknown'`. Claiming the later phases ran would be a false claim. On Linux the same dry run reaches all ten. |

## 3. What the guard tests actually prove

| Test | Proves |
|---|---|
| T5 branch / existing DB | a re-run on an installed host **refuses** and does not touch the database |
| T5b keys without a DB | a second install cannot silently adopt another install's keys; `--reuse-keys` is required and the key is not overwritten |
| clean host | a clean tree is accepted as a FRESH install |
| T4 dry-run | every mutation is suppressed; no database, key, env file or binary is created |
| T7 checksum | a mismatch **aborts and deletes** the artifact; a missing digest entry aborts |
| validate_domain | `localhost`, an IP literal, `notadomain` and an empty string are refused before anything happens |
| fingerprint | a key value never appears in output |

## 4. Honest summary

The installation **surface now exists**: a bootstrap installer, a permanent Go
CLI, a shared shell library, tests for all three, and a release workflow that
ships them together. Its **data-loss guards are tested**. What is **not** proven
is that any of it works on a real Linux host: no VM ran the installer, no
certificate was issued, and no backup has been restored. Bible §18 tests 30–34,
37, 38 and 39 are therefore **built, not verified**, and `docs/INSTALL.md` §F says
so row by row.
