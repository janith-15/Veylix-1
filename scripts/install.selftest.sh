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

# This script is automated. A controlling terminal is inherited by child
# processes, so without this every prompt inside install.sh would still find a
# working /dev/tty and block a test run that nobody is watching.
export VEYLIX_NONINTERACTIVE=1

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
# line, so sourcing would execute it; the copy drops that line and pins the
# location with VEYLIX_HERE, which install.sh honours before BASH_SOURCE. The
# copy lives in $WORK, so BASH_SOURCE alone would point at the wrong tree.
GUARDS="$WORK/guards.sh"
sed -e '/^main "\$@"$/d' "$ROOT/install.sh" >"$GUARDS"
# shellcheck source=/dev/null
VEYLIX_HERE="$ROOT" . "$GUARDS"

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
VEYLIX_HERE="$HERE"
. "$HERE/scripts/lib/common.sh"
VEYLIX_TOOL=install.sh
sed -e '/^main "\$@"$/d' "$HERE/install.sh" >"$VEYLIX_TEST_GUARDS"
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
# T8 - `out` exists. It was referenced nine times in install.sh and defined
# nowhere, so under `set -euo pipefail` every dry-run line aborted the installer
# with "out: command not found" (exit 127).
# ---------------------------------------------------------------------------

if type out >/dev/null 2>&1; then
	ok "T8 out() is defined by common.sh"
else
	bad "T8 out() is undefined - a --dry-run line would abort the installer"
fi
if grep -qF -- "command not found" "$WORK/dryrun.log"; then
	bad "T8 the dry run hit 'command not found'"
else
	ok "T8 the dry run hit no missing command"
fi
case "$dry_rc" in
	127) bad "T8 dry run exited 127 (missing command)" ;;
	*)   ok "T8 dry run did not exit 127 (rc $dry_rc)" ;;
esac

# ---------------------------------------------------------------------------
# T9 - repository slug validation. The shipped default is a placeholder; taking
# it to github.com produced a 404 reported as a network failure.
# ---------------------------------------------------------------------------

for good in acme/veylix difanmax/Veylix-2 a.b/c_d; do
	if ( validate_repo "$good" ); then
		ok "T9 validate_repo accepts '$good'"
	else
		bad "T9 validate_repo refused '$good'"
	fi
done

for bad_repo in "" "https://github.com/acme/veylix" \
                "git@github.com:acme/veylix.git" "acme" "acme/" "/veylix" "a/b/c" "ac me/veylix"; do
	if ( validate_repo "$bad_repo" ); then
		bad "T9 validate_repo accepted '$bad_repo'"
	else
		ok "T9 validate_repo refused '${bad_repo:-<empty>}'"
	fi
done

# --repo and --repo= must both reach $REPO, and the flag must win over the env.
REPO="$PLACEHOLDER_REPO"
parse_args --repo acme/veylix
if [ "$REPO" = "acme/veylix" ]; then ok "T9 --repo sets the slug"; else bad "T9 --repo left REPO=$REPO"; fi
REPO="$PLACEHOLDER_REPO"
parse_args --repo=other/repo
if [ "$REPO" = "other/repo" ]; then ok "T9 --repo= sets the slug"; else bad "T9 --repo= left REPO=$REPO"; fi


# A malformed repo slug must be refused BEFORE anything is downloaded, with exit
# 2. The shipped default is a real repository, so this uses an explicitly bogus
# one: the guarantee under test is that a typo names its correction instead of
# surfacing later as a 404 blamed on the network.
REPO="acme"
( main --domain vpn.example.com --dry-run --skip-dns --skip-ports --skip-caddy ) \
>"$WORK/placeholder.log" 2>&1
expect_rc "T9 a malformed repo is refused (exit 2)" $? 2
expect_grep "T9 the refusal names the slug shape" "owner" "$WORK/placeholder.log"

# ---------------------------------------------------------------------------
# T10 - deployment templates resolve, and only from a trusted source.
#
# A release used to publish binaries only, so a one-curl install had no
# Caddyfile, no unit and no backup script. resolve_deploy_dir() must find the
# release bundle, and must extract ONLY deploy/* from it.
# ---------------------------------------------------------------------------

# (a) a checkout next to the installer is used as-is
# The probe runs in a FUNCTION, not a subshell: resolve_deploy_dir sets the
# global DEPLOY_SRC, and a (subshell) would throw that assignment away.
probe_deploy_dir() { resolve_deploy_dir >/dev/null 2>&1; }
DEPLOY_SRC=""
probe_deploy_dir
if [ "$(deploy_file veylixd.service)" = "$ROOT/deploy/veylixd.service" ]; then
	ok "T10 the checkout deploy/ is used when present"
else
	bad "T10 the checkout deploy/ was not used"
fi
if ( deploy_file nosuchfile >/dev/null 2>&1 ); then
	bad "T10 deploy_file returned a path for a missing template"
else
	ok "T10 deploy_file reports a missing template"
fi

# (b) with no checkout, the release bundle is used - and a hostile member
#     outside deploy/ is NOT extracted.
BUNDLE_ROOT="$WORK/bundle"
mkdir -p "$BUNDLE_ROOT/deploy" "$BUNDLE_ROOT/outside"
printf 'unit\n' >"$BUNDLE_ROOT/deploy/veylixd.service"
printf 'site\n' >"$BUNDLE_ROOT/deploy/Caddyfile"
printf 'backup\n' >"$BUNDLE_ROOT/deploy/veylix-backup.sh"
printf 'PWNED\n' >"$BUNDLE_ROOT/outside/evil.txt"
( cd "$BUNDLE_ROOT" && tar -czf "$WORK/veylix-deploy.tar.gz" deploy outside ) >/dev/null 2>&1

HERE_NO_DEPLOY="$WORK/empty-hosts"
mkdir -p "$HERE_NO_DEPLOY"
SCRIPT_BASE=""
# Stand in for the network: 'curl' copies the local bundle, so the real
# extraction filter is what gets exercised.
curl() {
	local out="" url=""
	while [ $# -gt 0 ]; do
		case "$1" in
			-o) out=$2; shift 2 ;;
			-*|-fsSL|-fsS) shift ;;
			*)  url=$1; shift ;;
		esac
	done
	case "$url" in
		*veylix-deploy.tar.gz) [ -n "$out" ] && cp "$WORK/veylix-deploy.tar.gz" "$out"; return 0 ;;
		*) return 22 ;;
	esac
}
probe_bundle() { HERE="$HERE_NO_DEPLOY"; resolve_deploy_dir >/dev/null 2>&1; }
DEPLOY_SRC=""
probe_bundle
resolved="$DEPLOY_SRC"
if [ -f "${resolved:-/nonexistent}/veylixd.service" ]; then
	ok "T10 the release bundle supplies the systemd unit"
