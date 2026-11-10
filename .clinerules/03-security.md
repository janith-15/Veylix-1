# 03 · SECURITY REQUIREMENTS

Always loaded. Full threat detail and checklists: Skill `veylix-security`.

Source: `./MASTER_PRODUCT_BIBLE.md` §0.6 and §8, `./docs/AUTH.md`,
`./docs/3XUI_API_CONTRACT.md` §1.

## P0 secrets — treat every one as a production credential

- The 3X-UI **Bearer API token** (it is a *full-admin* panel credential).
- The AES-256-GCM key that seals it.
- The session signing secret.
- Any Google OAuth client secret.

They live **only** in `./.veylix-secrets/` (git-ignored) or the sealed column
`server_integrations.token_ciphertext`. They must never appear in: source,
docs, records, logs, error messages, API responses, the frontend bundle,
exports, or a command line.

## Non-negotiable rules

1. **Server-side authorization on every request.** A UI that hides a control is
   not authorization. Verify role **and** row-level ownership in the backend.
2. **Never disable TLS verification globally.** A per-integration, explicit,
   logged exception is the only acceptable form.
3. **SSRF guard on every operator-supplied panel URL.** Validate scheme and
   destination; block loopback, private, link-local and metadata ranges;
   refuse redirects to prohibited destinations.
4. **Seal tokens at rest** (AES-256-GCM). Hand the panel client the token only
   at call time. Plaintext tokens have no column to live in.
5. **argon2id for passwords; server-side sessions.** Never trust a client-
   supplied identity.
6. **Rate-limit** sign-in, registration, password reset, Free provisioning,
   chat, payment actions and panel operations.
7. **Transactions and locks for quota.** Two concurrent requests must never
   exceed a reseller's quota or create a duplicate panel client.
8. **Audit every privileged action**: credential changes, provisioning and
   revocation, quota and price edits, payment confirmation, role and permission
   changes. The audit log is append-only.
9. **Redact on every error path.** A stack trace that leaks a token is an
   incident, not a warning.
10. **Validate every input**: URL, protocol, limits, currency, price, remote ID.

## Failure must be honest

Never claim a remote effect from local state. An unreachable panel produces
`REVOCATION_PENDING` (or an equivalent explicit state) — never a fabricated
`REVOKED`, never a fabricated success.

## Before you call security work finished

- Negative test for **every** changed route: unauthenticated, wrong role,
  wrong owner.
- Grep the changed surface for the secret shapes above.
- Confirm no new log line can emit a token, key, UUID or password.
- Confirm the new path is covered by the audit log where it changes state.

If a change weakens any rule above, **stop and report it** instead of shipping it.
