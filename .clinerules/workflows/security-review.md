# WORKFLOW · SECURITY REVIEW

Use for a security audit pass, before a release, or after any change to auth,
sessions, tokens, roles, ownership or the panel integration.

**This workflow is read-only by design.** Stages 1–8 produce *findings*, not
edits. Fixes go through `bug-fixing.md` after the owner has seen the findings.
A reviewer who edits while auditing cannot tell what was already broken.

Load Skill `veylix-security`.

---

## Stage 1 — Enumerate the P0 assets

List what would hurt if it leaked or was bypassed, in this workspace:

- the 3X-UI Bearer token(s),
- the sealing key and the session secret,
- session cookies,
- other tenants' rows (clients, VPNs, payments, chat),
- the panel URL surface (SSRF).

**Output:** the asset list, with where each is *supposed* to live.

## Stage 2 — Authentication

- Every non-public route requires a real session.
- Passwords: argon2id; no client-side-only hashing.
- Sessions are server-side and revocable; logout actually invalidates.
- Rate limits exist on sign-in, registration and password reset.

## Stage 3 — Authorization (role)

For each route family, confirm the backend refuses the wrong role. A hidden
button is not a control. Expect `403`/`404`, not `200`.

## Stage 4 — Ownership (row level)

- An Admin reaches only assigned servers and their clients/VPNs.
- A Client reaches only its own rows: QR, link, usage, payment, chat.
- Cross-tenant reads are refused **in SQL or in the query scope**, not in the UI.

## Stage 5 — Secret egress

Grep the changed surface, and separately the built bundle, for token-shaped and
key-shaped strings. Confirm:

- no token in any API response,
- no token or key in any log line or error string,
- no token in the frontend bundle,
- no token in `./records/`, `./docs/`, or a test fixture.

## Stage 6 — Outbound and SSRF

- Operator-supplied panel URLs pass the SSRF guard.
- Scheme restricted to http/https.
- Loopback / private / link-local / metadata ranges refused.
- Redirects to a prohibited destination refused.
- Timeout bounded (10 s); retries on `GET` only.

## Stage 7 — Integrity and honesty

- Quota and provisioning use transactions/locks; no double-spend under
  concurrency.
- Audit log covers every privileged action, and is append-only.
- Failure states are explicit: no fabricated `REVOKED`, no fake success, no
  silent `200`.
- Release never deletes a panel client.

## Stage 8 — Report findings only

Produce a table:

| Severity | Finding | Evidence (file / route) | Suggested fix |
|---|---|---|---|

Severity: **P0** (secret or bypass), **P1** (wrong access), **P2** (weakness or
missing control), **P3** (hardening).

Rules for the report:

- Every row cites a real file, route or command output.
- "Cannot verify" is a valid result — say what blocked it.
- Do not fix anything in this pass.
- Do not report a control as present because documentation says so; check the
  code.

Archive the report under `./records/` with a dated, subject-bearing filename,
then append a one-line summary to `./docs/cline/progress.md`.