else
	bad "T10 the release bundle was not used (DEPLOY_SRC=${resolved:-<empty>})"
fi
if [ -f "${resolved:-/nonexistent}/veylix-backup.sh" ] && [ -f "${resolved:-/nonexistent}/Caddyfile" ]; then
	ok "T10 the release bundle supplies the Caddyfile and the backup script"
else
	bad "T10 the release bundle is missing a template"
fi
if find "${resolved:-/nonexistent}" -name 'evil.txt' 2>/dev/null | grep -q .; then
	bad "T10 a member OUTSIDE deploy/ was extracted"
else
	ok "T10 nothing outside deploy/ was extracted"
fi
unset -f curl

# (c) nothing found anywhere: honest refusal, no fabricated source.
#
# The raw-repository fallback (D-127) means "nothing found" can no longer be
# produced by an empty disk - a live network would answer it. So the stub here
# denies EVERY url, which is what an offline or firewalled host looks like. The
# assertion is unchanged: no source, exit 1, and the refusal names the routes.
HERE="$HERE_NO_DEPLOY"
SCRIPT_BASE=""
DEPLOY_SRC=""
curl() { return 22; }   # no network: the host is offline
( resolve_deploy_dir ) >"$WORK/deploy-none.log" 2>&1
expect_rc "T10 no templates is reported as a miss" $? 1
expect_grep "T10 the miss names --script-base" "--script-base" "$WORK/deploy-none.log"
unset -f curl

