# VEYLIX — INSTALL

**Honest scope.** The server ships as **one binary that serves the UI and the API on
one port** (D-029), plus **`vpn-platformctl`** for every operation after the first
install. `install.sh` is the one-line bootstrap (D-079). It is **built and its
data-loss guards are tested**, but it has **not been run on a clean Linux VM** —
that is the remaining gap, and §F says so precisely.

| Path | When to use | Time |
|---|---|---|
| **A. Windows / this machine** | development and owner review | 2 min |
| **B. Linux server — `install.sh`** | a real deployment | ~5 min |
| **C. Linux server — manual** | when you want to see every step | 15 min |

---

## B0. The one-line install (no flags needed)

```bash
bash <(curl -Ls https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh) --domain vpn.example.com
```

The installer resolves the repository itself; `--repo` is only for a fork. At
the end it asks for the Super Admin email, display name and password (the
password is not echoed) and creates the account through
`POST /api/setup/bootstrap`. The password travels in a 0600 temp file, never in
`argv`, because arguments are visible in `ps`. Leave the email blank -- or run
non-interactively with no TTY and no `VEYLIX_ADMIN_EMAIL` -- and the account is
created at `/setup` instead. No default password is ever written.

Non-interactive:

```bash
VEYLIX_ADMIN_EMAIL=you@example.com VEYLIX_ADMIN_PASSWORD=secret \
  bash <(curl -Ls https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh) --domain vpn.example.com
```
## B. Install on a Linux server with `install.sh`

```bash
# 1. Always dry-run first: it prints every phase and changes nothing.
sudo ./install.sh --domain vpn.example.com --dry-run

# 2. Then the real install.
sudo ./install.sh --domain vpn.example.com
```

**Exit codes carry meaning, and a script should branch on them:**

| Code | Meaning |
|---|---|
| `0` | installed **and** `https://<domain>/api/ready` answered 200 |
| `1` | failed — nothing half-installed is left running |
| `2` | **installed but not publicly served.** The service runs on `127.0.0.1:8080`; HTTPS failed (DNS, ports, ACME). Never reported as success. |

Until a release tag exists, pass `--version` or set `VEYLIX_REPO` to point at a
checkout's own release. Everything else is automatic:

- downloads `veylixd-linux-<arch>` and `vpn-platformctl-linux-<arch>`,
- **verifies `SHA256SUMS`** and deletes a mismatched artifact,
- creates the `veylix` system user and both directories with correct ownership,
- generates `enc.key` and `session.key` **only if absent**, printing fingerprints,
- renders `/etc/veylix/veylix.env` (0600 root:root) with the keys inlined,
- installs and starts `veylixd`, then waits for `/api/ready`,
- renders the Caddyfile, **validates it before reloading**, then proves HTTPS.

**Re-running on an installed host is refused.** That is the data-loss guard: the
branch checks for `$VEYLIX_DATA_DIR/veylix.db`, not for the binary. See
[`INSTALLATION_DESIGN.md`](INSTALLATION_DESIGN.md) §3–§4.

---

## A. Run on this machine (development)

```powershell
cd "h:\my apps\Veylix-2"
.\dev.ps1 -Restart -Preflight  # backend :8080 + Vite :5173, auth gate (27 checks)
# open http://localhost:5173/login
.\dev.ps1 -Stop
```

`.\dev.ps1` loads `.veylix-secrets\dev-env.ps1` first, because the backend refuses to
start without its keys. It then proves the login: anonymous calls are refused
**401** and a seeded account really signs in. See [`AUTH.md`](AUTH.md) §0 and §5.
[`LIVE_PREVIEW.md`](LIVE_PREVIEW.md) covers LAN access, logs and troubleshooting.

**There is no development bypass.** Authentication is a password or Google sign-in,
server-side (D-039) — see `AUTH.md` §2.

To run the **packaged** form locally:

```powershell
.\dev.ps1 -Package            # builds the SPA, embeds it, builds bin/veylixd.exe, asserts it
$env:VEYLIX_ADDR='127.0.0.1:8080'
.\backend\bin\veylixd.exe     # http://localhost:8080/dashboard
```

