# PUSH GUIDE — get the working install system onto `main`

**Why this file exists.** The `install.sh` on `main` today is the version from
before D-122. It cannot install Veylix on a host where 3X-UI already holds
80/443 — it hardcodes `for p in 80 443` and calls `die`. Everything built since
(control menu, port flags, port picker, Cloudflare DNS-01, the `vey` launcher)
is only in the working tree, because **this workspace has no Git repository**, so
nothing can be pushed from here.

**The one-line install will not work until these files are on `main`.**

---

## 1. What is actually different

GitHub currently has these 7 files in `backend/cmd/vpn-platformctl/`:

```
lifecycle.go  main.go  main_test.go  status.go  support.go  update.go  versions.go
```

The working tree has **25**. Eighteen are new — the whole control menu, the port
system, the DNS-01 support and their tests.

---

## 2. Files to add (NEW — they do not exist on GitHub)

### Backend — CLI package (`backend/cmd/vpn-platformctl/`)

| File | What it is |
|---|---|
| `menu.go` | the 20-item control menu, as data |
| `menu_test.go` | proves every menu item names a dispatched verb |
| `service.go` | `start` `stop` `restart` `enable` `disable`, each verifying the result |
| `inspect.go` | `logs` (journald) and `cert` (TLS dial) |
| `admin.go` | `reset-admin` |
| `admin_helpers.go` | Super Admin resolution |
| `admin_test.go` | reset-admin against a real isolated DB |
| `downgrade.go` | `downgrade` front door |
| `ports.go` | `ports` / `ports set` |
| `ports_test.go` | port validation, env rewrite, Caddyfile render |
| `dns.go` | Cloudflare DNS-01: provider table, plugin probe, marked TLS block |
| `dns_test.go` | token handling, render, token-file secrecy |
| `secret_unix.go` | password prompt without echo (unix) |
| `secret_windows.go` | the same, Windows |
| `tty_unix.go` | terminal detection (shared) |
| `tty_linux.go` | `TCGETS` ioctl |
| `tty_bsd.go` | `TIOCGETA` ioctl |
| `tty_windows.go` | Windows console detection |

### Everything else that changed

| File | Change |
|---|---|
| `install.sh` | **the important one** — port flags, port picker, `/dev/tty` prompts, `vey` launcher, control-menu offer |
| `scripts/lib/common.sh` | `has_tty`, `read_tty`, `read_tty_secret`, `validate_port` |
| `scripts/install.selftest.sh` | T11–T14 (109 checks) |
| `deploy/Caddyfile` | `__HTTP_PORT__` / `__HTTPS_PORT__` / `__APP_PORT__` placeholders + global port block |
| `backend/cmd/vpn-platformctl/main.go` | `ports` + `menu` verbs, `appPort()` default, usage text |
| `backend/cmd/vpn-platformctl/lifecycle.go` | `appPort()`, `defaultBaseURL()` |
| `backend/cmd/vpn-platformctl/update.go` | `domain-set --dns-provider/--cf-token` |
| `backend/cmd/vpn-platformctl/main_test.go` | `TestAppPort` ×14, `defaultBaseURL` ×3 |
| `backend/internal/httpapi/settings_routes.go` | `GET /api/super/settings/public-url` |
| `backend/internal/httpapi/settings_routes_test.go` | 4 public-URL tests incl. RBAC |
| `frontend/src/lib/api.ts` | `ApiPublicURL`, `api.publicURL()` |
| `frontend/src/pages/SuperSettingsPage.tsx` | the "This deployment" card |
| `README.md` | route/test counts (144 / 709) |

Docs (`docs/INSTALL.md`, `docs/DECISIONS.md`, `docs/new/DEVELOPMENT_LEDGER.md`,
`docs/cline/progress.md`) should go too, but nothing breaks without them.

---

## 3. How to push

### The bundle (easiest, and what I verified)

`_push-bundle/` in this workspace already contains **exactly** the 31 files, at
**exactly** the right paths, ready to drop onto a clone:

```
_push-bundle/
  install.sh
  README.md
  deploy/Caddyfile
  scripts/lib/common.sh
  scripts/install.selftest.sh
  backend/cmd/vpn-platformctl/*.go        (23 files)
  backend/internal/httpapi/settings_routes.go
  backend/internal/httpapi/settings_routes_test.go
  frontend/src/lib/api.ts
  frontend/src/pages/SuperSettingsPage.tsx
```

**Proven complete:** I copied the bundle over a clean tree and ran
`go build ./...`, `go vet ./...` and `go test ./cmd/vpn-platformctl/` — all
passed, so nothing is missing from it.

```bash
git clone https://github.com/janith-15/Veylix-1 /tmp/veylix-push
cp -r "_push-bundle/." /tmp/veylix-push/
cd /tmp/veylix-push
git status          # should list ~31 changed/added files
git add -A
git commit -m "Install system: control menu, port picker, DNS-01, vey launcher

- install.sh: --http-port/--https-port/--app-port, interactive port picker,
  /dev/tty prompts (works under bash <(curl ...)), vey launcher
- backend: menu/service/inspect/admin/ports/dns/tty packages
- Super Admin settings: public URL endpoint + card
- docs: D-121..D-126, push guide"
git push
```

### Option B — GitHub Desktop

1. Clone `janith-15/Veylix-1`.
2. Copy `_push-bundle/` contents over the clone, keeping paths.
3. Commit and push.

### Option C — web upload (no tools)

Open each path on GitHub → **Add file → Upload files**, drag the file in, commit.
GitHub allows 100 files per commit, so one batch is enough.

`install.sh` must be replaced at the **repo root**, not added under a folder.

---

## 4. Verify the push BEFORE installing

Run this from your workstation (not the server). It checks that `main` now has
what the installer needs.

```bash
BASE=https://raw.githubusercontent.com/janith-15/Veylix-1/main

# 1. install.sh must be the NEW one (old ~31,831 bytes, new ~48,447)
curl -fsSL "$BASE/install.sh" -o /tmp/i.sh
echo "bytes: $(wc -c < /tmp/i.sh)"      # expect 48447

# 2. the installer features must be present
for token in install_launcher choose_ports read_tty validate_ports_for_install \
             'VEYLIX_NONINTERACTIVE'; do
  if grep -q -- "$token" /tmp/i.sh; then echo "OK   $token"; else echo "MISS $token"; fi
done

# 3. the CLI package must be complete (new files -> must be 200)
for f in menu.go menu_test.go service.go inspect.go admin.go admin_helpers.go \
         downgrade.go ports.go ports_test.go dns.go dns_test.go \
         secret_unix.go secret_windows.go tty_unix.go tty_linux.go \
         tty_bsd.go tty_windows.go; do
  code=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/backend/cmd/vpn-platformctl/$f")
  echo "$code  $f"
done

# 4. the DNS-01 flag must be documented in the CLI source
if curl -fsSL "$BASE/backend/cmd/vpn-platformctl/update.go" | grep -q 'dns-provider'; then
  echo "OK   update.go has --dns-provider"
else
  echo "MISS update.go has --dns-provider"
fi
```

**All five tokens OK, all seventeen files 200, and the last line OK.** If
anything shows MISS or 404, the push is incomplete — do not install yet.

If `install.sh` still shows 31831 bytes, `raw.githubusercontent.com` is serving a
cached copy: wait two minutes, or check the file's commit SHA on GitHub.

### Expected output BEFORE you push (this is what I measured just now)

```
bytes: 31831
MISS install_launcher
MISS choose_ports
MISS read_tty
MISS validate_ports_for_install
MISS VEYLIX_NONINTERACTIVE
404  menu.go
404  service.go
...
404  tty_linux.go
```

That is the exact state of `main` today, and the reason the one-liner cannot
install on a 3X-UI host.


---

