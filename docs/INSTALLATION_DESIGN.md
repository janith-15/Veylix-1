# VEYLIX — INSTALLATION SYSTEM DESIGN

**Type:** Design audit produced by `workflows/system-design-audit.md`. Read-only up
to the approval gate; this document is the gate.
**Scope:** the installer, the upgrade path, the `vpn-platformctl` CLI, the
Caddy/systemd templates, backup/restore, uninstall.
**Bible authority:** §14 (Git-based release and one-command server installation),
§15 (production operations, upgrades and recovery), §18 acceptance tests **30–39**.
**Status:** **APPROVED FOR BUILD** — see §12.

Companion documents (each keeps its own home, per `DOC_INDEX.md`):
`docs/INSTALL.md` (operator runbook) · `docs/UPGRADE.md` · `docs/DOMAIN_CHANGE.md`
· `docs/BACKUP_RESTORE.md` · `docs/TROUBLESHOOTING.md`.

---

## 0. Why this document exists at all

Four separate facts make an installation surface mandatory, and none of them is
satisfiable by a runbook:

1. **Bible §14** requires a one-command install from a Git release.
2. **Bible §18 tests 30–39** are acceptance tests, not documentation tasks.
3. `docs/INSTALL.md` §F currently marks the one-line installer and
   `vpn-platformctl` as **❌ not built**. That table is a claim, and it must stay
   true until a later stage observes otherwise.
4. `VPN_FIX_LEDGER.md` records a verification script that deleted live data,
   **twice**. An installer is a root script that writes to `/var/lib`. It is the
   single highest data-loss risk surface in this product.

---

## 1. What exists today (verified by reading the tree, 2026-11-07)

| Component | Path | State |
|---|---|---|
| Single binary serving UI + API | `backend/cmd/veylixd/main.go` | ✅ built; UI embedded via `internal/httpapi/dist` |
| Liveness | `GET /api/health` | ✅ `router.go:159` |
| Readiness + `schema_version` + embedded-UI status | `GET /api/ready` | ✅ `router.go:169` |
| Bootstrap probe (no auth) | `GET /api/setup/status` → `{"bootstrap_required":bool}` | ✅ `auth_routes.go:70` |
| Migrations | `backend/internal/store/migrations/*.sql`, embedded, applied at startup | ✅ 21 files |
| Config loader, fail-closed on missing keys | `backend/internal/config/config.go` | ✅ refuses to start without `VEYLIX_ENCRYPTION_KEY` / `VEYLIX_SESSION_KEY` |
| Caddy template | `deploy/Caddyfile` | ✅ shipped — cert auto, HSTS, single upstream, X-Forwarded-For |
| systemd unit | `deploy/veylixd.service` | ✅ shipped — no capabilities, `ProtectSystem=strict`, `MemoryMax=384M` |
| Env template | `deploy/veylix.env.example` | ✅ shipped |
| Backup / restore / verify | `deploy/veylix-backup.sh` | ✅ shipped — stops service, copies DB + `-wal` + `-shm` + both keys, verifies |
| CI | `.github/workflows/ci.yml` | ✅ build · vet · gofmt · test · **race** · tsc · lint · UI-truth · docs:verify |
| Release | `.github/workflows/release.yml` | ✅ tag `v*` → amd64 + arm64, boots amd64, asserts `/api/ready` + `/dashboard` 200 + `/assets/nope.js` 404 → `SHA256SUMS` |
| **One-line installer** | — | ❌ **MISSING** |
| **`vpn-platformctl` CLI** | — | ❌ **MISSING** |
| **`scripts/lib/common.sh`** | — | ❌ **MISSING** (referenced by the deployment skill, does not exist) |
| Clean-VM install test | — | ❌ NOT PERFORMED |

**Conclusion:** the *artifact* pipeline is complete and honest. What is missing is
the layer that turns an artifact into a running, upgradeable installation.

---

## 2. Requirements matrix (Bible §14, §15, §18) — status per row

Status vocabulary: **Built** · **Partial** · **Missing** · **Bug** · **Blocked**.

