#!/usr/bin/env bash
# VEYLIX — shared helpers for every deploy script.
#
# Source it, do not execute it:
#     . "$(dirname "$0")/lib/common.sh"
#
# Why this file exists: `VPN_FIX_LEDGER.md` records a verification script that
# deleted live data, TWICE. Four behaviours prevent that class of incident, and
# every deploy script must get them from here rather than reimplement them:
#
#   1. `die`          - fail loudly, exit non-zero, never continue half-applied
#   2. `run`          - honours --dry-run for EVERY mutation, with no exceptions
#   3. `acquire_lock` - two concurrent runs cannot interleave
#   4. `redact`       - a key or token can never reach stdout, stderr or a log
#
# Nothing here is advisory. If a script needs different behaviour it does not get
# a local copy; it extends this file.

# Deliberately NOT `set -e` at source time: a sourcing script owns its own error
# mode, but `set -u` and `pipefail` are set here because an unset variable in
# `rm -rf "$DIR/"` is exactly how the wrong directory gets deleted.
set -uo pipefail

# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

VEYLIX_TOOL=${VEYLIX_TOOL:-veylix}

say()  { printf '%s: %s\n' "$VEYLIX_TOOL" "$*"; }
warn() { printf '%s: WARNING: %s\n' "$VEYLIX_TOOL" "$*" >&2; }
err()  { printf '%s: ERROR: %s\n' "$VEYLIX_TOOL" "$*" >&2; }

# out MESSAGE - a plain informational line that is NOT an error.
#
# install.sh uses it for every "[dry-run] would ..." line. It MUST live here
# beside say/warn/err: it was referenced nine times and defined nowhere, so under
# `set -euo pipefail` each --dry-run line aborted the installer with
# "out: command not found" (exit 127) instead of describing what it would do.
out() { printf '%s\n' "$*"; }

# die MESSAGE [EXIT_CODE]
# Exits non-zero. Never prints a stack trace - an operator needs a correction,
# not a call stack.
die() {
	local msg=$1 code=${2:-1}
	err "$msg"
	exit "$code"
}

# ---------------------------------------------------------------------------
# Phase banner
#
# A failure must name the phase that died, so an operator can quote it in a bug
# report and a later reader can find the mutation that produced it.
# ---------------------------------------------------------------------------

PHASE_N=0
phase() {
	PHASE_N=$((PHASE_N + 1))
	printf '\n==> [%d] %s\n' "$PHASE_N" "$*"
}

# skip PHASE_NAME REASON
# A skipped check is printed, never silently passed (release-readiness Stage 5b).
skip() {
	warn "SKIPPED: $1 - $2"
}

# ---------------------------------------------------------------------------
# Redaction
#
# Strips the four secret shapes this product owns before anything is printed:
#   - a 32-byte base64 key (44 chars, trailing '=' optional)  -> enc/session key
#   - a Bearer token
#   - a Google OAuth client secret prefix (GOCSPX-)
#   - an explicit VEYLIX_*KEY= assignment
# Used by any function that echoes a command line or file content.
# ---------------------------------------------------------------------------

redact() {
	printf '%s' "$1" | sed -E \
		-e 's/(VEYLIX_[A-Z_]*KEY=)[^[:space:]]*/\1<redacted>/g' \
		-e 's/(Bearer[[:space:]]+)[A-Za-z0-9._-]+/\1<redacted>/g' \
		-e 's/GOCSPX-[A-Za-z0-9_-]+/<redacted>/g' \
		-e 's/[A-Za-z0-9+\/]{42,44}={0,2}/<redacted>/g'
}

# ---------------------------------------------------------------------------
# Dry-run aware execution
#
# EVERY mutation goes through `run`. A `mkdir`, `install`, `systemctl`, `cp`,
# `rm` or `curl -o` invoked directly defeats --dry-run and is a review defect.
#
#   DRY_RUN=0  (default) -> executes
#   DRY_RUN=1            -> prints what it would do and changes nothing
# ---------------------------------------------------------------------------

DRY_RUN=${DRY_RUN:-0}