# (d) D-127: a bootstrap run with no checkout takes the raw-repository route.
#     This is the branch `bash <(curl ... install.sh)` depends on.
curl() {
	local out="" url=""
	while [ $# -gt 0 ]; do
		case "$1" in
			-o) out=$2; shift 2 ;;
			-*|-fsSL|-fsS) shift ;;
			*)  url=$1; shift ;;
		esac
	done
	case "$url" in
		*/deploy/*) [ -n "$out" ] && printf 'template\n' >"$out"; return 0 ;;
		*) return 22 ;;
	esac
}
DEPLOY_SRC=""
probe_raw() { HERE="$HERE_NO_DEPLOY"; resolve_deploy_dir >/dev/null 2>&1; }
probe_raw
if [ -s "${DEPLOY_SRC:-/nonexistent}/veylixd.service" ] \
	&& [ -s "${DEPLOY_SRC:-/nonexistent}/Caddyfile" ]; then
	ok "T10 the raw repository supplies templates when there is no checkout"
else
	bad "T10 the raw repository fallback is missing (DEPLOY_SRC=${DEPLOY_SRC:-<empty>})"
fi
unset -f curl

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

# ---------------------------------------------------------------------------
# T11 - the control menu. Two things must be true and both are easy to get
# wrong: the menu has to PRINT (so it lands in the install log), and it must not
# PROMPT when there is no terminal (or `curl ... | bash` hangs forever).
# stdin is /dev/null here, which is exactly the no-tty case.
# ---------------------------------------------------------------------------
control_menu >"$WORK/menu.log" 2>&1 </dev/null
expect_rc "T11 control_menu returns immediately with no terminal" $? 0

for want in 'VEYLIX CONTROL MENU' '1) Start' '4) Status' '16) Change ports' \
	'18) Reset admin' '20) Uninstall + purge' '0) Exit'; do
	expect_grep "T11 the menu shows '$want'" "$want" "$WORK/menu.log"
done

# The prompt must NOT appear without a terminal: printing it and then returning
# would look like a prompt the operator was never able to answer.
if grep -qF 'Open the control menu now?' "$WORK/menu.log" 2>/dev/null; then
	bad "T11 control_menu asked a question with no terminal to answer it"
else
	ok "T11 control_menu does not prompt without a terminal"
fi

# Every option the shell menu offers must map to a real vpn-platformctl command.
# The two lists are written by hand in two languages, so they can drift: the Go
# test (menu_test.go) proves the GO menu's verbs are dispatched, and this proves
# the SHELL menu offers the same NUMBER of options, 1..20 plus 0=Exit.
#
# The shell copy prints human labels ("Status"), not command names, so matching
# on a verb would be matching on a string that is deliberately not there. The
# count is the honest check: a new command that is not offered, or an offer that
# no command backs, both change it.
offered=$(grep -cE '^ *[0-9]+\) ' "$WORK/menu.log" 2>/dev/null || printf '0')
if [ "$offered" -eq 21 ]; then
	ok "T11 the menu offers 20 commands plus Exit"
else
	bad "T11 the menu offers $offered numbered options, expected 21 (20 commands + Exit)"
fi

# And the numbers must run 1..20 with no gap and no repeat - the same invariant
# the Go side enforces, checked here in case someone edits only the heredoc.
for n in $(seq 1 20); do
	if grep -qE "^ *${n}\\) " "$WORK/menu.log" 2>/dev/null; then
		:
	else
		bad "T11 the menu has no option numbered $n"
	fi
done
ok "T11 options 1..20 are all present"

# ---------------------------------------------------------------------------
# T12 - port flags (D-123). The reason this exists: 3X-UI and Veylix both want
# 80/443 on one VPS, so the installer must accept other ports AND must render
# them into the Caddyfile correctly. A wrong render is silent - Caddy serves
# plaintext on 8443 and never obtains a certificate - so it is checked here
# rather than discovered in production.
# ---------------------------------------------------------------------------

# validate_port is the gate for all three flags: 1..65535, no leading zeros.
for good in 1 80 443 8080 8443 65535; do
	if validate_port "$good"; then :; else
		bad "T12 validate_port refused the valid port $good"
	fi
done
ok "T12 validate_port accepts valid ports"

for badport in 0 65536 abc 08 -1 1.5 '' ' 80'; do
	if validate_port "$badport" 2>/dev/null; then
		bad "T12 validate_port accepted the invalid port '$badport'"
	fi
done
ok "T12 validate_port refuses invalid ports"

# Equal ports are a bind failure at runtime; the check must name the pair.
HTTP_PORT=80 HTTPS_PORT=443 APP_PORT=8080
if validate_ports_for_install >/dev/null 2>&1; then
	ok "T12 the default port trio is accepted"
else
	bad "T12 the default port trio (80/443/8080) was rejected"
fi

HTTP_PORT=8443 HTTPS_PORT=8443 APP_PORT=8080
if validate_ports_for_install >/dev/null 2>&1; then
	bad "T12 http-port == https-port was accepted"
else
	ok "T12 http-port == https-port is refused"
fi

HTTP_PORT=80 HTTPS_PORT=443 APP_PORT=443
if validate_ports_for_install >/dev/null 2>&1; then
	bad "T12 app-port == https-port was accepted"
else
	ok "T12 app-port == https-port is refused"
fi

HTTP_PORT=80 HTTPS_PORT=443 APP_PORT=80
if validate_ports_for_install >/dev/null 2>&1; then
	bad "T12 app-port == http-port was accepted"
else
	ok "T12 app-port == http-port is refused"
fi

# The 3X-UI pairing is the whole point of the feature: it must be ACCEPTED.
HTTP_PORT=8080 HTTPS_PORT=8443 APP_PORT=9090
if validate_ports_for_install >/dev/null 2>&1; then
	ok "T12 the documented 3X-UI pairing (8080/8443/9090) is accepted"
else
	bad "T12 the documented 3X-UI pairing was rejected"
fi

HTTP_PORT=80 HTTPS_PORT=443 APP_PORT=8080   # restore the defaults

# The Caddyfile must render every placeholder. A leftover is a template/installer
# drift that caddy validate would report far from its cause.
CADDY_TEMPLATE="$ROOT/deploy/Caddyfile"
if [ -f "$CADDY_TEMPLATE" ]; then
	sed -e "s|__HTTPS_PORT__|8443|g" -e "s|__HTTP_PORT__|8080|g" \
		-e "s|__APP_PORT__|9090|g" -e "s|vpn\.example\.com|vpn.t12.test|g" \
		"$CADDY_TEMPLATE" >"$WORK/caddy.custom"
	if grep -q '__[A-Z_]*PORT__' "$WORK/caddy.custom"; then
		bad "T12 a placeholder survived rendering the Caddyfile"
	else
		ok "T12 the Caddyfile renders with no leftover placeholder"
	fi

	expect_grep "T12 the site address carries https:// and the custom port" \
		'https://vpn.t12.test:8443 {' "$WORK/caddy.custom"
	expect_grep "T12 the global block sets http_port" 'http_port 8080' "$WORK/caddy.custom"
	expect_grep "T12 the global block sets https_port" 'https_port 8443' "$WORK/caddy.custom"
	expect_grep "T12 the upstream uses the app port" 'reverse_proxy 127.0.0.1:9090 {' "$WORK/caddy.custom"

	# Default ports must still produce a conventional address.
	sed -e "s|__HTTPS_PORT__|443|g" -e "s|__HTTP_PORT__|80|g" \
		-e "s|__APP_PORT__|8080|g" -e "s|vpn\.example\.com|vpn.t12.test|g" \
		"$CADDY_TEMPLATE" >"$WORK/caddy.default"
	expect_grep "T12 the default render still uses the conventional upstream" \
		'reverse_proxy 127.0.0.1:8080 {' "$WORK/caddy.default"

	# The scheme prefix is mandatory: without it Caddy treats :8443 as plain
	# HTTP and silently never issues a certificate.
	if grep -qE '^vpn\.example\.com:' "$CADDY_TEMPLATE"; then
		bad "T12 the Caddyfile has a site address with no https:// scheme"
	else
		ok "T12 the Caddyfile site address uses an explicit https:// scheme"
	fi
else
	bad "T12 deploy/Caddyfile is missing"
fi

# app-port must reach the daemon's own environment, or it would still bind 8080
# while Caddy proxies to 9090.
expect_grep "T12 install.sh writes VEYLIX_ADDR from APP_PORT" \
	'VEYLIX_ADDR=127.0.0.1:$APP_PORT' "$ROOT/install.sh"

# The port conflict message must name the 3X-UI remedy, not only "stop it":
# telling an operator to stop the service that serves their VPN clients is an
# outage, not a fix.
expect_grep "T12 the port conflict message offers the 3X-UI port remedy" \
	'--https-port 8443' "$ROOT/install.sh"

# public_url must drop :443 and keep any other port, or the printed link is wrong.
DOMAIN=app.t12.test
HTTPS_PORT=443
if [ "$(public_url)" = "https://app.t12.test" ]; then
	ok "T12 public_url omits :443"
else
	bad "T12 public_url produced '$(public_url)' for port 443"
fi
HTTPS_PORT=8443
if [ "$(public_url)" = "https://app.t12.test:8443" ]; then
	ok "T12 public_url keeps a non-default port"
else
	bad "T12 public_url produced '$(public_url)' for port 8443"
fi
HTTPS_PORT=443

# ---------------------------------------------------------------------------
# T13 - the interactive port step, and the TTY guard it depends on.
#
# The bug this locks: `[ -t 0 ]` is FALSE under `bash <(curl ... install.sh)`
# because stdin is the script's own pipe. Every prompt guarded by it silently
# never appeared, and every unguarded `read` ate a line of the script.
# ---------------------------------------------------------------------------

# has_tty must exist and must not depend on stdin.
expect_grep "T13 has_tty is defined in common.sh" 'has_tty()' "$ROOT/scripts/lib/common.sh"
expect_grep "T13 has_tty tests /dev/tty, not stdin" '/dev/tty' "$ROOT/scripts/lib/common.sh"
expect_grep "T13 read_tty reads from /dev/tty" 'read -r __line </dev/tty' "$ROOT/scripts/lib/common.sh"
expect_grep "T13 read_tty_secret does not echo" 'read -rs __line </dev/tty' "$ROOT/scripts/lib/common.sh"

# No `read` may remain on plain stdin in install.sh: every one of them is a
# hang under the documented invocation. `read_tty` reads elsewhere and is fine.
leftover_reads=$(grep -nE '^[[:space:]]*read (-r|-rs|read)' "$ROOT/install.sh" 2>/dev/null | grep -v 'read_tty' | wc -l | tr -d ' ')
if [ "${leftover_reads:-0}" -eq 0 ]; then
	ok "T13 no prompt reads from stdin (only from /dev/tty)"
else
	bad "T13 $leftover_reads prompt(s) still read from stdin and would hang under bash <(curl ...)"
fi

# choose_ports must be reachable and must not prompt when it cannot.
expect_grep "T13 choose_ports is defined" 'choose_ports()' "$ROOT/install.sh"
# expect_grep is `grep -F` (literal), so this is a plain substring, not a regex.
if grep -qE '^[[:space:]]*choose_ports[[:space:]]*$' "$ROOT/install.sh" 2>/dev/null; then
	ok "T13 choose_ports is called from main"
else
	bad "T13 choose_ports is defined but never called"
fi

# With stdin at /dev/null (the no-terminal case) it must return 0 and keep the
# defaults, never block and never fail the install.
(
	DRY_RUN=0
	HTTP_PORT=80
	HTTPS_PORT=443
	APP_PORT=8080
	PORTS_FROM_FLAGS=0
	choose_ports >/dev/null 2>&1 </dev/null
) 
if [ $? -eq 0 ]; then
	ok "T13 choose_ports returns without a terminal instead of hanging"
else
	bad "T13 choose_ports failed or blocked without a terminal"
fi

# A port flag answers the question, so the prompt must not run at all.
expect_grep "T13 parse_args records that a port flag was given" 'PORTS_FROM_FLAGS=1' "$ROOT/install.sh"

# D-131 replaced the picker with auto-detection. The old assertions named the
# prompt text; they are superseded by T19, which checks the behaviour that
# replaced it. What still matters here is that nothing PROMPTS: a prompt is what
# made a one-command install ask three questions and then refuse the answer.
if grep -qE 'Choose \[1\]:|read_tty choice' "$ROOT/install.sh"; then
	bad "T13 the installer no longer asks the operator for ports"
else
	ok "T13 the installer no longer asks the operator for ports"
fi


# ---------------------------------------------------------------------------
# T14 - the `vey` launcher (D-126).
#
# The bug this locks: after a fresh install the operator was told to type
# `vpn-platformctl`, a 16-character name. 3X-UI installs `x-ui`; Veylix must
# install `vey`. A short name that is absent is a support ticket on the first
# command anyone types.
# ---------------------------------------------------------------------------

expect_grep "T14 install.sh writes a launcher" 'install_launcher()' "$ROOT/install.sh"
expect_grep "T14 the launcher is called during the install" 'install_launcher' "$ROOT/install.sh"
expect_grep "T14 the launcher is named vey" 'target="$BIN_DIR/vey"' "$ROOT/install.sh"

# The launcher must be extracted and syntax-checked, not merely grepped for: it
# is shell run as root, and a heredoc typo is invisible to a pattern match.
awk "/<<'LAUNCHER'/{f=1;next} /^LAUNCHER\$/{f=0} f" "$ROOT/install.sh" >"$WORK/vey-launcher" 2>/dev/null
if [ -s "$WORK/vey-launcher" ]; then
	ok "T14 the launcher body is written by install.sh"
	if bash -n "$WORK/vey-launcher" 2>/dev/null; then
		ok "T14 the launcher is syntactically valid shell"
	else
		bad "T14 the launcher has a shell syntax error"
	fi
	expect_grep "T14 the launcher execs the control binary" 'exec "$CTL" "$@"' "$WORK/vey-launcher"
	expect_grep "T14 the launcher explains a missing binary" 'is missing' "$WORK/vey-launcher"
	# A launcher that sources or evals anything would be an injection path: it
	# runs as root, from a fixed path.
	if grep -qE '(source|\.)[[:space:]]+/|eval ' "$WORK/vey-launcher" 2>/dev/null; then
		bad "T14 the launcher sources or evals - it runs as root from a fixed path"
	else
		ok "T14 the launcher neither sources nor evals anything"
	fi
else
	bad "T14 install.sh writes no launcher body"
fi

# The report must teach the short command, and the menu offer must use it.
expect_grep "T14 the report tells the operator to run sudo vey" 'sudo vey' "$ROOT/install.sh"
expect_grep "T14 the menu offer names sudo vey" 'Run "sudo vey"' "$ROOT/install.sh"

# `menu` must be a dispatched verb: the launcher relies on it existing, and the
# Go side registers it in init() to avoid an initialization cycle.
expect_grep "T14 the menu verb is registered" 'dispatchTable["menu"] = cmdMenu' "$ROOT/backend/cmd/vpn-platformctl/main.go"


# ---------------------------------------------------------------------------
# T15 - every documented invocation form can actually start the installer.
#
# D-127 and D-128 both shipped because this selftest invoked install.sh AS A
# FILE. That is one of the four forms a user may paste, and it is the only one
# for which $HERE is correct. The documented forms are exercised here, so a
# change that breaks one fails the gate instead of failing on a customer's VPS.
# ---------------------------------------------------------------------------
runs_cli() {
	printf '%s' "$1" | grep -q 'VEYLIX installer' &&
		! printf '%s' "$1" | grep -qE 'unbound variable|No such file'
}

# (a) as a file
o=$(bash "$ROOT/install.sh" --help 2>&1 | head -20)
if runs_cli "$o"; then ok "T15 a file run reaches the CLI"; else bad "T15 a file run failed: $(printf '%s' "$o" | head -1)"; fi

# (b) process substitution - the form the owner first reported
o=$(bash <(cat "$ROOT/install.sh") --help 2>&1 | head -20)
if runs_cli "$o"; then ok "T15 bash <(cat install.sh) reaches the CLI"; else bad "T15 process substitution failed: $(printf '%s' "$o" | head -1)"; fi

# (c) stdin pipe - BASH_SOURCE[0] is unset here (D-128)
o=$(cat "$ROOT/install.sh" | bash -s -- --help 2>&1 | head -20)
if runs_cli "$o"; then ok "T15 cat install.sh | bash -s -- reaches the CLI"; else bad "T15 the stdin pipe failed: $(printf '%s' "$o" | head -1)"; fi

# (d) the script must not assume BASH_SOURCE is set under `set -u`
expect_grep "T15 the self path tolerates an unset BASH_SOURCE" 'BASH_SOURCE[0]:-' "$ROOT/install.sh"

# (e) no doc may present `sudo bash <(curl ...)` as a command to run. Any
#     occurrence must sit on a line that also warns against it.
T15_bad=0
while IFS= read -r line; do
	case "$line" in
		*"**NO**"*) : ;;                        # the table row that forbids it
		*"Do not use"*) : ;;                    # an explicit warning
		*"You used"*) : ;;                      # troubleshooting, naming the mistake
		*"runs a **separate process**"*) : ;;   # the explanation
		*) T15_bad=$((T15_bad + 1)) ;;
	esac
done <<EOF
$(grep -h 'sudo bash <(curl' "$ROOT/docs/INSTALL.md" "$ROOT/docs/PUSH_GUIDE.md" 2>/dev/null || true)
EOF
if [ "$T15_bad" -gt 0 ]; then
	bad "T15 the docs do not recommend sudo with process substitution ($T15_bad line(s))"
else
	ok "T15 the docs do not recommend sudo with process substitution"
fi




# ---------------------------------------------------------------------------
# T16 - the installer MUST SAY SOMETHING when it refuses.
#
# D-129: `bash install.sh` with no arguments exited 1 having printed ZERO bytes
# to stdout and stderr. parse_args ended on `[ "$DRY_RUN" -eq 1 ] && DRY_RUN=1`,
# which is false for a normal run, so the FUNCTION returned 1; main calls
# parse_args as a bare command, so `set -e` killed the script before any message.
# Every previous test passed --domain or --help, and --help exits at parse time,
# so nothing ever exercised the documented no-flag invocation.
#
# A refusal is fine. A SILENT refusal is the defect: the operator cannot tell a
# bug from a hang. This asserts a message reaches the operator either way.
# ---------------------------------------------------------------------------

# (a) no arguments at all: must not exit silently
o=$(bash "$ROOT/install.sh" </dev/null 2>&1)
rc=$?
if [ "$rc" -ne 0 ] && [ -z "$o" ]; then
	bad "T16 a no-argument run says something before exiting (exit $rc, no output)"
else
	ok "T16 a no-argument run says something before exiting"
fi
printf '%s\n' "$o" >"$WORK/t16a.log"
expect_grep "T16 the no-argument message names --domain" "--domain" "$WORK/t16a.log"

# (b) every function must end on a statement that cannot return non-zero.
#     `set -e` turns a bare-command function returning 1 into a silent death.
T16_risk=$(awk '
	/^[a-zA-Z_][a-zA-Z_0-9]*\(\)[[:space:]]*\{/ { fn = $1; last = ""; next }
	fn != "" && /^\}/ {
		if (last ~ /^[[:space:]]*\[/ || last ~ /&&/) { print fn }
		fn = ""; next
	}
	fn != "" && $0 !~ /^[[:space:]]*#/ && $0 !~ /^[[:space:]]*$/ { last = $0 }
' "$ROOT/install.sh")
if [ -n "$T16_risk" ]; then
	bad "T16 no function ends on a test that can return non-zero" "$T16_risk"
else
	ok "T16 no function ends on a test that can return non-zero"
fi

# (c) parse_args itself must report success with no arguments. This is the
#     precise defect, not a proxy for it.
sed -e '/^main "\$@"$/d' "$ROOT/install.sh" >"$WORK/pa.sh" 2>/dev/null
o=$(bash -c 'set -euo pipefail; . "'"$WORK"'/pa.sh"; parse_args; echo "parse_args rc=$?"' 2>&1 | tail -1)
if printf '%s' "$o" | grep -q 'parse_args rc=0'; then
	ok "T16 parse_args returns 0 when given no arguments"
else
	bad "T16 parse_args returns 0 when given no arguments" "$o"
fi


# ---------------------------------------------------------------------------
# T17 - --http mode: install with no domain, reached by IP.
#
# The documented one-liner is run on a fresh server by an operator who has no
# domain yet, and 3X-UI installs in exactly this shape. Refusing without
# --domain made the one-liner unusable; --http is the mode that answers it.
#
# IMPORTANT: these cases call the REAL main(), not a copy of its logic.
# A gate that restates the decision it is checking proves only that the test
# and the test agree - which is how D-127, D-128 and D-129 all shipped. The
# observable is what the installer PRINTS and whether it DIES, with the host
# probes stubbed so the run can reach the domain decision at all.
# ---------------------------------------------------------------------------
t17_run() {
	# Runs main() in a subroutine, with only the platform probes stubbed.
	# Returns main's output; the caller asserts on it.
	bash -c '
		set +e
		VEYLIX_HERE="'"$ROOT"'"
		. "'"$WORK/guards.sh"'"
		need_root(){ :; }; acquire_lock(){ :; }; have_flock(){ :; }
		detect_os(){ printf ubuntu; }; have(){ return 0; }; systemctl(){ return 0; }
		DRY_RUN=1
		main "$@" 2>&1 | head -12
	' _ "$@"
}

# (a) NO ARGUMENTS: must not demand a domain. This is the owner's command.
o=$(t17_run)
printf '%s\n' "$o" >"$WORK/t17a.txt"
if printf '%s' "$o" | grep -q -- '--domain is required'; then
	bad "T17 no arguments does not demand --domain"
else
	ok "T17 no arguments does not demand --domain"
fi
expect_grep "T17 no arguments still reaches the install plan" "preflight" "$WORK/t17a.txt"

# (b) --http with no domain: same, and it must say what it is doing.
o=$(t17_run --http)
if printf '%s' "$o" | grep -q -- '--domain is required'; then
	bad "T17 --http does not demand --domain"
else
	ok "T17 --http does not demand --domain"
fi

# (c) --http together with --domain must be refused, naming the conflict.
o=$(t17_run --http --domain vpn.example.com)
printf '%s\n' "$o" >"$WORK/t17c.txt"
expect_grep "T17 --http with --domain is refused as a conflict" "both given" "$WORK/t17c.txt"

# (d) a domain alone must still select HTTPS, so the two modes stay distinct.
o=$(t17_run --domain vpn.example.com)
if printf '%s' "$o" | grep -q 'plain HTTP'; then
	bad "T17 --domain alone still selects HTTPS"
else
	ok "T17 --domain alone still selects HTTPS"
fi

# The rendered site in --http mode: http://, and no HSTS.
#
# This used to carry its own copy of the installer's sed, with the old
# `http://0.0.0.0:80` site address. That is the mistake T17's first draft made:
# a test that RESTATES the decision cannot detect the decision changing, so it
# stayed green while install.sh shipped an address Caddy turns into a Host
# matcher (`{"match":[{"host":["0.0.0.0"]}]}`) that no browser request satisfies -
# a listening site answering every request with an empty 200.
#
# The render itself is now produced by install.sh's own install_caddy(), with
# only the root/system probes stubbed, so what is asserted is what ships.
t17_render() {
	local mode out
	mode=${1:-http}
	out="$WORK/t17-render-$mode"
	rm -f "$out" "$out.veylix-new"
	VEYLIX_T17_MODE=$mode VEYLIX_T17_OUT=$out bash "$ROOT/scripts/t17-render.sh" 2>/dev/null
	if [ -f "$out.veylix-new" ]; then
		cat "$out.veylix-new"
	elif [ -f "$out" ]; then
		cat "$out"
	fi
}

t17_render http >"$WORK/t17http.txt"
# `http://:PORT` with a `bind` line - NOT `http://0.0.0.0:PORT`. The host part of
# a Caddy site address is a Host-header matcher, so 0.0.0.0 matched nothing a
# browser ever sends and the site returned an empty 200.
expect_grep "T17 the --http site address has no host part" "http://:80 {" "$WORK/t17http.txt"
expect_grep "T17 the --http bind names the interface" "bind 0.0.0.0" "$WORK/t17http.txt"
if grep -qF 'http://0.0.0.0:80 {' "$WORK/t17http.txt"; then
	bad "T17 the --http site address is not a literal IP" \
		"0.0.0.0 in a site address becomes a Host matcher that no request satisfies"
else
	ok "T17 the --http site address is not a literal IP"
fi
if grep -q 'Strict-Transport-Security' "$WORK/t17http.txt"; then
	bad "T17 --http sends no HSTS header"
else
	ok "T17 --http sends no HSTS header"
fi
if grep -q '__[A-Z_]*__' "$WORK/t17http.txt"; then
	bad "T17 --http leaves no template placeholder"
else
	ok "T17 --http leaves no template placeholder"
fi

t17_render domain >"$WORK/t17dom.txt"
expect_grep "T17 the domain site address is https" "https://vpn.acme.com:443 {" "$WORK/t17dom.txt"
expect_grep "T17 the domain site keeps HSTS" "Strict-Transport-Security" "$WORK/t17dom.txt"

# The env file must offer a plain-http public base URL, and the report must
# name the plaintext risk. Both are text in install.sh, so grep the source.
expect_grep "T17 the installer can write a plain-http public base URL" \
	'VEYLIX_PUBLIC_BASE_URL=http://' "$ROOT/install.sh"

expect_grep "T17 the report names the plaintext risk" \
	"THIS HOST SERVES PLAIN HTTP" "$ROOT/install.sh"

# (e) No bare `[ ... ] && ...` at statement position. Under `set -e` a false
#     test there aborts the whole script - D-129 killed the no-flag run this
#     way, and D-130 found a second one that ended every --dry-run early.
#
#     The gate used to EXCUSE `&& die`, reasoning that a right-hand side of
#     die/exit means "it is meant to stop". That reasoning is wrong, and it let
#     a third instance through to production: `[ "$ui" = "not_embedded" ] && die`
#     aborts the install on the SUCCESS path, because the false test makes the
#     compound command exit 1 and die never runs - so nothing is printed and the
#     installer dies in silence right after "ready: schema_version=29".
#
#     There is no safe bare `&&` form. The deliberate-stop idiom is
#     `[ test ] || die`, which runs when the test FAILS. So the gate flags a
#     test-and-command that has NO `||` anywhere on the line: that is the shape
#     whose false branch ends the statement at status 1 under set -e.
#
#     Scanned across common.sh as well as install.sh. The first version only
#     read install.sh, so the library install.sh sources was unchecked - and it
#     still contained this shape in three places.
T17_footgun=$(grep -nE '^[[:space:]]*\[[^]]*\][[:space:]]*&&' \
	"$ROOT/install.sh" "$ROOT/scripts/lib/common.sh" 2>/dev/null \
	| grep -v '||' || true)
if [ -n "$T17_footgun" ]; then
	bad "T17 no bare test-and-command can abort a run under set -e" "$T17_footgun"
else
	ok "T17 no bare test-and-command can abort a run under set -e"
fi

# (f) No helper may hardcode the app port in CODE. install.sh chooses it (8080,
#     then 18080 when that is taken), so a library default of :8080 silently
#     polls the wrong port on exactly the hosts where the port moved. Traced
#     live: ready_field read :8080 while the app ran on :18080, returned "", and
#     the install died one line after printing "ready".
#
#     Comments are excluded. The first version flagged the explanatory comment
#     that documents the very bug, which would have trained the next reader to
#     stop explaining.
T17_port=$(grep -nE '127\.0\.0\.1:8080' "$ROOT/scripts/lib/common.sh" 2>/dev/null \
	| grep -vE '^[0-9]+:[[:space:]]*#' || true)
if [ -n "$T17_port" ]; then
	bad "T17 no helper hardcodes the app port" "$T17_port"
else
	ok "T17 no helper hardcodes the app port"
fi

# (g) A command substitution whose LAST command is a filter must tolerate a
#     no-match exit. `var=$(grep x file)` fails with status 1 when there is no
#     match - which is usually the GOOD outcome - and under `set -e` that kills
#     the installer. Traced live: `leftovers=$(grep -o '__PORT__' ...)` aborted
#     install.sh immediately after the sed that wrote the Caddyfile, so a
#     correct render was reported as a silent death.
T17_unguarded=$(grep -nE '=\$\(.*\|[[:space:]]*(grep|sort|head|awk|sed)[^)]*\)[[:space:]]*$' \
	"$ROOT/install.sh" "$ROOT/scripts/lib/common.sh" 2>/dev/null \
	| grep -v '||' | grep -v 'if !' || true)
if [ -n "$T17_unguarded" ]; then
	bad "T17 a filter in a substitution tolerates no match" "$T17_unguarded"
else
	ok "T17 a filter in a substitution tolerates no match"
fi

# (h) The Caddyfile must be READABLE by the caddy service user. The file is
#     written by a root shell carrying the 077 umask from generate_secrets, so
#     it lands as 0600 root:root and Caddy - which runs as `caddy` - dies with
#     "open /etc/caddy/Caddyfile: permission denied". `caddy validate` runs as
#     root and passes, so the failure is invisible until the service starts.
expect_grep "T21 the rendered Caddyfile is made readable" \
	'chmod 0644 "$CADDYFILE"' "$ROOT/install.sh"

# (i) A reload-or-restart fallback whose final command fails must not abort
#     under set -e. `run a || run b` ends at status 1 when both fail, killing
#     the installer with no message.
T17_svcfail=$(grep -nE '^[[:space:]]*run[[:space:]]+systemctl.*\|\|[[:space:]]*run[[:space:]]+systemctl' \
	"$ROOT/install.sh" 2>/dev/null || true)
if [ -n "$T17_svcfail" ]; then
	bad "T17 a service reload fallback reports its own failure" "$T17_svcfail"
else
	ok "T17 a service reload fallback reports its own failure"
fi


# ---------------------------------------------------------------------------
# T19 - ports are DETECTED, not asked for (D-131).
#
# The owner's last run asked three questions and then refused the answer: the
# same number typed for the public port and the internal one produced
# "WARNING: ... both 2231" followed by a stop, which read as "fine, going on".
# 3X-UI asks nothing, so this must ask nothing either.
#
# Each case stubs `ss` so a port set is "in use", which is the only way to test
# a probing function offline. The real `ss` path is exercised on a live host.
# ---------------------------------------------------------------------------
t19_probe() {
	local busy=$1
	shift
	bash -c '
	set +e
		VEYLIX_HERE="'"$ROOT"'"
		. "'"$WORK/guards.sh"'"
		# Stand in for the host: these ports answer, everything else is free.
		have() { [ "$1" = ss ]; }
		ss() {
			case "$*" in
				*"sport = :$BUSY_PORTS"*) return 0 ;;
			esac
			# Only report a hit when one of the busy ports was asked for.
			local p
			for p in $BUSY_PORTS; do
				case "$*" in *":$p"*) return 0 ;; esac
			done
			return 1
		}
		port_is_free() {
			local p=$1 b
			for b in $BUSY_PORTS; do [ "$p" = "$b" ] && return 1; done
			return 0
		}
		HTTP_MODE=1; PORTS_FROM_FLAGS=0; DRY_RUN=1
		HTTP_PORT=80; HTTPS_PORT=443; APP_PORT=8080
		choose_ports >/dev/null 2>&1
		echo "HTTP_PORT=$HTTP_PORT APP_PORT=$APP_PORT"
	' _ "$@" 2>&1 | tail -1
}
t19_case() {
	local name=$1 busy=$2 want=$3
	local out
	out=$(BUSY_PORTS="$busy" t19_probe "$busy")
	if printf '%s' "$out" | grep -q -- "$want"; then
		ok "T19 $name"
	else
		bad "T19 $name (wanted '$want', got '$out')"
	fi
}

# Nothing in use: the standard port, and an app port that differs from it.
t19_case "a bare host gets port 80 and a distinct app port" "" "HTTP_PORT=80 APP_PORT=8080"
# 3X-UI holds 80: step to the next candidate rather than dying or prompting.
t19_case "3X-UI on 80 moves to 8080" "80" "HTTP_PORT=8080"
# 80 and 8080 both taken: still finds one, and never reuses the public port.
t19_case "80 and 8080 taken still finds a port" "80 8080" "HTTP_PORT=8081"
# The exact bug: the app port must never equal the public port.
o=$(BUSY_PORTS="80 8080 8081 8880" t19_probe "80 8080 8081 8880")
hp=$(printf '%s' "$o" | sed -n 's/.*HTTP_PORT=\([0-9]*\).*/\1/p')
ap=$(printf '%s' "$o" | sed -n 's/.*APP_PORT=\([0-9]*\).*/\1/p')
if [ -n "$hp" ] && [ "$hp" != "$ap" ]; then
	ok "T19 the app port never equals the public port"
