# VEYLIX — DOMAIN CHANGE

**Moving the platform to a different domain.** Three things are domain-bound and
each fails differently if it is missed. Work through them in this order.

---

## 1. What is bound to the domain

| Binding | Where it lives | If it is wrong |
|---|---|---|
| TLS certificate + reverse proxy | `/etc/caddy/Caddyfile` | The site is unreachable, or serves a stale certificate |
| **CSRF origin check** | automatic — `SameOrigin` compares the request's `Origin` against the request's own `Host` | **Nothing to change**: because the UI and API share one origin behind the proxy, a new domain matches itself |
| **Google OAuth redirect URL** | Google Cloud console + `VEYLIX_GOOGLE_REDIRECT_URL` | Google sign-in fails with `redirect_uri_mismatch` |
| Cookie domain | *(none — the session cookie is host-only)* | Nothing to change |

The session cookie is deliberately **host-only** (D-043): it is not shared with
subdomains, so a change of domain simply signs everybody out instead of leaving
a cookie valid for an origin you no longer control.

> **Verified 2026-11-03:** `SameOrigin` (in `internal/httpapi/middleware.go`)
> derives the accepted origin from `schemeHost(r)` — the request's own Host — and
> additionally accepts any `Deps.PublicOrigins`. Nothing in `config` or
> `cmd/veylixd` sets `PublicOrigins`, so in a single-domain deployment the check
> is **self-referential and needs no configuration**. Only a genuinely *extra*
> origin (for example a marketing site posting into the API) would require
> wiring it up, so this document does not ask you to.

---

## 2. Before you start

```bash
sudo ./deploy/veylix-backup.sh backup
```

A domain change does not touch the database, but it is the moment people are
most likely to be editing settings. Take the backup anyway.

---

## 3. Steps

### 3a. Point DNS at the server

```bash
dig +short vpn.new-domain.com
# must already answer with this server's IP before Caddy requests a certificate
```

Caddy will not obtain a certificate for a name that does not resolve yet — it
retries, but the site stays down until DNS is correct. Do this first.

### 3b. Update the reverse proxy

```bash
sudo cp /etc/caddy/Caddyfile /etc/caddy/Caddyfile.bak
sudo sed -i 's/^vpn\.old-domain\.com {/vpn.new-domain.com {/' /etc/caddy/Caddyfile
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

`caddy validate` before `reload` is not optional: a syntax error takes the whole
site down, including the old domain.

### 3c. Nothing to configure for CSRF

The backend's CSRF check (`SameOrigin`) compares the request's `Origin` against
the request's own `Host`. Because the UI and the API are served from the **same
origin** behind Caddy, the new domain matches itself — there is **no origin list
to update** and no env var to set.

> The hard rule (D-043): the check **refuses a browser that cannot name its
> origin**. It is not weakened by removing the header — that only hides the
> problem behind a CSRF hole. If you ever front this with a second hostname,
> that is the one case where an extra allowed origin must be wired up in code
> (`Deps.PublicOrigins`), not by a flag.

### 3d. Update Google sign-in (only if it is enabled)

In **Google Cloud Console → APIs & Services → Credentials → your OAuth client**:

1. Add `https://vpn.new-domain.com/api/auth/google/callback` to **Authorized
   redirect URIs** (keep the old one until you have confirmed the new one works).
2. Set `VEYLIX_GOOGLE_REDIRECT_URL=https://vpn.new-domain.com/api/auth/google/callback`
   in `/etc/veylix/veylix.env` and restart.

If `VEYLIX_GOOGLE_CLIENT_ID` is unset the button answers `503
google_not_configured` and is disabled in the UI — nothing to do in that case.

### 3e. Re-enter nothing

Panel tokens are sealed with `VEYLIX_ENCRYPTION_KEY`, **not** with the domain
(AAD is bound to the panel URL, which has not changed). Do not rotate the keys
for a domain change.

---

## 4. Verify

```bash
# 1. The site answers over the new domain.
curl -sI https://vpn.new-domain.com/login | head -1        # 200

# 2. Readiness, from the new origin.
curl -s https://vpn.new-domain.com/api/ready

# 3. A POST works — this is the one the origin/host check actually gates.
curl -s -o /dev/null -w '%{http_code}\n' \
  -X POST https://vpn.new-domain.com/api/auth/login \
  -H 'Content-Type: application/json' -H 'Origin: https://vpn.new-domain.com' \
  -d '{"email":"x@example.com","password":"wrong","portal":"staff"}'
# 401 (bad password) = the origin matched the host, so the request got through.
# 403 csrf_blocked = the proxy is presenting a different Host than the browser
#                    used; check the Caddyfile site address.
```

Then sign in **in a browser** and change something (for example, save the rate
card) — a read-only check cannot prove the CSRF path works.

---

## 5. Rollback

Put the old domain back in the Caddyfile, restore
`VEYLIX_PUBLIC_ORIGINS`, reload Caddy and restart Veylix. Keep the old DNS record
pointing at the server until you are satisfied: reverting DNS is the slowest part
of any rollback and you do not want to need it.

---

## 6. Status

| Item | State |
|---|---|
| Procedure | documented above |
| Practised | **NO** — not exercised on a live domain |
| Automated | **NO** — no `vpn-platformctl domain-set` command exists (INSTALL §F) |

Treat this as an accurate runbook, not a tested script.