# json_str VALUE - quote VALUE as a JSON string literal, with stdout only.
#
# The installer POSTs the operator's email, display name and password to
# /api/setup/bootstrap. Building that body with printf and bare quotes breaks on
# the first name containing a quote or a backslash, and a password very often
# does. This escapes exactly what RFC 8259 requires and nothing else; it does
# not try to be a general JSON encoder.
json_str() {
local s=$1 out= c i
for ((i = 0; i < ${#s}; i++)); do
c=${s:i:1}
case "$c" in
'"') out+='\"' ;;
'\\') out+='\\\\' ;;
$'\n') out+='\\n' ;;
$'\r') out+='\\r' ;;
$'\t') out+='\\t' ;;
*) out+="$c" ;;
esac
done
printf '"%s"' "$out"
}

is_dry_run() { [ "${DRY_RUN:-0}" = "1" ]; }

# run CMD [ARGS...]
run() {
	if is_dry_run; then
		printf '[dry-run] %s\n' "$(redact "$*")"
		return 0
	fi
	"$@"
}

# run_quiet CMD [ARGS...]
# Same as `run`, but the command's output is suppressed on success and shown on
# failure. Used for noisy-but-recoverable steps.
run_quiet() {
	if is_dry_run; then
		printf '[dry-run] %s\n' "$(redact "$*")"
		return 0
	fi
	local out
	if ! out=$("$@" 2>&1); then
		printf '%s\n' "$(redact "$out")" >&2
		return 1
	fi
	return 0
}

# would_run CMD... - true when the command would actually execute
would_run() { ! is_dry_run; }

# ---------------------------------------------------------------------------
# Privileges and locking
# ---------------------------------------------------------------------------

need_root() {
	[ "$(id -u)" -eq 0 ] || die "must run as root - re-run with sudo"
}

# acquire_lock [PATH]
#
# flock on a fixed path, non-blocking, held for the life of the process because
# fd 9 is never closed. `-n` is deliberate: waiting silently on a lock looks
# exactly like a hang, and an operator will Ctrl-C it mid-mutation.
#
# Two concurrent runs MUST NOT interleave, so a missing `flock` is a hard failure
# rather than a silent no-op. `flock` ships with util-linux on every supported
# distribution; a host without it is not a supported host, and proceeding anyway
# is how a race becomes a lost database.
acquire_lock() {
	local lock=${1:-/run/veylix-install.lock}
	local dir
	dir=$(dirname "$lock")
	[ -d "$dir" ] || mkdir -p "$dir" 2>/dev/null || die "cannot create lock directory $dir"

	if ! command -v flock >/dev/null 2>&1; then
		die "flock is not installed, so concurrent installs cannot be prevented.
       Install util-linux (which ships flock), then re-run."
	fi

	# `exec 9>` truncates; that is fine because flock is the mutex, not the file.
	exec 9>"$lock" || die "cannot open lock file $lock"
	flock -n 9 || die "another veylix install/upgrade is running (lock: $lock)"

	# Release on ANY exit path, including Ctrl+C and Ctrl+Z.
	#
	# Without this a suspended run (^Z) kept fd 9 open and the lock held forever:
	# every later run answered "another veylix install is running" and there was
	# nothing to clean up, because the process was merely stopped. TSTP is
	# handled by releasing the lock and then re-stopping, so resuming the job
	# would run on without the mutex - which is why the handler un-suspends and
	# exits instead. A stopped installer is not going to finish; making that
	# explicit is better than leaving the host wedged.
	# shellcheck disable=SC2064
	trap '' HUP
	trap "release_lock '$lock'" EXIT INT TERM
	trap "release_lock '$lock'; exit 130" TSTP

	say "lock acquired: $lock"
}

# release_lock LOCKFILE - idempotent. Called from the trap and safe to call twice.
release_lock() {
	local lock=${1:-}
	# Close the fd first: flock is released when the descriptor closes, and
	# failing to do so is exactly the bug this function exists to prevent.
	exec 9>&- 2>/dev/null || true
	if [ -n "$lock" ]; then
		rm -f "$lock" 2>/dev/null || true
	fi
	return 0
}


