# WORKFLOW · SYSTEM DESIGN AUDIT

Use when the work is **to understand and plan a subsystem**, not to change it:
"inspect the current installation system", "audit the panel integration", "is X
already implemented, and what is missing?", "design the backup subsystem".

Use `feature-development.md` instead when the design is already agreed and you
are building it. Use `security-review.md` when the question is only "is this
safe". This workflow answers a broader question: **what exists, what is missing,
what would we build, in what order, and how would we know it works.**

**This workflow is read-only.** Stages 1–11 produce a report and a plan. Nothing
is implemented. Stage 12 is an approval gate, and it is the whole point.

Load Skill `veylix-architecture` for Stage 2 and Stage 6,
`veylix-security` for Stage 5, `veylix-testing-review` for Stage 9, and
workflow `performance-verification.md` for Stage 8.

---

## The rules that make this workflow work

**1. A filename is not an implementation.** A directory called `deploy/` proves
nothing. A command mentioned in a document proves nothing. Every status in the
matrix must cite a real file, a real line, or a real command output. If you
cannot cite it, the status is `NOT IMPLEMENTED` — not "probably fine".

**2. Confirmed requirements and proposed choices are never mixed in the same
table.** A row that says "we should use `/opt/veylix`" is a proposal, not a
requirement. Putting a proposal in the requirements column is how scope quietly
invents itself.

**3. Documentation is not an implementation.** A runbook describing a manual
install does not make an installer exist. Cite code, or mark it missing.

---

## Stage 1 — Scope the subsystem and enumerate the requirements

- Name the subsystem in one sentence, and its boundary: what is inside it, and
  what is explicitly outside.
- Collect requirements **from their homes only**:
  - `./MASTER_PRODUCT_BIBLE.md` (cite the section),
  - `./docs/REQUIREMENTS.md` (cite the section),
  - `./docs/DECISIONS.md` (cite the `D-nnn`),
  - the acceptance tests near the end of the Bible (cite the number).
- Quote each requirement in **one short line**. Do not paraphrase it into
  something easier to satisfy.
- If a requirement is not in one of those homes, it is **not a requirement**.
  Put it in the Stage 6 proposal table, or ask the owner.

**Output:** a numbered requirement list, each with its citation.

**Checkpoint:** every requirement has a citation, or the work pauses for the
owner.

## Stage 2 — Inventory what actually exists

Read the tree. Do not infer from names.

- List every file that could belong to the subsystem, with its **size** and, for
  scripts, whether it is executable and where it is called from.
- For each file you will cite later, **open it** and record one concrete fact
  from inside it: a function name, a flag, a guard, a default, a line.
- Record what is **absent** explicitly. "No `install.sh` anywhere in the tree"
  is a finding; silence is not.
- Load Skill `veylix-architecture` and place each file in its layer.

**Output:** an inventory table — path, bytes, verified fact, layer.

**Checkpoint:** every row has a fact that could only come from reading the file.

## Stage 3 — Requirements versus implementation matrix

One row per requirement. **Five statuses, no others:**

| Status | Means |
|---|---|
| `IMPLEMENTED AND VERIFIED` | it exists **and** a named command or test proved it in this pass |
| `IMPLEMENTED BUT NOT VERIFIED` | the code exists; nothing executed has proven it |
| `PARTIALLY IMPLEMENTED` | some of it is real; name the part that is not |
| `NOT IMPLEMENTED` | nothing real exists |
| `BLOCKED` | cannot be assessed or satisfied without owner action or an environment we do not have — name which |

For each row, name the **evidence**: `file:line`, a route, a test name, or a
command and its output. A row with an empty evidence cell is not a matrix, it is
an opinion.

**Rules:**

- `IMPLEMENTED AND VERIFIED` requires a command you ran **in this pass**. A test
  that exists but did not run is `IMPLEMENTED BUT NOT VERIFIED`.
