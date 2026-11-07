---
name: veylix-deployment
description: Use when building or changing the Veylix installer, upgrade script, vpn-platformctl CLI, Caddy or systemd templates, backup/restore path, or uninstall; when writing a shell script that runs as root; when a script must decide between fresh install and upgrade; or when handling secret generation, checksum verification, locking or log redaction. Provides the idempotency pattern, the three-way branch, the secret-handling rules and the phase ordering that keep a deployment script from destroying customer data. Load before writing any deploy script.
---

# VEYLIX — DEPLOYMENT

Applied form of workflow `installation-and-deployment.md`. Read
`./VPN_FIX_LEDGER.md` first: a verification script deleted live data here,
**twice**. Nothing in this skill is advisory.

## The shape every script in this repository follows

```bash
#!/usr/bin/env bash
set -euo pipefail
```

`set -e` alone is not enough: an unset variable in `rm -rf "$DIR/"` is how the
wrong directory gets deleted. `set -u` catches it; `set -o pipefail` catches a
failing command hidden behind a pipe.

Every script also:

- sources `scripts/lib/common.sh` (say, die, redact, lock) rather than
  reimplementing those four things;
- takes `--dry-run` and honours it in **every** mutation;
- prints numbered phases, so a failure says which phase died;
- exits **non-zero** on any failure, and never leaves a service that looks
  started but is not.

## The three-way branch — the core pattern

A deployment script must decide its mode from the filesystem, never from an
argument alone.

```bash
DB="$VEYLIX_DATA_DIR/veylix.db"
if [ -f "$DB" ]; then
  if [ "${MODE:-}" = "upgrade" ]; then run_upgrade
  else die "already installed at $VEYLIX_DATA_DIR - run 'vpn-platformctl update', not the installer"; fi
else
  [ "${MODE:-}" = "upgrade" ] && die "nothing to upgrade: no database at $DB"
  run_fresh
fi
```

Why this shape:

- An operator who re-runs the install one-liner by accident gets a **refusal**,
  not a reinitialised database.
- An operator who runs `upgrade` on an empty host gets a clear error instead of a
  half-created install.
- The check is on the **database**, not the binary: replacing a binary is routine
  and harmless; the database is the thing that must not be recreated.

## Locking

Two concurrent runs must not interleave. Use `flock` on a fixed path and keep the
descriptor open for the life of the script:

```bash
exec 9>"${LOCK:-/run/veylix-install.lock}"
flock -n 9 || die "another veylix installer/upgrade is running (lock: ${LOCK})"
```

`-n` (non-blocking) is deliberate: waiting silently on a lock looks like a hang.

## Secret generation

Two 32-byte keys. The backend **refuses to start** without them
(`internal/config/config.go`, `decodeKey`), which is correct — fail closed.

```bash
gen_key() { head -c 32 /dev/urandom | base64; }
```

Rules:

1. Generate **only** when the file is absent. Never overwrite an existing key.
2. Refuse to proceed if a key exists but the operator asked for a fresh install:
   that is either a mistake or a second install pointed at the first one's
   config. Make them say `--reuse-keys` explicitly.
3. Write with `umask 077` and `install -m 0600 -o root -g root`.
4. Print a **fingerprint**, never the key: `sha256sum enc.key | cut -c1-8`.
5. `enc.key` is unrecoverable by design: every sealed panel token depends on it.
   Say that once, at the end of the install.

## What must never be printed

Redact before logging:

- any value of `VEYLIX_ENCRYPTION_KEY` / `VEYLIX_SESSION_KEY`,
- anything matching a base64 blob of 40+ characters near a key name,
- `Authorization: Bearer ...`,
- a Super Admin password, even when the operator typed it.

Prompt for a password with `read -rsp`, never as an argument. A password in
`argv` is visible in `ps` and lands in shell history.

