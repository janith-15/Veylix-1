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

**Pick the form that matches your privileges.** This matters: two of the four
obvious invocations cannot work, for reasons that have nothing to do with
Veylix.

```bash
# As root already (the usual case: `sudo -i` first)
bash <(curl -Ls https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh) --domain vpn.example.com
```

```bash
# As a normal user with sudo: download, then run the file.
curl -Ls https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh -o /tmp/veylix.sh
sudo bash /tmp/veylix.sh --domain vpn.example.com
```

| Form | Works? | Why |
|---|---|---|
| `sudo bash install.sh` | **yes** | a real file on disk |
| `sudo -i` then `bash <(curl …)` | **yes** | process substitution runs in root's own shell |
| `curl … -o /tmp/x.sh` then `sudo bash /tmp/x.sh` | **yes** | a real file on disk |
| `bash <(curl …)` as a **non-root** user | starts, then refuses | `need_root` — it must run as root |
| `sudo bash <(curl …)` | **NO** | `sudo` is a separate process and does not inherit your shell's `/dev/fd/N`, so `bash` cannot open the script: `bash: /dev/fd/63: No such file or directory` |
| `curl … \| bash` | **yes** (since D-128) | `BASH_SOURCE[0]` is unset when the script arrives on stdin; the installer now tolerates that |
| `curl … \| sudo bash` | **NO** | inherits the `sudo` problem above |

If you are already root, the first form is the shortest. If you are not, the
second is the one to copy.

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

**After it finishes, type `sudo vey`.** That is the control menu: start, stop,
logs, backup, update, domain, ports, uninstall. `vpn-platformctl` is the same
binary under its long name (D-126).



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
| `2` | **installed but not publicly served.** The service runs on `127.0.0.1:<app-port>`; HTTPS failed (DNS, ports, ACME). Never reported as success. |

Until a release tag exists, pass `--version` or set `VEYLIX_REPO` to point at a
checkout's own release. Everything else is automatic:

- downloads `veylixd-linux-<arch>` and `vpn-platformctl-linux-<arch>`,
- **verifies `SHA256SUMS`** and deletes a mismatched artifact,
- creates the `veylix` system user and both directories with correct ownership,
- generates `enc.key` and `session.key` **only if absent**, printing fingerprints,
- renders `/etc/veylix/veylix.env` (0600 root:root) with the keys inlined,
- installs and starts `veylixd`, then waits for `/api/ready`,
- renders the Caddyfile, **validates it before reloading**, then proves HTTPS.