else
	bad "T19 the app port never equals the public port (http=$hp app=$ap)"
fi

# A port flag is the answer: detection must not override it.
out=$(BUSY_PORTS="80" bash -c '
	set +e
	VEYLIX_HERE="'"$ROOT"'"
	. "'"$WORK/guards.sh"'"
	port_is_free() { return 0; }
	HTTP_MODE=1; PORTS_FROM_FLAGS=1; DRY_RUN=1
	HTTP_PORT=2231; HTTPS_PORT=443; APP_PORT=18080
	choose_ports >/dev/null 2>&1
	echo "HTTP_PORT=$HTTP_PORT APP_PORT=$APP_PORT"
' 2>&1 | tail -1)
if printf '%s' "$out" | grep -q 'HTTP_PORT=2231 APP_PORT=18080'; then
	ok "T19 a port flag is honoured and detection stays out of the way"
else
	bad "T19 a port flag is honoured (got '$out')"
fi

# Nothing free at all: die with the flag that fixes it, never a WARNING alone.
out=$(BUSY_PORTS="80 8080 8081 8880 2080 18080 28080 38080" bash -c '
	set +e
	VEYLIX_HERE="'"$ROOT"'"
	. "'"$WORK/guards.sh"'"
	port_is_free() { return 1; }
	HTTP_MODE=1; PORTS_FROM_FLAGS=0; DRY_RUN=1
	HTTP_PORT=80; HTTPS_PORT=443; APP_PORT=8080
	choose_ports
	echo "REACHED-END"
' 2>&1)
if printf '%s' "$out" | grep -q 'REACHED-END'; then
	bad "T19 an exhausted port list stops the install"
else
	case "$out" in
		*--http-port*) ok "T19 an exhausted port list stops the install and names --http-port" ;;
		*) bad "T19 an exhausted port list stops the install (got '$out')" ;;
	esac
