# TOOLING STATUS — MCP SERVERS

**State of every MCP server configured for this workspace.** One home for this
fact. Routing rules are in `./.clinerules/06-tooling-and-routing.md`; this file
records *status and blockers only*.

MCP configuration lives at the **user level**, outside this repository:

```
C:\Users\<user>\.cline\data\settings\cline_mcp_settings.json
```

It is not committed, and it must never contain a credential written by an agent.

---

## Current state (supersedes every section below — 2026-11-07, headers restored)

**Added 2026-11-07.** Config was found rewritten with its `headers` object and
its `/readonly` path **both missing**, which is why the server stopped working.
Both restored. Evidence: `./records/2026-11-07-github-mcp-headers-restored.md`.
Append-only: every earlier section is retained as history.

| Server | Transport | Status | Verified how |
|---|---|---|---|
| **context7** | `streamableHttp` -> `https://mcp.context7.com/mcp` | **ACTIVE** (unchanged) | JSON-normalised before/after compare: exact match |
| **playwright** | `stdio` -> `npx --yes @playwright/mcp@latest …` | **ACTIVE** (unchanged) | JSON-normalised before/after compare: exact match |
| **github** | `streamableHttp` -> `https://api.githubcopilot.com/mcp/readonly` | **CONFIG CORRECT — placeholder credential.** `headers` restored, `/readonly` restored, `disabled: false`. Azure still rejects the placeholder. | Differential live probe: without `Authorization` -> `401 "No access token was provided"`; with the configured placeholder -> `400 "Invalid token"`. The header is now on the wire. **`tools/list` never succeeded. Not claimed ACTIVE.** |

The `github` block now reads:

```json
"github": {
  "transport": {
    "type": "streamableHttp",
    "url": "https://api.githubcopilot.com/mcp/readonly"
  },
  "headers": {
    "Authorization": "Bearer YOUR_REAL_PAT",
    "X-MCP-Toolsets": "context,repos,issues,pull_requests",
    "X-MCP-Readonly": "true"
  },
  "disabled": false,
  "timeout": 60
}
```

**Two defects were repaired, not one.** The missing `Authorization` header is the
obvious one. The missing `/readonly` in the URL is the dangerous one: it would
have pointed a valid token at the **read-write** MCP surface. Do not "simplify"
that path away.

18 checks PASS, including `context7` and `playwright` JSON-normalised unchanged,
and no token shape on disk. Backup:
`cline_mcp_settings.json.bak-prefixfix-20261107-230213` (pre-edit SHA-256
`480365F3C7BD1146D528A5E97A80A06D629802409D5C3007D8074BADA93CAEFE`,
post-edit SHA-256
`3404D843C59A5C4F0358ABA1D6F18A0EFB15C9C8876E24CE0A48B99B8E21110D`).

**Edit the file directly, not via Cline's MCP UI.** The evidence is consistent
with a UI save having dropped the `headers` object once already.

**Remaining owner step:** revoke the PAT that was pasted into chat, mint a new
fine-grained read-only PAT, replace `YOUR_REAL_PAT` in the file by hand (keep the
`Bearer ` prefix), reload the VS Code window, confirm `tools/list` returns tools.
**Only then** may this row be recorded `ACTIVE`.

---

## Current state (supersedes every section below — 2026-11-07, owner-requested shape)

**Added 2026-11-07.** The owner asked for an explicit `github` block shape.
Applied to the user-level settings file only; append-only, so every earlier
section is retained as history.

| Server | Transport | Status | Verified how |
|---|---|---|---|
| **context7** | `streamableHttp` -> `https://mcp.context7.com/mcp` | **ACTIVE** (unchanged) | JSON-normalised before/after compare: exact match |
| **playwright** | `stdio` -> `npx --yes @playwright/mcp@latest …` | **ACTIVE** (unchanged) | JSON-normalised before/after compare: exact match |
| **github** | `streamableHttp` -> `https://api.githubcopilot.com/mcp/readonly` | **BROKEN — placeholder credential.** `disabled: false`, so Cline attempts to connect and the endpoint rejects it. | Live `POST …/mcp/readonly` `initialize` with the configured headers returned **`400` `error="invalid_token", error_description="Invalid token"`**. No `tools/list`, no successful handshake. **Not claimed ACTIVE.** |

The `github` block now reads exactly as the owner specified:

```json
"github": {
  "type": "streamableHttp",
  "url": "https://api.githubcopilot.com/mcp/readonly",
  "disabled": false,
  "headers": {
    "Authorization": "Bearer YOUR_REAL_PAT",
    "X-MCP-Toolsets": "context,repos,issues,pull_requests",
    "X-MCP-Readonly": "true"
  }
}
```

`YOUR_REAL_PAT` is **a marked placeholder, not a credential** — the same safe
state as the previous `PASTE_YOUR_FINE_GRAINED_PAT_HERE`, with a different
string. `disabled` is `false`, so unlike the previous state Cline **will**
attempt a connection and show the server as failing. That is the expected
consequence of the requested shape, not a new defect.