**Ports are configurable** with `--http-port`, `--https-port` and `--app-port`
(defaults `80`, `443`, `8080`). You need them when the host already runs 3X-UI —
see [§B.8](#b8-ports-and-running-beside-3x-ui-on-one-vps), including the
certificate caveat for non-standard ports.

**Re-running on an installed host is refused.** That is the data-loss guard: the
branch checks for `$VEYLIX_DATA_DIR/veylix.db`, not for the binary. See
[`INSTALLATION_DESIGN.md`](INSTALLATION_DESIGN.md) §3–§4.

### B.8 Ports, and running beside 3X-UI on one VPS

VEYLIX uses **three** ports, and they are independent:

| Flag | Default | What it is | Public? |
|---|---|---|---|
| `--https-port` | `443` | the port a **customer types** — Caddy's TLS listener | yes |
| `--http-port` | `80` | Caddy's plain-HTTP listener: the ACME challenge and the redirect to HTTPS | yes |
| `--app-port` | `8080` | the loopback port `veylixd` binds; Caddy proxies to it | **no** — `127.0.0.1` only |

The defaults are the conventional ones, so an ordinary install needs no flags.

**Why this matters here.** The usual deployment target is a VPS that *already
runs 3X-UI*, and 3X-UI's Xray inbound normally holds `80` and `443`. Veylix's
installer refuses to continue when the port it needs is taken, so without these
flags the two cannot coexist on one host.

`--app-port` is read back from `veylix.env` (`VEYLIX_ADDR`) by
`vpn-platformctl`, so `status`, `doctor` and every other probe follow the port
you installed with — they cannot drift to a stale `8080`.

#### Three ways to share a host, worst to best

**1. Sharing a port between two servers — do not.** You cannot put Caddy and
Xray on the same port without Caddy ALPN-multiplexing Xray's traffic, which means
TLS termination inside the Xray inbound and a certificate Caddy no longer owns.
It is a bespoke configuration neither tool tests. Skip it.

**2. Give Veylix its own ports (recommended, one command).**

```bash
sudo ./install.sh --domain vpn.example.com --http-port 8080 --https-port 8443
```

Veylix then lives at **`https://vpn.example.com:8443`** — note the port. Customers
and admins must include it; a bare `https://vpn.example.com` will reach whatever
is on `443`.

> **Certificate caveat — read this before choosing the ports.** Caddy's two
> pre-authorized ACME challenge types need ports **80** (HTTP-01) and **443**
> (TLS-ALPN-01). If 3X-UI holds both, *neither is available*, and automatic
> issuance for your domain will usually fail.
>
> `install.sh` reports that outcome honestly as **exit code 2** ("installed but
> not publicly served") and prints `vpn-platformctl repair`. It never claims
> HTTPS works when it does not.
>
> To get a certificate you need at least one of:
> - a free `80` for HTTP-01 — pass `--http-port 80` and move the 3X-UI inbound
>   off `80`; or
> - a free `443` for TLS-ALPN-01 — pass `--https-port 443`;
> - or a **DNS-01** challenge, which needs no inbound port at all but requires a
>   DNS provider plugin in Caddy. That is not built into this installer and is a
### B.9 `bash <(curl ...)`: what it asks, and why

The one-line install runs straight off the URL:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh) --domain vpn.example.com
```

On the way in it asks **one** question — the ports — unless you already answered
it with a flag:

```
Ports. The defaults suit a server that is not already using 80 and 443.

  1) Standard   - HTTPS on 443, HTTP on 80            (https://<domain>)
  2) 3X-UI host - HTTPS on 8443, HTTP on 8080         (https://<domain>:8443)
                  use this when 3X-UI is already on this VPS
  3) Custom     - type your own

Choose [1]:
```

**Press Enter for the normal ports.** Choose `2` if 3X-UI is already on the box,
or `3` to type each port (Enter on any of them takes that port's default).

It never prompts when it cannot get an answer:

| Situation | Behaviour |
|---|---|
| you passed `--https-port` / `--http-port` / `--app-port` | no question — you already answered |
| `--dry-run` | no question; the plan is based on the defaults |
| no terminal, or `VEYLIX_NONINTERACTIVE=1` | no question; defaults, and it says so |

**Why this needed fixing rather than just adding.** The obvious guard for a
prompt is `[ -t 0 ]`, and it is **wrong for this invocation**: under
`bash <(curl ...)` stdin is the pipe carrying the *script*, so `-t 0` is false
even at an interactive terminal. The old installer therefore never offered the
Super Admin prompt or the control menu on a real terminal, and any `read` would
have swallowed a line of `install.sh` itself as the "answer".

Every prompt now reads from **`/dev/tty`** instead (`has_tty`, `read_tty`,
`read_tty_secret` in `scripts/lib/common.sh`), which is the operator's terminal
no matter what stdin is wired to.

**Automation.** Set `VEYLIX_NONINTERACTIVE=1` to force every prompt to be
skipped. This is not a nicety: a controlling terminal is inherited by child
processes, so a script that pipes the installer still hands it a working
`/dev/tty` and would block on a question nobody is watching.
`scripts/install.selftest.sh` sets it for exactly that reason.


>   deliberate change, not a flag.
>
> **This combination has not been verified against a live Let's Encrypt
> issuance.** Treat "custom HTTPS port + certificate" as untested until you have
> seen `/api/ready` answer 200 on that port on your own host.

**3. Move the 3X-UI inbound and keep Veylix on 443 (best if you can).** Veylix
keeps the clean `https://vpn.example.com` URL and a working certificate; 3X-UI
keeps serving VPN clients on whatever inbound port you move it to. Existing
clients must be re-issued with the new port, which is the real cost.

#### Checking before you commit

```bash
# what is holding 80 and 443 today?
sudo ss -lntp '( sport = :80 or sport = :443 )'

# dry run with your chosen ports: prints every phase, changes nothing
sudo ./install.sh --domain vpn.example.com \
  --http-port 8080 --https-port 8443 --app-port 9090 --dry-run
```