fi

# The lock must be released on suspension. Without it a ^Z left the lock held
# and every later run answered "another veylix install is running".
expect_grep "T19 the lock is released on exit" "trap \"release_lock" "$ROOT/scripts/lib/common.sh"
expect_grep "T19 the lock is released on suspension" "TSTP" "$ROOT/scripts/lib/common.sh"
expect_grep "T19 release_lock closes the descriptor" "exec 9>&-" "$ROOT/scripts/lib/common.sh"


# ---------------------------------------------------------------------------
# T20 - every network call is BOUNDED (D-132).
#
# The owner's run printed absolutely nothing and had to be suspended with ^Z.
# The bootstrap's `curl` had no --max-time, and on a host where github accepts
# the connection and then goes quiet, curl waits forever. An installer that
# hangs is worse than one that fails: there is no message to act on.
#
# A deadline is asserted two ways: the helper exists and carries both flags, and
# no `curl ... -o` anywhere in the installer or its library is missing one.
# ---------------------------------------------------------------------------
expect_grep "T20 a single helper sets the network deadline" "--connect-timeout" "$ROOT/install.sh"
expect_grep "T20 the helper sets an overall deadline" "--max-time" "$ROOT/install.sh"

T20_unbounded=""
# Join continuation lines first: `curl ... \` with the deadline on the next line
# is bounded, and a line-by-line scan would call it unbounded. That false
# positive is exactly the kind of noise that gets a real gate ignored.
T20_joined=$(awk '
	/\\[[:space:]]*$/ { buf = buf $0 " "; next }
	{ print buf $0; buf = "" }
	END { if (buf != "") print buf }
' "$ROOT/install.sh" "$ROOT/scripts/lib/common.sh" 2>/dev/null)

while IFS= read -r line; do
	[ -n "$line" ] || continue
	case "$line" in
	*max-time* | *connect-timeout*) : ;;          # bounded
	*'# '*) : ;;                                   # a comment
	*dry-run*) : ;;                                # a printed plan, not a call
	*fetch*) : ;;                                  # goes through the helper
	*) T20_unbounded="$T20_unbounded  $line
