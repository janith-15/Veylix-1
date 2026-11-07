# 04 · CHANGE CONTROL

Always loaded. Source: `./MASTER_PRODUCT_BIBLE.md` §0.4, §17 and §17
"Mandatory behavior for every implementation task".

## Before editing

1. **State the change scope in writing**: the files you will touch and why.
   No scope, no edit.
2. Identify the canonical document that governs the change (see Rule 01's map).
3. Check `./docs/IMPROVEMENT_LEDGER.md` and `./VPN_FIX_LEDGER.md` — the work may
   already be recorded, attempted, or deliberately deferred.
4. For a bug, check `./docs/DISCREPANCY_REGISTER.md` for a known cause first.

## During editing

- Small, coherent changes. Do not overwrite unrelated work.
- Do not "clean up" code you were not asked to change.
- Keep the rule consistent across **all four** surfaces: API, UI, database, tests.
  A change that lands in one and not the others is incomplete.
- Never reset production data, drop a table, or truncate a row to make a test pass.

## Git reality in this workspace

**There is no Git repository here.** Therefore:

- Do **not** run `git init`.
- Do **not** copy Git metadata from another directory (for example
  `H:\my apps\Veylix.worktrees\`).
- Do **not** perform any Git operation that changes repository state.
- Do **not** report a `git diff`, `git status` or `git log` you did not actually
  see — there is nothing to see.
- Review changes by **reading the files back**. That is the review.

## Evidence obligations

| What you did | Where it is recorded |
|---|---|
| Any defect found or fixed | append to `./docs/IMPROVEMENT_LEDGER.md` or `./VPN_FIX_LEDGER.md` |
| Any decision that changes behaviour | append to `./docs/DECISIONS.md` with the next `D-nnn` |
| Any change to the HTTP surface | `./docs/API_CONTRACT.md` |
| Any command actually run | `./docs/QA_REPORT.md` and/or a dated file under `./records/` |
| Session progress | append to `./docs/cline/progress.md` |

**Append, never rewrite.** Decisions and ledgers are history. An old entry that
is now wrong gets a new entry that supersedes it — the old text stays.

## Never do this

1. Never claim a command ran when it did not.
2. Never claim remote panel success from local database state.
3. Never leave a critical flow hidden behind a fake button or mocked data.
4. Never edit `./MASTER_PRODUCT_BIBLE.md`, `./docs/DOC_INDEX.md`, `.vscode/` or
   the Cline MCP configuration unless the owner asked for exactly that.
5. Never silently drop a requirement. Mark it `Missing`, `Partial`, `Bug` or
   `Blocked` and say so.

## When you stop early

Leave a **working build**. List precisely what is done, what is untested, and
what is blocked — and why.