Post-edit verification actually run (17 checks, all PASS): JSON parses; three
servers present; `github` `type`/`url`/`disabled`/three headers exact; the
previously present `timeout: 60` removed because the owner's block omits it;
`context7` and `playwright` JSON-normalised unchanged; old placeholder absent;
no `ghp_`/`github_pat_`/`gho_`/`ghu_`/`ghs_`/`ghr_` shape on disk; `oauthClient`,
the dead client ID `Ov23liWpWahs7rga9AlE` and the dead secret `5569e85f` all
absent. Backup:
`cline_mcp_settings.json.bak-yolopat-20261107-015917` (pre-edit SHA-256
`71FF212C0360BF744F86AA1E702F5249CC6DCE9B036A93C3799069C1C638C21D`,
post-edit SHA-256
`431AC73F771278BA4ED4BC0F1F0C4DC5A00F100596CBFEF7EE647A3F43BB83AB`).

**Remaining owner step is one line:** replace `YOUR_REAL_PAT` with a real
fine-grained read-only PAT (keep the `Bearer ` prefix), then reload the VS Code
window and confirm `tools/list` returns tools. **Only then** may this row be
recorded `ACTIVE` — the entry existing is not evidence.

---

## Current state (supersedes both rows above — 2026-11-07, later still)

**Added 2026-11-07.** The `oauthClient` approach was **abandoned**. Decision
`D-075` in `./docs/DECISIONS.md`; evidence
`./records/2026-11-07-github-mcp-pat-placeholder.md`. Append-only: the earlier
sections above are retained because they remain the correct account of the DCR
limitation and of the first attempt.

| Server | Transport | Status | Verified how |
|---|---|---|---|
| **context7** | `streamableHttp` -> `https://mcp.context7.com/mcp` | **ACTIVE** (unchanged) | JSON-normalised before/after compare after this session's edit: exact match |
| **playwright** | `stdio` -> `npx --yes @playwright/mcp@latest …` | **ACTIVE** (unchanged) | JSON-normalised before/after compare after this session's edit: exact match |
| **github** | `streamableHttp` -> `https://api.githubcopilot.com/mcp/readonly` | **DISABLED** — placeholder credential, waiting on the owner | `"disabled": true`, `oauthClient` removed, `Authorization: Bearer PASTE_YOUR_FINE_GRAINED_PAT_HERE`. No handshake attempted, per owner instruction. **Not claimed ACTIVE.** |

Why the OAuth route was dropped — two independent reasons, both confirmed this
session:

1. **The supplied OAuth App pair is dead.** `POST
   https://github.com/login/oauth/access_token` with
   `client_id=Ov23liWpWahs7rga9AlE` and the supplied secret returned
   `incorrect_client_credentials` — byte-identical to a control call using a
   deliberately fake secret. The earlier record saw `bad_verification_code` for
   the same pair, so the secret has been rotated out (or never belonged to this
   App). It cannot be used.
2. **Cline has no supported slot for an OAuth client secret.** A GitHub OAuth App
   is a confidential client and `/mcp/readonly` needs `client_secret` at token
   exchange. `oauthClient` does not appear in Cline's published MCP docs, so the
   previous `clientId`-only block was never a supported configuration.

No credential of any kind is on disk: `5569e85f` absent, and no
`ghp_`/`github_pat_`/`gho_`/`ghu_`/`ghs_`/`ghr_` token shape present. The
`Authorization` value is a **marked placeholder**, not a secret.

**Remaining owner steps:** mint a fine-grained read-only PAT; paste it over
`PASTE_YOUR_FINE_GRAINED_PAT_HERE` in the user-level settings (keep the `Bearer `
prefix); set `"disabled": false`; reload the VS Code window; confirm `tools/list`
returns tools; **only then** record it ACTIVE here. Also delete or re-check the
old OAuth App, whose secret was pasted into chat.

---

## Current state (supersedes the 2026-11-07 row above for `github` — see "D-note" below)

**Added 2026-11-07 (later same day).** The owner supplied a GitHub OAuth App
client ID, which lifts the blocker described in the next section without any
credential being written to disk. **Append-only:** the original BLOCKED analysis
is retained verbatim below because it remains the correct account of the DCR
limitation.

| Server | Transport | Status | Verified how |
|---|---|---|---|
| **context7** | `streamableHttp` -> `https://mcp.context7.com/mcp` | **ACTIVE** (unchanged) | JSON-normalised before/after compare after this session's edit: exact match |
| **playwright** | `stdio` -> `npx --yes @playwright/mcp@latest …` | **ACTIVE** (unchanged) | JSON-normalised before/after compare after this session's edit: exact match |
| **github** | `streamableHttp` -> `https://api.githubcopilot.com/mcp/readonly` | **PENDING_OAUTH** — no longer `DISABLED` | entry edited to add `"oauthClient": {"clientId": "Ov23liWpWahs7rga9AlE"}`; `disabled` set to `false`. Live `initialize` still returns `401`/`400 invalid_token` because no access token exists yet. **Not claimed ACTIVE.** |

