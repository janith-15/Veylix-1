#!/usr/bin/env bash
# Self-test for scripts/lib/common.sh. Runs everywhere bash does; never touches
# a repo path, a real data directory or a secret.
set -uo pipefail

# Resolve the repository root from this file's own location, so the test can be
# run from any working directory.
SELFTEST_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd "$SELFTEST_DIR/../.." && pwd)
cd "$REPO_ROOT" || exit 1

# shellcheck source=/dev/null
. "$SELFTEST_DIR/common.sh"

# TMPDIR is set on POSIX but not in Git Bash on Windows; fall back to /tmp.
: "${TMPDIR:=/tmp}"
mkdir -p "$TMPDIR" 2>/dev/null || true

fail=0
ok()   { printf 'PASS  %s\n' "$*"; }
bad()  { printf 'FAIL  %s\n' "$*"; fail=$((fail + 1)); }
expect_eq() {
	if [ "$2" = "$3" ]; then ok "$1"; else bad "$1 (want '$3', got '$2')"; fi
}
expect_rc() {
	if [ "$2" -eq "$3" ]; then ok "$1"; else bad "$1 (want rc $3, got $2)"; fi
}

say "common.sh self-test"

# --- redact ---------------------------------------------------------------
r=$(redact 'VEYLIX_ENCRYPTION_KEY=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=')
case "$r" in *AAAAAAAAAAAA*) bad "redact key assignment";; *) ok "redact key assignment";; esac

r=$(redact 'Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.abc.def')
case "$r" in *eyJhbGci*) bad "redact bearer";; *'Bearer <redacted>'*) ok "redact bearer";; *) bad "redact bearer (unexpected: $r)";; esac

r=$(redact 'secret=GOCSPX-abcdefghijklmnop')
case "$r" in *GOCSPX-abcdef*) bad "redact gocspx";; *) ok "redact gocspx";; esac

r=$(redact 'a bare 44 char base64 key abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG=')
case "$r" in *abcdefghijklmnopqrst*) bad "redact bare base64";; *) ok "redact bare base64";; esac

# Non-secrets must survive untouched.
expect_eq "redact leaves a path alone" "$(redact '/var/lib/veylix/veylix.db')" '/var/lib/veylix/veylix.db'
expect_eq "redact leaves a domain alone" "$(redact 'vpn.example.com')" 'vpn.example.com'

# --- validate_domain ------------------------------------------------------
for d in vpn.example.com a.b.co.uk sub-domain.example.org; do
	if validate_domain "$d"; then ok "domain valid: $d"; else bad "domain valid: $d"; fi
done
for d in localhost 1.2.3.4 notadomain "" '.example.com' 'example.com.' 'a..b.com'; do
	if validate_domain "$d"; then bad "domain refused: '$d'"; else ok "domain refused: '$d'"; fi
done

# --- dry-run must not mutate ---------------------------------------------
target="$TMPDIR/veylix-selftest-dry-$$"
DRY_RUN=1
run mkdir -p "$target" >/dev/null
if [ -e "$target" ]; then bad "dry-run created a directory"; rm -rf "$target"; else ok "dry-run created nothing"; fi
DRY_RUN=0

# --- fingerprint never reveals the key -----------------------------------
tmpk="$TMPDIR/veylix-selftest-key-$$"
printf 'not-a-real-key\n' >"$tmpk"
fp=$(fingerprint "$tmpk")
case "$fp" in
	*not-a-real-key*) bad "fingerprint leaked the value";;
	???????*)         ok "fingerprint is 8 chars: $fp";;
	*)                bad "fingerprint malformed: $fp";;
esac
expect_eq "fingerprint of absent file" "$(fingerprint "$TMPDIR/veylix-definitely-absent-$$")" 'absent'
rm -f "$tmpk"

# --- checksum -------------------------------------------------------------
work="$TMPDIR/veylix-selftest-sums-$$"
mkdir -p "$work"
printf 'artifact payload' >"$work/veylixd-linux-amd64"
(cd "$work" && sha256sum veylixd-linux-amd64 >SHA256SUMS)

