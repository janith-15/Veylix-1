# VEYLIX — QA REPORT

**This file is a redirect, not a report.** It used to be a duplicate of
`docs/PROCESS_MAP.md` frozen at **2026-10-23**, and a duplicated document is the
one that gets read while the real one moves on. It said 3 of 10 processes worked
and that billing, chat and notifications were broken — none of which has been
true since the backend and the Phase-L screens landed.

Read these instead. Each has one home, and each states what was verified and when:

| Question | Document |
|---|---|
| **What works, per process and per user action?** | [`docs/PROCESS_MAP.md`](docs/PROCESS_MAP.md) — §13 truth table, §15 counting (verified 2026-11-03) |
| **Which commands were run, and what did they return?** | [`docs/QA_REPORT.md`](docs/QA_REPORT.md) |
| **What is still missing, and why?** | [`docs/FINAL_REPORT.md`](docs/FINAL_REPORT.md) · [`docs/IMPROVEMENT_LEDGER.md`](docs/IMPROVEMENT_LEDGER.md) |
| **Which endpoint exists?** | [`docs/API_CONTRACT.md`](docs/API_CONTRACT.md) |
| **Why was it built this way?** | [`docs/DECISIONS.md`](docs/DECISIONS.md) (D-001 … D-049) |

## Where the numbers come from

`npm run docs:verify` derives the counts from the tree and **fails when the
README disagrees**, so they cannot drift silently:

```
tree: 17 migrations · 116 routes · 436 test functions in 13 packages
docs:verify: OK — README matches the tree.
```

## The gates themselves

| Gate | Command | Result (2026-11-03) |
|---|---|---|
| Backend build · vet | `go build ./...` · `go vet ./...` | ✅ clean |
| Backend tests | `go test ./... -count=1` | ✅ 12/12 packages ok |
| Frontend typecheck | `npx tsc -b` | ✅ 0 errors |
| Frontend lint | `npm run lint` | ✅ 0 warnings / 0 errors |
| UI truth gate | `npm run audit` | ✅ no unwired claims |
| Production build | `npm run build` | ✅ pass |
| QR encoder | `npm run qr:selftest` | ✅ 10 cases |
| Client ping parser | `npm run ping:selftest` | ✅ 15 cases |
| Audit gate self-test | `npm run audit:selftest` | ✅ 6 + 4 cases |

**Not run:** `go test -race` (no C compiler on this machine), the 39 Bible
acceptance tests, and any remote write against a **production** panel (D-018).
