# VEYLIX — EVIDENCE INDEX

**Status:** authoritative · **Established:** 2026-11-08 · **Baseline:** `1b09553`

**Purpose.** For every significant claim in `docs/new/`, say **how it was verified**
and **where the proof lives**. A reader who doubts a statement should be able to
check it in one step.

**Markers.** **C** confirmed · **A** assumed · **U** unknown (→ `UNKNOWNS.md`).

---

## 1. Verification methods used

| Code | Method | Reproducible by |
|---|---|---|
| `M-READ` | read the source file | anyone |
| `M-GREP` | measured with a search across the tree | anyone, command shown |
| `M-BUILD` | `go build ./...` / `npm run build` | anyone |
| `M-TEST` | `go test ./... -count=1` | anyone |
| `M-LINT` | `go vet`, `gofmt -l`, `npm run lint` | anyone |
| `M-AUDIT` | `npm run audit`, `npm run audit:selftest` | anyone |
| `M-DOCS` | `npm run docs:verify` | anyone |
| `M-SQL` | **read-only** `SELECT` against the dev database | anyone with the dev secrets |
| `M-REMOTE` | asked the 3X-UI panel itself (`cmd/panelprobe`) | panel must be reachable |
| `M-HTTP` | a live request to the running server | server must be up |

---

## 2. Evidence by claim

| Claim | Marker | Method | Proof |
|---|---|---|---|
| 617 test functions in 15 packages | C | `M-DOCS` | `docs:verify`: `617 test functions in 15 packages` |
| 23 migrations | C | `M-DOCS`, `M-READ` | `docs:verify`; files in `store/migrations/` |
| 127 route registrations / 133 route pairs | C | `M-GREP` | `mux.Handle` extraction over `httpapi/*.go` |
| Go: 136 files / 29,716 lines | C | `M-GREP` | Phase 2 audit §2 |
| TS/TSX: 148 files / 31,527 lines, 37 pages | C | `M-GREP` | Phase 2 audit §2 |
| The dependency graph (`ARCHITECTURE.md` §2) | C | `M-GREP` | import extraction per package |
| No import cycle exists | C | `M-BUILD` | `go build ./...` exit 0 |
| `httpapi` holds 12 SQL statements | C | `M-GREP` | statement count per file |
| 112 SQL statements live outside `store/` | C | `M-GREP` | same, whole tree |
| Only `panel` calls the 3X-UI panel | C | `M-GREP`, `M-READ` | `http.Client{` enumeration |
| FK actions: 38 CASCADE / 34 SET NULL / 14 RESTRICT | C | `M-GREP` | `ON DELETE …` count over 23 migrations |
| 23 named indexes | C | `M-GREP` | `CREATE INDEX` extraction |
| `inbounds.id` ≠ `inbounds.remote_id` (52 vs 1) | C | `M-SQL` | read-only `SELECT id, remote_id FROM inbounds` |
| `DEV-001` fixed; the panel received the right inbound | C | `M-TEST` + `M-REMOTE` | `TestCreateSendsThePanelsInboundNumber`; `panelprobe vpn_id=42 → FOUND` |
| `DEV-002` fixed; 193 rows removed, 46 remain | C | `M-SQL` + `M-HTTP` | dry run 193/46 then applied; `GET /api/notifications` shows 2 meaningful rows |
| Dev DB: 798,720 bytes, 8 users, 1 panel, 5 offers, 4 VPNs | C | `M-SQL` | read-only row counts |
| Session cookie is HttpOnly and server-side | C | `M-READ` | `auth/session.go`, `auth/service.go` |
| Two login doors enforced server-side | C | `M-READ` | `auth.Portal`; 403 `wrong_portal` |
| Audit trail is append-only | C | `M-READ` | `internal/audit`; no UPDATE/DELETE path |
| Release never deletes a panel client | C | `M-READ` | `releases/service.go` |
| `npm run audit` is itself honest (8+/6− cases) | C | `M-AUDIT` | `audit:selftest` output |
| All gates pass on 2026-11-08 | C | `M-BUILD`/`M-TEST`/`M-LINT`/`M-AUDIT` | `QA_TEST_REPORT.md`, real output |
| No Git repository existed before this session | C | `M-READ` | no `.git`; Phase 2 audit |
| Git baseline is 717 files / 165,391 insertions | C | `M-READ` | `git log --stat` on `1b09553` |

---

## 3. What is NOT verified (and must not be reported as if it were)

| Gap | Marker | Why it cannot be verified here |
|---|---|---|
| `go test -race` | **BLOCKED** | no C compiler on this machine |
| Production behaviour | **U** | no production deployment exists |
| HTTPS / TLS / certificate | **U** | no domain, nothing on 80/443 |
| Linux behaviour | **U** | this machine is Windows |
| 1 vCPU / 500 MB usage | **U** | a Windows working set is not a Linux VM measurement |
| Accessibility (focus rings) | **U** | `DEV-009`; no automated assertion |
| Mobile beyond 390 px + touch | **U** | `DEV-010`; the gate checks one width and no touch |
| `npm run preflight` on 2026-11-08 | **NOT RUN** | needs a running server; must run before any release claim |
| Google OAuth end to end | **A** | implemented and unit-tested; no live OAuth client exercised |
| SMTP delivery | **A** | no relay configured; the code correctly reports `outbox` |

---

## 4. Where the raw evidence lives

| Artifact | Location |
|---|---|
| Phase 2 audit (13,773 bytes) | `docs/restart/PHASE_2_AUDIT.md` |
| What was lost | `docs/restart/KNOWN_LOSSES.md` |
| What is unresolved | `docs/restart/UNKNOWNS.md` |
| Gate output with real console text | `docs/new/QA_TEST_REPORT.md` |
| Source archive manifest | `_veylix-archive\MANIFEST.md` |
| Archive helper | `scripts/archive-phase1.ps1` |
| Archive verifier | `scripts/verify-archive.ps1` |
| Session history | `docs/cline/progress.md` |
| Current state | `docs/cline/activeContext.md` |
| Git baseline | commit `1b09553` on `main` |

---

## 5. How to re-verify everything in ten minutes

```powershell
# 1. Git baseline intact
cd 'H:\my apps\Veylix-2'; git log --oneline -1

# 2. Backend gates
cd backend; go build ./...; go vet ./...; gofmt -l internal\ cmd\;
go test ./... -count=1

# 3. Frontend gates
cd ..\frontend; npm run lint; npm run build; npm run audit;
npm run audit:selftest; npm run docs:verify

# 4. The two measurements this document leans on
cd ..\backend
Get-ChildItem internal -Recurse -Filter *.go |
  Where-Object { $_.Name -notlike '*_test.go' -and $_.DirectoryName -notmatch '\\store$' } |
  ForEach-Object { (Select-String -LiteralPath $_.FullName `
    -Pattern 'SELECT |INSERT INTO|UPDATE \w+ SET|DELETE FROM' -AllMatches |
    Measure-Object).Count }

Get-ChildItem internal -Recurse -Filter *.go |
  Where-Object { $_.Name -notlike '*_test.go' } |
  Select-String -Pattern 'http\.Client\{|http\.NewRequest' |
  Select-Object -ExpandProperty Path -Unique

# 5. Archive still complete
cd ..; .\scripts\verify-archive.ps1
```

Each command's expected output is stated in the section that relies on it. **If a
real output disagrees with this document, the output wins and this file is wrong.**