## 5. Then install

Your server already runs 3X-UI on **80, 443, 8443 and 8080**. Pick ports that are
free on your host — check first:

```bash
sudo ss -lntp | awk '{print $4}' | grep -oE '[0-9]+$' | sort -un
```

Choose three that are **not** in that list, then:

```bash
# Download ONCE, then run the file. Do not use `sudo bash <(curl ...)`:
# sudo does not inherit your shell's /dev/fd/N, so it fails with
#   bash: /dev/fd/63: No such file or directory
BASE=https://raw.githubusercontent.com/janith-15/Veylix-1/main
curl -fsSL "$BASE/install.sh" -o /tmp/veylix-install.sh

# Dry run first: prints every phase, changes nothing.
sudo bash /tmp/veylix-install.sh --domain vpn.example.com \
  --http-port 8090 --https-port 9443 --app-port 10080 --dry-run

# Then the real install (same command without --dry-run).
sudo bash /tmp/veylix-install.sh --domain vpn.example.com \
  --http-port 8090 --https-port 9443 --app-port 10080
```

Or leave the flags off and it asks:

```
Ports. The defaults suit a server that is not already using 80 and 443.

  1) Standard   - HTTPS on 443, HTTP on 80            (https://<domain>)
  2) 3X-UI host - HTTPS on 8443, HTTP on 8080         (https://<domain>:8443)
  3) Custom     - type your own
```

**Option 2 uses 8443, which your 3X-UI already holds** — so for *your* host,
choose **3** and type free ports such as `9443`, `8090`, `10080`.

If you pick a port that is taken, the installer stops **before writing anything**
and names both remedies:

```
port 8090 is already in use.
  If it is 3X-UI (Xray) and you need it running, give Veylix its own ports:
    sudo ./install.sh --domain vpn.example.com --http-port 8080 --https-port 8443
  If it is a stray process you do not need, stop it and re-run.
```

---

## 6. After it finishes

```bash
sudo vey                    # the control menu  (3X-UI calls this `x-ui`)
sudo vey status             # service, readiness, database
sudo vey logs -n 20         # journald
sudo vey cert               # what TLS the domain actually serves
```

The menu:

```
 1) Start              8) Update              15) Change domain
 2) Stop               9) Versions            16) Change ports
 3) Restart           10) Downgrade           17) Certificate
 4) Status            11) Backup              18) Reset admin
 5) Logs              12) Restore             19) Uninstall
 6) Enable autostart  13) Doctor              20) Uninstall + purge
 7) Disable autostart 14) Repair
```

**Certificate:** with 3X-UI on 80/443, HTTP-01 and TLS-ALPN-01 are both
unavailable. Use DNS-01:

```bash
sudo vey domain-set vpn.example.com --dns-provider cloudflare --cf-token '<token>'
```

This needs a Caddy built with the Cloudflare plugin —
`caddy list-modules | grep dns.providers.cloudflare` answers in one line. See
`docs/INSTALL.md` §B.10.

Until a certificate exists, Veylix runs and answers on
`http://127.0.0.1:10080` — the installer reports that honestly as **exit code 2**
and never claims HTTPS works.

---

## 7. Before you push: the gates pass

| Gate | Result |
|---|---|
| `gofmt -l .` | clean |
| `go build ./...` / `go vet ./...` (native + `GOOS=linux`) | PASS |
| `go test ./... -count=1` | **17/17 packages, 0 FAIL** |
| `scripts/install.selftest.sh` | **109 PASS / 0 FAIL** |
| `bash -n` ×3 | PASS |
| linux/amd64 + arm64, both binaries | 4/4 PASS |
| `npm run lint` / `build` / `audit` / `audit:selftest` / `docs:verify` | 0 errors / PASS / OK / OK / OK |
| Negative controls (T12, T13, T14) | each breakage caught by name |

**Not verified:** none of this has run on a real Linux host. The code is
gate-green on Windows; your first install is the first real test.

