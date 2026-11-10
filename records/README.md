# records/ — evidence kept, scratch kept out of the way

This directory holds **dated evidence files**: their own names carry the date they
were produced, so a reader can tell a live measurement from a stale one without
opening the file.

Nothing in here is source. Nothing in here is imported by `backend/` or
`frontend/`. The repository root holds only files a newcomer must read
(`README.md`, `BUILD_BRIEF.txt`, `MASTER_PRODUCT_BIBLE.md`, `VPN_FIX_LEDGER.md`,
`QA_REPORT.md`, `dev.ps1`) — ad-hoc output belongs here instead of beside them.

## Layout

| Path | What it is |
|---|---|
| `2026-11-04-baseline.md` | The gate baseline recorded before the system-by-system completion pass. Every later gate compares against it. |
| `2026-10-22-dashboard-layout-*.{txt,json}` | The measured layout/token dump that backed the mobile + drawer spacing work of 2026-10-22 (410 px shell, 250 px sidebar offset, `app-sidebar--closed`). Renamed from `diag5.txt` / `diagnose*.json` so the date and the subject are in the name. |
| `_scratch-archive/` | The original root-level names those files used to carry (`diag5.txt`, `diagnose.json`, `diagnose2.json`, `diagnose3.json`, `diagnose4.txt`, `.vite-dev.log`). Kept so a note or a chat message that quotes an old path still resolves. **Git-ignored** — see `.gitignore`. |

## Rule

`diag*`, `diagnose*` and `*.log` are git-ignored at every level. If an output is
worth keeping, copy it in here with its date and subject in the file name (as the
2026-10-22 files above did) rather than weakening that ignore rule.

