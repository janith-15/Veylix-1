# 2026-11-07 — GitHub MCP: lost `headers` block restored, `/readonly` URL restored

**Scope:** the user-level MCP settings file ONLY. No application source, no
database, no `var/`, no `.veylix-secrets/`, no Git operation (there is no Git
repository here). **No credential was written.**

Supersedes `./records/2026-11-07-github-mcp-owner-shape-rewrite.md` for the
current content of the `github` block. That record stays as written.

---

## The reported symptom

"GitHub MCP is not working again." Investigation, run this session.

## Root cause — both headers AND the read-only URL were gone

The settings file had been **rewritten at 22:55:21** (size dropped 836 -> 647
bytes) and the `github` block had lost its entire `headers` object:

```json
"github": {
  "transport": {
    "type": "streamableHttp",
    "url": "https://api.githubcopilot.com/mcp/"
  }
}
```

Confirmed by inspection, not inference:

| Check | Observed |
|---|---|
| `github` keys | `transport` only — **no `headers` key at all** |
| `headers` property present | `False` |
| `Authorization` present | `False` |
| `Bearer` value length | **0** |
| `transport.url` | `https://api.githubcopilot.com/mcp/` — **`/readonly` missing** |

Two distinct defects:

1. **`Authorization` header dropped.** No token means no auth. Live probe
   confirmed the endpoint's own words: `401 invalid_request`,
   `"No access token was provided in this request"`.
2. **`/readonly` dropped from the URL.** Even with a valid token, the request
   would have hit the read-write MCP surface instead of the read-only one. This
   is a **scope-safety** regression, independent of the auth failure, and it
   would have been easy to miss because the server was failing anyway.

### Why the rewrite happened

The `github` block was normalised to Cline's own shape, where `type` moves inside
a `transport` object (identical in form to the `context7` entry). That
normalisation is benign. The destructive part is that **the `headers` object did
not survive the write**. The likely author is a save from Cline's MCP UI. Recorded
as an observation, not a proven cause.

---

## The change

`C:\Users\Dhanu\.cline\data\settings\cline_mcp_settings.json` — the
`mcpServers.github` entry only.

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

Four repairs: `headers` restored; `Authorization` placeholder restored;
`/readonly` appended to the URL; `timeout: 60` and `disabled: false` restored.
`context7` and `playwright` untouched.

Backup, written before the edit:

```
cline_mcp_settings.json.bak-prefixfix-20261107-230213   (647 bytes)
pre-edit  SHA-256: 480365F3C7BD1146D528A5E97A80A06D629802409D5C3007D8074BADA93CAEFE
post-edit SHA-256: 3404D843C59A5C4F0358ABA1D6F18A0EFB15C9C8876E24CE0A48B99B8E21110D
```

---

## Post-edit verification (actually run this session — 18 checks, all PASS)

| # | Check | Result |
|---|---|---|
| 1 | JSON parses | **PASS** |
| 2 | Server list `context7, playwright, github` | **PASS** |
| 3 | `github.headers` **exists** (was absent) | **PASS** |
| 4 | `github.transport.type` == `streamableHttp` | **PASS** |
| 5 | `github.transport.url` == `…/mcp/readonly` | **PASS** |
| 6 | URL ends with `/readonly` | **PASS** |
| 7 | `Authorization` == `Bearer YOUR_REAL_PAT` | **PASS** |
| 8 | `X-MCP-Toolsets` == `context,repos,issues,pull_requests` | **PASS** |
| 9 | `X-MCP-Readonly` == `true` | **PASS** |
| 10 | `disabled` == `false` | **PASS** |
| 11 | `timeout` == `60` | **PASS** |
| 12 | Key order `transport > headers > disabled > timeout` | **PASS** |
| 13 | `context7` JSON-normalised unchanged | **PASS** — exact match |
| 14 | `playwright` JSON-normalised unchanged | **PASS** — exact match |
| 15 | No `ghp_`/`github_pat_`/`gho_`/`ghu_`/`ghs_`/`ghr_` shape on disk | **PASS** — absent |
| 16 | `oauthClient` absent | **PASS** |
| 17 | Dead client ID, dead secret `5569e85f`, leaked-token prefix all absent | **PASS** |
| 18 | Leaked-token body fragment absent | **PASS** |

## Differential probe — the evidence that the fix is real

Two live `initialize` calls against `https://api.githubcopilot.com/mcp/readonly`,
differing only in whether the `Authorization` header is sent:

| State | Response | Interpretation |
|---|---|---|
| **Before** — no `Authorization` | `401` `error="invalid_request"`, `"No access token was provided in this request"` | the token was never transmitted |
| **After** — configured placeholder sent | `400` `error="invalid_token"`, `"Invalid token"` | **the header is now transmitted and evaluated by the endpoint** |

The change from "no token was provided" to "invalid token" is the proof that the
`Authorization` header is now on the wire. `invalid_token` for a placeholder is
the correct and expected result.

---

## What is NOT verified — stated plainly

- **The GitHub MCP server is NOT ACTIVE and does not work yet.** `tools/list`
  has never returned tools. `YOUR_REAL_PAT` is a placeholder, not a credential.
- **The config is now correct; the credential is not.** Everything an agent can
  fix without holding a secret has been fixed. What remains needs a real token.
- Whether Cline honours `transport` + sibling `headers` at this level for a
  `streamableHttp` entry is **UNVERIFIED** until a real token is in place and
  Cline's MCP panel is reloaded. The shape deliberately mirrors `context7`, which
  is known-good, but that is inference, not evidence.
- Whether the eventual PAT carries the scopes the four enabled toolsets need is
  unverified until the first successful `tools/list`.

## Data-safety note

The previously leaked PAT (`github_pat_11CQVHXWI0…`, pasted into chat) was never
written to this file, to any repository file, to `./docs/`, or to `./records/`.
Checks 17 and 18 above confirm no fragment of it is on disk. It must still be
revoked on GitHub by the owner.

## Owner steps (in this order, in a text editor — not the Cline UI)

1. **Revoke** the leaked PAT on GitHub (*Settings -> Developer settings ->
   Personal access tokens -> Fine-grained tokens*), then mint a new one.
   - **Repository access:** only what is needed.
   - **Permissions:** `Metadata: Read` (mandatory) plus `Contents: Read`,
     `Issues: Read`, `Pull requests: Read`.
   - **Expiration:** set one (90 days is sensible).
2. Open `C:\Users\Dhanu\.cline\data\settings\cline_mcp_settings.json` in **VS
   Code or Notepad** and replace `YOUR_REAL_PAT` with the token. **Keep the
   `Bearer ` prefix and the surrounding quotes.**
3. Save, then `Ctrl+Shift+P` -> **Developer: Reload Window**.
4. Confirm `tools/list` returns tools in Cline's MCP panel. **Only then** may
   `github` be recorded `ACTIVE` in `./docs/TOOLING_STATUS.md`.

**Do not re-save the `github` block from Cline's MCP UI.** The evidence in this
record is consistent with that UI having dropped the `headers` object once
already. Edit the file directly.

**Do not** paste the token into this repository, into `./docs/`, into a record,
or into chat. Rule 03 applies.

