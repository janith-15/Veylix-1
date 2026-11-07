# 2026-11-07 — GitHub MCP: `github` block rewritten to the owner-specified shape

**Scope:** the user-level MCP settings file ONLY. No application source, no
database, no `var/`, no `.veylix-secrets/`, no Git operation (there is no Git
repository here). **No credential was written.**

Supersedes `./records/2026-11-07-github-mcp-pat-placeholder.md` for the current
content of the `github` block. That record stays as written; it is history.

---

## The instruction

The owner supplied the exact block they wanted and asked for it to be applied
verbatim:

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

---

## The change

`C:\Users\Dhanu\.cline\data\settings\cline_mcp_settings.json` — the `mcpServers.github`
entry only. Exactly three deltas; nothing else touched.

| Field | Before | After |
|---|---|---|
| `Authorization` | `Bearer PASTE_YOUR_FINE_GRAINED_PAT_HERE` | `Bearer YOUR_REAL_PAT` |
| key order | `type, url, headers, disabled, timeout` | `type, url, disabled, headers` |
| `timeout` | `60` | **removed** — the owner's block does not contain it |

`type`, `url`, `X-MCP-Toolsets` and `X-MCP-Readonly` are unchanged. `context7`
and `playwright` are untouched, proven by JSON-normalised before/after compare.

Backup, written before the edit:

```
cline_mcp_settings.json.bak-yolopat-20261107-015917   (836 bytes)
pre-edit  SHA-256: 71FF212C0360BF744F86AA1E702F5249CC6DCE9B036A93C3799069C1C638C21D
post-edit SHA-256: 431AC73F771278BA4ED4BC0F1F0C4DC5A00F100596CBFEF7EE647A3F43BB83AB
```

---

## Post-edit verification (actually run this session)

| # | Check | Result |
|---|---|---|
| 1 | JSON still parses | **PASS** |
| 2 | Server list is `context7, playwright, github` | **PASS** |
| 3 | `github.type` == `streamableHttp` | **PASS** |
| 4 | `github.url` == `https://api.githubcopilot.com/mcp/readonly` | **PASS** |
| 5 | `github.disabled` == `false` | **PASS** |
| 6 | `Authorization` == `Bearer YOUR_REAL_PAT` | **PASS** |
| 7 | `X-MCP-Toolsets` == `context,repos,issues,pull_requests` | **PASS** |
| 8 | `X-MCP-Readonly` == `true` | **PASS** |
| 9 | `github.timeout` absent (owner's block omits it) | **PASS** |
| 10 | Key order `type > url > disabled > headers` | **PASS** |
| 11 | `context7` JSON-normalised unchanged | **PASS** — exact match |
| 12 | `playwright` JSON-normalised unchanged | **PASS** — exact match |
| 13 | Old placeholder `PASTE_YOUR_FINE_GRAINED_PAT_HERE` gone | **PASS** |
| 14 | No `ghp_`/`github_pat_`/`gho_`/`ghu_`/`ghs_`/`ghr_` shape on disk | **PASS** — absent |
| 15 | `oauthClient` absent | **PASS** |
| 16 | Dead client ID `Ov23liWpWahs7rga9AlE` absent | **PASS** |
| 17 | Dead secret `5569e85f` absent | **PASS** |

Live handshake probe with the configured headers (this session):

```
POST https://api.githubcopilot.com/mcp/readonly   (initialize)
  Authorization: Bearer YOUR_REAL_PAT
  X-MCP-Toolsets: context,repos,issues,pull_requests
  X-MCP-Readonly: true
-> 400
WWW-Authenticate: Bearer error="invalid_token", error_description="Invalid token"
```

---

## What is NOT verified — stated plainly

- **The GitHub MCP server is NOT ACTIVE and does not work.** `tools/list` has
  never returned tools. The live probe above returns `invalid_token`.
- `YOUR_REAL_PAT` is a **placeholder**. It is not a token, it is not logged, and
  it is not a secret. The server fails with `invalid_token`, which is the
  correct and expected outcome for a placeholder.
- Behaviour change worth noting: because `disabled` is now `false` (it was
  `true` under the previous record), Cline **will attempt to connect** on reload
  and will surface the server as failing. The previous state failed silently by
  being disabled. Both are safe; the visible-failure state is the one the owner
  asked for.
- Whether the eventual PAT carries the scopes the four enabled toolsets need is
  unverified until the first successful `tools/list`.

## Owner step (one line, then reload)

1. Mint a **fine-grained read-only** PAT:
   *Settings -> Developer settings -> Personal access tokens -> Fine-grained
   tokens -> Generate new token*.
   - **Repository access:** only what is needed.
   - **Permissions:** `Metadata: Read` (mandatory) plus `Contents: Read`,
     `Issues: Read`, `Pull requests: Read`.
   - **Expiration:** set one (90 days is sensible).
2. Replace `YOUR_REAL_PAT` with the token. **Keep the `Bearer ` prefix.**
3. Reload the VS Code window so Cline re-reads the settings file.
4. Confirm `tools/list` returns tools. **Only then** may `github` be recorded
   `ACTIVE` in `./docs/TOOLING_STATUS.md`.
5. Delete the old OAuth App (or confirm its secret was rotated) — that secret was
   pasted into chat and is now dead.

**Do not** paste the token into this repository, into `./docs/`, into a record,
or into chat. Rule 03 applies.