Owner-supplied client ID provenance: verified by
`POST https://github.com/login/oauth/access_token` with a dummy code returning
`200 {"error":"bad_verification_code"}` (an invalid pair returns
`incorrect_client_credentials`), so the App is real. Evidence:
`./records/2026-11-07-github-mcp-oauth-config.md`.

No `clientSecret`, `Authorization` header or token exists in the settings file —
confirmed by substring scan. Full detail in the record above.

**Remaining owner actions:** register callback `http://localhost:1456/mcp/oauth/callback`
on the App, reload the VS Code window, then Authorise `github` in Cline's MCP UI.

---

## Current state (verified 2026-11-07, pre-OAuth — retained)

| Server | Transport | Status | Verified how |
|---|---|---|---|
| **context7** | `streamableHttp` -> `https://mcp.context7.com/mcp` | **ACTIVE** | present and unchanged since before this work; no credentials in its block |
| **playwright** | `stdio` -> `npx --yes @playwright/mcp@latest --headless --isolated --browser chrome --viewport-size 1280x720` | **ACTIVE** | MCP `initialize` returned `serverInfo {name:"Playwright", version:"1.65.0-alpha-2026-10-30"}`, 25 `browser_*` tools listed; end-to-end navigate/snapshot/click/evaluate smoke test passed |
| **github** | `streamableHttp` -> `https://api.githubcopilot.com/mcp/readonly` | **DISABLED** | endpoint probed: `401` with OAuth `resource_metadata`; blocker below |

The `github` entry is deliberately written with `"disabled": true`. It carries
**no credentials** — only the read-only endpoint and two header values that
*restrict* scope (`X-MCP-Toolsets: context,repos,issues,pull_requests`,
`X-MCP-Readonly: true`).

---

## GitHub MCP blocker — dynamic client registration (DCR)

**Status: BLOCKED. Not a configuration error; an upstream limitation.**

Observed evidence (2026-11-07):

1. `POST https://api.githubcopilot.com/mcp/readonly` returns
   `401` with
   `WWW-Authenticate: Bearer error="invalid_request",
   error_description="No access token was provided in this request",
   resource_metadata="https://api.githubcopilot.com/.well-known/oauth-protected-resource/mcp/readonly"`.
   -> The endpoint is live and read-only OAuth is the intended auth mode.
2. `GET https://api.githubcopilot.com/.well-known/oauth-protected-resource/mcp/readonly`
   returns `authorization_servers: ["https://github.com/login/oauth"]`,
   `bearer_methods_supported: ["header"]`, `resource_name: "GitHub MCP Server"`.
   -> OAuth discovery for the MCP resource works.
3. `GET https://github.com/.well-known/oauth-authorization-server/login/oauth`
   returns `authorization_endpoint`, `token_endpoint`,
   `device_authorization_endpoint`, `code_challenge_methods_supported:["S256"]`
   — **but no `registration_endpoint`**.

Cline 4.1.26's built-in remote-MCP OAuth flow performs **dynamic client
registration** unless a client is pre-registered. Because GitHub advertises no
registration endpoint, the automated flow cannot self-register. This is the whole
of the blocker.

**Resolved 2026-11-07 by Option A below** — the owner supplied a pre-existing
OAuth App client ID and it was added as `oauthClient.clientId`. The DCR analysis
above is kept because it explains *why* pre-registration is mandatory.

### What is required to enable it later

Choose one; **none is a step an agent may take alone**, and none may put a secret
in a repository file.

**Option A — pre-registered OAuth client (recommended).**
1. The owner registers a GitHub OAuth App / GitHub App (not an agent action).
2. Callback URL must be exactly `http://localhost:1456/mcp/oauth/callback`
   (Cline binds `1456`, `1457`, `1458`; `/mcp/oauth/callback` is its fixed path).
3. Add `"oauthClient": { "clientId": "..." }` to the `github` entry in the
   **user-level** settings, set `"disabled": false`, then authorise through
   Cline's UI.
4. A `clientSecret`, if the app requires one, is a production credential: handle
   it under Rule 03, never in a project file, never in chat.

**Option B — personal access token.**
Requires a literal `Authorization: Bearer <pat>` header in the user-level
settings file. This places a credential on disk and was **explicitly declined**
for this workspace. Do not do it without a fresh, explicit owner decision.

**Option C — leave disabled.**
The entry stays as-is and is inert. This is the current, accepted state.

---

## Rules that keep this file honest

1. Never store a token, PAT, client secret, cookie or key in this file or in
   `cline_mcp_settings.json` written by an agent.
2. Do not claim a server is ACTIVE because an entry exists. Record the handshake
   evidence or mark it `DISABLED` / `UNVERIFIED`.
3. `context7` must be preserved byte-for-byte. Any change to it is out of scope
   unless the owner asks for exactly that.
4. Adding a browser download, a Docker container, `mcp-remote`, or any long-lived
   service needs owner approval against the 1 vCPU / 500 MB target.
5. Playwright MCP is **exploratory only** — never a gate. See Rule 06.