| Test | Requirement | Today | Owner of the fix |
|---|---|---|---|
| **30** | Clean Linux VM installs from one documented command; re-running does not wipe data | **Missing** | `install.sh` + Stage 2 refusal branch |
| **31** | Installer validates arch, DNS, ports, existing services; exits safely with instructions | **Missing** | `install.sh` Stage 1 preflight |
| **32** | First install creates DB/schema, systemd service, app config, Caddy config, initial Super Admin **with no hard-coded password** | **Partial** — templates exist, no orchestration | `install.sh` stages 4–9; Super Admin via `/setup` |
| **33** | Trusted cert issued, HTTP→HTTPS redirect, renewal configured | **Partial** — Caddyfile handles it, domain is manual | `install.sh` renders the domain; `vpn-platformctl domain-set` |
| **34** | Bad DNS / closed ports / stale IPv6 / port conflict / failed ACME → truthful incomplete state, never "HTTPS active" | **Missing** | `install.sh` preflight + HTTPS verification stage |
| **35** | Domain change via documented CLI without losing data; rollback if Caddy fails | **Missing** | `vpn-platformctl domain-set` |
| **36** | Panel URL/credential changeable in the UI, re-verified, no rebuild | **Built** — a product feature, not deployment | (no deployment work) |
| **37** | Upgrade runs pre-upgrade backup, applies migrations safely, confirms health, documented recovery | **Partial** — runbook only | `vpn-platformctl update` + `rollback` |
| **38** | Backup restores to a clean environment and preserves users, VPNs, payment/audit history, appearance prefs | **Partial** — script shipped, never rehearsed | `vpn-platformctl restore` + isolated rehearsal |
| **39** | Release download verifies checksum; no secret in logs/bundles/errors/network | **Partial** — `SHA256SUMS` published, nothing verifies it at install time | `install.sh` checksum stage + redaction library |

**What the matrix says plainly:** 6 of 9 deployment rows are **Missing**, and the
3 **Partial** ones are runbook-complete but machine-incomplete. No row is covered
by a document alone.

---

## 3. The core design decision: installer ≠ application

This is the answer to the owner's question — *"if the project is updated after the
installer is built, does the update reach the installer?"*

**The application is entirely inside the binary.** Go code, every SQL migration
and the whole React UI are compiled into one `veylixd` file (`release.yml`
"embed the UI" → `go build`). Therefore:

| Change | Artifact that changes | Installer must change? |
|---|---|---|
| Go business logic | new `veylixd` binary | **No** |
| New DB migration | new `veylixd` binary (migrations are embedded) | **No** |
| New UI page | new `veylixd` binary (UI is embedded) | **No** |
| Caddy/systemd template change | `deploy/` files in the release | **No** — `vpn-platformctl repair` re-renders them |
| Installer *logic* bug (e.g. a preflight check) | `install.sh` | Only for hosts not yet installed; existing hosts are repaired by `vpn-platformctl` |

So the installer is a **bootstrap**, executed once per host. Everything that must
keep working forever lives in `vpn-platformctl`, which ships inside the release
and is refreshed by `vpn-platformctl update` itself.

```
git tag v1.4.0 ──► release.yml ──► veylixd-linux-amd64 + SHA256SUMS
                                        │
        new host ───────────────────────┤
        install.sh  (bootstrap, once)   │  resolves "latest" release
          └─ installs veylixd + vpn-platformctl
                                        │
        existing host ──────────────────┘
        vpn-platformctl update  (forever)
          └─ backup → checksum → swap → migrate (at startup) → /api/ready verify
```

**Consequence for the design:** `install.sh` must contain **no version number**
and **no schema knowledge**. It resolves the release channel at run time. That is
what makes it immune to the project moving on.

---

## 4. Change classification (workflow Stage 1)

