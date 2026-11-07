# WORKFLOW · FEATURE DEVELOPMENT

Use for new capability. For a defect, use `bug-fixing.md` instead.

Each stage ends with a checkpoint. Do not skip ahead; a later stage assumes the
earlier one is true.

---

## Stage 0 — Subsystem audit (conditional, but mandatory when it applies)

Before Stage 1, ask: **does this feature touch infrastructure the repository has
never audited?**

Load `system-design-audit.md` and complete it **first**, stopping at its approval
gate, if any of these is true:

- it adds or changes an installer, upgrade path, CLI, deploy template or
  uninstaller (use `installation-and-deployment.md` for the build itself);
- it introduces a new subsystem with more than one new file;
- you cannot say, without reading the tree, whether the capability already
  exists;
- the requirement arrives from a source that is not one of the Bible,
  `./docs/REQUIREMENTS.md` or `./docs/DECISIONS.md`.

If none applies, skip Stage 0 and say so in one line — a skipped stage is
recorded, never silently omitted.

**Checkpoint:** Stage 0 either completed and approved, or explicitly not
applicable with a reason.

## Stage 1 — Inventory (read only)

Read, in this order:


1. `./docs/cline/activeContext.md`, then the tail of `./docs/cline/progress.md`.
2. `./docs/GAP_ANALYSIS.md` — is this feature already mapped, and as what?
3. `./docs/DISCREPANCY_REGISTER.md` — is there a known blocker?
4. The relevant canonical doc from Rule 01's map.

**Checkpoint:** I can name the requirement's home document and the ledger entry
(if any) that already tracks it.

## Stage 2 — Requirements gate

- Does the feature exist in `./MASTER_PRODUCT_BIBLE.md` or
  `./docs/REQUIREMENTS.md`?
- If it does **not**, stop. Do not invent product requirements. Ask the owner.
- If it conflicts with a confirmed business rule, record the conflict; do not
  silently override it.

**Checkpoint:** the requirement is cited by section, or the work is paused.

## Stage 3 — Architecture placement

Load Skill `veylix-architecture`. Answer in writing:

- Owning layer, and the closest existing precedent to copy.
- New files, by full path.
- Cross-boundary? Then `./docs/API_CONTRACT.md` needs a row.
- New dependency? Justify against 1 vCPU / 500 MB.

**Checkpoint:** file list written before any edit.

## Stage 4 — Data and authorization first

Load Skill `veylix-security` and, if schema changes, `veylix-database-engineering`.

- Migration first (append-only, idempotent, backward compatible).
- Authorization on the route **before** the UI is built.
- Invariants preserved; no local transaction wraps a remote call.

**Checkpoint:** the negative cases already have a failing test.

## Stage 5 — Implement in small pieces

- Smallest coherent change that moves the work forward.
- If it touches the panel, load `veylix-3xui-integration`
  (write → fresh read → record).
- If it touches the UI, load `veylix-ui-ux` (API-backed, tokens, honest states).

**Checkpoint:** each piece builds on its own.

## Stage 6 — Focused tests

Load Skill `veylix-testing-review`. Cover the applicable cases from the matrix:
normal, unauthorized, wrong role, wrong owner, invalid input, stale/offline,
duplicate/retry, empty. Tick the applicable areas of Rule 05's **11-area QA
coverage map** — a skipped area is `NOT RUN`, never `PASS`.

**Checkpoint:** focused tests pass and their output is recorded.

## Stage 7 — Full gates

Run, in order, and record real output:

```
cd backend ; go build ./... ; go vet ./... ; go test ./... -count=1
cd frontend ; npm run lint ; npm run build ; npm run audit ; npm run docs:verify
```

`go test -race` → **BLOCKED** (no C compiler). Say so.

## Stage 8 — Record

- Append to `./docs/cline/progress.md` (dated: what changed, what ran, results,
  what was **not** verified).
- Rewrite `./docs/cline/activeContext.md`.
- Append a `D-nnn` to `./docs/DECISIONS.md` if behaviour changed.
- Append the defect to the appropriate ledger if you found one.
- Update `./docs/API_CONTRACT.md` if the HTTP surface moved.

**Done only when** Rule 05's "Definition of done" is satisfied.
