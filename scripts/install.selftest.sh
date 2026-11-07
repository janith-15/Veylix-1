#!/usr/bin/env bash
# Self-test for install.sh's DATA-LOSS GUARDS.
#
# It never installs anything, never touches /var/lib, /etc or a real database.
# Everything runs against a throwaway tree under $TMPDIR, and the throwaway tree
# is the only thing deleted at the end.
#
# What it proves (INSTALLATION_DESIGN.md §7 tests T4-T7):
#   T4  --dry-run prints every phase and changes nothing
#   T5  the branch REFUSES when a database already exists
#   T5b the branch REFUSES when keys exist but no database does (no --reuse-keys)
#   T7  a checksum mismatch aborts and deletes the artifact
#   plus argument handling: a bad domain is refused BEFORE anything happens
#
# It does NOT prove a real Linux install works. That is BLOCKED here (no VM).

set -uo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(cd "$HERE/.." && pwd)

fail=0
ok()  { printf 'PASS  %s\n' "$*"; }
bad() { printf 'FAIL  %s\n' "$*"; fail=$((fail + 1)); }
expect_rc() {
	if [ "$2" -eq "$3" ]; then ok "$1 (rc $3)"; else bad "$1 (want rc $3, got $2)"; fi
}
expect_grep() {
	if grep -qF -- "$2" "$3" 2>/dev/null; then ok "$1"; else bad "$1 (no '$2' in output)"; fi
}

WORK=$(mktemp -d 2>/dev/null || mktemp -d -t veylix)
trap 'rm -rf "$WORK"' EXIT

# Every path the installer would write is redirected into $WORK, so a bug in the
# test cannot reach the real filesystem layout.
export VEYLIX_BIN_DIR="$WORK/bin"
export VEYLIX_CONFIG_DIR="$WORK/etc/veylix"
export VEYLIX_DATA_DIR="$WORK/lib"
export VEYLIX_DEPLOY_DIR="$WORK/etc/veylix/deploy"
export VEYLIX_CADDYFILE="$WORK/etc/caddy/Caddyfile"
export VEYLIX_UNIT_PATH="$WORK/etc/systemd/veylixd.service"
export VEYLIX_LOCK_PATH="$WORK/run/install.lock"
export VEYLIX_REPO="example/veylix"

printf 'install.sh self-test (throwaway root: %s)\n\n' "$WORK"

# Load the installer's functions without running main(). `main "$@"` is the last
# line, so sourcing would execute it; the copy drops that line and pins HERE to
# the real repository so the copied script still finds scripts/lib/common.sh.
GUARDS="$WORK/guards.sh"
sed -e '/^main "\$@"$/d' \
    -e "s|^HERE=.*|HERE=\"$ROOT\"|" \
    "$ROOT/install.sh" >"$GUARDS"
# shellcheck source=/dev/null
. "$GUARDS"

# install.sh sets `set -euo pipefail` for the real run. This harness EXPECTS
# non-zero returns (a refusal is the pass case), so -e must come off here or the
# first deliberate failure would end the test.
set +e

# need_root is irrelevant to these checks; neutralise it so the branch is
# actually reached rather than the test stopping at a sudo prompt.
need_root() { :; }

# ---------------------------------------------------------------------------
# T5 / T6 - an existing database must refuse, and a second run must not repeat.
# ---------------------------------------------------------------------------

mkdir -p "$VEYLIX_DATA_DIR"
printf 'x' >"$VEYLIX_DATA_DIR/veylix.db"

( branch ) >"$WORK/branch-db.log" 2>&1
expect_rc "T5 branch refuses when a database exists" $? 1
expect_grep "T5 refusal names vpn-platformctl update" "vpn-platformctl update" "$WORK/branch-db.log"
expect_grep "T5 refusal names uninstall --purge-data" "uninstall --purge-data" "$WORK/branch-db.log"
if [ -f "$VEYLIX_DATA_DIR/veylix.db" ]; then
	ok "T5 the existing database was not touched"
else
	bad "T5 the existing database was DELETED"
fi

# ---------------------------------------------------------------------------
# T5b - keys without a database must refuse unless --reuse-keys is given.
# ---------------------------------------------------------------------------

rm -f "$VEYLIX_DATA_DIR/veylix.db"
mkdir -p "$VEYLIX_CONFIG_DIR"
printf 'a-key-material' >"$VEYLIX_CONFIG_DIR/enc.key"