" ;;
	esac
done <<EOF
$(printf '%s\n' "$T20_joined" | grep 'curl .*-o ' || true)
EOF
if [ -n "$T20_unbounded" ]; then
	bad "T20 no download can wait forever" "$T20_unbounded"
else
	ok "T20 no download can wait forever"
fi

# The bootstrap download in particular: it runs before anything else can print,
# so an unbounded one is the silent hang.
expect_grep "T20 the bootstrap download is bounded" \
	'fetch "$_veylix_url"' "$ROOT/install.sh"

# A failed bootstrap must explain itself, not exit silently.
expect_grep "T20 a failed bootstrap names the URL" \
	"could not download \$_veylix_url" "$ROOT/install.sh"


# ---------------------------------------------------------------------------
# T21 - the three live-host failures, found on a real 3X-UI VPS.
#
# All three were found by running the documented one-liner on Ubuntu 24.04 with
# 3X-UI already installed, not by reading the code. Each one stopped the install
# with a message that did not name the fix, so each gets a gate here.
# ---------------------------------------------------------------------------

# (a) `sudo bash <(curl ...)` - sudo closes the /dev/fd pipe and bash dies with
#     "No such file or directory" before this script runs. It cannot be fixed
#     from inside, only DETECTED, so the gate checks the detection exists and
#     that it names a working alternative rather than just erroring.
expect_grep "T21 sudo + process substitution is detected" \
	'case "${0:-}" in' "$ROOT/install.sh"