The dry run prints the rendered ports and the URL it would report. Refusals are
explicit and happen **before** anything is written:

| Passed | Result |
|---|---|
| `--https-port 0`, `70000`, `abc`, `08` | refused — not a usable port |
| `--http-port 8443 --https-port 8443` | refused — the two listeners must differ |
| `--app-port 8443` (same as HTTPS) | refused — Caddy cannot front its own port |
| `--app-port 80` (same as HTTP) | refused — Veylix and Caddy would fight for it |

---


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

> **Changing ports later.** Use the CLI, not hand-editing — it validates the
> Caddyfile, backs it up, reloads and then verifies the service answers:
>
> ```bash
> vpn-platformctl ports                                  # show what is in use
> vpn-platformctl ports set --https-port 8443 --http-port 8080
> # or from the control menu: option 16, "Change ports"
> ```
>
> On a non-standard port the site address needs an **explicit `https://`** plus a
> global port block. `install.sh` and `ports set` both write this for you, but if
> you are doing it by hand:
>
> ```
> {
>     http_port 8080
>     https_port 8443
> }
>
> https://vpn.example.com:8443 {
>     encode zstd gzip
>     reverse_proxy 127.0.0.1:9090 {
>         header_up X-Forwarded-Proto {scheme}
>     }
> }
> ```
>
> Without the scheme Caddy reads `vpn.example.com:8443` as an HTTP site and never
> obtains a certificate.


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
| **`vpn-platformctl` CLI** | ✅ **BUILT** — `backend/cmd/vpn-platformctl`: `status` `doctor` `update` `rollback` `domain-set` `ports` `backup` `restore` `repair` `uninstall`, covered by `go test ./cmd/vpn-platformctl` |
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

### B.10 A certificate with no free port: Cloudflare DNS-01

Use this when **both 80 and 443 are taken** — typically by 3X-UI — and you cannot
move either. DNS-01 proves domain ownership by writing a TXT record, so it needs
**no inbound port at all**.

> **One prerequisite, and it is not optional.** The standard Caddy package
> contains only its built-in modules; the Cloudflare plugin is *not* in it. Caddy's
> own install page says: *"Our official packages come only with the standard
> modules."* You need a Caddy binary that includes `dns.providers.cloudflare`.

**Step 1 — create a scoped API token in Cloudflare.**

Cloudflare dashboard → *My Profile* → *API Tokens* → *Create Token* → *Edit zone
DNS*. Do **not** use a Global API Key: that is account-wide.

- Permissions: `Zone` → `DNS` → `Edit`
- Zone Resources: **Include → Specific zone →** your domain only
- Copy the token. It is shown once.

**Step 2 — get a Caddy that has the plugin.**

```bash
# Check what your binary contains. No output here = the plugin is missing.
caddy list-modules | grep dns.providers.cloudflare
```

If it is missing, either:

```bash
# (a) download a build that includes it - no Go toolchain needed
#     https://caddyserver.com/download?package=github.com/caddy-dns/cloudflare
#     choose Linux amd64/arm64, then:
sudo systemctl stop caddy
sudo cp caddy /usr/bin/caddy && sudo chmod +x /usr/bin/caddy
sudo systemctl start caddy

# (b) or build it here
xcaddy build --with github.com/caddy-dns/cloudflare
```

**Step 3 — set the domain and issue the certificate.**

```bash
sudo vpn-platformctl domain-set vpn.example.com \
  --dns-provider cloudflare \
  --cf-token '<the token from step 1>'
```

In the control menu this is **option 15, "Change domain"**.

What it does, in order, and what it refuses to do:

| Step | Behaviour |
|---|---|
| provider check | unknown name → refused, listing the supported ones |
| token check | wrong shape → refused **before** anything is written |
| **plugin check** | `caddy list-modules`; missing plugin → **refused**, with both install routes, nothing changed |
| Caddyfile | `tls { dns cloudflare … }` written inside marked `# BEGIN/END veylix-dns` lines, replacing any previous block |
| validate | `caddy validate` **before** reload; failure restores the previous Caddyfile |
| reload | `systemctl reload caddy` |
| verify | `GET https://<domain>/api/ready` must answer 200 |
| persist | token → `/etc/veylix/caddy-dns-cloudflare.token` (0600); `VEYLIX_PUBLIC_BASE_URL` + `VEYLIX_DNS_PROVIDER` → `veylix.env` |

