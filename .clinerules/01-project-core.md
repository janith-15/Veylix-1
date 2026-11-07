# 01 · VEYLIX — PROJECT CORE

This file is always loaded. It is the routing layer, not the specification.
Depth lives in `.clinerules/skills/`; the specification lives in
`./MASTER_PRODUCT_BIBLE.md`.

## What this product is

VEYLIX is a **self-hosted VPN reseller platform**. One Go binary serves both the
API and the React interface. **3X-UI is the source of truth for every VPN** —
Veylix mirrors it and never invents it.

Three roles: **Super Admin** (platform owner) · **Admin** (reseller with a
quota) · **Client** (buys or claims).

## Stack (re-verify before relying on a version)

Go (`./backend/go.mod`) · React + TypeScript + Vite (`./frontend/package.json`) ·
SQLite via `modernc.org/sqlite` (pure Go, no CGO) · 3X-UI panel over HTTP Bearer
auth. Frontend is built and **embedded** into the binary; Node never runs in
production. Target host: 1 vCPU / 500 MB.

## Source-of-truth order

When two documents disagree, the earlier entry wins:

1. **`./MASTER_PRODUCT_BIBLE.md`** — business rules and product requirements.
2. **`./docs/DECISIONS.md`** — every owner-approved decision (append-only).
3. **`./docs/ARCHITECTURE.md`** — layers and dependency direction.
4. **`./docs/3XUI_API_CONTRACT.md`** — the panel contract Veylix codes against.
5. **The code and its tests** — the only authority on what actually happens.

A document never overrides code evidence. If they disagree, that is a finding to
report — not a fact to pick a side on.

## Documentation map (read the row, not the tree)

All paths are **repository-relative under `./docs/`**. This matters: the
directory `./veylix-ui demo/docs/` is a **separate, unrelated** documentation
tree for a different app. Never cite it for Veylix behaviour.

| Question | Canonical file |
|---|---|
| What does the product require? | `./MASTER_PRODUCT_BIBLE.md` |
| Where is a document? | `./docs/DOC_INDEX.md` |
| Why was it built this way? | `./docs/DECISIONS.md` |
| How are the code layers split? | `./docs/ARCHITECTURE.md` |
| Which endpoint exists? | `./docs/API_CONTRACT.md` |
| What does the panel expose? | `./docs/3XUI_API_CONTRACT.md` |
| What works and what does not? | `./docs/FINAL_REPORT.md`, `./docs/DISCREPANCY_REGISTER.md` |
| What requirement is unmet? | `./docs/GAP_ANALYSIS.md` |
| Which defect was fixed, and how? | `./docs/IMPROVEMENT_LEDGER.md`, `./VPN_FIX_LEDGER.md` |
| What was actually executed? | `./docs/QA_REPORT.md`, `./records/` |
| How do I run it locally? | `./docs/LIVE_PREVIEW.md` |
| How do I install / upgrade / back up? | `./docs/INSTALL.md`, `UPGRADE.md`, `BACKUP_RESTORE.md`, `DOMAIN_CHANGE.md` |
| How does sign-in work? | `./docs/AUTH.md` |
| How is work sequenced? | `./docs/SDLC_PLAN.md`, `./docs/IMPLEMENTATION_PLAN.md` |
| What is active right now? | `./docs/cline/activeContext.md` |
| What happened in past sessions? | `./docs/cline/progress.md` |
| Which MCP server or workflow should I use? | `.clinerules/06-tooling-and-routing.md` |
| What is the state of each MCP server? | `./docs/TOOLING_STATUS.md` |

## One home per fact

`./docs/DOC_INDEX.md` requires exactly one home per fact. Honour it:

- endpoints → `./docs/API_CONTRACT.md`
- decisions → `./docs/DECISIONS.md`
- defects → the two ledgers
- executed evidence → `./docs/QA_REPORT.md` and `./records/`
- product requirements → `./MASTER_PRODUCT_BIBLE.md`

New files **link** to those homes. They never copy their content.

## Glossary — the words that carry rules

- **VPN** — a 3X-UI *client*. Not a server, not an inbound.
- **FREE / PREMIUM** — a **visibility tag**, stored locally, never written to the
  panel. Not an account tier.
- **vpn_instances** — a **mirror** of a confirmed panel client, never an intent row.
- **Release** — removal from Veylix only. It **never** deletes a panel client.
- **Quota** — a reseller's sellable slots, counted in-transaction.
- **Panel wins** — on any disagreement between Veylix and 3X-UI, the panel is right.

## Non-negotiables

1. Audit before changing anything. Never rewrite from scratch for preference.
2. Preserve working behaviour, user data and existing conventions.
3. Never report a feature done because a page or button exists.
4. Never claim a command ran when it did not run.
5. Never claim remote success from local database state alone.

## Session protocol

1. Read `./docs/cline/activeContext.md` and the tail of `./docs/cline/progress.md`.
2. State the change scope **before** editing.
3. Load the Skill that matches the work (see below) before touching that layer.
4. On finishing, append to `./docs/cline/progress.md` and rewrite
   `./docs/cline/activeContext.md`.

## Which Skill to load

| Work touches | Skill |
|---|---|
| package placement, new layer, dependency direction | `veylix-architecture` |
| auth, sessions, secrets, RBAC, tokens, SSRF | `veylix-security` |
| `internal/panel`, `integrations`, `provisioning`, `sync` | `veylix-3xui-integration` |
| migrations, schema, invariants, transactions | `veylix-database-engineering` |
| anything under `frontend/src` | `veylix-ui-ux` |
| installer, upgrade, CLI, Caddy/systemd, backup, deploy scripts | `veylix-deployment` |
| writing tests, or about to claim "done" | `veylix-testing-review` |

## Which Workflow to load

| Situation | Workflow (`.clinerules/workflows/`) |
|---|---|
| new capability | `feature-development.md` |
| understanding or planning a subsystem before building it | `system-design-audit.md` |
| installer, upgrade, CLI, deploy templates, uninstall | `installation-and-deployment.md` |
| a defect in existing behaviour | `bug-fixing.md` |
| security audit or auth/ownership/token change | `security-review.md` |
| correctness review: leaks, boundaries, compatibility | `code-review.md` |
| resource use, bundle size, query plans | `performance-verification.md` |
| release, fresh install, first-run | `release-readiness.md` |

`system-design-audit.md` is read-only and **ends at an owner approval gate**;
implementation starts only after approval. `release-readiness.md` Stage 5b owns
the single fresh-install check list — no other file keeps a copy of it.

Tooling choice (which MCP server, and the Playwright-vs-`verify-*.mjs`
boundary) is in `./.clinerules/06-tooling-and-routing.md`.


## Environment facts for this workspace

- **There is no Git repository.** Do not run `git init`, and do not claim a
  `git diff` you did not see. Review changes by reading the files.
- There is **no Memory Bank** in Cline 4.1.26. The two files under
  `./docs/cline/` are the memory, and they are plain Markdown — you must read
  them yourself.
- Documented historical investigations exist as **R1–R7** (architecture,
  frontend, backend, database, API/3X-UI, security-deployment-performance,
  documentation-roadmap). Treat them as **historical context only**:
  re-check the current source before acting on any of it, and never invent a
  finding or assume an old value is still current.