# expect_grep is grep -F (fixed string), so the pattern is literal - no glob.
expect_grep "T21 the /dev/fd invocation is recognised" \
	'/dev/fd/*' "$ROOT/install.sh"
# A guard with no fix in it is still a dead end: it must offer BOTH forms.
expect_grep "T21 the guard offers the pipe form" \
	'| sudo bash' "$ROOT/install.sh"
expect_grep "T21 the guard offers the bash -c form" \
	'sudo bash -c' "$ROOT/install.sh"

# (b) need_root must run BEFORE the lock. The lock lives in root-owned /run, so
#     a non-root run otherwise fails on "Permission denied" for a file the
#     operator has never heard of, when the actual problem is "forgot sudo".
T21_main=$(awk '/^main\(\)/{f=1} f' "$ROOT/install.sh" 2>/dev/null)
T21_root_at=$(printf '%s\n' "$T21_main" | grep -n '[[:space:]]need_root$' | head -1 | cut -d: -f1)
T21_lock_at=$(printf '%s\n' "$T21_main" | grep -n 'acquire_lock' | head -1 | cut -d: -f1)
if [ -z "$T21_root_at" ] || [ -z "$T21_lock_at" ]; then
	bad "T21 the root check precedes the lock" \
		"need_root (got '$T21_root_at') or acquire_lock (got '$T21_lock_at') not found in main()"
