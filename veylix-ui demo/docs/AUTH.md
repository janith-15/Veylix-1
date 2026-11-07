# AUTH — signing in to Veylix

> **No credentials live in this file.** They are in `.veylix-secrets/seed.json`
> (git-ignored). Never paste them into docs, tickets, chat, logs or source.

There is **one** way to become a user: authenticate. The development bypass that
used to inject an identity was **removed on 2026-10-28** (`docs/DECISIONS.md`
D-039). No flag, header or persona grants a session without a password or a
verified Google sign-in.

---

## 0. Quick start

```powershell
cd "h:\my apps\Veylix-1"
.\dev.ps1 -Restart -Preflight     # start both, then run the 27-check auth gate
```

Then open the door that matches the account (the server has **two** portals and
refuses the wrong one with `403 wrong_portal`):

| Account | Sign in at |
|---|---|
| `super_admin`, `admin` (staff) | **http://localhost:5173/staff/login** |
| `client` | **http://localhost:5173/login** |

Passwords are in `.veylix-secrets/seed.json` (the script prints the addresses).
`-Preflight` proves that sign-in, the role matrix, logout and the proxy all work
before you trust the app.

| Command | Does |
|---|---|
| `.\dev.ps1` | backend (:8080) + frontend (:5173), verifies login + seeded sign-in |
| `.\dev.ps1 -Restart` | stop :8080/:5173 first, then start |
| `.\dev.ps1 -Preflight` | start and run the full gate (`npm run preflight`) |
| `.\dev.ps1 -Status` | is it up, is auth **enforced**, and does the login **work**? |
| `.\dev.ps1 -Seed` | create the demo accounts (one per role) |
| `.\dev.ps1 -FakeAdmin` | + a second admin (needed to test disabling one) |
| `.\dev.ps1 -Verify <email>` | mark a local account's e-mail verified |
| `.\dev.ps1 -PurgeChat` | delete test chat data (backup written first) |
| `.\dev.ps1 -Stop` / `-Wait` / `-Lan` / `-Log` / `-Package` | as before |

## 1. The three roles

| Role | Lands on | Can do |
|---|---|---|
| `super_admin` | `/dashboard` | everything: servers, offers, admins, audit, payments |
| `admin` (reseller) | `/admin` | assigned servers, own clients, sale flow, retail price, payments |
| `client` | `/client` | own VPNs, buy premium, history, chat, notifications |

The server decides the role; the UI only displays what the API says.

## 2. Sign-in methods

### 2a. Email + password

`POST /api/auth/login` → HttpOnly session cookie (12h), `SameSite=Lax`, `Secure`
outside loopback. Wrong credentials answer `401 invalid_credentials`; too many
attempts answer `429 rate_limited`.

**Local plain-HTTP note:** a `Secure` cookie is never sent back over `http://`, so
the dev env sets `VEYLIX_INSECURE_COOKIES=1` (loopback only,
`config.resolveSecureCookies`). Without it the login looks fine and then every

### 2b. Continue with Google

Real OAuth 2.0, entirely server-side (D-039):

```
GET /api/auth/google/start     → 302 to accounts.google.com (PKCE S256 + state + nonce)
GET /api/auth/google/callback  → code exchanged server-side, ID token verified,
                                 session cookie set, 303 into the app
GET /api/auth/google/status    → { "configured": true|false }   (drives the button)
```

The ID token's **signature** (Google's JWKS), `iss`, `aud`, `exp`, `iat` and the
`nonce` this server issued are all checked. Then the account rules apply:

| Situation | Result |
|---|---|
| that Google account is already linked | sign in |
| the e-mail exists **and Google verified it** | link Google to that account |
| the e-mail exists but Google does **not** verify it | **refused** — no account takeover |
| nobody has the e-mail | create a **client** account (no password, verified) |

**Setup (once):**

1. Google Cloud Console → *APIs & Services* → *Credentials* → **Create OAuth
   client ID** → *Web application*.
2. Authorized redirect URI: `http://localhost:5173/api/auth/google/callback`
   (add the production URL too — it must match **exactly**).

   > **A client signing in from another device cannot use a `localhost`
   > redirect.** The browser is sent back to *their* machine, which has no
   > Veylix on it, and the sign-in dies after the Google consent screen. If
   > clients reach the portal over LAN or a domain, register **that** origin's
   > `/api/auth/google/callback` as a second authorized redirect URI and point
   > `VEYLIX_GOOGLE_REDIRECT_URL` at it. Until then the reliable way for a
   > Google-created client to also have a password is *Profile → Security →
   > Set a password*, and for everyone else the console's reset-password.
