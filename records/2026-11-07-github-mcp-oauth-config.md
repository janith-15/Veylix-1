# 2026-11-07 — GitHub MCP: OAuth client config applied

**Scope:** the user-level MCP settings file ONLY. No application source, no
database, no `var/`, no `.veylix-secrets/`, no Git operation.

**File changed (outside the repository, not committed):**

```
C:\Users\<user>\.cline\data\settings\cline_mcp_settings.json
```

Backup written first:

```
cline_mcp_settings.json.bak-oauth-20261107-014038
pre-edit SHA-256: 82E1456EBD69E48A0810859799211D9C884778764FCDA147C6C0BB51F6445331
```

**Change:** three lines added to the existing `github` entry, nothing else.

```json
"oauthClient": {
  "clientId": "Ov23liWpWahs7rga9AlE"
}
```

`clientId` is public by design. **No `clientSecret`, no `Authorization` header,
no token was written.**

---

## Credential triage (performed before any edit)

The owner supplied two values. They were identified, not trusted:

| Value supplied | Shape | Test | Result |
|---|---|---|---|
| `Ov23liWpWahs7rga9AlE` | GitHub OAuth **App client ID** | `Authorization: Bearer <value>` -> `GET https://api.github.com/user` | `401` — **not** an access token |
| `5569e85f…398d95` (40 hex) | OAuth **client secret** | `Authorization: Bearer <value>` -> `GET https://api.github.com/user` | `401` — **not** an access token |
| the pair together | OAuth client credentials | `POST https://github.com/login/oauth/access_token`, `code=ov23li0000000000000000` | `200` + `{"error":"bad_verification_code"}` -> **the App exists and the secret matches the client ID** |

The third row is the decisive one: a mismatched pair returns
`incorrect_client_credentials`, not `bad_verification_code`. So the App is real
and Cline's authorise step can use it.

`5569e85f` does **not** match GitHub's current `ghs_…` / `ghu_…` secret formats.
It was therefore **not** written anywhere, and it is reported as needing manual
rotation because it may not be revocable from the OAuth App page.

---

## Post-edit verification (actually run)

| Check | Command / method | Result |
|---|---|---|
| JSON still parses | `Get-Content -Raw \| ConvertFrom-Json` | **PASS** — 3 servers: `context7,playwright,github` |
| `github.oauthClient.clientId` present | read back property | **PASS** — `Ov23liWpWahs7rga9AlE` |
| `github.url` unchanged | read back property | **PASS** — `https://api.githubcopilot.com/mcp/readonly` |
| `context7` unchanged | JSON-normalised before/after compare | **PASS** — exact match |
| `playwright` unchanged | JSON-normalised before/after compare | **PASS** — exact match |
| Secret not on disk | substring scan for `5569e85f` | **PASS** — absent |
| No bearer header on disk | substring scan for `Authorization` | **PASS** — absent |
| Handshake with client ID as a token | `POST …/mcp/readonly` + `Bearer <clientId>` | `400` `invalid_token` — expected; a client ID is not a token, confirming the OAuth exchange is genuinely required |

---

## What is NOT verified — stated plainly

- **The GitHub MCP server is NOT ACTIVE.** A successful `initialize` +
  `tools/list` has not been observed. It is `PENDING_OAUTH`.
- Full end-to-end Discord-style OAuth handshake was not exercised: that needs the
  callback URL registered on the GitHub App and a human clicking Authorise in
  Cline's MCP UI.
- Whether Cline 4.1.26 honours `oauthClient.clientId` in
  `cline_mcp_settings.json` for a `streamableHttp` entry — **UNVERIFIED** until
  the UI is reloaded and the Authorise link appears. If it does not, the
  fallback is agent-external (see `docs/TOOLING_STATUS.md`).

## Still requires the owner

1. GitHub -> *Settings -> Developer settings -> OAuth Apps -> (this App)*:
   set **Authorization callback URL** to exactly
   `http://localhost:1456/mcp/oauth/callback`.
2. Reload the VS Code window so Cline re-reads the settings file.
3. Cline -> MCP Servers -> `github` -> **Authorise**, then approve on GitHub.
4. **Rotate the client secret** that was pasted into chat.