| Question | Answer |
|---|---|
| Fresh-install, upgrade, repair, uninstall, or shared machinery? | **All five.** Shared machinery (`scripts/lib/common.sh`, `vpn-platformctl`) is built first. |
| Can any path overwrite or delete existing data? | **Yes, four of them:** upgrade binary swap, restore, uninstall `--purge-data`, domain change reload. All are `P0`-class and every one takes a backup first. |
| Does it touch `enc.key` / `session.key`? | **Only generation, and only when absent.** Losing `enc.key` makes every sealed panel token unrecoverable. Existing keys are never overwritten. |
| Does it change the schema? | **No.** Migrations are embedded in the binary and applied at startup. The installer never runs SQL. |
| Does it change a documented command in `docs/INSTALL.md` §F? | **Yes — five rows flip ❌ → ✅, each only when Stage 5/6 observed it.** |

### Data-destruction surface (five explicit questions)

| # | Question | Answer for this design |
|---|---|---|
| 1 | Worst case if this script runs on a live host? | Fresh-install logic reaching a live database would reinitialise it. **Prevented by the DB-existence branch, which refuses and exits non-zero.** |
| 2 | How is the worst case detected *before* the mutation? | `[ -f "$DATA_DIR/veylix.db" ]` → refuse. Checked before any `mkdir`, `install`, or `systemctl` call. |
| 3 | What is preserved on every failure path? | `/var/lib/veylix` and `/etc/veylix` are never removed by any non-purge path. Restore moves the current state to `pre-restore-<ts>/` instead of deleting it. |
| 4 | Can two runs interleave? | No — `flock -n` on `/run/veylix-install.lock`, held for the script's lifetime. |
| 5 | Is the destructive path behind two independent gates? | `uninstall --purge-data` requires the flag **and** the operator typing the configured domain. |

---

## 5. Proposals — the 9 deliverables

Numbered so an approval can reference them individually. This section is
**proposal**, deliberately separate from §2's requirement status.

### P1 · `scripts/lib/common.sh` — the shared library

Every script sources it instead of reimplementing four things:

| Function | Contract |
|---|---|
| `say` / `warn` / `err` | prefixed output; `err` writes to stderr |
| `die` | message to stderr, exit 1 |
| `redact` | strips 32-byte base64 keys, `Bearer <token>` and `GOCSPX-…` from any string before it is printed |
| `need_root` | refuses to continue as non-root with "re-run with sudo" |
| `acquire_lock <path>` | `flock -n` on fd 9; failure names the lock path |
| `run <cmd…>` | executes, or prints `[dry-run] <cmd>` when `--dry-run` is set |
| `gen_key` | `head -c 32 /dev/urandom \| base64` |
| `fingerprint <file>` | `sha256sum file \| cut -c1-8` — never the value |
| `phase <n> <name>` | numbered phase banner so a failure names the phase that died |

### P2 · `install.sh` — the bootstrap (executed once per host)

**Preflight order** (cheap first; every failure prints a *correction*, never a
stack trace):

| # | Check | Failure message |
|---|---|---|
| 1 | root | "re-run with sudo" |
| 2 | lock acquired | "another veylix install is running (lock: …)" |
| 3 | OS in allow-list | prints the supported list |
| 4 | architecture `amd64` / `arm64` | prints what is supported |
| 5 | ≥ 200 MB free on the target filesystem | prints the shortfall |
| 6 | ≥ 400 MB RAM | prints the shortfall |
| 7 | ports 80/443 free (`ss`/`netstat`) | names the holder |
| 8 | DNS A record resolves to this host's IP | prints both values |
| 9 | release artifact exists for this arch | names the missing release |

Checks 7 and 8 accept `--skip-ports` / `--skip-dns`, and **skipping is printed**.

**Phases:**

```
1  preflight           the table above; nothing written yet
2  branch              fresh | refuse(installed) | refuse(upgrade w/o db)
3  fetch artifact      resolve latest release -> download -> SHA256SUMS -> verify
                       mismatch => rm -f artifact => die
4  user + dirs         veylix system user; /var/lib/veylix (0750 veylix:veylix); /etc/veylix (0750 root:root)
5  secrets             enc.key + session.key ONLY if absent (umask 077, install -m 0600 root:root)
                       present key + fresh-install intent => die, needs --reuse-keys
6  config              render /etc/veylix/veylix.env from template, keys inlined, 0600
7  binary + cli        install veylixd (0755 root:root) and vpn-platformctl
8  systemd             render unit; daemon-reload; enable; start; poll /api/ready
9  caddy               render domain into Caddyfile; caddy validate; reload; verify HTTPS
10 first-run report    print /setup URL, key fingerprints, backup instruction
```

