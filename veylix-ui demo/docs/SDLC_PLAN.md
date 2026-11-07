# VEYLIX — SDLC PLAN (phases, gates, definition of done)

**Version:** 1.0 · **Created:** 2026-10-23 · **Decision:** D-014
**Purpose:** give the project an explicit software-development lifecycle so work advances through named phases with hard gates instead of "it compiles, therefore done" (`new_update.md` §13).
**Companion:** `docs/QA_PLAN.md` (what is tested inside each gate), `docs/IMPLEMENTATION_PLAN.md` (the task checklists), `docs/IMPROVEMENT_LEDGER.md` (findings).

---

## 1. Lifecycle

```
 0 Documentation  → 1 Baseline  → 2 Architecture  → 3 Audit  → 4 Prioritise
        ↓                                                          ↓
 9 Release  ←  8 Live verify  ←  7 QA  ←  6 Unit test  ←  5 Build
```

| Phase | Name | Maps to `new_update.md` | Produces | Exit gate |
|---|---|---|---|---|
| 0 | Documentation | §1, §2 | GAP_ANALYSIS, LEDGER, 3XUI_API_CONTRACT, DECISIONS | docs approved |
| 1 | Baseline | §1.3, §9 | clean tree, recorded test baseline | **BASELINE RECORDED** |
| 2 | Architecture | §4, §11 Stage B | `backend/` skeleton, ARCHITECTURE.md, API_CONTRACT.md | **SPEC REVIEW** |
| 3 | Audit | §11 Stage C | line-level file + design review | **AUDIT COMPLETE** |
| 4 | Prioritise | §10, §11 Stage D | P0…P3 order in the ledger | **APPROVED** |
| 5 | Build | §11 Stage E | working code per stage E1–E7 | **BUILD PASS** |
| 6 | Unit test | §9, §11 Stage F | unit/contract/integration/race results | **UNIT PASS** |
| 7 | QA | §9, §10 | QA_PLAN checklist → QA_REPORT | **QA PASS / WAIVED** |
| 8 | Live verify | §11 Stage G | real-panel evidence | **LIVE VERIFIED / BLOCKED** |
| 9 | Release | §11 Stage H, §13 | FINAL_REPORT, version tag, rollback notes | **RELEASE** |

---

## 2. Gate definitions (hard, evidence-based)

### GATE 1 — BASELINE RECORDED
- [ ] `git status` understood; owner's work identified and not disturbed
- [ ] every command that was run and its real output is written down
- [ ] test baseline stated truthfully (currently: **none**)

### GATE 2 — SPEC REVIEW
- [ ] `go build ./...` passes (backend compiles)
- [ ] `go vet ./...` reports 0 issues
- [ ] `gofmt -l .` prints nothing
- [ ] `ARCHITECTURE.md` shows layer boundaries + dependency direction
- [ ] every mock store in `UI_INVENTORY.md` §10 has an endpoint in `API_CONTRACT.md`
- [ ] owner reviewed the API contract

### GATE 3 — AUDIT COMPLETE
- [ ] each touched file has: responsibility, invariants, identified defect, dependants
- [ ] findings recorded with file:line evidence in `IMPROVEMENT_LEDGER.md`

### GATE 4 — PRIORITISED
- [ ] every finding carries P0–P3 and a stage
- [ ] P0/P1 scheduled first

### GATE 5 — BUILD PASS
- [ ] backend builds, vets, formats clean
- [ ] frontend type-checks, lints (0/0), builds
- [ ] no endpoint used that is absent from `3XUI_API_CONTRACT.md`
- [ ] no secret in any response, log line or bundle

### GATE 6 — UNIT PASS
- [ ] `go test ./...` green; `go test -race ./...` green
- [ ] contract tests replay the verified OpenAPI examples
- [ ] new regression test exists for every fixed defect
- [ ] no test weakened to obtain a green run

### GATE 7 — QA PASS
- [ ] `docs/QA_PLAN.md` checklist executed; results in `docs/QA_REPORT.md`
- [ ] security checks (IDOR / SSRF / token leak / quota race) evidenced
- [ ] UI responsive check at 360 / 768 / 1280 + theme sweep
- [ ] anything not executed is listed as UNTESTED, never implied as passing

### GATE 8 — LIVE VERIFIED
- [ ] operations executed against a real 3X-UI panel with real results
- [ ] if no panel: everything is labelled **"implemented but not live-verified"**

### GATE 9 — RELEASE
- [ ] `docs/FINAL_REPORT.md` complete (`new_update.md` §13, 14 items)
- [ ] rollback procedure written and rehearsed at least once

---

## 3. Definition of done (per feature)

| Dimension | Required evidence |
|---|---|
| Functionality | works end-to-end **or** is explicitly labelled partial |
| Authorization | negative test: wrong role → 403, wrong owner → 404 |
| Remote write | fresh-read confirmation; ambiguous outcome resolved (not retried blindly) |
| Data | migration applied; no data loss; history preserved |
| Tests | unit + contract; live where possible |
| Security | no secret in response/log/bundle; input validated |
| Documentation | docs updated in the **same** commit (Bible §0) |
| Honesty | no fake UI action, no fabricated result (`new_update.md` §13) |

---

## 4. Regression discipline

- One defect = one ledger entry = one test.
- A finding may not be downgraded in severity to obtain a green run (`new_update.md` §9).
- Every stage ends with: *what was done · what passed · what is untested · what is blocked*.