**Where the token lives.** Its own 0600 file, root only — **not** `veylix.env`,
because that file is loaded into the daemon's environment and is readable with
`systemctl show`. It is never printed, never logged, and never echoed back.

**If it fails**, the message names the cause and the Caddyfile is restored. The
common ones:

| Message | Cause |
|---|---|
| `built WITHOUT the dns.providers.cloudflare plugin` | stock Caddy — do step 2 |
| `cannot list the caddy modules` | caddy not running/installed; this is *not* the same as a missing plugin |
| `does not look like a Cloudflare API token` | truncated paste, or a Global API Key instead of a token |
| `domain changed but HTTPS is NOT active` | the token works but the zone/permission is wrong — Cloudflare will have a `DNS edit` error in its audit log |

**Not verified.** A real issuance through Cloudflare has not been run on this
project. The plugin gate, the token handling and the Caddyfile rendering are
unit-tested; the ACME round trip is not. Check with:

```bash
vpn-platformctl cert          # what the domain actually serves
vpn-platformctl logs -n 50    # caddy logs the ACME failure verbatim
```


```bash
# first install (once per host)
sudo ./install.sh --domain vpn.example.com --dry-run    # always first
sudo ./install.sh --domain vpn.example.com

# the control menu - type it with no arguments
sudo vpn-platformctl
```

`vpn-platformctl` with **no arguments** opens the control menu: nineteen numbered
commands plus `0) Exit`. It is the same menu the installer prints when it
finishes, and every item runs the identical subcommand listed below — the menu
delegates to the CLI rather than reimplementing it.

```
==============================================================
  VEYLIX CONTROL MENU
==============================================================

  SERVICE
   1) Start                   start veylixd
   2) Stop                    stop veylixd
   3) Restart                 restart veylixd
   4) Status                  service, readiness and database
   5) Logs                    show the end of the service log
   6) Enable autostart        start on boot
   7) Disable autostart       do not start on boot

  MAINTAIN
   8) Update                  upgrade to the latest release
   9) Versions                list published releases
  10) Downgrade               install an older release
  11) Backup                  take a backup now
  12) Restore                 restore from a backup
  13) Doctor                  diagnose this host
  14) Repair                  re-render config and reload

  DOMAIN & ACCESS
  15) Change domain           point Veylix at another domain
  16) Certificate             check the TLS certificate
  17) Reset admin             change the Super Admin password

  DANGER
  18) Uninstall               remove the service, keep the data
  19) Uninstall + purge       remove EVERYTHING including data

   0) Exit
```

**When stdin is not a terminal the menu prints the usage text instead of
prompting.** That matters: `vpn-platformctl | grep status` and a CI run must not
block forever on a prompt nobody can answer.

The same commands, for scripts:

```bash
sudo vpn-platformctl start|stop|restart   # each verifies the new state
sudo vpn-platformctl status               # what is this host actually doing
sudo vpn-platformctl logs -f              # journald, follows
sudo vpn-platformctl enable|disable       # autostart on boot
sudo vpn-platformctl doctor               # why is it not healthy
sudo vpn-platformctl update               # upgrade: backup -> verify -> swap -> ready
sudo vpn-platformctl versions             # which releases exist
sudo vpn-platformctl downgrade v1.0.0 --restore-backup <dir>
sudo vpn-platformctl rollback             # back to the previous binary
sudo vpn-platformctl domain-set vpn.newdomain.com
sudo vpn-platformctl cert                 # what certificate is actually served
sudo vpn-platformctl reset-admin          # Super Admin password recovery
sudo vpn-platformctl backup
sudo vpn-platformctl restore <dir>
sudo vpn-platformctl repair               # re-render + reload; never touches data
sudo vpn-platformctl uninstall [--purge-data]
```

**`start`, `stop`, `restart`, `enable` and `disable` verify their own result.**
`systemctl` returns 0 as soon as a unit is queued, so each one reads the state
back and fails loudly if it did not change; `restart` additionally requires
`/api/ready` to answer. A command that reports success it did not achieve is the
defect this product keeps fixing, so it is not repeated here.