# ---------------------------------------------------------------------------
# Secrets
#
# The backend refuses to start without both keys (config.decodeKey), which is
# correct. These helpers only ever create a key that does not exist, and only
# ever print a fingerprint.
# ---------------------------------------------------------------------------

gen_key() { head -c 32 /dev/urandom | base64; }

# fingerprint FILE -> first 8 hex chars of the file's sha256, or "absent".
# Never the key itself.
fingerprint() {
	local f=$1
	if [ -f "$f" ]; then
		sha256sum "$f" 2>/dev/null | cut -c1-8
	else
		printf 'absent'
	fi
}

# write_secret FILE MODE
# Reads the secret from stdin and installs it with the given mode, root-owned.
# Refuses to overwrite an existing file: losing enc.key makes every sealed panel
# token unrecoverable, so overwriting is never a convenience.
#
# Returns non-zero when the install fails. It does NOT die: the caller decides
# whether a failure is fatal, and a function that silently succeeds on a failed
# install is exactly how "wrote the key" becomes a false claim.
write_secret() {
	local file=$1 mode=${2:-0600}
	if [ -e "$file" ]; then
		err "refusing to overwrite existing secret $file (fingerprint $(fingerprint "$file"))"
		return 1
	fi
	if is_dry_run; then
		printf '[dry-run] write secret %s (mode %s)\n' "$file" "$mode"
		return 0
	fi
	local tmp
	tmp=$(mktemp) || { err "cannot create a temporary file"; return 1; }
	cat >"$tmp"

	# -o root -g root only makes sense as root. A non-root caller still gets a
	# correctly-moded file rather than an "invalid user 'root'" failure that
	# would leave the file absent while the function reported success.
	if [ "$(id -u)" -eq 0 ]; then
		install -o root -g root -m "$mode" "$tmp" "$file" || { rm -f "$tmp"; err "cannot install $file"; return 1; }
	else
		install -m "$mode" "$tmp" "$file" || { rm -f "$tmp"; err "cannot install $file"; return 1; }
	fi
	rm -f "$tmp"

	# Belt and braces: `install -m` already sets the mode, but an explicit chmod
	# makes the guarantee independent of the installer. A key file that ends up
	# group- or world-readable on a real host is a credential leak.
	chmod "$mode" "$file" 2>/dev/null || true

	# Prove the file exists before claiming it does.
	[ -f "$file" ] || { err "install reported success but $file is missing"; return 1; }
	say "wrote $file (mode $mode, fingerprint $(fingerprint "$file"))"
}

# ---------------------------------------------------------------------------
# Checksum verification
#
# An artifact that does not match its published digest is deleted, not left on
# disk where a later step could pick it up.
# ---------------------------------------------------------------------------

# verify_checksum ARTIFACT SHA256SUMS NAME
verify_checksum() {
	local artifact=$1 sums=$2 name=$3
	[ -f "$artifact" ] || die "verify_checksum: $artifact not found"
	[ -f "$sums" ]     || die "verify_checksum: $sums not found"

	local expected actual
	expected=$(awk -v n="$name" '$2 == n || $2 == "*"n { print $1; exit }' "$sums")
	[ -n "$expected" ] || die "verify_checksum: no entry for $name in $sums"

	actual=$(sha256sum "$artifact" | awk '{print $1}')
	if [ "$expected" != "$actual" ]; then
		rm -f "$artifact"
		die "checksum mismatch for $name (expected ${expected:0:12}..., got ${actual:0:12}...) - artifact deleted"
	fi
	say "checksum ok: $name (${actual:0:12}...)"
}

# ---------------------------------------------------------------------------
# HTTP probes
#
# The only honest way to claim a service is up is to ask it.
# ---------------------------------------------------------------------------

# http_status URL [TIMEOUT] -> the status code, or 000 on connection failure.
http_status() {
	curl -s -o /dev/null -w '%{http_code}' --max-time "${2:-5}" "$1" 2>/dev/null || printf '000'
}

