# VEYLIX — BACKUP & RESTORE

**What must be backed up, why a `cp` is not enough, and how to prove a backup
before you need it.** The last part matters most: an unrestorable backup is
worse than no backup, because it is trusted.

---

## 1. What to back up

| Item | Path (default) | What happens if it is lost |
|---|---|---|
| `veylix.db` | `/var/lib/veylix/` | Every account, server, order and audit row is gone |
| `veylix.db-wal` | same directory | **Committed transactions that have not yet checkpointed** are lost |
| `veylix.db-shm` | same directory | SQLite rebuilds it; safe to lose, but copy it with the others |
| `enc.key` | `/etc/veylix/` | **Every sealed 3X-UI panel token becomes unrecoverable** — each panel token must be re-entered in the UI |
| `session.key` | `/etc/veylix/` | Everyone is signed out. Harmless |
| `veylix.env` | `/etc/veylix/` | Settings lost; re-create from `deploy/veylix.env.example` |

> **What is NOT in the backup:** the VPN clients themselves. Every VPN lives on
> its 3X-UI panel, and Veylix never deletes a remote client (D-027.3). Restoring
> Veylix does not restore a VPN; it restores *Veylix's record of* one.

---

## 2. Why `cp veylix.db` is wrong

SQLite runs in **WAL mode** (that is how this platform gets concurrent reads
without a separate server). In WAL mode:

- a committed transaction may live only in `-wal`, not in `veylix.db`;
- copying `veylix.db` alone therefore gives you a database that is *several
  transactions old*, and it will still open cleanly — so the mistake is silent.

That is why the shipped script stops the service first. It costs a few seconds
of downtime and removes the entire class of "the backup looked fine" problems.

---

## 3. Take a backup

```bash
sudo ./deploy/veylix-backup.sh backup
```

What it does, in order:

1. **Stops `veylixd`** (only if it was running — a stopped service is not an error).
2. Copies `veylix.db`, `-wal`, `-shm` **and** both keys into
   `/var/backups/veylix/<UTC timestamp>/`.
3. **Verifies the copy** — `PRAGMA integrity_check` when `sqlite3` is available,
   otherwise a SQLite-header check — and counts applied migrations.
4. `chmod go-rwx` so customer data is not world-readable.
5. **Restarts the service** in a `trap`, so a failed backup never leaves the
   platform down.

Exit code is non-zero on any failure. Nothing is deleted.

Set a different destination with `VEYLIX_BACKUP_DIR=/mnt/backups/veylix`.

---

## 4. Verify a backup without restoring it

```bash
sudo ./deploy/veylix-backup.sh verify /var/backups/veylix/20261103T101500Z
```

Expected output:

```
veylix-backup: verify: ok — integrity ok, 17 migration(s) applied
```

Run this **on the backup every time**, and again before a restore. A backup that
fails this check must not be used.

---

## 5. Restore

```bash
sudo ./deploy/veylix-backup.sh restore /var/backups/veylix/20261103T101500Z
```

The script is deliberately cautious:

1. **Verifies the backup first** and refuses to continue if it is bad. This is
   the check that stops a corrupt backup from destroying live data.
2. Stops the service.
3. **Moves the current database aside** to
   `/var/lib/veylix/pre-restore-<timestamp>/` — it is never deleted, so a restore
   made by mistake is itself undoable.
4. Installs the database and sidecars with the correct owner (`veylix:veylix`,
   mode `0640`) and the keys as `root:root 0600`.
5. Starts the service and restores.

Afterwards, confirm:

```bash
curl -s http://127.0.0.1:8080/api/ready
# schema_version must be what you expect; ui must say embedded
sudo -u veylix ls -l /var/lib/veylix
```

---

## 6. Restore rehearsal — do this before you need it

A backup is a claim until it has been restored once. On a **non-production**
machine:

```bash
# 1. Copy the latest backup to a scratch host.
# 2. Restore it into an empty data directory.
VEYLIX_DATA_DIR=/tmp/veylix-rehearsal sudo -E ./deploy/veylix-backup.sh restore <dir>

# 3. Start a throwaway instance against it and sign in.
VEYLIX_DATA_DIR=/tmp/veylix-rehearsal VEYLIX_ADDR=127.0.0.1:18081 ./veylixd
curl -s http://127.0.0.1:18081/api/ready
```

**Status of this rehearsal in this repository: NOT PERFORMED.** There is no
second Linux host in the development environment. It is listed as an open item
in `docs/INSTALL.md` §F and must be done on a clean VM before the first
production deployment.

---

## 7. Suggested schedule

| What | How often | Retention |
|---|---|---|
| Automated `veylix-backup.sh backup` (cron) | every 6 hours | 14 days |
| Copy off the server (`rsync`/`rclone` to another host) | daily | 30 days |
| **Restore rehearsal** on a scratch host | monthly, and after every upgrade | — |

Example cron entry (root):

```cron
0 */6 * * * /opt/veylix/deploy/veylix-backup.sh backup >> /var/log/veylix-backup.log 2>&1
```

Keep the keys somewhere *other than* the database's own disk: a host-level
failure otherwise takes both copies of `enc.key` with it, and the panel tokens
in every backup become unreadable at the exact moment they are needed.
