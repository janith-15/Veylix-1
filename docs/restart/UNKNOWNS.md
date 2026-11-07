# VEYLIX — UNKNOWNS

**Purpose.** Questions this audit could not answer from repository evidence.
Each is a **blocker or an input** for Phase 3–5, so each must be resolved by the
owner or by a targeted investigation — not by assumption.

Created 2026-11-08. Status values: `OPEN` · `RESOLVED` (with the answer) · `WAIVED`.

---

## Business / product

| # | Unknown | Why it matters | Status |
|---|---|---|---|
| Q-01 | **Is this a from-scratch rebuild, or a documented continuation?** The directive says "substantially new Veylix system" but also "do not blindly change what works". The Phase 2 audit recommends continuing on the existing backend. | Decides whether Phase 5 continues the tree or starts a new one. **Every other decision depends on this.** | OPEN |
| Q-02 | **Is the existing dev database (798,720 bytes, 8 users) meant to be migrated, or discarded?** It looks like seed data plus test rows, but that is an inference. | Drives `MIGRATION_PLAN.md` entirely. | OPEN |
| Q-03 | **What are the real production deployment details?** No domain, no host, no TLS certificate, no live panel credentials beyond the dev one. | Blocks any release-readiness claim. | OPEN |
| Q-04 | **Which of the 37 frontend pages are still wanted?** Several staff screens were never audited in this pass (`StaffPage` 24 KB, `SamplePage` 14 KB, `SaleFlowPage`). | Scope for Phase 5 UI work. | OPEN |
| Q-05 | **Is the internal chat feature in scope?** It has 3 tables and a page, but the Bible describes it only briefly (§2.6). | A whole subsystem to keep or drop. | OPEN |

## Technical

| # | Unknown | Why it matters | Status |
|---|---|---|---|
| Q-06 | **Should the new system use Git?** Recommended strongly; not yet approved. | Without it, `L-01` can happen again. | OPEN |
| Q-07 | **Does `vpn_transfers` (I-34) / `usage_snapshots` (I-35) still need fixing**, or does the new schema remove the cascade entirely? | Two open P3 defects with no home yet. | OPEN |
| Q-08 | **Is a frontend unit-test runner acceptable?** The product targets 1 vCPU / 500 MB and currently relies on 22 `verify-*.mjs` browser scripts instead. | Owner decision recorded as open since 2026-11-07. | OPEN |
| Q-09 | **What is the real production data volume?** The performance targets (median latency, payload size) were measured against 4 VPNs. | Bounds any performance claim. | OPEN |

## Documentation

| # | Unknown | Why it matters | Status |
|---|---|---|---|
| Q-10 | **Should `I-10`–`I-26` be reconstructed at all?** Their text is unrecoverable; only their existence is known. | Owner has said to defer. May be waived permanently. | **WAIVED** (owner, 2026-11-08) |
| Q-11 | **Should `D-062`, `D-077`, `D-078`, `D-089` be re-created or formally retired?** | An unexplained gap in a decision log invites invented explanations later. | OPEN |
| Q-12 | **Which documentation set is authoritative after Phase 3?** `MASTER_PRODUCT_BIBLE.md` (70 KB) and the proposed `MASTER_SPEC.md` would overlap. | Two sources of truth is exactly the failure this restart is meant to fix. | **RESOLVED 2026-11-08** — `docs/new/MASTER_SPEC.md` governs; the Bible is archived to `_veylix-archive\docs-legacy-2026-11-08\` and retained in place as historical reference only (ADR-003). |
| Q-13 | **Does `httpapi` keep its 12 SQL statements, or does the rule change?** Measured: 112 SQL statements live outside `store/`, 12 of them in the transport layer (`usage_store.go` 7, `vpn_lister.go` 2, `vpn_lister_panel.go` 2, `panel_cache.go` 1). | The stated rule "only `store` writes SQL" is false as written. Either the code or the rule must move — leaving both is what lets a contributor trust a rule that isn't real. | OPEN — Option A (move the 12 queries into `store`) or Option B (rewrite the rule to describe reality). Recorded as `DEV-003`; recommended reading in `ARCHITECTURE.md` §6.1. |
| Q-14 | **Should `inbounds.id` be renumbered to equal `remote_id`?** The dev rows are `id=52,53,54` with `remote_id=1,2,3`, which is what produced `DEV-001` / `LEGACY-I-36`. | Renumbering is superficially attractive but would break every existing `vpn_offers.inbound_id` foreign key and any recorded offer. The translation in `provisioning.panelInboundID` already fixes the defect without touching data. | OPEN — current lean **NO** (see `MIGRATION_PLAN.md` §6). |


---

## How these were established

Each entry above was produced by a read-only inspection on 2026-11-08: reading
`MASTER_PRODUCT_BIBLE.md`, `docs/DECISIONS.md`, `docs/IMPROVEMENT_LEDGER.md`,
`docs/UI_UX_DEFECT_BOARD.md`, the 23 migrations, `backend/go.mod`,
`frontend/package.json`, and a read-only query of the development database.
Nothing was assumed from a document without checking the repository, and nothing
in the repository was taken as a requirement without checking the Bible.

Where those two disagreed, the disagreement is recorded here rather than silently
resolved.