- Never round `PARTIALLY IMPLEMENTED` up to `IMPLEMENTED`.
- A requirement satisfied only by a document is `NOT IMPLEMENTED`.

**Checkpoint:** the matrix has no empty evidence cell.

## Stage 4 — Missing functionality list, tiered

Do not produce one flat list. Tier it by what it blocks:

- **Tier 1** — blocks the stated objective of the subsystem.
- **Tier 2** — needed before the subsystem can honestly be called production
  ready.
- **Tier 3** — nice to have, or deliberately deferred with a reason.

Each item names the requirement it closes (from Stage 1) and the file that would
have to exist.

**Checkpoint:** every Tier 1 item traces to a Stage 1 requirement.

## Stage 5 — Security and data-loss risk assessment

Two axes, because they fail differently. **Data loss is not a security risk and
must not be buried inside one.**

Load Skill `veylix-security`.

**Strengths first.** List the controls that genuinely already exist, with
evidence. A report that lists only risks hides the fact that the work is
partially done and invites rebuilding what already works.

**Security risks** — secrets, authn/authz, ownership, SSRF, tokens, egress.

**Data-loss risks** — this axis is mandatory, and it is where this project has a
recorded incident: `./VPN_FIX_LEDGER.md` documents a verification script that
deleted live data, **twice**. For the subsystem under audit, ask:

- Can any operation destroy, overwrite, truncate or reinitialise existing data?
- Can a second run of the same operation do it by accident — no lock, no
  idempotency check, no "already installed" branch?
- Can a new installation inherit another installation's database, keys or
  credentials?
- Can an upgrade or migration leave data half-transformed?
- Is every destructive path recoverable, and is the recovery **rehearsed** or
  only documented?

Every risk gets a severity, an evidence line, and the condition that makes it
real:

| Severity | Meaning |
|---|---|
| `P0` | secret or credential exposure, auth bypass, **data destruction** |
| `P1` | wrong access, or data loss with a narrow window |
| `P2` | weakness or missing control |
| `P3` | hardening |

**Conflicts section (mandatory).** List anything where the requirements disagree
with the repository or with each other — naming conventions, file locations,
tool choices. **Report the conflict; do not silently pick a side.** Rule 04
forbids silently changing a convention, so a conflict is an owner decision, not
a judgement call.

**Checkpoint:** both axes present, every risk cited, conflicts listed.

## Stage 6 — Proposed architecture

- **Extend, do not rewrite.** Name the existing precedent you are following and
  say why it fits. A proposal that replaces working code needs a written reason
  in `./docs/DECISIONS.md` terms.
- Separate **confirmed requirements** (Stage 1) from **proposed implementation
  choices** in two labelled tables. A proposal must never appear in the
  requirements column.
- Respect the resource ceiling: 1 vCPU / 500 MB. A proposal that needs more is a
  design change and needs the owner, not a footnote.
- Respect the layer boundaries. Nothing proposed may make `httpapi` import
  `store`, put business rules in a handler, or let a local transaction wrap a
  remote call.
- State the **paths, names and file list** you propose, so the change is
  reviewable before it exists.

**Checkpoint:** every proposed file has a full path and one sentence of purpose.

## Stage 7 — Sequences

Write each sequence as numbered steps with the **failure branch at every step**.
A sequence with no failure branch is a wish.

- **Fresh / first-time path**
- **Upgrade path** — existing data preserved
- **Repair path** — something is broken, data must survive
- **Rollback path** — including the case where a migration already ran

For each: what is checked before the step, what happens if it fails, and what
state the system is left in. "Abort" is an acceptable outcome. "Continue anyway"
is not, unless it is written as an explicit operator flag.

**Checkpoint:** every step has a failure branch and a described end state.

## Stage 8 — Resource measurement plan

Load workflow `performance-verification.md` for the measurement rules.

This stage produces a **plan**, not numbers. For each metric:

| Metric | Method | Target | Status |
|---|---|---|---|
| idle RSS | `ps` / `systemd-cgtop` after 5 min | < 80 MB | NOT RUN |

