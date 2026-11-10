# WORKFLOW · CODE REVIEW

Use before merging or reporting any non-trivial change, and whenever a change
touches request handling, data access, or the frontend data path.

**This workflow is read-only by design.** Stages 1–10 produce *findings*, not
edits. A fix goes through `bug-fixing.md` after the owner has seen the findings —
or after a separate, explicit instruction. A reviewer who edits while auditing
cannot tell what was already broken.

**Relationship to `security-review.md`:** that workflow owns the threat model —
secrets, authn/authz, ownership, SSRF, integrity, audit. This one owns
**correctness under load and change**: efficiency, leaks, duplication, boundary
drift, compatibility. Where they overlap (authorization, injection), report it
once, here, and cite the security workflow rather than restating its rules.

Load Skill `veylix-architecture` before Stage 8 and Skill `veylix-security`
before Stage 2.

---

## Stage 1 — Scope the diff honestly

- **There is no Git repository here.** Do not run `git diff` and never report
  one. Establish the changed set by reading the files back, or from
  `./docs/cline/progress.md` for the session.
- List every changed file and classify it by layer: `cmd`, `httpapi`, `domain`,
  `store`, `panel`, support package, frontend, docs.
- If the changed set cannot be established, **stop**. Reviewing an unknown diff
  is theatre.

**Output:** the changed-file list with layers.

## Stage 2 — Security vulnerabilities (delegate depth)

Confirm the change was already run through `security-review.md`, or run its
Stages 2–7 now. Do not re-derive its findings here; record **only** new ones
with a file and line.

Specifically check the shapes this workflow sees that a pure security pass
misses:

- a query built by string concatenation rather than a placeholder,
- an error returned to the caller that embeds an internal path or driver error,
- a client-supplied field flowing into a store call without validation.

## Stage 3 — Authentication and authorization

- Is the route registered with its auth middleware, and is that middleware the
  one the neighbouring routes use?
- Is the role check present **and** the row-level ownership scope present?
- Is the ownership predicate inside the SQL/query scope, not applied after the
  fact in Go or in the UI?

**Verification:** name the middleware and the scope predicate for each changed
route. "It looked right" is not a result.

## Stage 4 — RBAC and IDOR

For every changed route that takes an identifier:

- Can an Admin of tenant A reach tenant B's row?
- Can a Client reach another Client's row?
- Is a numeric/opaque ID a **guessable** identifier used without an ownership
  predicate?

**Output:** a table of route → identifier → ownership predicate → verdict.
A route with a predicate that does not reference the caller's scope is an
**IDOR finding**, regardless of the UI.

## Stage 5 — Database query efficiency

- Any query in a loop (N+1) introduced by the change?
- Is there an index supporting each new `WHERE` / `ORDER BY` / `JOIN`? Name it,
  or record it as missing.
- Is a `SELECT *` used where a narrow projection is needed?
- Does a new migration rebuild or copy a table without a plan for existing rows?
- Are transactions scoped to the minimum, and does any transaction span a
  **remote** call? That last one violates the architecture rule — report it.

**Verification:** `EXPLAIN QUERY PLAN` on the new statement against an
**isolated** database, not `./var/`. Record the plan.

## Stage 6 — Memory and resource leaks

- Goroutine started without a termination path?
- `rows` / `resp.Body` / file handle without a deferred close?
- Timer or ticker without `Stop()`?
- Unbounded slice, map or cache keyed by something a caller controls?
- Context created without cancellation on a path that can block?
- On the frontend: a `useEffect` subscription, listener, interval or store
  subscription without cleanup?

**Output:** leak, file, and the evidence that the lifecycle is or is not closed.

## Stage 7 — Error handling

- Any error swallowed (`_ =`, empty `if err != nil {}`, bare `catch {}`)?
- Is a failure surfaced as an honest state, or as a fabricated success?
  A `200` on a failed remote action is a **finding**, not a style issue.
- Does the caller get a stable, non-leaking message while the detail goes to the
  log — with no secret in either?
- Is `REVOCATION_PENDING`-style explicit state preserved where the panel may be
  unreachable?

## Stage 8 — Duplication and architecture violations

- Is a helper reimplemented instead of reused? Point at the existing one.
- **Boundary drift** — the highest-value category here:
  - `httpapi` importing the SQL driver or building SQL → finding
  - business rules living in a handler → finding
  - a local transaction wrapping a remote panel call → finding
  - `panel` called from `httpapi` directly → finding
  - frontend deriving a limit, quota or role locally instead of reading the API
    → finding
  - frontend holding a panel token → **P0 finding**
- Does anything upstream now import a package that already imports it
  (cycle risk)?

**Verification:** for each changed file, read its import block. That is the
evidence.

## Stage 9 — API compatibility, frontend performance, regressions

**API compatibility:** does the change remove, rename or retype a field a client
already depends on? Adding an optional field is compatible; removing or changing
the meaning of an existing one is not. If the HTTP surface moved, it needs an
`./docs/API_CONTRACT.md` row — flag it as missing if absent.

**Frontend performance:** does the change add an eager import that defeats route
splitting, a render-time computation that belongs in a selector, a store
subscription that re-renders a whole page, or a new dependency against the
1 vCPU / 500 MB budget? For a bundle-affecting change, run `npm run build` and
compare the reported chunk sizes — measured, not recalled.

**Regressions:** does the change alter behaviour an existing test encodes
deliberately? Read the relevant `D-nnn` in `./docs/DECISIONS.md` **before**
concluding the test is wrong.

## Stage 10 — Report findings only

Produce one table, ordered by severity:

| Severity | Area | Finding | Evidence (file:line / route) | Suggested fix |
|---|---|---|---|---|

Severity scale:

- **P0** — secret exposure, auth bypass, data corruption, fabricated success
- **P1** — wrong access, broken contract, leak on a hot path
- **P2** — inefficiency, swallowed error, boundary drift with no current impact
- **P3** — duplication, readability with real maintenance risk

Rules for the report:

- Every row cites a real file, line, route or command output.
- **Actual findings beat stylistic preferences.** Do not pad the table with
  formatting opinions; if the only observations are stylistic, say so in one
  line and stop.
- "Cannot verify" is a valid result — say what blocked it.
- Fix nothing in this pass.
- Do not report a control as present because a comment or doc says so. Read the
  code.

Archive the report under `./records/` with a dated, subject-bearing filename,
then append a one-line summary to `./docs/cline/progress.md`.