elif [ "$T21_root_at" -lt "$T21_lock_at" ]; then
	ok "T21 the root check precedes the lock (line $T21_root_at < $T21_lock_at)"
else
	bad "T21 the root check precedes the lock" \
		"need_root is at $T21_root_at, AFTER acquire_lock at $T21_lock_at - a non-root run fails on /run permissions instead of saying 're-run with sudo'"
fi

# (c) --http mode binds ONE public port. Checking 443 refused the documented
#     3X-UI coexistence case, because 3X-UI/Xray holds 443 itself.
if grep -q 'ports_to_check=$HTTP_PORT$' "$ROOT/install.sh"; then
	ok "T21 --http checks only its own public port"
else
	bad "T21 --http checks only its own public port" \
		"--http mode still validates the HTTPS port, which 3X-UI owns"
fi

# (d) The port-clash fix command must be PASTEABLE. It used to interpolate an
#     empty $DOMAIN, printing "--domain  --http-port 8080" - a command that
#     errors when run, in the one message meant to rescue the install.
if grep -q 'if \[ -n "\$DOMAIN" \]; then' "$ROOT/install.sh"; then
	ok "T21 the port-clash fix command is pasteable without a domain"
else
	bad "T21 the port-clash fix command is pasteable without a domain" \
		"the die message still interpolates \$DOMAIN unconditionally, so a --http run prints an empty --domain"
fi

# (e) The installer must INSTALL Caddy, not instruct the operator to.
#     A live run reported "VEYLIX is installed" while nothing listened on port
#     80, because the old code did `warn "caddy is not installed"` and moved on.
#     A warning beside a success banner is a false success.
expect_grep "T21 caddy is installed automatically" \
	'install_caddy_package' "$ROOT/install.sh"
expect_grep "T21 the caddy install has a manual fallback" \
	'caddy_install_notes' "$ROOT/install.sh"
if grep -qF 'caddy is not installed. Install it from https://caddyserver.com/docs/install' "$ROOT/install.sh"; then
	bad "T21 caddy is not merely reported missing" \
		"the old warn-and-continue message is still present"
else
	ok "T21 caddy is not merely reported missing"
fi

# A heredoc that is not quoted still performs command substitution, so a
# backtick or $( in the body EXECUTES. `vpn-platformctl <command>` inside one ran
# as a command, dumped the CLI usage and printed a syntax error mid-install,
# beside a success banner. Unquoted heredocs are legitimate - they interpolate
# $vars - so the rule is not "quote them all", it is "no backtick inside one".
#
# Written with sed for the range and a real backtick variable for the match.
# `grep '\x60'` was the first attempt and it NEVER matches without -P: the gate
# was silently green on every run, and only a negative control caught it. A gate
# that cannot fail is worse than no gate, because it is reported as proof.
T21_tick=$(sed -n '/^[[:space:]]*cat <<EOF/,/^EOF$/p' "$ROOT/install.sh" 2>/dev/null \
	| grep -n "$(printf '\140')" || true)
if [ -n "$T21_tick" ]; then
	bad "T21 no backtick inside an unquoted heredoc" "$T21_tick"
else
	ok "T21 no backtick inside an unquoted heredoc"
fi

# The Caddyfile render is verified POSITIVELY. "No leftover placeholders" is not
# proof: the released v1.0.0 bundle shipped a template with none at all, so the
# render was a no-op, the leftover check passed, caddy validate accepted it, and
# a site for an unconfigured domain went live while the installer printed
# "VEYLIX is installed". The live host served Caddy's default page on :80.
expect_grep "T21 the rendered Caddyfile address is asserted" \
	'want_addr=' "$ROOT/install.sh"
expect_grep "T21 a mismatched template is refused" \
	'the release bundle and this installer are not the same version' "$ROOT/install.sh"


printf '\n'
if [ "$fail" -eq 0 ]; then
	printf 'install.sh: all guard checks passed\n'
	exit 0
fi
printf 'install.sh: %d guard check(s) FAILED\n' "$fail"
exit 1