`Status` is exactly one of:

- `MEASURED` — and then show the value and the command;
- `ESTIMATED` — and say which measured number it was derived from;
- `NOT RUN` — and say why.

**Never present a plan target as a measurement.** A `MemoryMax=384M` in a unit
file is a *limit*, not a measurement, and must be labelled as a limit.

**Checkpoint:** no row claims a measured value that was not measured in this
pass.

## Stage 9 — Automated testing strategy

Load Skill `veylix-testing-review`.

- Map each new behaviour to the layer that proves it: Go test, HTTP test,
  committed `verify-*.mjs`, or a shell assertion.
- Name the **existing** runner that will execute it. Do not invent a runner that
  is not installed; if the only viable path is a runner this repo does not have,
  say so and treat it as a Tier 1 missing item or a Stage 6 proposal.
- State which checks **cannot** be automated here, and what a human must do.
- State the isolation rule for every test that touches persistent data: an
  isolated database and an isolated data directory — never `./var/`, never
  `./.veylix-secrets/`, never the seeded development panel.
- For any install, upgrade or migration subsystem, the **idempotency** test and
  the **"second run changes nothing"** test are mandatory, because those are the
  two ways this class of work destroys data.

**Checkpoint:** every proposed test names a runner that exists, or is declared
missing.

## Stage 10 — Implementation stages with dependencies and acceptance criteria

Two tables, both required.

**Stages** — `#`, work, depends on, produces. A stage that depends on another
cannot start first; say so explicitly rather than ordering by enthusiasm.

**Acceptance criteria per stage** — what must be **observed** for that stage to
count as done. "It runs" is not a criterion. "Fresh install into an empty data
directory reports ready, and a second run refuses and changes nothing" is.

**Checkpoint:** every stage has at least one observable criterion.

## Stage 11 — Report

Write the report to `./records/<YYYY-MM-DD>-<subject>-audit.md` and append a
one-line summary to `./docs/cline/progress.md`.

The report contains the deliverables it was asked for, in order. The reference
shape, used for the installation-system audit on 2026-11-07, is:

1. current architecture · 2. implementation inventory · 3. requirements matrix ·
4. missing functionality · 5. security and data-loss risks · 6. proposed
architecture · 7. fresh sequence · 8. upgrade sequence · 9. repair and rollback
sequence · 10. resource plan · 11. testing strategy · 12. stages with
dependencies · 13. acceptance criteria per stage.

Do not pad it. A short report with real evidence beats a long one that restates
the requirements.

## Stage 12 — Approval gate

**Stop.** Present the report. Implement nothing.

The owner reviews it and answers one of:

- **approved** — the work then moves to `feature-development.md`, starting from
  the approved Stage 10 order;
- **approved with changes** — record the changes as a `D-nnn` in
  `./docs/DECISIONS.md`, then proceed;
- **rejected** — record why, and stop.

An audit that turns straight into implementation is not an audit. It is
implementation with paperwork.

---

## Checklist

- [ ] Every requirement cited to Bible / REQUIREMENTS / DECISIONS / acceptance number.
- [ ] Inventory facts came from reading files, not from filenames.
- [ ] Matrix uses only the five statuses, and every evidence cell is filled.
- [ ] `IMPLEMENTED AND VERIFIED` only where a command or test actually ran.
- [ ] Missing list tiered, each item traced to a requirement.
- [ ] Security **and** data-loss axes both present; conflicts listed.
- [ ] Architecture proposal separates requirements from choices.
- [ ] Every sequence step has a failure branch.
- [ ] No plan target presented as a measurement.
- [ ] Every proposed test names an existing runner or is declared missing.
- [ ] Stages ordered by dependency, each with observable acceptance criteria.
- [ ] Report archived under `./records/`, summary in `./docs/cline/progress.md`.
- [ ] **Stopped at the approval gate.** Nothing implemented.


