# 06 · TOOLING AND ROUTING

Always loaded. Deliberately short — a routing table, not a policy document.
Depth lives in the workflows and skills it points at.

Purpose: decide **which tool** answers a question, so an agent does not reach for
a heavyweight mechanism when a cheap one already exists.

## MCP servers available here

| Server | Use it for | Never use it for |
|---|---|---|
| **context7** | looking up current library/framework API docs and usage patterns | Veylix behaviour — the code and `./docs/` are the only authority on that |
| **playwright** | interactive, exploratory browser work: reproduce a UI defect, read an accessibility snapshot, confirm what a page actually renders | proving a fix. It is not a gate. See below. |
| **github** | read-only research on upstream repositories, issues and pull requests | any write action — it is configured read-only and is currently **disabled** (see `./docs/TOOLING_STATUS.md`) |

Context7 is a remote streamableHttp endpoint with no credentials. Playwright runs
`--headless --isolated --browser chrome`: no browser download, no persisted
profile, and **never** pointed at a production Veylix service or the seeded
development panel.

## The browser test rule (the one that gets confused)

Two browser mechanisms exist. **They do not overlap.**

| Mechanism | Role | Counts as a gate? |
|---|---|---|
| `frontend/scripts/verify-*.mjs` (`npm run verify:*`) | committed, deterministic, asserts specific facts, exits non-zero on failure | **Yes** — the only browser regression proof |
| Playwright MCP `browser_*` tools | exploratory investigation by an agent | **No** — no reproducible assertion |

Use Playwright MCP to **find** the problem; commit a `verify-*.mjs` to **prove**
it is fixed. Reporting a Playwright session as `PASS` for browser coverage is a
false claim.

## Which skill to load

| Work touches | Skill |
|---|---|
| package placement, new layer, dependency direction | `veylix-architecture` |
| auth, sessions, secrets, RBAC, tokens, SSRF | `veylix-security` |
| `internal/panel`, `integrations`, `provisioning`, `sync` | `veylix-3xui-integration` |
| migrations, schema, invariants, transactions | `veylix-database-engineering` |
| anything under `frontend/src` | `veylix-ui-ux` |
| installer, upgrade, CLI, Caddy/systemd, backup, deploy scripts | `veylix-deployment` |
| writing tests, or about to claim "done" | `veylix-testing-review` |

## Which workflow to run

| Situation | Workflow |
|---|---|
| new capability | `workflows/feature-development.md` |
| **understanding or planning a subsystem before building it** | **`workflows/system-design-audit.md`** |
| **installer, upgrade, CLI, deploy templates, uninstall** | **`workflows/installation-and-deployment.md`** |
| a defect in existing behaviour | `workflows/bug-fixing.md` |
| security audit, or a change to auth/ownership/tokens/panel | `workflows/security-review.md` |
| reviewing a change for correctness, leaks, boundaries, compatibility | `workflows/code-review.md` |
| resource use, bundle size, query plans, 1 vCPU / 500 MB | `workflows/performance-verification.md` |
| building a release, or a fresh install / first-run | `workflows/release-readiness.md` |

## How the workflows fit together

```
system-design-audit  →  APPROVAL GATE  →  feature-development / installation-and-deployment
      (read-only)        (owner)              (build + record)
                                                    ↓
                          release-readiness  ←  code-review · security-review
                                                    
   performance-verification runs alongside any change that could move resource use.
```

- `system-design-audit.md` is **read-only** and **ends at an approval gate**. It
  answers "what exists, what is missing, what would we build, in what order".
  Nothing is implemented in that workflow.
- `installation-and-deployment.md` executes an approved installation design. It
  is the only workflow that can destroy a customer's database, so it carries the
  data-loss checks and the `--dry-run`-first rule.
- `release-readiness.md` **owns the fresh-install check list** (Stage 5b). No
  other file may keep a second copy of it. `installation-and-deployment.md`
  points at it rather than restating it.

`security-review.md` and `code-review.md` are both **read-only**: they produce
findings, and fixes go through `bug-fixing.md`. Security owns the threat model;
code review owns efficiency, leaks, duplication, boundary drift and
compatibility. Data-loss risk is owned by `system-design-audit.md` Stage 5 and
`installation-and-deployment.md` Stage 1 — it is not a security sub-category.


## Precedence when tools disagree

1. The code and its tests.
2. `./MASTER_PRODUCT_BIBLE.md`.
3. `./docs/DECISIONS.md` (append-only — super-sede, never rewrite).
4. Everything else, including any MCP server's output.

An MCP server's answer is **evidence to verify**, never an authority over the
source tree.