# Each verify runs in a subshell: `die` calls `exit`, and a failing case must
# not terminate the test harness.
( verify_checksum "$work/veylixd-linux-amd64" "$work/SHA256SUMS" veylixd-linux-amd64 ) >/dev/null 2>&1
expect_rc "verify_checksum accepts a match" $? 0

printf 'tampered' >"$work/veylixd-linux-amd64"
( verify_checksum "$work/veylixd-linux-amd64" "$work/SHA256SUMS" veylixd-linux-amd64 ) >/dev/null 2>&1
expect_rc "verify_checksum rejects a mismatch" $? 1
if [ -e "$work/veylixd-linux-amd64" ]; then bad "mismatched artifact was left on disk"; else ok "mismatched artifact was deleted"; fi

# A missing entry must abort rather than silently pass.
printf 'other' >"$work/veylixd-linux-arm64"
( verify_checksum "$work/veylixd-linux-arm64" "$work/SHA256SUMS" veylixd-linux-arm64 ) >/dev/null 2>&1
expect_rc "verify_checksum aborts on a missing entry" $? 1
rm -f "$work/veylixd-linux-arm64"

# --- write_secret refuses to overwrite -----------------------------------
existing="$TMPDIR/veylix-selftest-existing-$$"
printf 'keep-me' >"$existing"
before=$(cat "$existing")
( printf 'replace' | write_secret "$existing" 0600 ) >/dev/null 2>&1
expect_rc "write_secret refuses an existing file" $? 1
expect_eq "write_secret left the file untouched" "$(cat "$existing")" "$before"
rm -f "$existing"

# write_secret creates a file that did not exist, with the requested mode.
fresh="$TMPDIR/veylix-selftest-fresh-$$"
rm -f "$fresh"
( printf 'a-new-secret-value' | write_secret "$fresh" 0600 ) >/dev/null 2>&1
expect_rc "write_secret creates an absent file" $? 0
if [ -f "$fresh" ]; then ok "created file exists"; else bad "created file missing"; fi
expect_eq "created file holds exactly what was piped in" "$(cat "$fresh" 2>/dev/null)" 'a-new-secret-value'
# A key must be unreadable by group and other.
#
# Caveat, stated rather than hidden: Git Bash on Windows mounts with `noacl`,
# so POSIX modes are emulated and `chmod 600` reads back as 644. The assertion
# therefore only applies where the filesystem can represent the mode; where it
# cannot, that fact is printed instead of being reported as a pass.
if mkdir -p "$TMPDIR/veylix-permcheck-$$" 2>/dev/null &&
   touch "$TMPDIR/veylix-permcheck-$$/p" 2>/dev/null &&
   chmod 600 "$TMPDIR/veylix-permcheck-$$/p" 2>/dev/null &&
   [ "$(stat -c '%a' "$TMPDIR/veylix-permcheck-$$/p" 2>/dev/null)" = "600" ]; then
	perm=$(stat -c '%a' "$fresh" 2>/dev/null)
	expect_eq "created file is mode 600" "$perm" '600'
else
	skip "created file is mode 600" "this filesystem cannot represent POSIX modes (noacl mount)"
fi
rm -rf "$TMPDIR/veylix-permcheck-$$"
rm -f "$fresh"

# --- preflight primitives on this host ------------------------------------
expect_eq "detect_arch returns a known value" \
	"$(case "$(detect_arch)" in amd64|arm64) echo known;; *) echo other;; esac)" known
check_ram 1; expect_rc "check_ram 1 MB passes" $? 0
check_ram 100000000; expect_rc "check_ram absurd amount fails" $? 1

rm -rf "$work"
printf '\n'
if [ "$fail" -eq 0 ]; then
	say "common.sh: all checks passed"
	exit 0
fi
say "common.sh: $fail check(s) FAILED"
exit 1
