# VEYLIX — KNOWN LOSSES

**Purpose.** An honest record of information that existed and no longer does.
Without this file a future reader would conclude that the missing items never
existed, which would be false.

Created 2026-11-08.

---

## L-01 · `IMPROVEMENT_LEDGER.md` lost entries I-01 – I-35

**What happened.** On 2026-11-08 an agent editing
`docs/IMPROVEMENT_LEDGER.md` to append two new entries (`I-36`, `I-37`) used
`editor` (insert at line 1), then repaired the resulting misplacement with
PowerShell `Get-Content` / `Set-Content -Encoding UTF8` and an array slice. The
slice boundary was wrong, the re-encode double-encoded the file, and a further
repair pass wrote literal `U+FFFD` replacement characters over every non-ASCII
character. The file was finally restored to correct UTF-8, but the **I-series
section was gone**.

**What is lost.**

| Range | Recoverable? | From where |
|---|---|---|
| `I-08`, `I-09` | **Yes** | `docs/cline/progress.md` lines ~738–750 |
| `I-27`–`I-32` | **Yes** | `docs/cline/progress.md` lines ~1044–1052 |
| `I-33` | **Yes** | `docs/cline/progress.md` lines ~1185–1210, plus `docs/DECISIONS.md` D-087 |
| `I-34`, `I-35` | **Yes** | `docs/cline/progress.md` lines ~1199–1200 |
| `I-10`–`I-26` | **NO** | only referenced by ID in `progress.md`; their text is gone |

**What was searched, and found nothing.**

| Source | Result |
|---|---|
| `H:\my apps\Veylix-1\docs\IMPROVEMENT_LEDGER.md` | dated 2026-11-04, also lacks I-series |
| `H:\my apps\Veylix-2 - Copy\docs\...` | same file, same date |
| `H:\my apps\Veylix-2 - Copy (2)\docs\...` | same file, same date |
| `H:\my apps\New folder\veylix-4\docs\...` | same file, same date |
| `veylix-ui demo\docs\IMPROVEMENT_LEDGER.md` | a different application's ledger |
| Any `*.bak` / `*.orig` | none exist |

**Impact.** Medium. The *code* is unaffected — every defect those entries
described was fixed and the fixes are still in the tree, with their tests. What
is lost is the **narrative**: why each fix was necessary. `I-33` in particular
explains migration `0023_release_tombstone.sql`, so D-087 and
`progress.md` are now the only surviving explanation.

**Per owner instruction, full reconstruction is deferred.** The directive is to
document the new system rather than rebuild the old ledger. The entries that are
reconstructable are listed above so the work is bounded if it is ever resumed.

---

## L-02 · Four decision numbers have no record

`docs/DECISIONS.md` contains **86** decisions spanning `D-001`–`D-088`. Four
numbers in that range have no entry:

| Number | Reachable? |
|---|---|
| `D-062` | no reference found |
| `D-077` | **referenced** by an `IMPROVEMENT_LEDGER` entry — text itself is absent |
| `D-078` | **referenced** by an `IMPROVEMENT_LEDGER` entry — text itself is absent |
| `D-089` | no reference found; `DECISIONS.md` jumps `D-087` → `D-090`, then `D-088` was appended afterwards |

`D-077` and `D-078` being referenced means those decisions were made and written,
then lost. Their subject matter can be partly inferred from the ledger entries
that cite them, but **the decisions themselves cannot be quoted**.

---

## L-03 · No version-control history exists

There is no Git repository in `H:\my apps\Veylix-2`. Consequently:

- No commit log, no branches, no tags.
- No diff between any two states.
- No way to bisect a regression.
- Every change ever made was made in place.

This is not a loss of content — the content is present. It is a loss of
**history**, and it is why `L-01` was unrecoverable.

---

## L-04 · Earlier database snapshots are unlabelled

`.veylix-secrets/` holds seven `veylix.pre-*.db` snapshots from 2026-10-28 to
2026-11-08. Their filenames record the moment they were taken, but **not what
changed immediately afterwards**, so it is not possible to say which rows differ
between, for example, `pre-purge-20261030-044351` and `pre-purge-20261030-045614`.
They are preserved as-is; they are not interpreted here.

---

## Rule this file establishes

When information is lost, **say so in writing, in a file that is never deleted.**
An absent entry must never be mistaken for a fact that never existed.
