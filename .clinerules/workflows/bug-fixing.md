# WORKFLOW · BUG FIXING

Use for a defect in existing behaviour. For new capability, use
`feature-development.md`.

The rule that makes this workflow work: **fix the cause, prove it with a test
that fails before and passes after.** A fix with no regression test is a guess.

---

## Stage 1 — Reproduce, with evidence

- Get a concrete, minimal reproduction: the exact request, route, input, or
  sequence of clicks.
- Capture what actually happened (status code, message, rendered state).
- If it cannot be reproduced, **stop** and say so. Do not "fix" an unproven bug.

**Checkpoint:** the reproduction is written down and repeatable.

## Stage 2 — Check the ledgers first

- `./docs/DISCREPANCY_REGISTER.md` — is this cause already known?
- `./docs/IMPROVEMENT_LEDGER.md` and `./VPN_FIX_LEDGER.md` — has this been
  attempted, deferred, or deliberately left open?

A previously deferred item is not automatically in scope. Ask before reopening.

**Checkpoint:** I know whether this is new, known, or previously deferred.

## Stage 3 — Locate the root cause

- Trace the actual path: `httpapi` → `domain` → `store` / `panel`, or
  `page` → `store` → `lib/api.ts` on the frontend.
- Name the layer that is wrong. Symptom is not cause: a UI symptom with an
  authz cause is an authz bug.
- Load the matching Skill before changing that layer.

**Checkpoint:** I can state the cause in one sentence, naming the file.

## Stage 4 — Check blast radius

- Who else calls this code?
- Does the same defect exist on a sibling route or store?
- Would fixing it break an existing test that encodes a deliberate decision?

**Checkpoint:** blast radius named. If a test encodes a decision, read the
`D-nnn` before changing the test.

## Stage 5 — Fix the cause

- Smallest change that removes the cause.
- Do not clean up unrelated code in the same change.
- Do not silence a failure (no `try/catch` that hides, no retry that masks, no
  test deletion).

## Stage 6 — Regression test

- Write the test **first** if practical: it must fail on the old code.
- Then make it pass.
- Cover the neighbouring negatives (unauthorized, wrong owner) if the bug was
  in that family.
- Check the applicable rows of Rule 05's **11-area QA coverage map**; a browser
  fix needs a committed `verify-*.mjs`, not a Playwright MCP session.

**Checkpoint:** test fails before, passes after — shown, not asserted.

## Stage 7 — Full gates

```
cd backend ; go build ./... ; go vet ./... ; go test ./... -count=1
cd frontend ; npm run lint ; npm run build ; npm run audit
```

Evidence follows Rule 05's data-safety rules: no test against live data, no
mutation of `./var/` or `./.veylix-secrets/`.

## Stage 8 — Record

Append to the ledger using its existing format:

```
## <status> · <short title>

- Symptom:
- Cause:
- Fix:
- Test:
- Verified:
```

Then append to `./docs/cline/progress.md`. Update `./docs/DECISIONS.md` only if
the fix changes behaviour in a way a future reader must know.

**Never** rewrite an earlier ledger entry. Supersede it.
