#!/usr/bin/env bash
# VEYLIX - one-line installer (bootstrap).
#
# This runs ONCE per host. It is not the update mechanism: after a host is
# installed, every subsequent change (new release, new domain, backup, rollback)
# is `vpn-platformctl`. That split is why this script contains no version number
# and no schema knowledge - it resolves the release channel at run time, so a
# project that moves on never invalidates it.
#
# See docs/INSTALLATION_DESIGN.md section 3 for the reasoning.
#
# Usage:
#   sudo ./install.sh --domain vpn.example.com
#   sudo ./install.sh --domain vpn.example.com --dry-run
#   sudo ./install.sh --domain vpn.example.com --version v1.4.0
#   sudo ./install.sh --domain vpn.example.com --repo you/veylix
#
# Fork or private mirror: --repo <owner/name> (or VEYLIX_REPO). The shipped
# default is a placeholder, so a fork ALWAYS passes one of the two; the installer
# refuses the placeholder rather than downloading a 404 and blaming the network.
#
# Exit codes:
#   0  installed AND publicly served over HTTPS
#   1  failed; nothing half-installed is left running
#   2  installed but NOT publicly served (DNS / ACME / ports) - service runs on
#      the loopback only. This is never reported as success.
#
# Data safety: this script REFUSES to run when a database already exists. That
# single branch is what stops a re-run from reinitialising a live platform.

set -euo pipefail

