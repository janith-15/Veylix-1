# VEYLIX — UPGRADE

**How to move a running install to a new binary without losing data.** The
schema only ever moves forward, so the rule is simple: **back up first, and a
rollback across a migration needs that backup.**

Two paths exist. **Use the first one**; the manual path is kept because a reader
must be able to see exactly what the tool does.

---

## 0. The supported path — `vpn-platformctl update`

```bash
sudo vpn-platformctl status                  # note schema_version
sudo vpn-platformctl update --dry-run        # print every phase, change nothing
sudo vpn-platformctl update
```

`update` runs this sequence, and the order cannot be rearranged:

| # | Phase | Why it is here and not later |
|---|---|---|
| 1 | probe current `/api/ready` | a comparison has to exist *before* the swap |
| 2 | **backup** via `deploy/veylix-backup.sh` | a failing upgrade must have something to return to |
| 3 | download binary + `SHA256SUMS` | — |
| 4 | **verify checksum**, delete on mismatch | a tampered or partial artifact must not reach `/usr/local/bin` |
| 5 | keep `veylixd.prev`, install the new binary | the rollback artefact |
| 6 | restart, **poll `/api/ready`** | the only proof it came up |
| 7 | compare `schema_version` | **backwards means failure**, not success |

It also refreshes `vpn-platformctl` itself from the same release, so the tool
never drifts behind the binary it manages.

## 0.1 Rollback

```bash
sudo vpn-platformctl rollback                      # binary only
sudo vpn-platformctl rollback --restore /var/backups/veylix/<ts>   # binary + data
```

If a migration ran, the binary swap alone is **not** a rollback — `rollback`
says so and exits non-zero rather than claiming success.

---

## 1. Before you touch anything (manual path)

| Check | Command | Why |
|---|---|---|
| Current version | `curl -s http://127.0.0.1:8080/api/ready` | Note `schema_version` — you will compare against it |
| Current binary kept | `sudo cp /usr/local/bin/veylixd /usr/local/bin/veylixd.prev` | The fastest rollback is swapping this file back |
| Backup taken | `sudo ./deploy/veylix-backup.sh backup` | Migrations are irreversible without it |
| Release checksum | `sha256sum -c veylixd-linux-amd64.sha256` | Never install an artifact you did not verify |

---

## 2. Upgrade

```bash
# 1. Verify the artifact you downloaded.
sha256sum -c veylixd-linux-amd64.sha256

# 2. Keep the old binary and take a verified backup.
sudo cp /usr/local/bin/veylixd /usr/local/bin/veylixd.prev
sudo ./deploy/veylix-backup.sh backup

# 3. Stop, swap, start.
sudo systemctl stop veylixd
sudo install -o root -g root -m 0755 veylixd-linux-amd64 /usr/local/bin/veylixd
sudo systemctl start veylixd

# 4. Prove it came up and migrated.
curl -s http://127.0.0.1:8080/api/ready
```

`/api/ready` must report:

- `"status": "ready"`
- `"ui": "embedded:<n>_files"` — **if it says `not_embedded`, you installed a
  binary built without the frontend** and every page will answer a JSON 404.
- `schema_version` **>=** the value you noted before upgrading.

Then sign in as the Super Admin and confirm **Servers** lists your panel as
`ACTIVE`. A migration that succeeds but leaves the panel unreachable is still a
failed upgrade from the operator's point of view.

---

## 3. Rollback

### 3a. Same schema (no migration ran)

Only possible if `schema_version` did not change. Swap the binary back:

```bash
sudo systemctl stop veylixd
sudo cp /usr/local/bin/veylixd.prev /usr/local/bin/veylixd
sudo systemctl start veylixd
```

### 3b. A migration ran

**You cannot roll the schema back.** The old binary expects tables and columns
that were altered. Restore the pre-upgrade backup instead:

```bash
sudo ./deploy/veylix-backup.sh restore /var/backups/veylix/<pre-upgrade-timestamp>
sudo install -o root -g root -m 0755 /usr/local/bin/veylixd.prev /usr/local/bin/veylixd
sudo systemctl restart veylixd
```

**Any data written between the backup and the rollback is lost.** That is why
step 2 above takes the backup before the service is stopped.

---

## 4. What a migration actually does

Migrations are **embedded in the binary** (`backend/internal/store/migrations/`,
currently 17 files) and applied **at startup**, inside a transaction, each
recorded in `schema_migrations`.

- There is no separate migrate command and no `--force` flag.
- A failed migration aborts startup rather than leaving a half-applied schema.
- Migrations are additive and backward-compatible by policy (Bible §0.4): no
  column is dropped to make a test pass.

---

## 5. Verifying a release before installing it

Each release publishes `SHA256SUMS`. Verify, then boot it on a throwaway port
before it touches the real service:

```bash
sha256sum -c SHA256SUMS

mkdir -p /tmp/veylix-check
VEYLIX_DATA_DIR=/tmp/veylix-check \
VEYLIX_ADDR=127.0.0.1:18082 \
VEYLIX_ENCRYPTION_KEY="$(sudo cat /etc/veylix/enc.key)" \
VEYLIX_SESSION_KEY="$(sudo cat /etc/veylix/session.key)" \
  ./veylixd-linux-amd64 &
curl -s http://127.0.0.1:18082/api/ready
kill %1
```

This runs the **real** migration path against a scratch copy. If it succeeds
there, it will succeed against the real database.

---

## 6. Status

| Item | State |
|---|---|
| `vpn-platformctl update` | ✅ **BUILT** — backup → checksum → swap → restart → `/api/ready` → schema comparison. `--dry-run` supported |
| `vpn-platformctl rollback` | ✅ **BUILT** — binary swap; refuses to claim success if the schema moved forward |
| Binary swap procedure (manual) | documented above; performed by hand on Windows during development |
| Backup/restore automation | `deploy/veylix-backup.sh` — shipped, **not yet rehearsed on a clean VM** |
| Release artifacts + checksums | `.github/workflows/release.yml` — builds amd64 + arm64 for **both** `veylixd` and `vpn-platformctl`, smoke-tests amd64, publishes `SHA256SUMS` |
| **Clean-VM upgrade test** | ❌ **NOT PERFORMED** — no Linux VM on this machine |

