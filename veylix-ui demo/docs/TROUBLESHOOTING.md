# VEYLIX — TROUBLESHOOTING

**Symptom → cause → fix.** Every entry names the check that distinguishes one
cause from another, so you are not guessing.

Start with these two, always:

```bash
curl -s http://127.0.0.1:8080/api/ready      # is the backend up, is the UI embedded?
sudo journalctl -u veylixd -n 100 --no-pager # what did it say on the way up?
```

---

## 1. The site will not load

| Symptom | Likely cause | Check / fix |
|---|---|---|
| Connection refused on :443 | Caddy not running, or the site block is wrong | `sudo systemctl status caddy`, `sudo caddy validate --config /etc/caddy/Caddyfile` |
| Certificate error / `ERR_CERT_*` | DNS does not point here, or port 80 is blocked so the ACME challenge failed | `dig +short your.domain` must equal the server IP; open 80 **and** 443 |
| Loads, but every page is a JSON 404 | The binary was built **without the UI** | `curl /api/ready` → `"ui":"not_embedded"`. Install a release built by `release.yml`, or run `.\dev.ps1 -Package` |
| Blank page, console `Cannot reach the server` | Backend down behind a working proxy | `systemctl status veylixd`; `journalctl -u veylixd` |

## 2. The UI loads but nothing saves (`403`)

This is the CSRF origin check (D-043) working as designed.

| Detail | Meaning | Fix |
|---|---|---|
| `403 csrf_blocked`, "cross-origin request blocked" | The browser's `Origin` does not match the `Host` the request arrived on | You are on a **different port or hostname** than the proxy expects. Reach the app through Caddy, not `:8080` |
| `403 csrf_blocked`, "cross-site request blocked" | A browser sent **no** `Origin` on a state-changing request | Something is stripping the header — check for a second proxy |
| `403 wrong_portal` | Correct password, wrong door (D-041) | Staff → `/staff/login`; clients → `/login` |
| `403 cross_origin` on plain-HTTP dev | Vite hopped to `5174` | `vite.config.ts` pins 5173 with `strictPort`; free 5173 and restart |

> **Never "fix" a CSRF 403 by relaxing the check.** It is the only thing stopping
> another site from posting to your API with a signed-in user's cookie.

## 3. Everything answers `401 authentication required`

| Cause | Fix |
|---|---|
| The backend started **without its env file**, so no session key loaded | Start it via systemd (which reads `/etc/veylix/veylix.env`), never `./veylixd` in a bare shell |
| Session cookie not sent because it is `Secure` and you are on plain HTTP | Expected in production. In local dev use `.\dev.ps1`, which sets `VEYLIX_INSECURE_COOKIES=1` for a loopback bind only |
| Cookies blocked by the browser | Check the site is not blocking all cookies; the session is host-only by design |
| Everyone signed out after a domain change | Expected — the cookie is host-only (D-043). Sign in again |

## 4. Sign-in succeeds but the role is wrong, or a password stopped working

Passwords are rotated by the operator tooling, and **`seed.json` can be stale** —
it records the value at seed time, not the current one.

```powershell
# Development: reset to a fresh one-time password (printed once, saved to file)
.\dev.ps1 -CreateAdmin -NewEmail 'reseller@veylix.test' -NewUsername 'reseller' -Quota 25
```

The generated password is written to `.veylix-secrets/admins.txt` (or
`superadmin.txt`) and shown once. A login failing with `401` on **both** portals
means the password is wrong — not the portal.

## 5. The panel shows `REAUTH_REQUIRED` / `CONNECTION_UNAVAILABLE`

| Meaning | Fix |
|---|---|
| The 3X-UI API token was revoked or changed | Re-enter the token on **Servers → Edit**. The URL cannot change without a new token (the sealed ciphertext is AAD-bound to the URL) |
| The panel is unreachable from this host | `curl -sk https://panel:port/panel/api/server/status` from the server itself |
| Panel writes rejected | Confirm the panel version and path prefix — 3X-UI serves its API under the **web base path** (`docs/3XUI_API_CONTRACT.md` §0) |

A failed health check stops new provisioning for that server; it does **not**
delete anything.

## 6. "The panel could not be read" on a VPN or server page

This is an **outage message, not an empty result** (D-045). The UI deliberately
refuses to render an empty list or a blank QR when the read failed, because that
would be indistinguishable from "this VPN has no links".

- Check the server row on **Servers** for `ACTIVE`.
- Check `GET /api/sync/status` for the last successful sync.
- Retry once the panel answers; nothing was lost.

## 7. A QR code says "too long to encode"

The in-repo encoder supports up to **666 bytes** (versions 1–20, level M). A
`vless://` link above that cannot be encoded. This is a refusal, never a silent
truncation — a truncated link would scan and then fail to connect.

Copy the link instead, or shorten the link (fewer query parameters) in the panel.
`npm run qr:selftest` proves the cap.

## 8. "Could not measure your latency"

P-05 measures a **WebSocket handshake from your browser** to the VPN endpoint.
It reports a failure rather than a number when:

| Reason shown | Cause |
|---|---|
| "did not accept a WebSocket handshake" | The inbound's port does not speak WebSocket (for example a raw TCP inbound), or a firewall drops it |
| "did not answer within the time allowed" | The port is filtered; 4 s elapsed |
| "The panel could not be read" | The server address is unknown right now — see §6 |

**This is not a fault in the VPN.** Use the **Ping server** button beside it for
the server's own reachability — a different measurement, labelled as such.

## 9. The database will not open, or `integrity_check` fails

```bash
sudo systemctl stop veylixd
sudo ./deploy/veylix-backup.sh verify /var/backups/veylix/<latest>
sudo ./deploy/veylix-backup.sh restore /var/backups/veylix/<latest>
```

Do not delete `-wal` or `-shm` to "fix" an error: a missing or unmerged WAL is
how committed data disappears. Restore a verified backup instead — see
[`BACKUP_RESTORE.md`](BACKUP_RESTORE.md) §5.

## 10. High memory on a small box

The target is 1 vCPU / 500 MB. If `veylixd` grows:

| Check | Command |
|---|---|
| Is sync too frequent? | `VEYLIX_SYNC_INTERVAL` (default `5m`; `0` disables) |
| Is one panel hanging and holding a worker? | `VEYLIX_SYNC_TIMEOUT` (default `60s`) bounds **one** server |
| Is a client list read whole? | Drill-down and pickers filter server-side; avoid unfiltered reads on large panels |
| Was the service OOM-killed? | `journalctl -u veylixd \| grep -i oom`; the unit sets `MemoryMax=384M` |

**A 1-vCPU / 500-MB benchmark has NOT been run.** Treat the sizing as the design
target, not a measured result (Bible §19).

## 11. Development-only problems

| Symptom | Fix |
|---|---|
| `Port 8080/5173 already in use` | `.\dev.ps1 -Restart` (it stops what it started) |
| Backend starts then dies silently | Start via `.\dev.ps1`; a bare `go run` has no env |
| Vite serves on `::1` only | Expected — use `http://localhost:5173`, not `127.0.0.1` |
| `npm run verify:vpn` fails every check | There is no VPN in the dev database. Create one; the check is not lying about the UI |
| `npm run verify:ui` prints 404 console lines | The route id does not exist in this database yet (e.g. `/servers/1` with zero servers) |

---

## Still stuck?

Collect these four things; the answer is usually in them:

```bash
curl -s http://127.0.0.1:8080/api/ready
sudo journalctl -u veylixd -n 200 --no-pager
sudo systemctl status caddy veylixd --no-pager
sudo -u veylix ls -l /var/lib/veylix
```