---

## B. Install on a Linux server (manual, today's supported path)

### B.1 Prerequisites

| Need | Why |
|---|---|
| Linux x86_64 or arm64 | the Go binary |
| A 3X-UI panel (3.10.2 verified) with an **API token** | every VPN lives there |
| A domain (recommended) + Caddy | HTTPS — the binary does **not** terminate TLS |
| ~500 MB RAM / 1 vCPU | the product target |

The binary is self-contained: **no Node, no Redis, no Postgres, no Docker at runtime.**

### B.2 Build the artifact (on your workstation)

```powershell
.\dev.ps1 -Package            # embeds the UI and asserts the result
cd backend
$env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
go build -trimpath -ldflags "-s -w" -o ../dist/veylixd-linux-amd64 ./cmd/veylixd
Remove-Item Env:GOOS, Env:GOARCH
```


### B.4 Environment file

`/etc/veylix/veylix.env`:

```ini
VEYLIX_ADDR=127.0.0.1:8080
VEYLIX_DATA_DIR=/var/lib/veylix
VEYLIX_ENCRYPTION_KEY=<contents of enc.key>
VEYLIX_SESSION_KEY=<contents of session.key>
VEYLIX_SYNC_INTERVAL=5m
# Production default — do not change:
VEYLIX_INSECURE_COOKIES=0
```

The full documented template is [`../deploy/veylix.env.example`](../deploy/veylix.env.example).

> `VEYLIX_DEV_AUTH` no longer exists: the development auth bypass was **removed**
> from the source in D-039, so there is no flag to turn off. Authentication is a
> password or a verified Google sign-in, always.

`VEYLIX_ADDR` must stay on loopback: the binary is designed to sit behind a reverse proxy.

### B.5 systemd unit (least privilege)

`/etc/systemd/system/veylixd.service`:

```ini
[Unit]
Description=Veylix VPN platform
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=veylix
Group=veylix
EnvironmentFile=/etc/veylix/veylix.env
ExecStart=/usr/local/bin/veylixd
Restart=on-failure
RestartSec=3

# Hardening — the binary needs only its data directory.
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/veylix
CapabilityBoundingSet=
AmbientCapabilities=

[Install]
WantedBy=multi-user.target
```

```bash
sudo chown -R veylix:veylix /var/lib/veylix
sudo systemctl daemon-reload
sudo systemctl enable --now veylixd
systemctl status veylixd --no-pager
curl -s http://127.0.0.1:8080/api/health   # {"service":"veylix","status":"ok",...}
curl -s http://127.0.0.1:8080/api/ready    # must contain "ui":"embedded:..."
```

If `ui` reports **`not_embedded`**, the binary was built without `.\dev.ps1 -Package` — the
API works but there is no interface. Rebuild.

### B.6 Reverse proxy with automatic HTTPS (Caddy)

`/etc/caddy/Caddyfile`:

```
vpn.example.com {
    encode zstd gzip
    reverse_proxy 127.0.0.1:8080 {
        header_up X-Forwarded-Proto {scheme}
    }
}

---

## C. Backup and restore

The database is SQLite in WAL mode. Copy it **with** its sidecar files while the service is
stopped:

```bash
sudo systemctl stop veylixd
sudo mkdir -p /root/veylix-backup-$(date +%F)
sudo cp /var/lib/veylix/veylix.db{,-wal,-shm} /root/veylix-backup-$(date +%F)/
sudo systemctl start veylixd
```

Also back up `/etc/veylix/*.key` — without them the sealed panel tokens are useless.
Restore = stop the service, put the three files back, start.

> `vpn-platformctl backup` / `vpn-platformctl restore <dir>` are shipped (D-079) and
> delegate to this same script, so the WAL handling above is the only implementation.
> `vpn-platformctl rollback` now takes its own backup before swapping the binary and
> defaults `--restore` to the directory it just wrote (D-093). The manual procedure
> above remains the fallback when the service is in an unknown state.

---

## D. Upgrade

```bash
sudo systemctl stop veylixd
sudo mkdir -p /root/veylix-pre-upgrade-$(date +%F)
sudo cp /var/lib/veylix/veylix.db{,-wal,-shm} /root/veylix-pre-upgrade-$(date +%F)/
sudo install -o root -g root -m 0755 veylixd-linux-amd64 /usr/local/bin/veylixd
sudo systemctl start veylixd
curl -s http://127.0.0.1:8080/api/ready     # schema_version must be >= the previous value
```

Migrations are embedded and applied at startup; the schema only moves forward. Rolling back
across a migration requires restoring the backup taken above.

---

## E. Uninstall

```bash
sudo systemctl disable --now veylixd
sudo rm /etc/systemd/system/veylixd.service
sudo rm /usr/local/bin/veylixd
# keep /var/lib/veylix and /etc/veylix unless you intend to destroy the data
```

Removing Veylix **never** deletes clients from 3X-UI — that is a product invariant (D-027.3).

---

## F. Automation status

| Item | State |
|---|---|
| `deploy/Caddyfile` | ✅ shipped — copy to `/etc/caddy/Caddyfile`, replace the domain |
| `deploy/veylixd.service` | ✅ shipped — hardened unit (no capabilities, `ProtectSystem=strict`, `MemoryMax=384M`) |
| `deploy/veylix.env.example` | ✅ shipped — install as `/etc/veylix/veylix.env`, mode `0600` |
| `deploy/veylix-backup.sh` | ✅ shipped — `backup` / `restore` / `verify`, stops the service and verifies the copy |
| **`veylix-deploy.tar.gz` in every release** | ✅ **shipped (D-093)** — `release.yml` bundles `deploy/` and asserts the unit, Caddyfile and backup script are inside, so a host installed from a release has the templates. `install.sh` resolves: checkout → bundle → `--script-base`. Only `deploy/*` is extracted. |
| `--repo <owner/name>` | shipped (D-120): the shipped default is the real repository, so a one-line install needs no flags. A malformed slug is still refused with the exact correction; `VEYLIX_REPO` overrides it |
| `--script-base <url>` | ✅ **shipped** — legacy per-file fallback for the templates |
| `rollback` takes its own backup | ✅ **shipped (D-093, F-061)** — refuses without the script, and `--restore` defaults to the backup it just took; `--skip-backup` is the only way past, and it warns |
| **Lifecycle in the UI** | ⛔ **deliberately not built (D-093)** — install/upgrade/rollback/restore stay on the command line: a screen cannot repair the service it depends on |
| CI (`.github/workflows/ci.yml`) | ✅ shipped — build, vet, gofmt, tests, **`go test -race`**, typecheck, lint, UI-truth gate, self-tests, docs verify, build |
| Release (`.github/workflows/release.yml`) | ✅ shipped — linux amd64 + arm64, **boots the amd64 artifact** and asserts `/api/ready`, `/dashboard` 200 and `/assets/nope.js` 404, publishes `SHA256SUMS` **including both binaries** |
| **One-line installer** | ✅ **BUILT** — `install.sh`. Refuses an existing database, verifies the release checksum, never overwrites a key, and claims HTTPS only after a real 200. **Not yet run on a clean Linux VM — see below.** |
| **`vpn-platformctl` CLI** | ✅ **BUILT** — `backend/cmd/vpn-platformctl`: `status` `doctor` `update` `rollback` `domain-set` `backup` `restore` `repair` `uninstall`, covered by `go test ./cmd/vpn-platformctl` |
| `vpn-platformctl` in a release | ✅ **ships together** — `vpn-platformctl-linux-<arch>` is built and checksummed in `release.yml` |
| Installer guard self-test | ✅ **runs here** — `scripts/install.selftest.sh`: **PASS=59 · FAIL=0 · SKIP=1** (branch refusals, dry-run, checksum, `out()`, repo-slug validation, template resolution incl. a bundle member outside `deploy/`). The 1 SKIP needs Linux for the full phase walk |
| `common.sh` self-test | ✅ **runs here** — `scripts/lib/common.selftest.sh`: 31 checks. 1 check **SKIPPED** (POSIX modes on Windows) |
| **Stage 5b isolated rehearsal** | ✅ **PASS=12 · FAIL=0 · BLOCKED=1 · ESTIMATED=1** — `scripts/stage5b-rehearsal.ps1` against the real binary in a throwaway `$env:TEMP` tree. See `QA_REPORT.md` §3.2 |
| **Clean-VM install test** | ❌ **NOT PERFORMED** — no Linux VM or public domain on this machine |
| **HTTPS issuance** | ❌ **NOT PERFORMED** — needs a real domain and ports 80/443 |
| **Backup/restore rehearsal** | ✅ **PASSED in isolation** — the backup was restored into a *second* path and the real binary **served it** (schema 21). Still ❌ **not rehearsed on a clean VM** |
| **1 vCPU / 500 MB benchmark** | 🟡 **ESTIMATED only** — binary 20.4 MB measured; idle RSS ~12.8 MB on Windows, **not** a Linux VM measurement |

Everything marked ✅ is a real file in `deploy/`, `.github/workflows/`, `scripts/` or
`backend/cmd/`, and is checked by `npm run workflows:check` or `go test`. Everything
marked ❌ is a genuine gap: ask before relying on it.

**Honest summary of the installer:** it exists, and its data-loss guards are
tested — but **no clean Linux VM has run it**. Bible §18 tests 30–34, 37 and 39
are therefore *built, not verified*. The design and reasoning are in
[`INSTALLATION_DESIGN.md`](INSTALLATION_DESIGN.md).

### F.1 Commands that now exist

```bash
# first install (once per host)
sudo ./install.sh --domain vpn.example.com --dry-run    # always first
sudo ./install.sh --domain vpn.example.com

# everything afterwards
sudo vpn-platformctl status         # what is this host actually doing
sudo vpn-platformctl doctor         # why is it not healthy
sudo vpn-platformctl update         # upgrade: backup -> verify -> swap -> ready
sudo vpn-platformctl rollback       # back to the previous binary
sudo vpn-platformctl domain-set vpn.newdomain.com
sudo vpn-platformctl backup
sudo vpn-platformctl restore <dir>
sudo vpn-platformctl repair         # re-render + reload; never touches data
sudo vpn-platformctl uninstall [--purge-data]
```

Re-running `install.sh` on an installed host is **refused**. That refusal is the
data-loss guard, and it is tested by `scripts/install.selftest.sh`.

Companion runbooks: [`UPGRADE.md`](UPGRADE.md) ·
[`BACKUP_RESTORE.md`](BACKUP_RESTORE.md) ·
[`DOMAIN_CHANGE.md`](DOMAIN_CHANGE.md) ·
[`TROUBLESHOOTING.md`](TROUBLESHOOTING.md).

```

```bash
sudo systemctl reload caddy
```

Caddy obtains the certificate automatically. Because the UI and the API share one origin, the
backend's CSRF origin check passes without any CORS configuration.

### B.7 First run — create the Super Admin

There is **no default password** (by design).

```bash
curl -s https://vpn.example.com/api/setup/status
# {"bootstrap_required":true}
```

Open `https://vpn.example.com/setup` and complete the form (12+ characters, no repeats or
sequences). Then add your 3X-UI panel under **Servers** with its URL and API token — the token
is sealed immediately and never returned again.

> `CGO_ENABLED=0` gives a static binary (the SQLite driver is pure Go). It also means
> `go test -race` cannot run on this machine — run the race detector in CI.

### B.3 Prepare the server

```bash
sudo useradd --system --home /var/lib/veylix --shell /usr/sbin/nologin veylix
sudo mkdir -p /var/lib/veylix /etc/veylix
sudo install -o root -g root -m 0755 veylixd-linux-amd64 /usr/local/bin/veylixd
```

Generate the two 32-byte keys **once**, root-readable only:

```bash
sudo sh -c 'head -c 32 /dev/urandom | base64 > /etc/veylix/enc.key'
sudo sh -c 'head -c 32 /dev/urandom | base64 > /etc/veylix/session.key'
sudo chmod 600 /etc/veylix/*.key
```

> Losing `enc.key` makes every sealed panel token unrecoverable (the token must be re-entered
> in the UI). Back both keys up outside the server.