# wait_for_ready URL [ATTEMPTS] [SLEEP]
# Polls /api/ready until it answers 200. Non-zero if it never does.
wait_for_ready() {
	local url=$1 attempts=${2:-30} sleep_s=${3:-1} code
	local i=0
	while [ "$i" -lt "$attempts" ]; do
		code=$(http_status "$url" 3)
		[ "$code" = "200" ] && { say "ready: $url"; return 0; }
		i=$((i + 1))
		sleep "$sleep_s"
	done
	return 1
}

# ready_field FIELD [URL] -> one key from /api/ready as plain text.
# An absent key prints an empty string; never a fabricated default.
ready_field() {
	local field=$1 url=${2:-http://127.0.0.1:8080/api/ready}
	curl -s --max-time 5 "$url" 2>/dev/null |
		sed -n 's/.*"'"$field"'"[[:space:]]*:[[:space:]]*"\{0,1\}\([^",}]*\)"\{0,1\}.*/\1/p'
}

# ---------------------------------------------------------------------------
# Preflight primitives
#
# Order matters: the cheap checks come first so a failure costs nothing.
# ---------------------------------------------------------------------------

have() { command -v "$1" >/dev/null 2>&1; }

# detect_arch -> amd64 | arm64 | <uname -m>
detect_arch() {
	local m
	m=$(uname -m 2>/dev/null || printf 'unknown')
	case "$m" in
		x86_64|amd64)  printf 'amd64' ;;
		aarch64|arm64) printf 'arm64' ;;
		*)             printf '%s' "$m" ;;
	esac
}

# detect_os -> the /etc/os-release ID, or "unknown". Never guessed.
detect_os() {
	if [ -r /etc/os-release ]; then
		local id
		id=$(sed -n 's/^ID=["]*\([^"]*\)["]*$/\1/p' /etc/os-release | head -n1)
		[ -n "$id" ] && { printf '%s' "$id"; return; }
	fi
	if [ -r /etc/alpine-release ]; then
		printf 'alpine'
		return
	fi
	printf 'unknown'
}

# check_disk PATH MB -> 0 when at least MB is free.
check_disk() {
	local path=$1 need_mb=$2 avail_mb
	avail_mb=$(df -Pm "$path" 2>/dev/null | awk 'NR==2 {print $4}')
	[ -n "${avail_mb:-}" ] || return 1
	[ "$avail_mb" -ge "$need_mb" ]
}

# check_ram MB -> 0 when at least MB of physical memory is installed.
# Reads MemTotal (installed RAM), never currently-free memory: a busy but
# correctly sized host must not be refused.
check_ram() {
	local need_mb=$1 total_kb
	total_kb=$(awk '/^MemTotal:/ {print $2}' /proc/meminfo 2>/dev/null)
	[ -n "${total_kb:-}" ] || return 1
	[ $((total_kb / 1024)) -ge "$need_mb" ]
}

# port_in_use PORT -> 0 when something is listening, 1 when free.
# Tries ss, then netstat, then /proc/net/tcp.
port_in_use() {
	local port=$1
	if have ss; then
		ss -Hltn "sport = :$port" 2>/dev/null | grep -q . && return 0
		return 1
	fi
	if have netstat; then
		netstat -ltn 2>/dev/null | awk '{print $4}' | grep -qE "[:.]$port$" && return 0
		return 1
	fi
	local hex
	hex=$(printf '%04X' "$port")
	awk -v p=":$hex" 'NR>1 && $2 ~ p {found=1} END {exit !found}' /proc/net/tcp 2>/dev/null
}

# port_is_free PORT -> 0 when nothing is listening on it right now.
#
# The complement of port_in_use, and the only thing choose_ports() needs: the
# installer looks for a free port instead of asking the operator for one.
# Deliberately prefers a definite answer: when ss AND netstat are both absent it
# falls through to /proc/net/tcp, and if that is unreadable too it reports the
# port as BUSY rather than free. Claiming a port is free when we cannot tell is
# how an install ends in "address already in use" three phases later.
port_is_free() {
	local port=$1
	if port_in_use "$port"; then
		return 1
	fi
	# port_in_use returns 1 both for "free" and "cannot tell". Distinguish them
	# when we have no probe at all: then we must not promise the port.
	if ! have ss && ! have netstat && [ ! -r /proc/net/tcp ]; then
		return 1
	fi
	return 0
}