**HTTPS truth rule (test 34):** success is claimed **only** when
`curl -fsS https://<domain>/api/ready` returns 200 from the public origin. A DNS
or ACME failure prints
`INSTALL INCOMPLETE — the service runs on http://127.0.0.1:8080 but HTTPS is not active`
and exits **2** — a distinct code meaning "installed, not publicly served". Exit 2
is never reported as success.

### P3 · `backend/cmd/vpn-platformctl/main.go` — the permanent CLI

Written in **Go**, not shell, because it is versioned with the binary it manages,
it gets a test suite in the same `go test` gate, and it cannot drift from a shell
copy of the same logic. Single file, stdlib only, `CGO_ENABLED=0`.

| Command | Does | Destructive? |
|---|---|---|
| `version` | prints build version + schema version | no |
| `status` | reads `/api/ready` + `systemctl is-active` + DB size | no |
| `doctor` | re-runs every preflight check against the installed host | no |
| `update [--channel stable] [--version vX.Y.Z]` | backup → verify checksum → keep `.prev` → swap → restart → poll `/api/ready`, compare `schema_version` | **yes** (backup first) |
| `rollback` | swap `veylixd.prev` back; if a migration ran, point at the pre-upgrade backup | **yes** |
| `domain-set <domain>` | render → `caddy validate` → reload → verify → **auto-revert on failure** | **yes** |
| `backup` | delegates to `deploy/veylix-backup.sh` (no second implementation) | no |
| `restore <dir>` | delegates to `deploy/veylix-backup.sh restore` | **yes** |
| `repair` | re-render systemd + Caddy, reload, re-check readiness. **Cannot touch DB, keys or accounts** | no |
| `uninstall [--purge-data]` | stops + removes unit and binaries; data preserved unless flag + typed domain | **conditional** |

### P4 · `docs/INSTALL.md` §F status table

Five rows flip ❌ → ✅, and **only on evidence from Stage 5/6**. Anything not
observed stays ❌ with the reason.

### P5 · `.github/workflows/release.yml`

Add `vpn-platformctl-linux-<arch>` to the artifact set and `SHA256SUMS`, so the
CLI is distributed with the binary it manages. The existing boot smoke-test is
reused; a CLI that cannot build is caught by `go build ./...` in CI.

### P6 · `docs/INSTALLATION_DESIGN.md` (this file) + `scripts/README.md`

The design of record, so a later agent does not re-derive it.

### P7 · Verification: `release-readiness.md` Stage 5b

Not copied here (that stage is the single owner). Stage 5b's 13 checks are the
acceptance proof, plus the Bible 30–39 rows in §2.

### P8 · Records

`docs/QA_REPORT.md`, a dated `records/` file, `docs/DECISIONS.md` (**D-079**),
`docs/IMPROVEMENT_LEDGER.md`, `docs/cline/progress.md`.

### P9 · No change to application behaviour

**Nothing in `internal/` changes.** No route, no migration, no business rule. The
only backend addition is a new `cmd/` — which the architecture rules place
squarely in `cmd/`, the wiring layer, holding no business rules.

---

## 6. Build order (dependency-driven, not preference)

```
P1 common.sh
   ├── P2 install.sh ──────────┐
   └── P3 vpn-platformctl      │   (both depend only on P1 and the shipped templates)
                                 │
P5 release.yml ──────────────────┤   (must ship the CLI the installer installs)
P4 INSTALL.md §F ◄───────────────┘   (truth table flips last, on evidence)
P6 design doc · P8 records
P7 Stage 5b verification ← after all of the above
```

A later version of the installer never invalidates an existing host because §3's
split holds: the host runs `vpn-platformctl update`, not `install.sh`.

---

## 7. Testing plan (isolated only — never live data)