3. Put the client in `.veylix-secrets/dev-env.ps1`:

```powershell
$env:VEYLIX_GOOGLE_CLIENT_ID     = '<id>.apps.googleusercontent.com'
$env:VEYLIX_GOOGLE_CLIENT_SECRET = '<secret>'
$env:VEYLIX_GOOGLE_REDIRECT_URL  = 'http://localhost:5173/api/auth/google/callback'
```

4. `.\dev.ps1 -Restart` — the log then says `google sign-in enabled`.

Without those three values the button is **disabled** with a reason and
`/api/auth/google/start` answers `503 google_not_configured`. It never pretends.

## 3. E-mail verification

`Register` creates the account unverified. Verification is a single-use token (24h):

```
POST /api/auth/verify/request   → 202 { sent, email, token?, link? }
GET  /api/auth/verify/{token}   → 303 /login?verified=1   (or verified=invalid)
```

**SMTP is not wired yet**, so the token is returned **only to a loopback caller**
(a local operator); a deployment never hands it back. Until a relay exists:

```powershell
.\dev.ps1 -Verify someone@example.com
```

Replaying a link is refused (`verified=invalid`) — checked live, and
`TestOAuthStateIsSingleUse` covers the same single-use rule for the OAuth state.

## 4. Accounts

Only a Super Admin mints staff accounts (`POST /api/admins`); clients self-register
or sign up with Google. For a second admin:

```powershell
.\dev.ps1 -Restart -FakeAdmin     # adds admin2@veylix.test
```

`PATCH /api/admins/{id}` refuses your own account (`422 cannot_modify_self`), which
is why one admin is never enough to test disabling.

## 5. The verification gate

```powershell
.\dev.ps1 -Restart -Preflight        # 27 checks
cd frontend ; npm run preflight      # the same gate against a running stack
```

It signs in as all three roles and asserts:

* five guarded routes answer **401** anonymously, and a wrong password is
  `401 invalid_credentials`;
* each role signs in and `/api/me` reports that role;
* the matrix: `admin` is **403** on `/api/admins` and `/api/audit`, 200 on
  `/api/clients`; `client` is 403 on `/api/clients` and `/api/admins`;
* **logout ends the session** (the same cookie → 401 afterwards);
* the SPA and `/login` load, and the proxy refuses a stranger.

## 6. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| login returns 200, then everything 401 | `VEYLIX_INSECURE_COOKIES=1` missing (plain http) | `dev-env.ps1` sets it; start with `.\dev.ps1 -Restart` |
| `401 invalid_credentials` for a seeded account | the DB password differs from `seed.json` | `go run ./cmd/seedacct --role <role> --reset-password` |
| Google button disabled | no client configured | see §2b, then `.\dev.ps1 -Restart` |
| `?google=email_unverified` | Google says the address is not verified | use a verified Google account or the password form |
| `?google=state_invalid` | the attempt expired (5 min) or was replayed | start again from `/login` |
| `?verified=invalid` | link expired or already used | `POST /api/auth/verify/request` again |
| signed out unexpectedly | 12h lifetime, or a password change (all sessions revoked) | sign in again |

## 7. What changed on 2026-10-28

| Removed | Why |
|---|---|
| `VEYLIX_DEV_AUTH`, `VEYLIX_DEV_ROLE_SWITCH`, `VITE_DEV_AUTH` | one switch was all that separated "authenticated" from "not" |
| `httpapi.DevPrincipal*`, `RequireAuthOrDev`, `auth.DevRoleHeader` | identity without a session |
| `frontend/src/lib/devMode.ts`, `components/dev/*`, the DEV chip, `dev_impersonating` | the UI affordances for that identity |

| Added | Why |
|---|---|
| `internal/auth/google` (verifier, PKCE, JWKS cache) | sign-in/up with Google, server-side |
| `auth.GoogleAuth` (`Start` / `Finish` / `Resolve`) | the linking rules live in one tested place |
| `auth.EmailVerifier` + `email_verifications` | verification was a promise with no implementation |
| `migration 0012` | `auth_identities`, `oauth_states`, `email_verifications` |
| `cmd/accounttool` | `-verify` and `-purge-chat` for an operator |
| `frontend/scripts/auth-login.mjs` | browser scripts sign in for real instead of faking a role |

**A defect the new tests caught:** the ID-token verifier passed `0` instead of
`crypto.SHA256` to `rsa.VerifyPKCS1v15`, so every genuine Google token would have
been rejected. Fixed; `TestVerifyIDTokenAcceptsAValidToken` is the test that found it.

call answers 401 (IMPROVEMENT_LEDGER F-013).
