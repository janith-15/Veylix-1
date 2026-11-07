# 2026-11-07 — GitHub MCP: switched to PAT-with-placeholder, disabled

**Scope:** the user-level MCP settings file ONLY. No application source, no
database, no `var/`, no `.veylix-secrets/`, no Git operation. No credential was
written.

Supersedes `./records/2026-11-07-github-mcp-oauth-config.md` (that record stays
as written; it is history).

---

## Why the earlier config was replaced

The owner supplied an OAuth App pair. The **client secret is now dead.** Live
evidence, run this session:

| Test | Result |
|---|---|
| `POST https://github.com/login/oauth/access_token` with `client_id=Ov23liWpWahs7rga9AlE`, the supplied secret, `code=aaaaaaaaaaaaaaaaaaaa` | `200 {"error":"incorrect_client_credentials"}` |
| Control: same call with a deliberately fake secret | `200 {"error":"incorrect_client_credentials"}` — **identical** |
| `POST https://api.githubcopilot.com/mcp/readonly` (`initialize`, no auth) | `401`, `No access token was provided in this request` |

The control row is decisive: the supplied secret now behaves **exactly like a
wrong secret**. The previous record observed `bad_verification_code` for this
pair; it now returns `incorrect_client_credentials`, so the secret has been
rotated out or never belonged to this App. **It is unusable.**

Second, independent blocker: a GitHub OAuth App is a **confidential client**. The
`/mcp/readonly` token exchange requires `client_secret`. Cline exposes no
supported field for a client secret in `cline_mcp_settings.json`, and
`oauthClient` appears nowhere in Cline's published MCP documentation — so the
previous `oauthClient.clientId` block was never a supported configuration. It
was recorded as `UNVERIFIED` at the time and is now removed.

---

## The change

`C:\Users\Dhanu\.cline\data\settings\cline_mcp_settings.json` — the `github`
block only.

| Before | After |
|---|---|
| `oauthClient.clientId: Ov23liWpWahs7rga9AlE` | **removed** |
| `disabled: false` | **`disabled: true`** |
| `headers`: toolsets + readonly only | **`Authorization: Bearer PASTE_YOUR_FINE_GRAINED_PAT_HERE`** added |
| `url`, `type`, both `X-MCP-*` headers | unchanged |

The `Authorization` value is a **marked placeholder**. No token was invented, and
no token was requested or received in chat. The owner inserts the real
fine-grained PAT locally, then flips `disabled` to `false`.

Backup, written before the edit:

```
cline_mcp_settings.json.bak-patoption-20261107-014711
pre-edit SHA-256: 76D01AC00C21C1232082CC4AB1611C4FD5E0DF774B4DCD81689FEC169A62099E
```

---

## Post-edit verification (actually run)

| # | Check | Result |
|---|---|---|
| — | JSON still parses | **PASS** — servers: `context7, playwright, github` |
| 1 | `oauthClient` absent | **PASS** — `True` |
| 1 | `Ov23liWpWahs7rga9AlE` absent | **PASS** — `True` |
| 2 | `url` exactly `https://api.githubcopilot.com/mcp/readonly` | **PASS** |
| 3 | `type` exactly `streamableHttp` | **PASS** |
| 4 | `X-MCP-Toolsets: context,repos,issues,pull_requests` | **PASS** |
| 4 | `X-MCP-Readonly: true` | **PASS** |
| 5 | `Authorization` equals the placeholder exactly | **PASS** |
| 6 | `disabled` is `true` | **PASS** |
| 8 | `context7` JSON-normalised before/after compare | **PASS** — exact match |
| 8 | `playwright` JSON-normalised before/after compare | **PASS** — exact match |
| 9 | Backup present | **PASS** |
| — | Dead secret `5569e85f` not on disk | **PASS** — absent |
| — | No real token shape on disk (`ghp_`/`github_pat_`/`gho_`/`ghu_`/`ghs_`/`ghr_`) | **PASS** — absent |

---

## What is NOT verified — stated plainly

- **The GitHub MCP server is `DISABLED` with a placeholder credential.** No
  `initialize`, no `tools/list`, no tool call was attempted. Per the owner's
  instruction, no authentication or connection was tried while the placeholder
  remains.
- The placeholder is **not** a valid token; if `disabled` were flipped to `false`
  as-is, the server would fail with `401 invalid_token`. That is expected and is
  the intended safe state.
- Whether the owner's eventual PAT carries the scopes the four requested
  toolsets need is **unverified** until the first `tools/list`.

## Owner steps (manual, in this order)

1. GitHub -> *Settings -> Developer settings -> Personal access tokens ->
   Fine-grained tokens -> Generate new token*.
   - **Repository access:** only the repositories you need read access to.
   - **Permissions:** `Metadata: Read` (mandatory) + `Contents: Read`,
     `Issues: Read`, `Pull requests: Read` for the enabled toolsets.
   - **Expiration:** set one (90 days is sensible).
2. Open `C:\Users\Dhanu\.cline\data\settings\cline_mcp_settings.json` **or**
   Cline -> MCP Servers -> `github` -> edit, and replace
   `PASTE_YOUR_FINE_GRAINED_PAT_HERE` with the token. Keep the `Bearer ` prefix.
3. Set `"disabled": false` in the same `github` block.
4. Reload the VS Code window so Cline re-reads the file.
5. Verify: the `github` server lists tools. Only then may it be recorded
   `ACTIVE` in `./docs/TOOLING_STATUS.md`.
6. Delete the old OAuth App (or at minimum confirm its secret was rotated), since
   the old secret was pasted into chat and is now dead.

**Do not** paste the token into this repository, into `./docs/`, into a record,
or into chat.