| # | Test | Method | If not runnable here |
|---|---|---|---|
| T1 | `bash -n` on every new script | `bash -n install.sh scripts/lib/common.sh` | — |
| T2 | `shellcheck` | `shellcheck -x install.sh` | if absent: report **NOT RUN**, never "clean" |
| T3 | Go CLI gate | `go build ./... && go vet ./... && go test ./...` | — |
| T4 | `--dry-run` prints every phase, mutates nothing | run against a temp root | — |
| T5 | Branch refuses over an existing DB | pre-create `$TMP/veylix.db`, run, expect exit 1 | — |
| T6 | Idempotency | second run reports "already installed", exit 0, no mutation | — |
| T7 | Checksum mismatch aborts and removes the artifact | corrupt `SHA256SUMS` | — |
| T8 | Live install on a clean Linux VM | real VM + real DNS + ACME | **BLOCKED** — no Linux VM, no domain |
| T9 | HTTPS issuance + redirect | real domain | **BLOCKED** |
| T10 | Upgrade + rollback rehearsal | throwaway DB | partial — binary swap on Windows is out of scope |

T8–T9 are reported **BLOCKED with the reason**, never PASS.

---

## 8. Security review of this design

| Threat | Control |
|---|---|
| Installer leaks a key | `redact` in `common.sh`; only `fingerprint` is printed; no `set -x` anywhere |
| Artifact substitution (MITM) | `SHA256SUMS` verified before install **and** before every update; mismatch deletes the artifact |
| Two concurrent installs corrupt state | `flock -n`, non-blocking |
| Fresh install on a live host | DB-existence branch refuses, exit 1 |
| Second install pointed at the first's keys | existing key + fresh intent ⇒ `die`; `--reuse-keys` required |
| Uninstall destroys data | data preserved by default; purge needs flag **and** typed domain |
| Domain change takes the site down | `caddy validate` before `reload`; auto-revert to the previous Caddyfile on failure |
| Installer widens network exposure | `VEYLIX_ADDR` stays `127.0.0.1:8080`; the installer never edits firewall rules and never touches the SSH port |
| Secret in logs | no CLI output path prints a key, token or DB content |

---

## 9. Out of scope (stated so it is not silently dropped)

- Docker / Kubernetes packaging — excluded by `02-architecture-boundaries.md`.
- A web-based installer — the Bible asks for a one-line command, not a wizard.
- Automatic background self-update — an unattended root updater is a larger risk
  than it removes; `vpn-platformctl update` is explicit and operator-invoked.
- Multi-host / clustering — not a product requirement.

---

## 10. Known limitations accepted at design time

1. `MemoryMax=384M` in the unit is a **limit, not a measurement**. No benchmark
   has been run (Bible test 28 remains open).
2. Migrations are **forward-only**. Rollback across a migration is backup-restore;
   `docs/UPGRADE.md` §3b already says so.
3. Root is required. There is no rootless mode.
4. The installer assumes a **systemd** host. Non-systemd init is refused at
   preflight with a clear message rather than half-supported.

---

## 11. What this design does **not** claim

- It does not claim a clean-VM install has ever succeeded. Nothing has run on
  Linux yet.
- It does not claim HTTPS works. Caddy will do it; nobody has observed it.
- It does not claim the 1 vCPU / 500 MB target is met.

Every one of those is a Stage 5b row that stays **NOT RUN / BLOCKED** until it is
actually executed on a machine that can execute it.

---

## 12. Approval gate

`workflows/system-design-audit.md` Stage 12 requires an explicit owner decision
before implementation. **Recorded 2026-11-07:** the owner approved building this
design in Act mode, choosing:

| Question | Owner answer |
|---|---|
| Q1 — CLI language | **Go** (P3) |
| Q2 — artifact source | **GitHub releases** — `releases/latest/download/…` |
| Q3 — update channel | **stable channel**, latest release |
| Q4 — upgrade rollback may lose post-backup writes | **accepted** (`UPGRADE.md` §3b) |
| Q5 — no Linux VM available for T8/T9 | **reported BLOCKED**, never PASS |

Implementation proceeds under `workflows/installation-and-deployment.md`.
