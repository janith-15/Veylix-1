# VEYLIX — DOCUMENT INDEX

**28 documents** (counted 2026-11-05). Read the row that matches your question; every doc states its own date and
what it verified, and none of them is allowed to claim something the code does not do
(`docs/new_reqierment/new_update.md` §1, §13).

---

## Start here

| Question | Document | Status |
|---|---|---|
| What is this product, and how do I run it? | [`../README.md`](../README.md) | current |
| **What works and what doesn't?** | [`PROCESS_MAP.md`](PROCESS_MAP.md) §13 / §15 | **current — verified 2026-11-03** |
| **What is still broken, and which item fixes it?** | [`DISCREPANCY_REGISTER.md`](DISCREPANCY_REGISTER.md) | **current — live, 2026-11-04** |
| How do I get a local live preview? | [`LIVE_PREVIEW.md`](LIVE_PREVIEW.md) | current |
| How do I install it on a server? | [`INSTALL.md`](INSTALL.md) | current — manual steps, **not VM-tested** |
| How do I upgrade / roll back? | [`UPGRADE.md`](UPGRADE.md) | current — runbook, **not rehearsed** |
| How do I back up and restore? | [`BACKUP_RESTORE.md`](BACKUP_RESTORE.md) | current — script shipped, **restore not rehearsed** |
| How do I change the domain? | [`DOMAIN_CHANGE.md`](DOMAIN_CHANGE.md) | current — runbook, **not practised** |
| Something is broken — what now? | [`TROUBLESHOOTING.md`](TROUBLESHOOTING.md) | current |

---

## Requirements & planning

| Document | Purpose | Last re-synced |
|---|---|---|
| [`REQUIREMENTS.md`](REQUIREMENTS.md) | what the product must do (Bible-derived) | 2026-10-23 |
| [`ROADMAP.md`](ROADMAP.md) | milestone progress map | **2026-10-24** |
| [`IMPLEMENTATION_PLAN.md`](IMPLEMENTATION_PLAN.md) | stage-by-stage checklist + progress log | **2026-10-24** |
| [`DECISIONS.md`](DECISIONS.md) | every owner-approved decision (D-001 … **D-078**) | **2026-11-05** |
| [`SDLC_PLAN.md`](SDLC_PLAN.md) | how work is sequenced and gated | 2026-10-23 |

## Architecture & integration

| Document | Purpose | Last re-synced |
|---|---|---|
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | layers, dependency direction, data invariants | 2026-10-23 |
| [`SYSTEM_MAP.md`](SYSTEM_MAP.md) | module view | 2026-10-24 |
| [`PROCESS_MAP.md`](PROCESS_MAP.md) | process × actor flows | 2026-10-24 |
| [`3XUI_API_CONTRACT.md`](3XUI_API_CONTRACT.md) | the verified 3X-UI 3.10.2 contract | 2026-10-24 |
| [`API_CONTRACT.md`](API_CONTRACT.md) | the Veylix HTTP surface (126 routes) | **2026-11-05** |

## Quality & evidence

| Document | Purpose | Last re-synced |
|---|---|---|
| [`QA_PLAN.md`](QA_PLAN.md) | the test strategy and its gates | 2026-10-24 |
| [`QA_REPORT.md`](QA_REPORT.md) | commands actually executed + results | **2026-10-24** |
| [`FINAL_REPORT.md`](FINAL_REPORT.md) | the mandated §13 handover report | **2026-10-24** |
| [`IMPROVEMENT_LEDGER.md`](IMPROVEMENT_LEDGER.md) | every defect F-001 … F-033 with evidence | **2026-10-24** |
| [`GAP_ANALYSIS.md`](GAP_ANALYSIS.md) | requirement → reality map (§0-A is the live board) | **2026-10-24** |

## UI & design

| Document | Purpose | Last re-synced |
|---|---|---|
| [`UI_INVENTORY.md`](UI_INVENTORY.md) | every screen, theme and component | 2026-10-23 |
| [`DESIGN_SPACING.md`](DESIGN_SPACING.md) | spacing/rhythm rules | 2026-10-23 |
| [`AUTH.md`](AUTH.md) | sign-in: password, Google OAuth, e-mail verification, the auth gate (D-039) | 2026-10-28 |

## Historical

| Document | Note |
|---|---|
| [`new_reqierment/new_update.md`](new_reqierment/new_update.md) | the owner's improvement mandate — the process rules this repo follows |
| [`../MASTER_PRODUCT_BIBLE.md`](../MASTER_PRODUCT_BIBLE.md) | the original product specification |
| [`../reports/2026-10-22-frontend-implementation-report.md`](../reports/2026-10-22-frontend-implementation-report.md) | point-in-time frontend report (superseded by FINAL_REPORT) |

---

## Conventions

- **Dates matter.** A doc with an older "Last re-synced" date may describe a state that has
  since changed; check `FINAL_REPORT.md` §7 first.
- **No unverifiable claims.** UI copy is gated by `npm run audit`; docs are gated by review at
  every milestone.
- **One home per fact.** Endpoints live in `API_CONTRACT.md`, decisions in `DECISIONS.md`,
  defects in `IMPROVEMENT_LEDGER.md`, evidence in `QA_REPORT.md`. Other docs link, not copy.
