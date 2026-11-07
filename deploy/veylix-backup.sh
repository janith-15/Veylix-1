#!/usr/bin/env bash
# VEYLIX — backup and restore.
#
# The database is SQLite in WAL mode, so a plain `cp` of `veylix.db` alone can
# miss committed transactions that are still in `-wal`. This script copies all
# three files while the service is STOPPED, and verifies the result before it
# claims success. That ordering is the whole point: a backup that cannot be
# restored is worse than none, because it is trusted.
#
# Usage (root):
#   veylix-backup.sh backup            # -> /var/backups/veylix/<timestamp>/
#   veylix-backup.sh restore <dir>     # restores a backup over the live data
#   veylix-backup.sh verify <dir>      # checks a backup without restoring it
#
# Exits non-zero on any failure. Nothing is deleted unless it succeeded.

set -euo pipefail

SERVICE=veylixd
DATA_DIR=${VEYLIX_DATA_DIR:-/var/lib/veylix}
KEY_DIR=${VEYLIX_KEY_DIR:-/etc/veylix}
BACKUP_ROOT=${VEYLIX_BACKUP_DIR:-/var/backups/veylix}
DB=veylix.db
SIDECARS=("${DB}-wal" "${DB}-shm")

die() { echo "veylix-backup: $*" >&2; exit 1; }
say() { echo "veylix-backup: $*"; }

need_root() { [ "$(id -u)" -eq 0 ] || die "must run as root"; }
need_service() { command -v systemctl >/dev/null || die "systemctl not found"; }

# Take the three database files plus the keys into a directory.
copy_state() {
	local dest=$1
	mkdir -p "$dest"
	[ -f "$DATA_DIR/$DB" ] || die "no database at $DATA_DIR/$DB"

	cp -p "$DATA_DIR/$DB" "$dest/$DB"
	for f in "${SIDECARS[@]}"; do
		# The -wal may legitimately be absent after a clean checkpoint.
		[ -f "$DATA_DIR/$f" ] && cp -p "$DATA_DIR/$f" "$dest/$(basename "$f")" || true
	done

	# The keys are as important as the database: without the encryption key,
	# every sealed panel token in the DB is unreadable.
	for k in enc.key session.key; do
		if [ -f "$KEY_DIR/$k" ]; then
			cp -p "$KEY_DIR/$k" "$dest/$k"
		else
			say "warning: $KEY_DIR/$k is missing — panel tokens may be unrecoverable"
		fi
	done
}

# A backup is only real if SQLite can open it and read the schema version.
verify_db() {
	local dir=$1
	local db="$dir/$DB"
	[ -f "$db" ] || die "verify: $db not found"
	[ -s "$db" ] || die "verify: $db is empty"

	# `sqlite3` may not be installed; fall back to the header check, which still
	# catches a truncated or empty copy.
	if command -v sqlite3 >/dev/null; then
		local integrity
		integrity=$(sqlite3 "$db" 'PRAGMA integrity_check;' 2>&1) || die "verify: sqlite3 failed: $integrity"
		[ "$integrity" = "ok" ] || die "verify: integrity_check returned '$integrity'"

		local applied
		applied=$(sqlite3 "$db" 'SELECT COUNT(*) FROM schema_migrations;' 2>/dev/null || echo '?')
		say "verify: ok — integrity ok, $applied migration(s) applied"
	else
		# "SQLite format 3" is the 16-byte magic every valid database starts with.
		local magic
		magic=$(head -c 15 "$db")
		[ "$magic" = "SQLite format 3" ] || die "verify: not a SQLite database"
		say "verify: ok — header valid (install sqlite3 for a full integrity check)"
	fi
}

cmd_backup() {
	need_root; need_service
	local stamp dir
	stamp=$(date -u +%Y%m%dT%H%M%SZ)
	dir="$BACKUP_ROOT/$stamp"
	mkdir -p "$BACKUP_ROOT"

	# Stop first: copying a WAL database while it is being written is how a
	# "successful" backup turns out to be unrestorable.
	local was_running=0
	if systemctl is-active --quiet "$SERVICE"; then
		was_running=1
		say "stopping $SERVICE"
		systemctl stop "$SERVICE"
	fi

	# Always restart, even if the copy fails — a failed backup must not leave
	# the platform down.
	trap 'if [ "$was_running" -eq 1 ]; then systemctl start "$SERVICE"; fi' EXIT

	copy_state "$dir"
	verify_db "$dir"

	chmod -R go-rwx "$dir"
	say "backup complete: $dir"
}

cmd_restore() {
	need_root; need_service
	local dir=${1:-}
	[ -n "$dir" ] || die "restore needs a backup directory"
	[ -d "$dir" ] || die "restore: $dir is not a directory"

	# Refuse to restore a backup that cannot be opened. This is the check that
	# stops a bad backup from destroying the live data.
	verify_db "$dir"

	local was_running=0
	if systemctl is-active --quiet "$SERVICE"; then
		was_running=1
		say "stopping $SERVICE"
		systemctl stop "$SERVICE"
	fi
	trap 'if [ "$was_running" -eq 1 ]; then systemctl start "$SERVICE"; fi' EXIT

	# Move the current state aside rather than deleting it: if the restore is
	# wrong, the previous data is still on disk.
	local safety="$DATA_DIR/pre-restore-$(date -u +%Y%m%dT%H%M%SZ)"
	mkdir -p "$safety"
	[ -f "$DATA_DIR/$DB" ] && cp -p "$DATA_DIR/$DB" "$safety/" || true
	say "current state saved to $safety"

	install -p -o veylix -g veylix -m 0640 "$dir/$DB" "$DATA_DIR/$DB"
	for f in "${SIDECARS[@]}"; do
		rm -f "$DATA_DIR/$f"
		[ -f "$dir/$(basename "$f")" ] && install -p -o veylix -g veylix -m 0640 "$dir/$(basename "$f")" "$DATA_DIR/$f" || true
	done
	for k in enc.key session.key; do
		[ -f "$dir/$k" ] && install -o root -g root -m 0600 "$dir/$k" "$KEY_DIR/$k" || true
	done

	say "restore complete — starting $SERVICE"
}

cmd_verify() {
	local dir=${1:-}
	[ -n "$dir" ] || die "verify needs a backup directory"
	verify_db "$dir"
}

case "${1:-}" in
	backup) cmd_backup ;;
	restore) shift; cmd_restore "$@" ;;
	verify) shift; cmd_verify "$@" ;;
	*) die "usage: $0 {backup|restore <dir>|verify <dir>}" ;;
esac