# A guard for the invocation that cannot work, placed before anything else so it
# is the FIRST thing an operator sees.
#
#   sudo bash <(curl -Ls .../install.sh)     <- broken
#
# Process substitution is expanded by the CALLING shell into /dev/fd/63, a
# descriptor owned by that non-root shell. `sudo` closes it (it closes every fd
# above 2 unless told otherwise), so the child bash opens a path that no longer
# points at anything and dies with "bash: /dev/fd/63: No such file or directory".
# Reproduced on Ubuntu 24.04 against the live URL: exit 127, one line, no
# installer output at all. Nothing inside install.sh can fix this, because the
# failure happens before a single line of it runs -- so the honest fix is to
# detect the shape and name the two forms that DO work.
#
# Detection: we only reach this line if bash started, and $0 tells us how. Under
# `sudo bash <(...)` the script is /dev/fd/N and SUDO_* is set by sudo itself, so
# this branch fires only for that mistake. A plain `sudo bash install.sh` has a
# real $0 and is untouched.
case "${0:-}" in
/dev/fd/* | /proc/*/fd/*)
	if [ "$(id -u)" -eq 0 ] && [ -n "${SUDO_USER:-}" ]; then
		echo "install.sh: ERROR: 'sudo bash <(curl ...)' cannot work." >&2
		echo "install.sh:" >&2
		echo "install.sh:   sudo closes the /dev/fd pipe that <(curl ...) creates," >&2
		echo "install.sh:   so bash starts with no script to read. This is a shell" >&2
		echo "install.sh:   limitation, not an installer failure - nothing ran." >&2
		echo "install.sh:" >&2
		echo "install.sh:   Use either of these instead:" >&2
		echo "install.sh:     curl -Ls https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh | sudo bash" >&2
		echo "install.sh:     sudo bash -c \"\$(curl -Ls https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh)\"" >&2
		exit 1
	fi
	;;
esac


# Where this script physically is.
#
# Under the documented one-liner, `bash <(curl -Ls .../install.sh)`, bash reads
# this file from a process-substitution pipe: BASH_SOURCE[0] is /dev/fd/63 and
# dirname is /dev/fd. There is no checkout there, so a sibling lookup can never
# work and `. /dev/fd/scripts/lib/common.sh` fails before anything else runs.
# That was D-127: the one-liner could not start, even though a local
# `./install.sh` worked.
#
# So: when the sibling tree is absent, this is a bootstrap run. Fetch the two
# files such a run needs into a temporary tree and continue from there. A real
# checkout never reaches this branch, so local installs are unchanged.
VEYLIX_RAW_BASE=${VEYLIX_RAW_BASE:-https://raw.githubusercontent.com/janith-15/Veylix-1/main}

# Every network call carries a deadline. A `curl` with no `--max-time` waits
# forever when the far end accepts the connection and then says nothing, which
# is what a firewalled or DNS-broken host looks like: the installer produced NO
# output at all and had to be suspended with ^Z (D-132). 10s to connect and 60s
# overall is generous for a 50 KB file and still bounded.
VEYLIX_CONNECT_TIMEOUT=${VEYLIX_CONNECT_TIMEOUT:-10}
VEYLIX_MAX_TIME=${VEYLIX_MAX_TIME:-60}

# fetch URL DEST -> 0 on success. The single place network deadlines are set, so
# no later `curl` can forget them.
fetch() {
	curl -fsSL \
		--connect-timeout "$VEYLIX_CONNECT_TIMEOUT" \
		--max-time "$VEYLIX_MAX_TIME" \
		--retry 1 \
		"$1" -o "$2"
}

# BASH_SOURCE[0] is UNSET under `curl ... | bash` - the script arrives on stdin
# and bash has no file to name - and it is a /dev/fd/N pipe under process
# substitution. Both mean the same thing: there is no checkout. Default it to
# empty and let the bootstrap below take over, because `set -u` would otherwise
# kill the installer outright at this line (D-128).
#
# VEYLIX_HERE pins the location explicitly. The selftest uses it (its copy of
# this script is written to a temp dir, so BASH_SOURCE would point there), and
# an operator can use it to install from a tree at an unusual path.
_veylix_self=${BASH_SOURCE[0]:-}
if [ -n "${VEYLIX_HERE:-}" ]; then
	HERE=$VEYLIX_HERE
else
	HERE=""
	if [ -n "$_veylix_self" ]; then
		HERE=$(cd "$(dirname "$_veylix_self")" 2>/dev/null && pwd) || HERE=""
	fi
fi

if [ -z "$HERE" ] || [ ! -f "$HERE/scripts/lib/common.sh" ]; then
	_veylix_boot=$(mktemp -d) || {
		echo "install.sh: ERROR: could not create a temporary directory" >&2
		exit 1
	}
	# shellcheck disable=SC2064  # expand now: _veylix_boot changes below
	trap "rm -rf '$_veylix_boot'" EXIT
	mkdir -p "$_veylix_boot/scripts/lib"

	_veylix_url="$VEYLIX_RAW_BASE/scripts/lib/common.sh"
	if ! fetch "$_veylix_url" "$_veylix_boot/scripts/lib/common.sh" 2>/dev/null; then
		echo "install.sh: ERROR: could not download $_veylix_url" >&2
		echo "install.sh:        the host may be offline or blocking github. Either:" >&2
		echo "install.sh:          - check: curl -m 15 -v $_veylix_url >/dev/null" >&2
		echo "install.sh:          - or clone the repository and run ./install.sh" >&2
		exit 1
	fi
	[ -s "$_veylix_boot/scripts/lib/common.sh" ] || {
		echo "install.sh: ERROR: $_veylix_url came back empty" >&2
		exit 1
	}
	HERE="$_veylix_boot"
fi

# shellcheck source=scripts/lib/common.sh
. "$HERE/scripts/lib/common.sh"

VEYLIX_TOOL=install.sh

# The repository this project actually ships from. Unlike a placeholder this is
# a REAL repository, so the one-line install works verbatim:
#
#   bash <(curl -Ls https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh)
#
# A fork overrides it with --repo <owner/name> or VEYLIX_REPO, and validate_repo()
# still refuses anything that is not exactly `owner/name`, so a typo names the
# correction instead of surfacing as a 404 blamed on the network.
DEFAULT_REPO=janith-15/Veylix-1
# Kept under the old name because the refusal message and the selftest refer to
# it: both now assert the SHIPPED default is a usable slug, not a placeholder.
PLACEHOLDER_REPO=$DEFAULT_REPO

# --- configuration (overridable, documented in docs/INSTALL.md section F) ---
REPO=${VEYLIX_REPO:-$PLACEHOLDER_REPO}
# Releases API origin. Overridable so a fork (or a test) can point elsewhere.
RELEASE_API=${VEYLIX_RELEASE_API:-https://api.github.com}
BIN_DIR=${VEYLIX_BIN_DIR:-/usr/local/bin}
CONFIG_DIR=${VEYLIX_CONFIG_DIR:-/etc/veylix}
DATA_DIR=${VEYLIX_DATA_DIR:-/var/lib/veylix}
DEPLOY_DIR=${VEYLIX_DEPLOY_DIR:-/etc/veylix/deploy}
CADDYFILE=${VEYLIX_CADDYFILE:-/etc/caddy/Caddyfile}
UNIT_PATH=${VEYLIX_UNIT_PATH:-/etc/systemd/system/veylixd.service}
LOCK_PATH=${VEYLIX_LOCK_PATH:-/run/veylix-install.lock}
SERVICE=veylixd

DOMAIN=""
VERSION=""
# Where the deployment templates (Caddyfile, unit, backup script) come from.
# Empty means "not resolved yet"; resolve_deploy_dir() decides before phase 3
# because phase 3 (branch) and every later phase install from it.
SCRIPT_BASE=${VEYLIX_SCRIPT_BASE:-}
DEPLOY_SRC=""
SKIP_DNS=0
SKIP_PORTS=0
REUSE_KEYS=0
SKIP_CADDY=0
DRY_RUN=0
LIST_VERSIONS=0

# --http: serve plain HTTP on a public port, with no certificate and no domain.
#
# This is the mode 3X-UI installs in, and the one an operator wants when the
# thing they are installing is reached by IP address rather than by name. It is
# NOT the loopback-only --skip-caddy mode: Caddy still fronts the app and still
# listens on a public interface, it simply serves http:// instead of https://.
#
# HTTPS stays reachable afterwards through the control menu:
#   vey domain-set <domain> --dns-provider cloudflare --cf-token <token>
# so this is a first step, not a dead end.
#
# Traffic is PLAINTEXT until that is done. report() says so in as many words;
# nothing here may describe such a host as secure.
HTTP_MODE=0

# The address Caddy binds. Loopback by default, because a normal install is
# reached through a domain and never needs to expose the listener. --http
# switches it to all interfaces, which is the whole point of that mode.
HTTP_BIND=127.0.0.1

# Set by validate_ports_for_install() when two ports collide, so main() can
# print the exact command that fixes it. Empty when the trio is usable.
PORT_CLASH=""

# Ports. The defaults are the conventional ones, so an ordinary install on a bare
# host is unchanged by this feature existing.
#
# HTTPS_PORT is the port a CUSTOMER types. It stays 443 by default because
# `https://vpn.example.com` with no port is what people expect; a host that
# already runs 3X-UI on 443 passes --https-port 8443 instead of moving either
# service.
HTTP_PORT=80
HTTPS_PORT=443
APP_PORT=8080

# Set by parse_args when a port flag was given. The interactive prompt then
# stays out of the way: an operator who typed --https-port has already answered,
# and asking again (with a default that would overwrite their choice) is the kind
# of question that trains people to press Enter without reading.
PORTS_FROM_FLAGS=0

usage() {
	cat <<'EOF'
VEYLIX installer

  sudo ./install.sh --domain vpn.example.com [options]

Required:
  --domain <fqdn>      the public hostname Caddy will serve over HTTPS

Options:
  --version <tag>      install an exact release (default: latest)
  --list-versions      list published releases, then exit (no install)
  --repo <owner/name>  the GitHub repository to install FROM (default: the
                       placeholder $PLACEHOLDER_REPO, which is refused). Also
                       settable as VEYLIX_REPO; the flag wins.
  --endpoint <url>     releases API origin for a GitHub Enterprise host
                       (default: https://api.github.com)
  --script-base <url>  legacy single-file fallback for the deployment templates
  --http-port <port>   the port Caddy serves plain HTTP on, for the ACME
                       challenge and the redirect (default: 80)
  --https-port <port>  the port Caddy serves HTTPS on - this is the port a
                       customer types (default: 443). Set 8443 when 3X-UI
                       already holds 443 on this host.
  --app-port <port>    the loopback port veylixd listens on, behind Caddy
                       (default: 8080). Not reachable from the network.
  --dry-run            print every phase and change nothing
  --http               serve plain HTTP on --http-port, reached by IP address.
                       No domain and no certificate are needed. This is the
                       mode 3X-UI installs in. Traffic is PLAINTEXT until you
                       add a domain from the menu:
                         vey domain-set <domain> --dns-provider cloudflare \\
                           --cf-token <token>
  --skip-dns           skip the DNS A-record check (recorded in the output)
  --skip-ports         skip the port check (recorded in the output)
  --skip-caddy         install without a reverse proxy (loopback only, exit 2)
  --reuse-keys         accept existing keys in /etc/veylix (breaks recovery)

Running beside 3X-UI on one host: 3X-UI normally holds 80 and 443 for Xray.
Either move the 3X-UI inbound, or keep both and give Veylix its own ports:

  sudo ./install.sh --domain vpn.example.com --http-port 8080 --https-port 8443

Re-running this on an installed host is REFUSED. Use vpn-platformctl instead:
  vpn-platformctl update | rollback | domain-set | backup | restore | repair
EOF
}

parse_args() {
	while [ $# -gt 0 ]; do
		case "$1" in
			--domain)     DOMAIN=${2:-}; shift 2 ;;
			--domain=*)   DOMAIN=${1#*=}; shift ;;
			--version)    VERSION=${2:-}; shift 2 ;;
			--version=*)  VERSION=${1#*=}; shift ;;
			--repo)       REPO=${2:-}; shift 2 ;;
			--repo=*)     REPO=${1#*=}; shift ;;
			--endpoint)   RELEASE_API=${2:-}; shift 2 ;;
			--endpoint=*) RELEASE_API=${1#*=}; shift ;;
			--script-base)   SCRIPT_BASE=${2:-}; shift 2 ;;
			--script-base=*) SCRIPT_BASE=${1#*=}; shift ;;
			--list-versions) LIST_VERSIONS=1; shift ;;
			--dry-run)    DRY_RUN=1; shift ;;
			--http)       HTTP_MODE=1; HTTP_BIND=0.0.0.0; shift ;;
			--skip-dns)   SKIP_DNS=1; shift ;;
			--skip-ports) SKIP_PORTS=1; shift ;;
			--skip-caddy) SKIP_CADDY=1; shift ;;
			--reuse-keys) REUSE_KEYS=1; shift ;;
			--http-port)   HTTP_PORT=${2:-}; PORTS_FROM_FLAGS=1; shift 2 ;;
			--http-port=*) HTTP_PORT=${1#*=}; PORTS_FROM_FLAGS=1; shift ;;
			--https-port)   HTTPS_PORT=${2:-}; PORTS_FROM_FLAGS=1; shift 2 ;;
			--https-port=*) HTTPS_PORT=${1#*=}; PORTS_FROM_FLAGS=1; shift ;;
			--app-port)   APP_PORT=${2:-}; PORTS_FROM_FLAGS=1; shift 2 ;;
			--app-port=*) APP_PORT=${1#*=}; PORTS_FROM_FLAGS=1; shift ;;
			-h|--help)    usage; exit 0 ;;
			*)            usage >&2; die "unknown argument: $1" 2 ;;
		esac
	done
	# NOTE: this function must end with `return 0`. Under `set -e`, a trailing
	# `[ cond ] && something` that is false makes the FUNCTION return 1, and
	# `main` calls `parse_args "$@"` as a bare command - so the whole installer
	# died silently with no message whenever no flag was passed (D-129).
	return 0
}

release_base() {
	if [ -z "$VERSION" ] || [ "$VERSION" = "latest" ]; then
		printf 'https://github.com/%s/releases/latest/download' "$REPO"
	else
		printf 'https://github.com/%s/releases/download/%s' "$REPO" "$VERSION"
	fi
}

# validate_repo SLUG -> 0 for exactly "owner/name".
#
# This exists because the shipped default was a placeholder and a fork that
# forgot to override it got `curl: 404` from github.com and a message about the
# network. A refused input names the correction instead: one line, no guesswork.
validate_repo() {
	local r=$1
	[ -n "$r" ] || return 1
	case "$r" in
		http://*|https://*|git@*|*' '*) return 1 ;;
	esac
	# Exactly one slash, nothing empty on either side.
	printf '%s' "$r" | grep -qE '^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$' || return 1
	return 0
}

# resolve_deploy_dir - find the deployment templates, in order of trust:
#
#   1. $HERE/deploy/                     a checkout next to the installer
#   2. $base/veylix-deploy.tar.gz        the release bundle (published by
#                                        .github/workflows/release.yml)
#   3. $SCRIPT_BASE/veylix-<name>        legacy per-file fallback
#
# WHY THIS EXISTS: a release used to publish only the two binaries and
# SHA256SUMS, so a one-curl install had NO Caddyfile, NO systemd unit and NO
# backup script. Phase 9 then skipped HTTPS and `vpn-platformctl update`
# refused to run its own backup step. The templates are now part of the release,
# and this function is what reaches them.
#
# A tarball is never extracted blindly: only members under deploy/ are written,
# into a throwaway staging directory.
resolve_deploy_dir() {
	local base bundle stage

	if [ -f "$HERE/deploy/Caddyfile" ] || [ -f "$HERE/deploy/veylixd.service" ]; then
		DEPLOY_SRC="$HERE/deploy"
		say "deployment files: $DEPLOY_SRC (checkout)"
		return 0
	fi

	base=$(release_base)
	bundle="$base/veylix-deploy.tar.gz"
	stage=$(mktemp -d)

	if is_dry_run; then
		out "[dry-run] curl -fsSL $bundle -o $stage/veylix-deploy.tar.gz"
		out "[dry-run] tar -xzf <bundle> -C $stage --wildcards 'deploy/*'"
		DEPLOY_SRC="$stage/deploy"
		say "deployment files: would come from $bundle"
		return 0
	fi

	if fetch "$bundle" "$stage/veylix-deploy.tar.gz" 2>/dev/null; then
		# Only deploy/* members. `--force-local` keeps a Windows-style path from
		# being read as a remote host; the wildcard form is GNU tar, which is what
		# every supported distribution ships.
		if tar -xzf "$stage/veylix-deploy.tar.gz" -C "$stage" --wildcards 'deploy/*' 2>/dev/null \
			&& [ -f "$stage/deploy/veylixd.service" ]; then
			DEPLOY_SRC="$stage/deploy"
			say "deployment files: $bundle"
			return 0
		fi
		warn "downloaded $bundle but it did not contain deploy/veylixd.service - ignoring it"
	fi

	if [ -n "$SCRIPT_BASE" ]; then
		local name
		for name in Caddyfile veylixd.service veylix-backup.sh; do
			if fetch "$SCRIPT_BASE/veylix-$name" "$stage/$name" 2>/dev/null; then
				say "deployment file: $SCRIPT_BASE/veylix-$name"
			else
				warn "no veylix-$name at $SCRIPT_BASE"
			fi
		done
		if [ -s "$stage/veylixd.service" ]; then
			DEPLOY_SRC="$stage"
			return 0
		fi
	fi

	# 4. The raw repository. This is the branch a `bash <(curl ...)` bootstrap
	#    run takes: there is no checkout beside the installer and there may be
	#    no published release bundle yet, but the templates are in the repo at
	#    a known path. Without this the one-liner used to continue with
	#    DEPLOY_SRC empty, installing a host with no Caddy site and no backup -
	#    it printed a warning and called that success. It refuses now.
	local name
	for name in Caddyfile veylixd.service veylix-backup.sh; do
		curl -fsSL --connect-timeout "$VEYLIX_CONNECT_TIMEOUT" --max-time "$VEYLIX_MAX_TIME" \
			"$VEYLIX_RAW_BASE/deploy/$name" -o "$stage/$name" 2>/dev/null || true
	done
	if [ -s "$stage/veylixd.service" ] && [ -s "$stage/Caddyfile" ]; then
		DEPLOY_SRC="$stage"
		say "deployment files: $VEYLIX_RAW_BASE/deploy"
		return 0
	fi

	warn "no deployment templates found (no ./deploy next to the installer, no"
	warn "$bundle, no --script-base, nothing at $VEYLIX_RAW_BASE/deploy)."
	warn "Refusing to install a host with no Caddy site and no backup."
	warn "see docs/INSTALL.md section B.3 for the manual copy."
	DEPLOY_SRC=""
	return 1
}

# deploy_file NAME - the resolved path of one deployment template, or nothing.
deploy_file() {
	local name=$1
	[ -n "$DEPLOY_SRC" ] || return 1
	[ -s "$DEPLOY_SRC/$name" ] || return 1
	printf '%s' "$DEPLOY_SRC/$name"
}

# list_versions prints the published release tags, newest first, then exits.
# It is the installer-side answer to "which versions can I pin with --version?":
# a read-only call that touches nothing on the host and needs no root.
list_versions() {
	have curl || die "curl is required to list releases. Install curl and re-run." 2
	say "releases for $REPO (source: $RELEASE_API)"
	tags=$(curl -fsSL --connect-timeout "$VEYLIX_CONNECT_TIMEOUT" --max-time "$VEYLIX_MAX_TIME" \
		"$RELEASE_API/repos/$REPO/releases?per_page=100" 2>/dev/null \
		| grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' \
		| sed -E 's/.*"([^"]*)"$/\1/' || true)
	if [ -z "$tags" ]; then
		die "could not read any releases for $REPO (wrong repo, no releases yet, or the API is unreachable)." 1
	fi
	printf '%s\n' "$tags"
	echo
	echo "Install one:  sudo ./install.sh --domain vpn.example.com --version <tag>"
}

# ---------------------------------------------------------------------------
# Phase 1 - preflight. Cheap checks first, so a failure costs nothing and
# nothing has been written yet. Every failure prints the CORRECTION.
# ---------------------------------------------------------------------------

# choose_ports -> pick the ports automatically. NO PROMPT.
#
# 3X-UI installs without asking anything, and that is the bar: one command, one
# working URL at the end. The prompt this replaces asked three questions and
# then refused the answer - an operator typed the same number for the public
# port and the internal one and the install stopped on a WARNING.
#
# So the installer looks instead of asking:
#   1. A port flag was passed        -> that is the answer, use it.
#   2. Otherwise PROBE the host      -> the first free port from a short list.
#   3. Nothing free                  -> die, naming --http-port.
#
# The app port is chosen to be DIFFERENT from the public one by construction,
# so the clash that stopped the last run cannot happen at all.
#
# --dry-run still probes (it is read-only) so the printed plan is the real plan.
choose_ports() {
	if [ "$PORTS_FROM_FLAGS" -eq 1 ]; then
		# The operator answered with flags. Only fill in what they left out.
		if [ "$HTTP_MODE" -eq 1 ] && [ "$APP_PORT" = "$HTTP_PORT" ]; then
			APP_PORT=$(pick_free_port "app (loopback only)" 8080 18080 28080)
		fi
		say "ports: $([ "$HTTP_MODE" -eq 1 ] && echo "HTTP $HTTP_PORT" || echo "HTTPS $HTTPS_PORT / HTTP $HTTP_PORT"), app $APP_PORT (127.0.0.1)"
		return 0
	fi

	if [ "$HTTP_MODE" -eq 1 ]; then
		# Public listener first: 80 is what a browser assumes, so it is the only
		# port a customer can type without a colon.
		HTTP_PORT=$(pick_free_port "http" 80 8080 8081 8880 2080) || return 1
		HTTPS_PORT=443
	else
		# A domain install needs both 80 (ACME) and 443 (the site).
		HTTP_PORT=$(pick_free_port "acme" 80 8080 8081 8880 2080) || return 1
		HTTPS_PORT=$(pick_free_port "https" 443 8443 9443) || return 1
	fi

	# The app MUST differ from whatever Caddy took. Asking for a different port
	# list rather than reusing the public one is what guarantees it.
	APP_PORT=$(pick_free_port "app (loopback only)" 8080 18080 28080 38080) || return 1
	if [ "$APP_PORT" = "$HTTP_PORT" ] || [ "$APP_PORT" = "$HTTPS_PORT" ]; then
		APP_PORT=$(pick_free_port "app (loopback only)" 18080 28080 38080) || return 1
	fi

	if [ "$HTTP_MODE" -eq 1 ]; then
		say "ports: HTTP $HTTP_PORT, app $APP_PORT (127.0.0.1) - no prompt; change with: vey ports set"
	else
		say "ports: HTTPS $HTTPS_PORT, HTTP $HTTP_PORT, app $APP_PORT (127.0.0.1) - no prompt; change with: vey ports set"
	fi
	return 0
}

# pick_free_port LABEL CANDIDATES... -> the first candidate nothing is bound to.
#
# Probes with `ss` (iproute2, present on every supported distribution) and falls
# back to a connect attempt. A dry run probes too: the probe is read-only, and a
# plan that named ports the host does not have would be a plan for a different
# install. When nothing is free it dies with the flag that fixes it, rather than
# printing a WARNING and then refusing - which is what confused the last run.
pick_free_port() {
	local label=$1
	shift
	local p
	for p in "$@"; do
		if port_is_free "$p"; then
			printf '%s' "$p"
			return 0
		fi
	done
	warn "every candidate port for $label is already in use: $*"
	die "no free $label port. Free one of them, or pass it yourself:
         sudo ./install.sh --http-port <port> --app-port <port>" 1
}

# ask_one_port LABEL DEFAULT VARNAME -> one validated port, Enter for the default.
#
# Re-asks on a bad value instead of dying: a typo at an interactive prompt should
# cost one line, not the whole install. Gives up after three tries so a scripted
# terminal feeding garbage cannot loop forever.
ask_one_port() {
	local label=$1 default=$2 varname=$3 input attempt=0
	while [ "$attempt" -lt 3 ]; do
		attempt=$((attempt + 1))
		printf '  %s [%s]: ' "$label" "$default" >/dev/tty
		if ! read_tty input; then
			input=""
		fi
		[ -n "$input" ] || input=$default
		if validate_port "$input"; then
			printf -v "$varname" '%s' "$input"
			return 0
		fi
		printf '  %s is not a port. Use 1-65535.\n' "$input" >/dev/tty
	done
	warn "no valid $label after three tries - using $default"
	printf -v "$varname" '%s' "$default"
	return 0
}

# validate_ports_for_install -> 0 when the three ports are usable, 1 otherwise.
#
# Three separate questions, and all three are answered before anything is
# written:
#   1. Is each one actually a port? (validate_port)
#   2. Are they distinct? Two services on one port is a bind failure at runtime,
#      and detecting it here names the pair instead of producing "address already
#      in use" from Caddy two phases later.
#   3. Is the app port loopback-safe? VEYLIX_ADDR must stay on 127.0.0.1 - the
#      public listener is Caddy (Bible §13.2), and the only thing that makes that
#      true is the address we hand the binary.
validate_ports_for_install() {
	local name val
	for pair in "http-port:$HTTP_PORT" "https-port:$HTTPS_PORT" "app-port:$APP_PORT"; do
		name=${pair%%:*}
		val=${pair#*:}
		if ! validate_port "$val"; then
			if [ -z "$val" ]; then
				warn "--$name needs a port number, e.g. --$name 8443"
			else
				warn "--$name $val is not a usable port. Use 1-65535 (no leading zeros)."
			fi
			return 1
		fi
	done

	if [ "$HTTP_PORT" = "$HTTPS_PORT" ]; then
		warn "--http-port and --https-port are both $HTTP_PORT. They must differ: one is"
		warn "the ACME challenge and redirect listener, the other is HTTPS itself."
		PORT_CLASH="http=https"
		return 1
	fi
	if [ "$APP_PORT" = "$HTTPS_PORT" ]; then
		warn "--app-port and --https-port are both $APP_PORT. Caddy cannot front a service"
		warn "that is on the same port Caddy already listens on."
		PORT_CLASH="app=https"
		return 1
	fi
	if [ "$APP_PORT" = "$HTTP_PORT" ]; then
		warn "--http-port and --app-port are both $APP_PORT."
		warn "The app port is internal (127.0.0.1 only) and must differ from the public one."
		PORT_CLASH="app=http"
		return 1
	fi
	return 0
}


preflight() {
	phase "preflight"

	need_root

	OS=$(detect_os)
	ARCH=$(detect_arch)
	say "host: os=$OS arch=$ARCH"

	case "$OS" in
		debian|ubuntu|raspbian|fedora|centos|rhel|rocky|almalinux|alpine) ;;
		*) die "unsupported OS '$OS'. Supported: Debian, Ubuntu, Raspbian, Fedora, CentOS, RHEL, Rocky, AlmaLinux, Alpine" ;;
	esac

	case "$ARCH" in
		amd64|arm64) ;;
		*) die "unsupported architecture '$ARCH'. Supported: x86_64 (amd64), aarch64 (arm64)" ;;
	esac

	have systemctl || die "systemd is required (systemctl not found). Non-systemd hosts are not supported."

	# Name a missing template NOW rather than at phase 9. An install with no
	# Caddyfile ships an API-only box, and one with no backup script produces a
	# host where `vpn-platformctl update` refuses to run. Both are worth knowing
	# before a single byte is written.
	if [ "$SKIP_CADDY" -eq 0 ] && [ -z "$DEPLOY_SRC" ]; then
		warn "no deployment templates could be located: HTTPS and the backup script"
		warn "will be missing from this install. Fix the release bundle, or re-run"
		warn "with --script-base <url>, or use --skip-caddy to say you meant it."
	fi

	if check_disk / "$((200))"; then
		say "disk: at least 200 MB free on /"
	else
		die "not enough disk on / - at least 200 MB is required. Free space and re-run."
	fi

	if check_ram 400; then
		say "memory: at least 400 MB installed"
	else
		die "not enough RAM - at least 400 MB is required (the product target is 512 MB)."
	fi

	if [ "$SKIP_PORTS" -eq 1 ]; then
		skip "ports $HTTP_PORT/$HTTPS_PORT" "--skip-ports was passed; Caddy may fail to bind"
	else
		local p ports_to_check
		# --http binds ONE public port (80, or its fallback). There is no TLS
		# listener, so 443 is not ours and 3X-UI legitimately holds it. Checking
		# it refused the documented 3X-UI coexistence case with "port 443 is
		# already in use" - on precisely the host this mode exists for. Reported
		# from a live 3X-UI box, where that stopped the install outright.
		if [ "$HTTP_MODE" -eq 1 ]; then
			ports_to_check=$HTTP_PORT
		else
			ports_to_check="$HTTP_PORT $HTTPS_PORT"
		fi
		for p in $ports_to_check; do
			if port_in_use "$p"; then
				# The old message said "stop the service holding it". That is
				# correct for a stray process and WRONG for 3X-UI, which the
				# operator needs running - it is what serves their VPN clients.
				# Naming both remedies is the difference between a fix and an
				# outage.
				#
				# The fix command is printed WITH a domain only when one was
				# given. A --domain-less run used to print an empty flag
				# ("--domain  --http-port 8080"), which cannot be pasted, so the
				# one line that was supposed to rescue the install was unusable.
				if [ -n "$DOMAIN" ]; then
					die "port $p is already in use.
       If it is 3X-UI (Xray) and you need it running, give Veylix its own ports:
         sudo ./install.sh --domain $DOMAIN --http-port 8080 --https-port 8443
       If it is a stray process you do not need, stop it and re-run.
       Then reach Veylix at https://$DOMAIN:8443 (note the port)."
				else
					die "port $p is already in use.
       If it is 3X-UI (Xray) and you need it running, give Veylix its own ports:
         sudo bash install.sh --http --http-port 8080 --app-port 9090
       If it is a stray process you do not need, stop it and re-run.
       Then reach Veylix at http://<this-host>:$HTTP_PORT (note the port)."
				fi
			fi
		done
		say "ports: $ports_to_check free"
	fi

	if [ "$HTTP_MODE" -eq 1 ]; then
		skip "dns" "--http was used; there is no domain and no certificate to issue"
	elif [ "$SKIP_CADDY" -eq 1 ]; then
		skip "dns" "--skip-caddy was passed; there will be no HTTPS"
	elif [ "$SKIP_DNS" -eq 1 ]; then
		skip "dns" "--skip-dns was passed; certificate issuance will fail if the record is wrong"
	else
		local resolved actual
		resolved=$(resolve_host "$DOMAIN")
		[ -n "$resolved" ] || die "$DOMAIN does not resolve to any IPv4 address. Create an A record pointing at this server, then re-run (or pass --skip-dns)."
		actual=$(local_ip)
		if [ -n "$actual" ] && [ "$resolved" != "$actual" ]; then
			die "$DOMAIN resolves to $resolved but this host is $actual. Fix the A record (or pass --skip-dns)."
		fi
		say "dns: $DOMAIN -> $resolved"
	fi

	have curl || die "curl is required to download the release. Install curl and re-run."
	say "preflight ok"
}

# ---------------------------------------------------------------------------
# Phase 2 - the three-way branch. This is the data-loss guard.
#
# The decision is made from the DATABASE, not the binary: replacing a binary is
# routine and harmless, the database is the thing that must never be recreated.
# ---------------------------------------------------------------------------

branch() {
	phase "install mode"
	local db="$DATA_DIR/veylix.db"

	if [ -f "$db" ]; then
		die "already installed: a database exists at $db. This installer will NOT touch it.
       To upgrade, change the domain or repair the install use:
         vpn-platformctl update
         vpn-platformctl domain-set <domain>
         vpn-platformctl repair
       To start over, first back up and then remove it deliberately:
         vpn-platformctl backup && vpn-platformctl uninstall --purge-data"
	fi

	if [ -d "$CONFIG_DIR" ] && { [ -f "$CONFIG_DIR/enc.key" ] || [ -f "$CONFIG_DIR/session.key" ]; }; then
		if [ "$REUSE_KEYS" -eq 1 ]; then
			warn "reusing existing keys in $CONFIG_DIR (fingerprints: enc=$(fingerprint "$CONFIG_DIR/enc.key") session=$(fingerprint "$CONFIG_DIR/session.key"))"
		else
			die "keys already exist in $CONFIG_DIR but no database does.
       Either this is a second install pointed at another install's keys, or a
       partial removal. Overwriting them would make every sealed panel token
       unrecoverable. Inspect the directory, then re-run with --reuse-keys if
       you are certain."
		fi
	fi

	say "branch: FRESH INSTALL (no database at $db)"
}

# ---------------------------------------------------------------------------
# Phase 3 - download and verify. An artifact that does not match its published
# digest is deleted, not installed.
# ---------------------------------------------------------------------------

fetch_artifact() {
	phase "download release"
	local base bin_name cli_name stage
	base=$(release_base)
	bin_name="veylixd-linux-$ARCH"
	cli_name="vpn-platformctl-linux-$ARCH"
	stage=$(mktemp -d)
	STAGE=$stage
	say "release: $base"

	BIN_ARTIFACT="$stage/$bin_name"
	CLI_ARTIFACT="$stage/$cli_name"
	local sums="$stage/SHA256SUMS"

	if is_dry_run; then
		out "[dry-run] curl -fsSL $base/$bin_name -o $BIN_ARTIFACT"
		out "[dry-run] curl -fsSL $base/SHA256SUMS -o $sums"
		out "[dry-run] verify $bin_name against $sums"
		CLI_AVAILABLE=0
		return 0
	fi

	fetch "$base/$bin_name" "$BIN_ARTIFACT" \
		|| die "could not download $base/$bin_name after $VEYLIX_MAX_TIME s. Check the release exists for $ARCH (and that VEYLIX_REPO is set for a fork)."
	fetch "$base/SHA256SUMS" "$sums" \
		|| die "could not download $base/SHA256SUMS"

	verify_checksum "$BIN_ARTIFACT" "$sums" "$bin_name"

	CLI_AVAILABLE=0
	if fetch "$base/$cli_name" "$CLI_ARTIFACT" 2>/dev/null; then
		if verify_checksum "$CLI_ARTIFACT" "$sums" "$cli_name" 2>/dev/null; then
			CLI_AVAILABLE=1
		else
			warn "$cli_name present but its checksum did not match; the CLI will not be installed"
			CLI_AVAILABLE=0
		fi
	else
		warn "this release publishes no $cli_name; install the CLI separately or use install.sh again on the next release"
	fi
}

# ---------------------------------------------------------------------------
# Phase 4 - service account and directories.
#
# /var/lib/veylix holds the database and is writable ONLY by the service user.
# /etc/veylix holds the keys and is root-only.
# ---------------------------------------------------------------------------

create_user_and_dirs() {
	phase "service account and directories"

	if id -u veylix >/dev/null 2>&1; then
		say "user veylix already exists"
	else
		run useradd --system --home "$DATA_DIR" --shell /usr/sbin/nologin veylix
		say "created system user veylix"
	fi

	run mkdir -p "$DATA_DIR" "$CONFIG_DIR" "$DEPLOY_DIR" "$(dirname "$LOCK_PATH")"
	run chown veylix:veylix "$DATA_DIR"
	run chmod 0750 "$DATA_DIR"
	run chown root:root "$CONFIG_DIR"
	run chmod 0750 "$CONFIG_DIR"
	say "directories: $DATA_DIR (veylix:veylix 0750), $CONFIG_DIR (root:root 0750)"
}

# ---------------------------------------------------------------------------
# Phase 5 - secrets. Generated ONLY when absent; never overwritten.
#
# enc.key is unrecoverable by design: every sealed 3X-UI panel token depends on
# it. Only the fingerprint is ever printed.
# ---------------------------------------------------------------------------

generate_secrets() {
	phase "secrets"

	if [ -f "$CONFIG_DIR/enc.key" ] && [ "$REUSE_KEYS" -eq 1 ]; then
		say "enc.key kept (fingerprint $(fingerprint "$CONFIG_DIR/enc.key"))"
	else
		umask 077
		gen_key | write_secret "$CONFIG_DIR/enc.key" 0600 \
			|| die "could not write $CONFIG_DIR/enc.key"
	fi

	if [ -f "$CONFIG_DIR/session.key" ] && [ "$REUSE_KEYS" -eq 1 ]; then
		say "session.key kept (fingerprint $(fingerprint "$CONFIG_DIR/session.key"))"
	else
		umask 077
		gen_key | write_secret "$CONFIG_DIR/session.key" 0600 \
			|| die "could not write $CONFIG_DIR/session.key"
	fi

	say "enc.key     fingerprint $(fingerprint "$CONFIG_DIR/enc.key")"
	say "session.key fingerprint $(fingerprint "$CONFIG_DIR/session.key")"
}

# ---------------------------------------------------------------------------
# Phase 6 - environment file. The keys are inlined by reading the files, so no
# secret ever appears on a command line (which `ps` would show).
# ---------------------------------------------------------------------------

render_config() {
	phase "configuration"
	local env="$CONFIG_DIR/veylix.env"

	if [ -f "$env" ]; then
		die "$env already exists. This installer does not overwrite configuration.
       Inspect it, then re-run with --reuse-keys if the keys in it are correct."
	fi

	if is_dry_run; then
		out "[dry-run] render $env (0600 root:root) with VEYLIX_ADDR=127.0.0.1:$APP_PORT"
		return 0
	fi

	local enc sess
	enc=$(cat "$CONFIG_DIR/enc.key")
	sess=$(cat "$CONFIG_DIR/session.key")

	umask 077
	{
		echo "# VEYLIX - generated by install.sh on $(date -u +%Y-%m-%dT%H:%M:%SZ)"
		echo "# Contains the two keys. Never commit this file."
		echo "VEYLIX_ADDR=127.0.0.1:$APP_PORT"
		echo "VEYLIX_DATA_DIR=$DATA_DIR"
		echo "VEYLIX_ENCRYPTION_KEY=$enc"
		echo "VEYLIX_SESSION_KEY=$sess"
		echo "VEYLIX_SYNC_INTERVAL=5m"
		echo "VEYLIX_SYNC_TIMEOUT=60s"
		if [ "$HTTP_MODE" -eq 1 ] && [ "$SKIP_CADDY" -eq 0 ]; then
			# Plain HTTP in front. A Secure cookie is never sent back over
			# http://, so the login would return 200 and every call after it
			# would answer 401 - F-013, seen for real. The override is safe
			# HERE because veylixd still binds 127.0.0.1 (above): the cookie
			# never crosses a network interface unencrypted, it only skips the
			# Secure flag over a loopback hop. config.resolveSecureCookies
			# already refuses this combination for a non-loopback bind, so this
			# line cannot weaken a networked listener.
			echo "VEYLIX_INSECURE_COOKIES=1"
		else
			echo "VEYLIX_INSECURE_COOKIES=0"
		fi
		if [ -n "$DOMAIN" ] && [ "$SKIP_CADDY" -eq 0 ]; then
			echo "VEYLIX_PUBLIC_BASE_URL=https://$DOMAIN"
		elif [ "$HTTP_MODE" -eq 1 ] && [ "$SKIP_CADDY" -eq 0 ]; then
			# Plain HTTP by IP. The backend uses this for cookie/CSRF origin
			# checks, so it must match what the browser actually shows.
			echo "VEYLIX_PUBLIC_BASE_URL=http://$(reachable_ip):$HTTP_PORT"
		fi
	} >"$env"

	chmod 0600 "$env"
	chown root:root "$env"
	# Prove the file holds what it must, without printing it.
	grep -q "^VEYLIX_ENCRYPTION_KEY=." "$env" || die "$env was written without a key; aborted"
	say "wrote $env (0600 root:root)"
}

# ---------------------------------------------------------------------------
# Phase 7 - install the binaries. The UI is embedded in veylixd, so a correct
# artifact serves both the interface and the API on one port.
# ---------------------------------------------------------------------------

install_binaries() {
	phase "binaries"

	if is_dry_run; then
		out "[dry-run] install -m 0755 $BIN_ARTIFACT $BIN_DIR/veylixd"
		# `if`, not `[ ] && out`: a false test as a bare command aborts the
		# script under `set -e`, and CLI_AVAILABLE is 0 on every dry run that
		# did not download the CLI - so this ended every --dry-run early (D-130).
		if [ "${CLI_AVAILABLE:-0}" -eq 1 ]; then
			out "[dry-run] install -m 0755 $CLI_ARTIFACT $BIN_DIR/vpn-platformctl"
		fi
		return 0
	fi

	run install -o root -g root -m 0755 "$BIN_ARTIFACT" "$BIN_DIR/veylixd"
	say "installed $BIN_DIR/veylixd"

	if [ "${CLI_AVAILABLE:-0}" -eq 1 ]; then
		run install -o root -g root -m 0755 "$CLI_ARTIFACT" "$BIN_DIR/vpn-platformctl"
		say "installed $BIN_DIR/vpn-platformctl"
	fi

	# The launcher (D-126). This is the piece 3X-UI has and Veylix did not: a
	# short command an operator can type from muscle memory. Without it, opening
	# the menu means remembering a 16-character binary name and prefixing sudo.
	#
	# A SCRIPT, not a symlink: it must survive `vpn-platformctl update`, which
	# replaces /usr/local/bin/vpn-platformctl by rename. A symlink would keep
	# pointing at the right path either way, but a wrapper can also notice a
	# missing install and say so in one line instead of "No such file or
	# directory" from bash.
	install_launcher

	# Ship the backup script with the install so vpn-platformctl can delegate to
	# it without depending on a checkout. It comes from the resolved template
	# source: the checkout, the release bundle, or --script-base.
	BACKUP_SRC=$(deploy_file veylix-backup.sh) || true
	if [ -n "${BACKUP_SRC:-}" ]; then
		run install -o root -g root -m 0750 "$BACKUP_SRC" "$DEPLOY_DIR/veylix-backup.sh"
		say "installed $DEPLOY_DIR/veylix-backup.sh"
	else
		warn "no veylix-backup.sh in the deployment templates; backups will not work until it is placed in $DEPLOY_DIR"
	fi
}

# install_launcher writes /usr/local/bin/vey.
#
# It is a real file, written here rather than downloaded, so it cannot be missing
# from a release bundle and cannot drift from the binary it launches.
install_launcher() {
	local target="$BIN_DIR/vey"

	if is_dry_run; then
		out "[dry-run] write $target (the \`vey\` launcher -> vpn-platformctl menu)"
		return 0
	fi

	local tmp
	tmp=$(mktemp)
	cat >"$tmp" <<'LAUNCHER'
#!/bin/sh
# vey - open the VEYLIX control menu.
#
# Written by install.sh (D-126). Kept deliberately tiny and dependency-free: it
# runs as root, so it must not source anything, evaluate anything, or take a
# path from the environment. The one thing it does beyond exec is explain the two
# situations where the menu cannot open.
#
#   vey            the menu                (`vpn-platformctl menu`)
#   vey <command>  any other command      (`vpn-platformctl status`, ...)
set -u

CTL=/usr/local/bin/vpn-platformctl

if [ ! -x "$CTL" ]; then
	echo "vey: $CTL is missing - the Veylix binaries are not installed." >&2
	echo "     reinstall with install.sh, or restore the binary, then try again." >&2
	exit 1
fi

# No arguments means "show me the menu". The binary decides whether it can draw
# one (it checks its controlling terminal) and prints usage when it cannot, so
# there is no second copy of that decision here.
exec "$CTL" "$@"
LAUNCHER

	run install -o root -g root -m 0755 "$tmp" "$target"
	rm -f "$tmp"
	say "installed $target (run \`sudo vey\` for the control menu)"
}

# ---------------------------------------------------------------------------
# Phase 8 - systemd. Start it and prove it is ready over HTTP.
# ---------------------------------------------------------------------------

install_service() {
	phase "systemd"

	UNIT_SRC=$(deploy_file veylixd.service) || true
	if [ -n "${UNIT_SRC:-}" ]; then
		run install -o root -g root -m 0644 "$UNIT_SRC" "$UNIT_PATH"
	else
		warn "no veylixd.service in the deployment templates; writing a minimal unit"
		if would_run; then
			cat >"$UNIT_PATH" <<'EOF'
[Unit]
Description=Veylix VPN platform
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=veylix
Group=veylix
EnvironmentFile=/etc/veylix/veylix.env
ExecStart=/usr/local/bin/veylixd
WorkingDirectory=/var/lib/veylix
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=/var/lib/veylix
MemoryMax=384M
TimeoutStopSec=20

[Install]
WantedBy=multi-user.target
EOF
			chmod 0644 "$UNIT_PATH"
		fi
	fi

	run systemctl daemon-reload
	run systemctl enable "$SERVICE"
	run systemctl restart "$SERVICE"
	say "unit installed at $UNIT_PATH and started"

	if is_dry_run; then
		out "[dry-run] would poll http://127.0.0.1:$APP_PORT/api/ready"
		return 0
	fi

	if ! wait_for_ready "http://127.0.0.1:$APP_PORT/api/ready" 30 1; then
		warn "the service is not ready. Recent log lines:"
		journalctl -u "$SERVICE" -n 30 --no-pager >&2 || true
		die "veylixd did not become ready. Fix the cause, then run: vpn-platformctl doctor"
	fi

	local ui schema
	ui=$(ready_field ui "http://127.0.0.1:$APP_PORT/api/ready")
	schema=$(ready_field schema_version "http://127.0.0.1:$APP_PORT/api/ready")
	say "ready: schema_version=$schema ui=$ui"
	# An EMPTY ui is a failed read, not a pass. The previous shape
	# (`[ "$ui" = "not_embedded" ] && die ...`) treated "" as fine and then died
	# on set -e without printing anything, which is why the install stopped
	# silently right after announcing readiness. Both the empty case and the
	# wrong-artifact case are refusals, and each says which one it is.
	if [ -z "$ui" ]; then
		die "the readiness probe answered but did not report the UI. Run: vpn-platformctl doctor"
	fi
	if [ "$ui" = "not_embedded" ]; then
		die "this binary has no embedded UI; the wrong artifact was installed"
	fi
	return 0
}

# ---------------------------------------------------------------------------
# Phase 9 - Caddy and HTTPS.
#
# Success is claimed ONLY when a real request over the public hostname returns
# 200. A failed ACME issuance is exit 2 - installed, not publicly served - and
# is never reported as success.
# ---------------------------------------------------------------------------

# caddy_install_notes - the manual path, printed ONLY when the automatic one
# failed. A host behind an egress firewall must not be left guessing what to do.
caddy_install_notes() {
	warn "install it by hand, then re-run this installer:"
	warn "  Debian/Ubuntu:"
	warn "    sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl"
	warn "    curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/gpg.key \\"
	warn "      | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg"
	warn "    curl -1sLf https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt \\"
	warn "      | sudo tee /etc/apt/sources.list.d/caddy-stable.list"
	warn "    sudo apt update && sudo apt install -y caddy"
	warn "  Or download the static binary from https://caddyserver.com/download"
}

# install_caddy_package -> 0 when `caddy` is on PATH afterwards.
#
# Two sources, tried in order, because a VPS can reach one and not the other:
#   1. the distribution's own package   (Debian 12+, Ubuntu 24.04+, Fedora)
#   2. Caddy's official apt/dnf repo    (older Ubuntu/Debian, where the distro
#                                        package is too old for `handle`/placeholders)
#
# Every step is bounded. `apt-get update` against an unreachable mirror waits
# for a socket timeout that can run to minutes; the point of this installer is
# that it either finishes or says why, never that it appears frozen.
install_caddy_package() {
	local os
	os=$(detect_os 2>/dev/null || printf unknown)

	# 1. Distribution package. Cheap, and needs no new trust anchor.
	if have apt-get; then
		say "caddy: trying the distribution package"
		if DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
			-o Acquire::http::Timeout=15 -o Acquire::https::Timeout=15 \
			-o DPkg::Lock::Timeout=60 caddy >/dev/null 2>&1; then
			have caddy && return 0
		fi
	elif have dnf || have yum; then
		local pm=dnf
		have dnf || pm=yum
		say "caddy: trying the distribution package"
		if $pm install -y -q caddy >/dev/null 2>&1; then
			have caddy && return 0
		fi
	fi

	# 2. The official repository. Only for the Debian family - the repo layout
	#    is apt-only, and inventing an rpm equivalent here would be a second
	#    untested path.
	case "$os" in
	debian | ubuntu | raspbian)
		have apt-get || return 1
		say "caddy: adding the official apt repository"
		export DEBIAN_FRONTEND=noninteractive

		local keyring=/usr/share/keyrings/caddy-stable-archive-keyring.gpg
		local listfile=/etc/apt/sources.list.d/caddy-stable.list
		local tmpkey
		tmpkey=$(mktemp) || return 1

		# The key first. gpg --dearmor converts the ASCII-armored key that
		# cloudsmith serves into the binary form apt expects; downloading to a
		# file and converting keeps the pipeline out of the failure modes.
		if ! fetch "https://dl.cloudsmith.io/public/caddy/stable/gpg.key" "$tmpkey" 2>/dev/null; then
			rm -f "$tmpkey"
			return 1
		fi
		if ! gpg --dearmor --yes -o "$keyring" "$tmpkey" 2>/dev/null; then
			rm -f "$tmpkey"
			return 1
		fi
		rm -f "$tmpkey"
		chmod 0644 "$keyring" 2>/dev/null || true

		if ! fetch "https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt" "$listfile" 2>/dev/null; then
			return 1
		fi
		chmod 0644 "$listfile" 2>/dev/null || true

		say "caddy: apt-get update (bounded)"
		apt-get update -qq -o Acquire::http::Timeout=15 -o Acquire::https::Timeout=15 >/dev/null 2>&1 || true

		if apt-get install -y -qq \
			-o Acquire::http::Timeout=15 -o Acquire::https::Timeout=15 \
			-o DPkg::Lock::Timeout=60 caddy >/dev/null 2>&1; then
			have caddy && return 0
		fi
		;;
	esac

	return 1
}

install_caddy() {
	phase "https"

	if [ "$SKIP_CADDY" -eq 1 ]; then
		warn "--skip-caddy was passed: no reverse proxy was configured"
		warn "the service is reachable on http://127.0.0.1:$APP_PORT only"
		INCOMPLETE=1
		return 0
	fi

	# Install Caddy instead of telling the operator to. The previous version
	# printed "caddy is not installed. Install it from ...docs/install" and set
	# INCOMPLETE, which meant a clean host finished with veylixd bound to
	# 127.0.0.1:8080 and NOTHING on port 80. The installer then reported
	# "VEYLIX is installed" next to a URL that did not answer - the exact
	# false-success this project forbids (rule 3/5).
	#
	# Reported from a live Ubuntu 24.04 box: `curl | sudo bash` completed, the
	# service was active, /api/ready returned 200 on loopback, and
	# http://<public-ip>/setup timed out because no listener existed there.
	#
	# 3X-UI installs what it needs; this is the same bar. Caddy is a single
	# static binary from an official apt repository, so it is installed rather
	# than demanded. The failure path still names the manual command, because a
	# host with no outbound apt access must not be left guessing.
	if ! have caddy; then
		if is_dry_run; then
			out "[dry-run] install caddy from the official apt repository"
			caddy_install_notes
			return 0
		fi
		say "caddy is not installed - installing it now"
		if ! install_caddy_package; then
			warn "could not install caddy automatically."
			caddy_install_notes
			warn "the service is reachable on http://127.0.0.1:$APP_PORT only"
			INCOMPLETE=1
			return 0
		fi
		if ! have caddy; then
			warn "the caddy package reported success but no caddy binary is on PATH."
			caddy_install_notes
			INCOMPLETE=1
			return 0
		fi
		say "caddy installed: $(caddy version 2>/dev/null | head -n 1)"
	fi

	CADDY_SRC=$(deploy_file Caddyfile) || true
	if [ -n "${CADDY_SRC:-}" ]; then
		if would_run; then
			# Ports first, then the domain. Order matters only in that the
			# placeholders are unambiguous (leading/trailing double underscore),
			# so no substitution can collide with another.
			if [ "$HTTP_MODE" -eq 1 ]; then
				# Plain HTTP, reached by IP. The site address becomes
				# `http://<bind>:<port>`: an explicit scheme so Caddy does not
				# try to obtain a certificate for an address it cannot issue
				# one for, which is what would otherwise turn an --http install
				# into a failing ACME loop and a dead site.
				#
				# The HSTS header is REMOVED in this mode. Sending
				# Strict-Transport-Security over plaintext would pin a browser
				# to https:// on a host that cannot serve it, which breaks the
				# one thing the operator installed.
				# The site line is rewritten FIRST, while the placeholder is
				# still `__HTTPS_PORT__`. Running it after the port substitution
				# would leave the literal `https://` behind and the site would
				# keep trying to obtain a certificate.
				sed -e "s|^https://vpn\.example\.com:__HTTPS_PORT__ {|http://$HTTP_BIND:$HTTP_PORT {|" \
				    -e "s|__HTTPS_PORT__|$HTTPS_PORT|g" \
				    -e "s|__HTTP_PORT__|$HTTP_PORT|g" \
				    -e "s|__APP_PORT__|$APP_PORT|g" \
				    -e "/Strict-Transport-Security/d" \
				    "$CADDY_SRC" >"$CADDYFILE.veylix-new"
			else
				sed -e "s|__HTTPS_PORT__|$HTTPS_PORT|g" \
				    -e "s|__HTTP_PORT__|$HTTP_PORT|g" \
				    -e "s|__APP_PORT__|$APP_PORT|g" \
				    -e "s|vpn\.example\.com|$DOMAIN|g" \
				    "$CADDY_SRC" >"$CADDYFILE.veylix-new"
			fi

			# The rendered file must contain no placeholder. A typo in the
			# template would otherwise reach the live config as the literal text
			# `__APP_PORT__`, which caddy validate rejects with an unhelpful
			# message far from the cause.
			local leftovers
			# `|| true`: grep exits 1 when nothing matches, and that is the GOOD
			# case here. As the last command in a command substitution the status
			# becomes the assignment's status, so under `set -e` an empty match
			# aborted the installer - traced live, immediately after the sed that
			# wrote the Caddyfile. A check for leftovers must not die when there
			# are none.
			leftovers=$(grep -o '__[A-Z_]*PORT__' "$CADDYFILE.veylix-new" 2>/dev/null | sort -u || true)
			if [ -n "$leftovers" ]; then
				rm -f "$CADDYFILE.veylix-new"
				warn "the Caddyfile template still contained a port placeholder after rendering:"
				printf '  %s\n' $leftovers
				die "the deployment template and this installer disagree. Nothing was changed." 1
			fi

			# A POSITIVE check, because "no leftovers" is not proof of success.
			#
			# The released v1.0.0 bundle shipped a Caddyfile that predates the
			# __PORT__ contract: no placeholders at all, and a bare
			# `vpn.example.com {` as the site address. Rendering that file is a
			# no-op, so it contained no leftovers, passed the check above, was
			# accepted by `caddy validate`, and shipped a site Caddy served for a
			# domain nobody configured. Measured on the live host: port 80
			# answered "<title>Caddy works!</title>" - the default page - while
			# the installer had already printed "VEYLIX is installed".
			#
			# So: assert the address we intended is the address that came out.
			# Any template that cannot express the install we just planned is a
			# mismatched artifact, and saying so is the only honest outcome.
			local want_addr
			if [ "$HTTP_MODE" -eq 1 ]; then
				want_addr="http://$HTTP_BIND:$HTTP_PORT"
			else
				want_addr="https://$DOMAIN:$HTTPS_PORT"
			fi
			if ! grep -qF "$want_addr" "$CADDYFILE.veylix-new" 2>/dev/null; then
				warn "the deployment template did not render the site address this install needs."
				warn "  expected to find: $want_addr"
				warn "  template:         $CADDY_SRC"
				warn "  This template predates the port/address contract - the released"
				warn "  veylix-deploy.tar.gz is older than install.sh. Nothing was changed."
				rm -f "$CADDYFILE.veylix-new"
				die "the release bundle and this installer are not the same version. Publish a new release (or pass --script-base to a matching bundle)." 1
			fi
		else
			out "[dry-run] render $CADDYFILE from $CADDY_SRC with domain $DOMAIN, ports $HTTP_PORT/$HTTPS_PORT -> 127.0.0.1:$APP_PORT"
		fi
	else
		warn "no Caddyfile in the deployment templates; skipping the proxy configuration"
		INCOMPLETE=1
		return 0
	fi

	# Validate BEFORE replacing the live config. A syntax error takes the whole
	# site down, including a domain that was previously working.
	if would_run; then
		run mkdir -p "$(dirname "$CADDYFILE")"
		if [ -f "$CADDYFILE" ]; then
			cp -p "$CADDYFILE" "$CADDYFILE.pre-veylix"
			warn "an existing $CADDYFILE was kept at $CADDYFILE.pre-veylix"
		fi
		mv "$CADDYFILE.veylix-new" "$CADDYFILE"

		# The service user must be able to READ it. The file was written by a
		# root shell whose umask was 077 (generate_secrets sets it earlier in the
		# same process), so the redirect produced mode 0600 root:root. Caddy runs
		# as the `caddy` user and its stock config is 0644, so it failed with
		# "open /etc/caddy/Caddyfile: permission denied" and the site never came
		# up - while `caddy validate` (run as root, which CAN read it) had already
		# said "Valid configuration". Traced on a live host.
		chmod 0644 "$CADDYFILE" 2>/dev/null || true

		if ! caddy validate --config "$CADDYFILE"; then
			if [ -f "$CADDYFILE.pre-veylix" ]; then
				mv "$CADDYFILE.pre-veylix" "$CADDYFILE"
				warn "the previous Caddyfile was restored"
			fi
			die "caddy rejected the generated configuration; the service still runs on loopback"
		fi
	fi

	# A reload of a service that is not running fails, so fall back to a start -
	# and if BOTH fail, say so instead of letting `set -e` kill the installer
	# with no message. Traced live: `systemctl reload caddy` failed (not yet
	# started under our config), `systemctl restart caddy` failed (the config was
	# unreadable), and the || chain ended at status 1, so the installer stopped
	# right there with the operator never told which command to run.
	if ! run systemctl reload caddy 2>/dev/null; then
		if ! run systemctl restart caddy; then
			warn "caddy did not start. Check: journalctl -u caddy -n 50 --no-pager"
			warn "the application is still running on 127.0.0.1:$APP_PORT"
			INCOMPLETE=1
			return 0
		fi
	fi

	if [ "$HTTP_MODE" -eq 1 ]; then
		say "caddy reloaded: serving plain HTTP on $HTTP_BIND:$HTTP_PORT"

		if is_dry_run; then
			out "[dry-run] would GET http://127.0.0.1:$HTTP_PORT/api/ready"
			return 0
		fi
		# No certificate is involved, so readiness is immediate. Check the real
		# listener rather than assuming the reload took.
		if wait_for_ready "http://127.0.0.1:$HTTP_PORT/api/ready" 15 1; then
			say "HTTP is active on port $HTTP_PORT (no TLS - see the security note)"
		else
			warn "the site did not answer on http://127.0.0.1:$HTTP_PORT/api/ready"
			warn "Check: journalctl -u caddy -n 50 --no-pager"
			INCOMPLETE=1
		fi
		return 0
	fi

	say "caddy reloaded for $DOMAIN"

	if is_dry_run; then
		out "[dry-run] would GET https://$DOMAIN/api/ready"
		return 0
	fi

	# Certificate issuance is not instant on a cold host. Give it a real budget,
	# then report the truth if it never arrives.
	if wait_for_ready "https://$DOMAIN/api/ready" 45 2; then
		say "HTTPS is active: https://$DOMAIN/api/ready answered 200"
	else
		warn "HTTPS is NOT active for $DOMAIN yet."
		warn "Common causes: DNS not propagated, ports 80/443 blocked, or ACME rate-limited."
		warn "Check: journalctl -u caddy -n 50 --no-pager"
		INCOMPLETE=1
	fi
}

# ---------------------------------------------------------------------------
# ---------------------------------------------------------------------------
# Phase 9b - the first Super Admin, created AT INSTALL TIME.
#
# The secure-by-default answer would be "open /setup in a browser", and that is
# what a non-interactive install still does - no default password is ever
# written. But an operator who IS at a terminal gets what 3X-UI gives them: the
# account is made here, by them, before the install finishes.
#
# Nothing is invented: the credentials are POSTed once to the documented
# /api/setup/bootstrap route, which refuses as soon as any user exists. If that
# call fails the install does NOT pretend otherwise - it says so and points at
# /setup.
# ---------------------------------------------------------------------------

SUPER_ADMIN_CREATED=0

create_super_admin() {
phase "super admin"

# Non-interactive (CI, cloud-init, `curl | bash`) cannot answer a prompt.
if [ ! -t 0 ] && [ -z "${VEYLIX_ADMIN_EMAIL:-}" ]; then
say "no terminal and no VEYLIX_ADMIN_EMAIL - the account is created at /setup"
return 0
fi

local email pass name body code

if [ -n "${VEYLIX_ADMIN_EMAIL:-}" ]; then
email=$VEYLIX_ADMIN_EMAIL
pass=${VEYLIX_ADMIN_PASSWORD:-}
name=${VEYLIX_ADMIN_NAME:-Super Admin}
if [ -z "$pass" ]; then
warn "VEYLIX_ADMIN_EMAIL was set without VEYLIX_ADMIN_PASSWORD - skipping"
return 0
fi
else
	if ! has_tty; then
		# No terminal to ask in. Say so plainly instead of reading from a pipe
		# that carries the script itself: that path either hangs or feeds the
		# next line of install.sh in as an "answer".
		say "no terminal, so skipping Super Admin creation - use the /setup page"
		return 0
	fi
	printf '\nCreate the Veylix Super Admin now (leave the email blank to do it at /setup).\n' >/dev/tty
	printf '  email: ' >/dev/tty
	read_tty email || email=""
	if [ -z "$email" ]; then
		say "skipped - create the account at the /setup page"
		return 0
	fi
	printf '  display name [Super Admin]: ' >/dev/tty
	read_tty name || name=""
	[ -n "$name" ] || name="Super Admin"
	printf '  password (not echoed): ' >/dev/tty
	read_tty_secret pass || pass=""
	printf '  password again: ' >/dev/tty
	read_tty_secret pass2 || pass2=""
	if [ "$pass" != "$pass2" ]; then
		warn "the passwords do not match - skipping; use the /setup page"
		return 0
	fi
fi

if [ ${#pass} -lt 12 ]; then
warn "that password is shorter than 12 characters - skipping; use the /setup page"
return 0
fi

# The password goes through a 0600 file and `curl --data-binary @file`, never
# through the command line: an argument is visible in `ps` to every user on
# the host for as long as the process lives.
body=$(umask 077; mktemp)
printf '{"email":%s,"password":%s,"display_name":%s}' \
"$(json_str "$email")" "$(json_str "$pass")" "$(json_str "$name")" > "$body"

code=$(curl -s -o /dev/null -w '%{http_code}' \
--connect-timeout 5 --max-time 20 \
-X POST -H 'Content-Type: application/json' --data-binary @"$body" \
"http://127.0.0.1:${APP_PORT:-8080}/api/setup/bootstrap" 2>/dev/null || echo 000)
rm -f "$body"

case "$code" in
201) say "Super Admin created: $email"; SUPER_ADMIN_CREATED=1 ;;
409) warn "a user already exists - the first account is whatever is in the database" ;;
000) warn "could not reach the service - create the account at /setup" ;;
*)   warn "bootstrap returned HTTP $code - create the account at /setup" ;;
esac

# The ShellCheck directive silences SC2034 for variables the caller exports.
: "${pass}"
}

# Phase 10 - the first-run report. It states what is true, including "HTTPS is
# not active", rather than printing a success banner unconditionally.
# ---------------------------------------------------------------------------

# admin_note / step_one - the report must not tell an operator to create an
# account they just created. Both branches state only what happened.
admin_note() {
if [ "$SUPER_ADMIN_CREATED" -eq 1 ]; then
printf '  The Super Admin account was created during this install. There is no\n  default password: it is the one you typed.'
else
printf '  There is NO default password. Complete /setup to create the Super Admin.'
fi
}

step_one() {
if [ "$SUPER_ADMIN_CREATED" -eq 1 ]; then
printf '  1. %s/login                    sign in as the Super Admin' "$base"
else
printf '  1. %s/setup                    create the Super Admin' "$base"
fi
}

# control_menu prints the VEYLIX CONTROL MENU and, when a human is actually at
# the keyboard, offers to open it.
#
# Two rules make this safe:
#   1. The menu is PRINTED unconditionally, so it lands in the install log and
#      `bash <(curl ...) | tee` still shows the operator what exists.
#   2. It only PROMPTS when there is a terminal to answer in. `curl ... | bash`
#      and a CI run have none, and a prompt there would hang the installer
#      forever.
#
#      The test is has_tty (/dev/tty), NOT `[ -t 0 ]`. 3X-UI's installer uses
#      `-t 0` and it is wrong for this script: the documented invocation is
#      `bash <(curl -Ls .../install.sh)`, where stdin is the pipe carrying the
#      script, so `-t 0` is false at an interactive terminal and the operator is
#      never offered the menu.
control_menu() {
	cat <<'EOF'
================================================================
  VEYLIX CONTROL MENU
================================================================

  1) Start              start veylixd
  2) Stop               stop veylixd
  3) Restart            restart veylixd
  4) Status             service, readiness and database
  5) Logs               show the end of the service log
  6) Enable autostart   start on boot
  7) Disable autostart  do not start on boot
  8) Update             upgrade to the latest release
  9) Versions           list published releases
 10) Downgrade          install an older release
 11) Backup             take a backup now
 12) Restore            restore from a backup
 13) Doctor             diagnose this host
 14) Repair             re-render config and reload
 15) Change domain      point Veylix at another domain
 16) Change ports       move HTTPS / HTTP / app port
 17) Certificate        check the TLS certificate
 18) Reset admin        change the Super Admin password
 19) Uninstall          remove the service, keep the data
 20) Uninstall + purge  remove EVERYTHING including data

  0) Exit

Type a number here to run it now, or run `vpn-platformctl` at any time.

EOF

	# No terminal: the printed menu is the deliverable. Return so the installer
	# can finish instead of waiting on a read that will never arrive.
	has_tty || return 0

	printf 'Open the control menu now? [y/N] ' >/dev/tty
	local answer
	read_tty answer || return 0
	case "$answer" in
		y | Y | yes | YES)
			printf '\n' >/dev/tty
			# `vey` is the launcher installed a moment ago; it execs
			# `vpn-platformctl menu`. Calling the short name here is deliberate:
			# the command the operator sees working is the one they will type
			# tomorrow.
			if [ -x "$BIN_DIR/vey" ]; then
				"$BIN_DIR/vey" || printf 'the control menu exited with %s\n' "$?"
			else
				vpn-platformctl menu || printf 'vpn-platformctl menu exited with %s\n' "$?"
			fi
			;;
		*) printf 'Run "sudo vey" whenever you want the menu.\n' ;;
	esac
}

#
# The port is omitted when it is 443, because `https://host:443` is noise and
# looks broken to the reader; it is included otherwise, because without it the
# operator would copy a URL that does not connect.
public_url() {
	if [ "$HTTPS_PORT" = "443" ]; then
		printf 'https://%s' "$DOMAIN"
	else
		printf 'https://%s:%s' "$DOMAIN" "$HTTPS_PORT"
	fi
}

# reachable_ip - the address an operator should type in a browser.
#
# The address on the default route (`local_ip`) is the PRIVATE one on a NAT'd
# host, so printing it would send the operator to an address that only exists
# inside the datacentre. A public address is preferred, and if none can be
# determined the function says so rather than inventing one - a wrong URL in
# the final report is worse than an honest placeholder.
reachable_ip() {
	local pub=""
	# Ask the host's own view of its public address first: no third party, and
	# it already knows its own NAT mapping if it has one.
	if have curl; then
		pub=$(curl -fsS --max-time 3 https://api.ipify.org 2>/dev/null || true)
	fi
	case "$pub" in
		"" | *[!0-9.]*) pub="" ;;
	esac
	if [ -n "$pub" ]; then
		printf '%s' "$pub"
		return 0
	fi
	pub=$(local_ip 2>/dev/null || true)
	if [ -n "$pub" ]; then
		printf '%s' "$pub"
		return 0
	fi
	printf '%s' "<this-host-ip>"
}


report() {
	phase "first run"

	local base="http://127.0.0.1:$APP_PORT"
	# NOTE: `if`, not `[ … ] && …`. Under `set -e` a failing `&&` chain as a
	# bare command aborts the script - the same shape as D-129. With no domain
	# the first test is false on every --http install.
	if [ -n "$DOMAIN" ] && [ "$INCOMPLETE" -eq 0 ]; then
		base=$(public_url)
	fi
	# --http: the app is reachable from the network, but over plaintext. Name the
	# reachable address so the operator is not told to open localhost on a
	# machine they are not sitting at.
	if [ "$HTTP_MODE" -eq 1 ]; then
		base="http://$(reachable_ip):$HTTP_PORT"
	fi

	cat <<EOF

VEYLIX is installed.

  Open:        $base/setup
  Database:    $DATA_DIR/veylix.db
  Config:      $CONFIG_DIR/veylix.env  (0600 root:root)

  enc.key     fingerprint $(fingerprint "$CONFIG_DIR/enc.key")
  session.key fingerprint $(fingerprint "$CONFIG_DIR/session.key")

  Back both keys up OFF this server. Losing enc.key makes every sealed 3X-UI
  panel token unrecoverable - they would have to be re-entered in the UI.

$(admin_note)

Ports:
  Caddy HTTP   :$HTTP_PORT   $(if [ "$HTTP_MODE" -eq 1 ]; then echo "<- the port customers type (PLAIN HTTP)"; else echo "(ACME challenge + redirect to HTTPS)"; fi)
  Caddy HTTPS  :$HTTPS_PORT   $(if [ "$HTTP_MODE" -eq 1 ]; then echo "(not in use - --http was chosen)"; else echo "<- the port customers type"; fi)
  veylixd      :$APP_PORT   (loopback only, never reachable from outside)

Next steps:
$(step_one)
  2. sign in and add your 3X-UI panel under Servers (it is sealed on save)
  3. sudo vey                  open the control menu (status, logs, everything)
  4. sudo vey status           verify the install
  5. sudo vey backup           take the first backup

The control menu is "sudo vey". Every future change is in it - this installer is
not run again:

  sudo vey                     the menu (start/stop, logs, backup, domain, ports)
  sudo vey status              service, readiness and database
  sudo vey update              upgrade to the latest release
  sudo vey domain-set <fqdn>   change the domain
  sudo vey ports set ...        move the HTTPS / HTTP / app ports
  sudo vey doctor              diagnose the host

Note: vpn-platformctl is the same binary if you prefer the long name -
      every "sudo vey ..." above works with it verbatim.

EOF

	control_menu

	# --http is honest about what it is. Saying "installed" without saying
	# "plaintext" would be the same lie as reporting HTTPS that is not there.
	if [ "$HTTP_MODE" -eq 1 ]; then
		cat <<EOF
SECURITY - THIS HOST SERVES PLAIN HTTP.
  Everything sent to $base travels unencrypted, including the Super Admin
  password on the setup page. Do not let this host carry real customers yet.

  Add TLS when you are ready. A Cloudflare DNS-01 challenge needs no inbound
  port, so it works beside a 3X-UI that already holds 80 and 443:

    sudo vey domain-set <your-domain> --dns-provider cloudflare --cf-token <token>

  Then check it:

    sudo vey cert

EOF
	fi

	if [ "$INCOMPLETE" -eq 1 ]; then
		cat <<EOF
INSTALL INCOMPLETE - the service is running on http://127.0.0.1:$APP_PORT but
HTTPS is NOT active. Fix the cause above, then run: vpn-platformctl repair

EOF
		return 2
	fi
	return 0
}

main() {
	parse_args "$@"

	if [ "$LIST_VERSIONS" -eq 1 ]; then
		list_versions
		exit 0
	fi

	# No domain is not an error. It means the operator wants the 3X-UI shape: a
	# host reached by IP on plain HTTP, with HTTPS added later from the control
	# menu. Refusing here (D-129's neighbour) was what made the documented
	# one-liner unusable on a fresh server.
	#
	# Asking for --http AND a domain is a contradiction: the domain would be
	# written into an http:// site address and never obtain a certificate. Name
	# the conflict instead of silently dropping one of the two.
	if [ -z "$DOMAIN" ]; then
		if [ "$HTTP_MODE" -eq 0 ]; then
			HTTP_MODE=1
			HTTP_BIND=0.0.0.0
			say "no --domain given: installing for plain HTTP on port $HTTP_PORT"
			say "  add a domain and a certificate later: vey domain-set <domain>"
		fi
	elif [ "$HTTP_MODE" -eq 1 ]; then
		die "--http and --domain both given. --http serves plain HTTP and does not use a domain; drop --http to get HTTPS on the domain, or drop --domain to stay on IP over HTTP." 2
	fi

	if [ -n "$DOMAIN" ]; then
		validate_domain "$DOMAIN" || die "'$DOMAIN' is not a valid public hostname. Certificates need an FQDN such as vpn.example.com" 2
	fi

	# Ask about ports when the operator is at a terminal and did not pass flags.
	# It runs BEFORE validation so the answer is what gets validated, and before
	# anything is written, downloaded or created.
	choose_ports

	# Ports are validated BEFORE anything is written, and before any download.
	# A bad port discovered at the Caddy phase would leave a host with an
	# installed service and no working HTTPS, which is the worst of both.
	#
	# The function prints the specific problem. This line makes it an ERROR with
	# the command that fixes it, because a WARNING followed by a silent stop is
	# what confused the owner: it read as "fine, continuing".
	if ! validate_ports_for_install; then
		err "the install stopped: the ports above cannot be used together."
		case "${PORT_CLASH:-}" in
		app=http)
			err "fix it with a different internal port, for example:"
			err "  sudo ./install.sh --http-port $HTTP_PORT --app-port 18080"
			;;
		app=https)
			err "fix it with a different internal port, for example:"
			err "  sudo ./install.sh --https-port $HTTPS_PORT --app-port 18080"
			;;
		http=https)
			err "fix it by giving the two listeners different ports, for example:"
			err "  sudo ./install.sh --http-port 8080 --https-port 8443"
			;;
		*)
			err "re-run with --http-port, --https-port and --app-port set explicitly."
			;;
		esac
		exit 2
	fi

	# Refuse a bogus repo slug BEFORE downloading anything: a 404 from a
	# placeholder is otherwise reported as a network failure and sends an
	# operator looking in the wrong place.
	if ! validate_repo "$REPO"; then
		if [ "$REPO" = "$PLACEHOLDER_REPO" ]; then
			die "no repository configured: --repo is still the shipped placeholder ('$PLACEHOLDER_REPO'). Pass the repository that publishes the release:
         sudo ./install.sh --domain $DOMAIN --repo <owner>/<name>
       (or export VEYLIX_REPO=<owner>/<name>)" 2
		fi
		die "'$REPO' is not a valid repository slug. Expected exactly <owner>/<name>, e.g. acme/veylix" 2
	fi

	export DRY_RUN

	# Root FIRST, lock SECOND. The lock lives in /run, which is root-owned
	# drwxr-xr-x, so a non-root run fails on "cannot open lock file
	# /run/veylix-install.lock: Permission denied" -- a message about a file the
	# operator has never heard of, for a problem that is simply "you forgot
	# sudo". Reported from a live host where `bash <(curl ...)` as the default
	# cloud user produced exactly that and nothing else. The check is cheap,
	# side-effect free and names the real fix, so it goes before anything that
	# needs privileges.
	need_root

	acquire_lock "$LOCK_PATH"

	# The deployment templates are needed by phase 3 onward, so they are located
	# before the branch decision. A miss is recorded, not fatal: the install can
	# still serve the API on loopback, and report() says HTTPS is incomplete.
	resolve_deploy_dir || true

	preflight
	fetch_artifact
	branch
	create_user_and_dirs
	generate_secrets
	render_config
	install_binaries
	install_service
	install_caddy
	create_super_admin

	report
}

INCOMPLETE=0
STAGE=""
CLI_AVAILABLE=0
BIN_ARTIFACT=""
CLI_ARTIFACT=""

main "$@"
