---
name: veylix-security
description: Use when a change touches authentication, sessions, passwords, roles, permissions, row-level ownership, API tokens, secret storage, encryption, SSRF and outbound requests, rate limiting, CSRF, audit logging or redaction in Veylix. Provides the P0 secret inventory, the RBAC and ownership matrix, the sealing rules, the SSRF guard requirements and a per-change security checklist. Load before editing anything under backend/internal/auth, internal/integrations, internal/audit, or any route that reads or writes an owned row.
---

# VEYLIX — SECURITY

Applied form of Rule 03. Authoritative text: `./MASTER_PRODUCT_BIBLE.md` §0.6
and §8, `./docs/AUTH.md`, `./docs/3XUI_API_CONTRACT.md` §1.

## Threat model in one table

| Asset | If it leaks | Control |
|---|---|---|
| 3X-UI Bearer token | full admin over a customer's VPN panel | AES-256-GCM at rest in `server_integrations.token_ciphertext`; handed to the client only at call time |
| Sealing key / session secret | decrypt or forge everything | `./.veylix-secrets/`, never in source or bundle |
| Session cookie | account takeover | server-side sessions, httpOnly, CSRF/origin checks |
| Other clients' VPN rows | privacy breach between tenants | row-level ownership enforced in SQL, not in the UI |
| Panel URL | SSRF pivot into the host network | SSRF guard in `internal/panel` |

## P0 secrets — the only legal homes

- `./.veylix-secrets/` (git-ignored) for local development values.
- `server_integrations.token_ciphertext` (BLOB) for stored panel tokens.

Illegal in **all** cases: source files, comments, docs, `./records/`, logs,
error strings, API responses, the frontend bundle, exported JSON, a command
line, a commit message, or a test fixture.

**Token shape checklist** — before finishing, grep the changed surface for
`Bearer`, `token`, `secret`, `key`, `password`, `uuid`, and confirm no new line
emits a value.

## Authorization: role AND ownership

Every route must answer three questions server-side:

1. **Who is calling?** — authenticated session, not a client-supplied id.
2. **May this role do this?** — Super Admin / Admin / Client.
3. **Do they own this row?** — an Admin sees only assigned servers and the
   clients/VPNs under them; a Client sees only its own rows.

The UI hiding a button is **not** a control. A direct API call must fail with
`403`/`404`, and that failure must be tested (see `veylix-testing-review`).

## SSRF guard (mandatory on every operator-supplied URL)

- Allow only `http`/`https`.
- Resolve and reject loopback, private, link-local, CGNAT and cloud-metadata
  ranges unless an explicit deployment policy allows them.
- Refuse redirects to a prohibited destination.
- Bound it: per-request timeout (10 s), bounded retries on `GET` only.
- Never delete the guard to "make the test panel work".

## Honest crypto and hashing

- argon2id for passwords; never a client-side hash as the credential.
- AES-256-GCM for sealing; never ECB, never a static IV.
- TLS verification is per-integration at most; **never** disabled globally.

## Audit obligations

Append an audit entry for: panel credential change, server add/remove/
disable, provisioning, revocation, quota change, price change, payment
confirmation, role or permission edit, and client deletion. The audit table is
append-only — no update, no delete.

## Rate-limit surfaces

Sign-in · registration · password reset · Free VPN claim · chat message ·
payment action · panel operation. Each needs a bound and a clear refusal.

## Truthful failure states

An unreachable panel yields an explicit pending/unknown state. Never a
fabricated `REVOKED`. Never a claimed success. Never a silent 200.

## Per-change security checklist

- [ ] New route has a negative test for unauthenticated access.
- [ ] New route has a negative test for wrong role.
- [ ] New route has a negative test for wrong owner.
- [ ] No secret, token, key, UUID or password in any new log or error string.
- [ ] New outbound URL goes through the SSRF guard.
- [ ] New state-changing action writes an audit entry.
- [ ] New input is validated (URL, protocol, limit, currency, price, remote id).
- [ ] Concurrency: quota and provisioning paths use a transaction/lock.

## Findings (R6) — historical only

Prior investigations covered security, deployment and performance together.
Treat any conclusion drawn there as **historical**: re-check
`./backend/internal/auth` and `./backend/internal/panel` as they exist now, and
never assume a previously-recorded control is still present.