The Super Admin is then created through the **application's own bootstrap
route** (`POST /api/setup/bootstrap`), which already refuses when any user
exists. Do not insert a user into the database directly, and do not ship a
default password.

## Checksum verification

Never install an artifact that was not verified.

```bash
curl -fsSLO "$BASE/$ARTIFACT"
curl -fsSLO "$BASE/$ARTIFACT.sha256"
expected=$(awk '{print $1}' "$ARTIFACT.sha256")
actual=$(sha256sum "$ARTIFACT" | awk '{print $1}')
[ "$expected" = "$actual" ] || { rm -f "$ARTIFACT"; die "checksum mismatch"; }
```

The `rm -f` on mismatch is intentional: a partial or tampered download must not
stay on disk where a later step could pick it up.

## Preflight — check before installing anything

Order matters: cheap checks first, so a failure costs nothing.

| Order | Check | On failure |
|---|---|---|
| 1 | root | "re-run with sudo" |
| 2 | lock acquired | "another run in progress" |
| 3 | OS + version in the allow-list | print the supported list |
| 4 | architecture | print what is supported |
| 5 | >= 200 MB disk on the target filesystem | print the shortfall |
| 6 | >= 400 MB RAM | print the shortfall |
| 7 | ports 80/443 free | name the holder |
| 8 | DNS A record resolves to this host | print both values |
| 9 | release artifact exists for this arch | name the missing release |

Every failure prints the **correction**, never a stack trace. A network-dependent
check (7, 8) gets an explicit `--skip-*` flag, and skipping it is recorded in the
output.

## Firewall

Do not modify firewall rules by default. If a rule is needed, print the exact
proposed rule and require an explicit flag. Never flush a chain, never disable a
firewall, and never touch the SSH port — locking the operator out of a server is
an outage the installer caused.

## systemd and Caddy

- Bind loopback only: `VEYLIX_ADDR=127.0.0.1:8080`. The binary does **not**
  terminate TLS; Caddy fronts it.
- `MemoryMax` is a **limit, not a measurement**. Never report it as measured
  usage.
- Caddy: render the template, then `caddy validate --config ...` **before**
  `systemctl reload caddy`. A syntax error takes the whole site down, including
  the old domain.
- Do not claim HTTPS succeeded until a real request over the public hostname
  returns 200. A failed ACME issuance is an **incomplete** install with a clear
  message, never "HTTPS active".

## Backup before anything destructive

Any path that overwrites or removes a database takes a backup first, using
`deploy/veylix-backup.sh`, which already stops the service, copies all three WAL
files plus both keys, and verifies the copy. Do not write a second backup
implementation.

## Uninstall

Data preserved by default:

```bash
systemctl disable --now veylixd
rm -f /etc/systemd/system/veylixd.service
rm -f /usr/local/bin/veylixd /usr/local/bin/veylixd.prev
# /var/lib/veylix, /etc/veylix and /var/backups/veylix stay.
```

A purge needs `--purge-data` **and** the operator typing the domain name to
confirm. Removing Veylix never deletes a 3X-UI client (D-027.3).

## Verify against the one canonical list

Fresh-install verification is `release-readiness.md` **Stage 5b**, and only
there. Do not keep a second copy of those 13 checks in a script comment — a
duplicated checklist drifts.

## Checklist before a deploy script is considered done

- [ ] `set -euo pipefail` present.
- [ ] `bash -n` clean; `shellcheck` clean, or its absence stated.
- [ ] `--dry-run` covers every mutation.
- [ ] Three-way branch present and tested both ways.
- [ ] `flock` guard present.
- [ ] Keys generated only when absent; reuse refused without an explicit flag.
- [ ] No secret in any output path; a fingerprint instead.
- [ ] Checksum verified before install.
- [ ] `caddy validate` before reload.
- [ ] Backup before any destructive path.
- [ ] Uninstall preserves data; purge double-confirmed.
- [ ] Ran isolated, with the throwaway paths recorded.