**`reset-admin` is the recovery path for the one account that has no other.**
It applies the same password policy as the UI, hashes with the same argon2id
parameters, and revokes every session:

```bash
sudo vpn-platformctl reset-admin
New password:
Repeat it:
Super Admin password changed.
  account:  owner@example.com (Owner)
  sessions: 3 revoked
```

A lost Super Admin password used to be a permanent lockout — there was no route
back in short of a reinstall, which destroys the platform.

**`downgrade` needs `--restore-backup`.** Migrations are forward-only, so an
older binary cannot read a database a newer one has migrated; the matching
backup is what makes the downgrade safe. `downgrade` refuses without one rather
than bricking the install.

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

---

## G. Troubleshooting

### G.1 The one-liner fails before it installs anything

Three different messages, three different causes. Match the one you saw.

**a. `bash: /dev/fd/63: No such file or directory`**

You used `sudo bash <(curl …)`. `sudo` runs a **separate process** and does not
inherit your shell's `/dev/fd/N`, so the inner `bash` cannot open the script.
Nothing is wrong with the installer. Use one of these instead:

```bash
# already root
bash <(curl -Ls https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh) --domain vpn.example.com

# normal user with sudo
curl -Ls https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh -o /tmp/veylix.sh
sudo bash /tmp/veylix.sh --domain vpn.example.com
```

**b. `/dev/fd/scripts/lib/common.sh: No such file or directory`**

You ran an older `install.sh` (before **D-127**). `bash <(curl …)` gives bash a
pipe, so `BASH_SOURCE[0]` was `/dev/fd/63` and every sibling path was built from
`/dev/fd`. The current script bootstraps itself instead. Confirm which version
your URL serves:

```bash
curl -fsSL https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh | wc -c
# 49,000+ means D-127 and D-128 are included; ~31,800 is the old one
```

**c. `bash: line 47: BASH_SOURCE[0]: unbound variable`**

You used `curl … | bash` against a version before **D-128**. When the script
arrives on stdin, `BASH_SOURCE[0]` is unset and `set -u` aborted the installer.
Fixed; the same length check above tells you whether you have it.

**Still failing?** The install does not need the network to fetch `install.sh`
itself — clone and run the file:

```bash
git clone https://github.com/janith-15/Veylix-1 /tmp/veylix
cd /tmp/veylix && sudo ./install.sh --domain vpn.example.com
```

### G.2 `install.sh: ERROR: flock is not installed`

`flock` ships with `util-linux`. Install it and re-run:

```bash
sudo apt-get install -y util-linux    # Debian / Ubuntu
sudo dnf install -y util-linux        # RHEL / Fedora
```

The installer refuses rather than run without it: two concurrent installs on one
host must never both proceed.

### G.3 `install.sh: ERROR: cannot create lock directory /run`

You are not root, or `/run` is not writable. The installer creates
`/run/veylix-install.lock`; use `sudo`. If `/run` is genuinely missing (a
container without systemd), point the lock elsewhere:

```bash
sudo VEYLIX_LOCK_PATH=/tmp/veylix-install.lock ./install.sh --domain vpn.example.com
```

### G.4 `port 80 is already in use`

Expected when 3X-UI owns 80 and 443. Give Veylix its own ports (D-123):

```bash
sudo ./install.sh --domain vpn.example.com \
  --http-port 8090 --https-port 9443 --app-port 10080
```

Find genuinely free ports first:

```bash
sudo ss -lntp | awk '{print $4}' | grep -oE '[0-9]+$' | sort -un
```

### G.5 `no deployment templates found` — and the install refuses

The installer needs `deploy/Caddyfile`, `deploy/veylixd.service` and
`deploy/veylix-backup.sh`. It looks in four places: a checkout beside the
script, the release `veylix-deploy.tar.gz`, `--script-base`, then the raw
repository. If all four miss, it stops rather than install a host with no HTTPS
site and no backup. Copy them by hand (see B.3), or pass:

```bash
sudo ./install.sh --domain vpn.example.com \
  --script-base https://raw.githubusercontent.com/janith-15/Veylix-1/main/deploy
```

