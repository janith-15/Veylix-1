# VEYLIX — INSTALL

**Honest scope.** As of 2026-10-24 the server ships as **one binary that serves the UI and the
API on one port** (D-029). There is **no one-line installer yet** — that is Milestone 7. This
document describes the manual install that is actually possible today, and names exactly which
steps remain manual.

> **What an installation is** (Bible §1.0/§1.1, D-079…D-083): **one owner, one server, one
> independent Veylix instance.** The same repository installs on as many servers as there are
> owners, and the resulting installations share **nothing** — not a database, not a user, not a key,
> not a panel, not a version. Installing Veylix never imports data from anywhere, and this manual
> procedure never restores a backup.
>
> **3X-UI is external and owner-supplied** (D-080): you bring your own panel(s) and API token.
> Veylix does **not** install, bundle or provision 3X-UI for you.

| Path | When to use | Time | State |
|---|---|---|---|
| **A. Windows / this machine** | development and owner review | 2 min | ✅ works |
| **B. Linux server (manual)** | a real deployment until the installer lands | 15 min | ✅ documented, ⚠️ **never rehearsed on a clean VM** |
| **One-line installer (future)** | the intended experience | — | ❌ **does not exist** (§F, `DISCREPANCY_REGISTER.md` D15) |


---

## A. Run on this machine (development)

```powershell
cd "h:\my apps\Veylix-1"
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

> An automated `vpn-platformctl backup/restore` is planned in Milestone 7. Until then this is a
> manual, documented procedure — not a tested tool.

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
| CI (`.github/workflows/ci.yml`) | ✅ shipped — build, vet, gofmt, tests, **`go test -race`**, typecheck, lint, UI-truth gate, self-tests, docs verify, build |
| Release (`.github/workflows/release.yml`) | ✅ shipped — linux amd64 + arm64, **boots the amd64 artifact** and asserts `/api/ready`, `/dashboard` 200 and `/assets/nope.js` 404, publishes `SHA256SUMS` |
| One-line installer | ❌ not built — the steps above are manual |
| **Installation-aware installer** (detects an existing install, offers upgrade, never overwrites) | ❌ **not built** — no installer exists to carry the check (`DISCREPANCY_REGISTER.md` D15) |
| **Installation identity** (created at first startup, preserved across upgrades) | ❌ **not implemented** — no `installation_id` in any migration (D17) |
| **Restore provenance** (validate backup identity before restoring) | ❌ **not implemented** — `veylix-backup.sh restore` verifies integrity only (D17) |
| `vpn-platformctl` (status/logs/domain-set/upgrade) | ❌ not built — use systemd, `cp`, and the docs (D18) |
| **Clean-VM install test** | ❌ **NOT PERFORMED** — nothing below is proven on a fresh Linux box |
| **Two-install isolation test** (A and B share nothing; upgrade A, B untouched) | ❌ **NOT PERFORMED** — Bible §18 tests 40–48 |
| **Backup/restore rehearsal** | ❌ **NOT PERFORMED** (see `BACKUP_RESTORE.md` §6) |
| **1 vCPU / 500 MB benchmark** | ❌ **NOT RUN** — the target is a requirement, not a measurement |

Everything marked ✅ is a real file in `deploy/` or `.github/workflows/` and is
checked by `npm run workflows:check`. Everything marked ❌ is a genuine gap: ask
before relying on it.

> **What today's manual install does and does not guarantee.** It produces a **clean, independent**
> installation: an empty database built from embedded migrations, a one-shot Super Admin setup,
> and keys you generated on this server. What it does **not** do — because nothing does yet — is
> *prove* which installation a database or backup belongs to, refuse to overwrite an existing one,
> or give you a one-command upgrade. Those are Milestone 7/8 items; do not describe them as
> available.

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

> **These keys belong to THIS installation** (D-079). Generate them on this server, for this server:
> - **Do not copy them from another Veylix installation.** Two installations must hold different keys,
>   or the token sealing stops proving anything about which installation a ciphertext came from.
> - **Never regenerate them over existing files.** A new `enc.key` orphans every sealed panel token
>   (each must be re-entered in the UI); a new `session.key` only signs everyone out.
> - An installer must generate a key **only when it is absent** (Bible §14.0) — that rule does not
>   exist yet because there is no installer (`DISCREPANCY_REGISTER.md` D15).

> Losing `enc.key` makes every sealed panel token unrecoverable (the token must be re-entered
> in the UI). Back both keys up outside the server.