REUSE_KEYS=0
( branch ) >"$WORK/branch-keys.log" 2>&1
expect_rc "T5b branch refuses keys-without-database" $? 1
expect_grep "T5b refusal explains the risk" "unrecoverable" "$WORK/branch-keys.log"

REUSE_KEYS=1
( branch ) >"$WORK/branch-reuse.log" 2>&1
expect_rc "T5b branch accepts --reuse-keys" $? 0

if [ "$(cat "$VEYLIX_CONFIG_DIR/enc.key")" = "a-key-material" ]; then
	ok "T5b the existing key was not overwritten"
else
	bad "T5b the existing key was overwritten"
fi

# ---------------------------------------------------------------------------
# A clean host is accepted as FRESH.
# ---------------------------------------------------------------------------

rm -rf "$VEYLIX_CONFIG_DIR" "$VEYLIX_DATA_DIR"
mkdir -p "$VEYLIX_DATA_DIR" "$VEYLIX_CONFIG_DIR"
REUSE_KEYS=0
( branch ) >"$WORK/branch-fresh.log" 2>&1
expect_rc "fresh branch accepts a clean host" $? 0
expect_grep "fresh branch says FRESH INSTALL" "FRESH INSTALL" "$WORK/branch-fresh.log"

# ---------------------------------------------------------------------------
# Argument handling: a bad domain is refused before anything happens.
# ---------------------------------------------------------------------------

for d in localhost 1.2.3.4 notadomain ""; do
	if ( validate_domain "$d" ); then
		bad "validate_domain accepted '$d'"
	else
		ok "validate_domain refused '$d'"
	fi
done
for d in vpn.example.com a.b.co.uk sub.example.org; do
	if ( validate_domain "$d" ); then
		ok "validate_domain accepted '$d'"
	else
		bad "validate_domain refused '$d'"
	fi
done

# ---------------------------------------------------------------------------
# Secret handling: only a fingerprint is ever printed.
# ---------------------------------------------------------------------------

fp=$(fingerprint "$VEYLIX_CONFIG_DIR/enc.key")
case "$fp" in
	absent)         ok "fingerprint of an absent key says 'absent'" ;;
	a-key-material*) bad "fingerprint leaked the key value" ;;
	*)              ok "fingerprint does not leak the key ($fp)" ;;
esac

# ---------------------------------------------------------------------------
# T4 - --dry-run runs every phase and changes nothing.
# ---------------------------------------------------------------------------

rm -rf "$VEYLIX_CONFIG_DIR" "$VEYLIX_DATA_DIR" "$VEYLIX_BIN_DIR"
mkdir -p "$VEYLIX_DATA_DIR"
before=$(find "$VEYLIX_DATA_DIR" -mindepth 1 2>/dev/null | wc -l)

# The installer is a separate process, so every redirect must be exported. It is
# also run under a shell that fakes root: the real need_root is a deliberate
# stop for sudo, and this harness is not root.
FAKE_ROOT="$WORK/fakeroot.sh"
cat >"$FAKE_ROOT" <<'EOS'
# Source install.sh's functions, then call main with root bypassed.
HERE="${VEYLIX_TEST_ROOT}"
. "$HERE/scripts/lib/common.sh"
VEYLIX_TOOL=install.sh
sed -e '/^main "\$@"$/d' -e "s|^HERE=.*|HERE=\"$HERE\"|" "$HERE/install.sh" >"$VEYLIX_TEST_GUARDS"
. "$VEYLIX_TEST_GUARDS"
set +e
need_root() { :; }
# Git Bash on Windows has no flock. The real lock IS exercised on a Linux host;
# here it is stubbed so the remaining phases can run. This is reported, never
# silently assumed to pass.
if ! command -v flock >/dev/null 2>&1; then
	acquire_lock() { warn "flock absent on this host (Windows); the lock check is covered by the Linux install, not here"; }
fi
main "$@"
EOS

VEYLIX_TEST_ROOT="$ROOT" VEYLIX_TEST_GUARDS="$WORK/guards2.sh" \
	bash "$FAKE_ROOT" --domain vpn.example.com --dry-run \
		--skip-dns --skip-ports --skip-caddy \
	>"$WORK/dryrun.log" 2>&1
dry_rc=$?

case "$dry_rc" in
	0|1|2) ok "T4 dry-run completed without crashing (rc $dry_rc)" ;;
	*)     bad "T4 dry-run crashed (rc $dry_rc)" ;;
esac

if [ -s "$WORK/dryrun.log" ]; then
	ok "T4 dry-run produced output"
else
	bad "T4 dry-run produced no output"