# resolve_host HOST -> the first IPv4 address, or nothing.
resolve_host() {
	local host=$1
	if have getent; then
		getent ahostsv4 "$host" 2>/dev/null | awk 'NR==1 {print $1}'
	elif have dig; then
		dig +short A "$host" 2>/dev/null | awk 'NR==1 {print}'
	elif have host; then
		host -t A "$host" 2>/dev/null | awk '{print $NF}' | awk 'NR==1 {print}'
	fi
}

# local_ip -> the primary outbound IPv4 address of this host, or nothing.
local_ip() {
	ip -4 route get 1.1.1.1 2>/dev/null |
		awk '{for (i = 1; i <= NF; i++) if ($i == "src") { print $(i + 1); exit }}'
}

# has_tty -> 0 when a prompt can actually be answered.
#
# `[ -t 0 ]` is the obvious test and it is WRONG for the way this installer is
# actually run:
#
#   bash <(curl -Ls .../install.sh)
#
# Under that form stdin is the process-substitution pipe carrying the SCRIPT
# itself, so `[ -t 0 ]` is false on an operator's interactive terminal and any
# `read` consumes script text instead of their answer. The same is true of
# `curl ... | bash`.
#
# /dev/tty is the operator's controlling terminal regardless of what stdin is
# wired to. If it can be opened for reading, a question can be asked and the
# answer will be real.
#
# VEYLIX_NONINTERACTIVE=1 forces the answer to be "no". A controlling terminal
# is inherited by child processes, so a script that pipes a subprocess still
# hands it a working /dev/tty - without this escape hatch, an automated test (or
# a cron job) would sit at a prompt nobody is watching. install.selftest.sh sets
# it; so does any CI.
has_tty() {
	[ "${VEYLIX_NONINTERACTIVE:-0}" = "1" ] && return 1
	[ -r /dev/tty ] && ( : </dev/tty ) 2>/dev/null
}

# read_tty VARNAME -> reads one line from the terminal into VARNAME.
# Returns 1 when there is no terminal or the read failed, so the caller can fall
# back to a default instead of hanging or taking the empty string as an answer.
read_tty() {
	local __var=$1
	has_tty || return 1
	local __line
	if ! IFS= read -r __line </dev/tty; then
		return 1
	fi
	printf -v "$__var" '%s' "$__line"
	return 0
}

# read_tty_secret VARNAME -> the same, without echo, for a password.
read_tty_secret() {
	local __var=$1
	has_tty || return 1
	local __line
	if ! IFS= read -rs __line </dev/tty; then
		printf '\n' >/dev/tty 2>/dev/null
		return 1
	fi
	printf '\n' >/dev/tty 2>/dev/null
	printf -v "$__var" '%s' "$__line"
	return 0
}

# validate_port PORT -> 0 when PORT is a usable TCP port, 1 otherwise.
#
# Accepts 1-65535 only: port 0 means "any free port" to the kernel, which would
# leave Caddy listening somewhere nobody can find, and 65536+ is not a port.
# Leading zeros are refused too - `08` looks like eight and is a shell octal trap.
validate_port() {
	local p=$1
	[ -n "$p" ] || return 1
	printf '%s' "$p" | grep -qE '^[1-9][0-9]{0,4}$' || return 1
	[ "$p" -ge 1 ] && [ "$p" -le 65535 ] || return 1
	return 0
}

# validate_domain DOMAIN -> 0 for a syntactically valid public FQDN.
# Refuses IP literals, localhost and anything without a dot: those cannot get a
# publicly trusted certificate and would produce a false "HTTPS active".
validate_domain() {
	local d=$1
	[ -n "$d" ] || return 1
	case "$d" in
		localhost|*..*|.*|*.) return 1 ;;
	esac
	printf '%s' "$d" | grep -qE '^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$' || return 1
	printf '%s' "$d" | grep -qE '^[0-9.]+$' && return 1
	return 0
}