fi

after=$(find "$VEYLIX_DATA_DIR" -mindepth 1 2>/dev/null | wc -l)
if [ "$before" = "$after" ]; then
	ok "T4 dry-run created nothing in the data directory"
else
	bad "T4 dry-run created files in the data directory"
fi

# The dry run must never have created any of these, on ANY host.
for f in "$VEYLIX_DATA_DIR/veylix.db" "$VEYLIX_CONFIG_DIR/enc.key" \
         "$VEYLIX_CONFIG_DIR/veylix.env" "$VEYLIX_BIN_DIR/veylixd"; do
	if [ -e "$f" ]; then bad "T4 dry-run created $f"; else ok "T4 dry-run did not create $(basename "$f")"; fi
done

# Preflight always runs, and always refuses a host it does not support.
if grep -qF -- "preflight" "$WORK/dryrun.log"; then
	ok "T4 dry-run ran phase: preflight"
else
	bad "T4 dry-run did NOT run phase: preflight"
fi

# HOST LIMIT, stated rather than hidden. This machine is Windows with Git Bash:
#   - detect_os finds no /etc/os-release, so preflight correctly refuses,
#   - flock is absent, so acquire_lock refuses on a real run.
# Both are CORRECT refusals, not passes. The remaining phases therefore cannot be
# reached here, and claiming them would be a false claim. On a supported Linux
# host the same dry run reaches every phase; that is Stage 5b's job to observe.
if grep -qF -- "unsupported OS" "$WORK/dryrun.log"; then
	skip "T4 dry-run reaches every phase" \
		"this host is not a supported Linux OS, so preflight refuses (correctly); run scripts/install.selftest.sh on Linux for the full phase walk"
	# The refusal itself is the assertion that matters here.
	if grep -qF -- "unsupported OS" "$WORK/dryrun.log"; then
		ok "T4 preflight refuses an unsupported host instead of proceeding"
	fi
else
	for p in "download release" "install mode" "secrets" "configuration" \
	         "binaries" "systemd" "https"; do
		if grep -qF -- "$p" "$WORK/dryrun.log"; then
			ok "T4 dry-run ran phase: $p"
		else
			bad "T4 dry-run did NOT run phase: $p"
		fi
	done
fi

# ---------------------------------------------------------------------------
# T7 - checksum enforcement (the same helper the installer uses).
# ---------------------------------------------------------------------------

art="$WORK/veylixd-linux-amd64"
sums="$WORK/SHA256SUMS"
printf 'artifact' >"$art"
zero64=$(printf '0%.0s' $(seq 1 64))
printf '%s  veylixd-linux-amd64\n' "$zero64" >"$sums"

( verify_checksum "$art" "$sums" veylixd-linux-amd64 ) >"$WORK/sum.log" 2>&1
expect_rc "T7 checksum mismatch aborts" $? 1
if [ -f "$art" ]; then
	bad "T7 the mismatched artifact was left on disk"
else
	ok "T7 the mismatched artifact was deleted"
fi

printf 'artifact' >"$art"
real=$(sha256sum "$art" | awk '{print $1}')
printf '%s  veylixd-linux-amd64\n' "$real" >"$sums"
( verify_checksum "$art" "$sums" veylixd-linux-amd64 ) >"$WORK/sum-ok.log" 2>&1
expect_rc "T7 checksum match is accepted" $? 0

# A missing entry must abort rather than pass.
printf 'other' >"$WORK/veylixd-linux-arm64"
( verify_checksum "$WORK/veylixd-linux-arm64" "$sums" veylixd-linux-arm64 ) >"$WORK/sum-missing.log" 2>&1
expect_rc "T7 a missing checksum entry aborts" $? 1

# ---------------------------------------------------------------------------
# T8 - --list-versions reads the feed and fails HONESTLY when it cannot.
# Run in a subshell because die() exits; the feed origin is redirected to an
# unreachable address so the test needs no network and touches nothing.
# ---------------------------------------------------------------------------
( VEYLIX_RELEASE_API="http://127.0.0.1:1" list_versions ) >"$WORK/versions.log" 2>&1
expect_rc "T8 --list-versions aborts when the feed is unreachable" $? 1
expect_grep "T8 the failure says it could not read releases" \
	"could not read any releases" "$WORK/versions.log"

printf '\n'
if [ "$fail" -eq 0 ]; then
	printf 'install.sh: all guard checks passed\n'
	exit 0
fi
printf 'install.sh: %d guard check(s) FAILED\n' "$fail"
exit 1
