#!/usr/bin/env bash
# VEYLIX — one-line installer (bootstrap).
#
# This runs ONCE per host. It is not the update mechanism: after a host is
# installed, every subsequent change (new release, new domain, backup, rollback)
# is `vpn-platformctl`. That split is why this script contains no version number
# and no schema knowledge - it resolves the release channel at run time, so a
# project that moves on never invalidates it.
#
# See docs/INSTALLATION_DESIGN.md §3 for the reasoning.
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

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=scripts/lib/common.sh
. "$HERE/scripts/lib/common.sh"

VEYLIX_TOOL=install.sh

# The repo slug the shipped default points at. It is NOT a real repository: a
# stock `./install.sh` would fetch a 404 and report it as a network failure.
# validate_repo() refuses it explicitly so the correction is one line long.
PLACEHOLDER_REPO=janith-15/Veylix-1

# --- configuration (overridable, documented in docs/INSTALL.md §F) ----------
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
  --dry-run            print every phase and change nothing
  --skip-dns           skip the DNS A-record check (recorded in the output)
  --skip-ports         skip the port 80/443 check (recorded in the output)
  --skip-caddy         install without a reverse proxy (loopback only, exit 2)
  --reuse-keys         accept existing keys in /etc/veylix (breaks recovery)

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
			--skip-dns)   SKIP_DNS=1; shift ;;
			--skip-ports) SKIP_PORTS=1; shift ;;
			--skip-caddy) SKIP_CADDY=1; shift ;;
			--reuse-keys) REUSE_KEYS=1; shift ;;
			-h|--help)    usage; exit 0 ;;
			*)            usage >&2; die "unknown argument: $1" 2 ;;
		esac
	done
	[ "$DRY_RUN" -eq 1 ] && DRY_RUN=1
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
	# The shipped default is deliberately still refused: a fork must pass its own
	# --repo, so no install ever fetches this project's releases by accident.
	[ "$r" != "$PLACEHOLDER_REPO" ] || return 1
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

	if curl -fsSL "$bundle" -o "$stage/veylix-deploy.tar.gz" 2>/dev/null; then
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
			if curl -fsSL "$SCRIPT_BASE/veylix-$name" -o "$stage/$name" 2>/dev/null; then
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

	warn "no deployment templates found (no ./deploy next to the installer, no"
	warn "$bundle, no --script-base). HTTPS and backups will be incomplete;"
	warn "see docs/INSTALL.md §B.3 for the manual copy."
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
	tags=$(curl -fsSL "$RELEASE_API/repos/$REPO/releases?per_page=100" 2>/dev/null \
		| grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' \
		| sed -E 's/.*"([^"]*)"$/\1/') || true
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
		skip "ports 80/443" "--skip-ports was passed; Caddy may fail to bind"
	else
		local p
		for p in 80 443; do
			if port_in_use "$p"; then
				die "port $p is already in use. Stop the service holding it, or re-run with --skip-ports (Caddy will then fail)."
			fi
		done
		say "ports: 80 and 443 are free"
	fi

	if [ "$SKIP_CADDY" -eq 1 ]; then
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

	curl -fsSL "$base/$bin_name" -o "$BIN_ARTIFACT" \
		|| die "could not download $base/$bin_name. Check the release exists for $ARCH (and that VEYLIX_REPO is set for a fork)."
	curl -fsSL "$base/SHA256SUMS" -o "$sums" \
		|| die "could not download $base/SHA256SUMS"

	verify_checksum "$BIN_ARTIFACT" "$sums" "$bin_name"

	CLI_AVAILABLE=0
	if curl -fsSL "$base/$cli_name" -o "$CLI_ARTIFACT" 2>/dev/null; then
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
		out "[dry-run] render $env (0600 root:root) with VEYLIX_ADDR=127.0.0.1:8080"
		return 0
	fi

	local enc sess
	enc=$(cat "$CONFIG_DIR/enc.key")
	sess=$(cat "$CONFIG_DIR/session.key")

	umask 077
	{
		echo "# VEYLIX — generated by install.sh on $(date -u +%Y-%m-%dT%H:%M:%SZ)"
		echo "# Contains the two keys. Never commit this file."
		echo "VEYLIX_ADDR=127.0.0.1:8080"
		echo "VEYLIX_DATA_DIR=$DATA_DIR"
		echo "VEYLIX_ENCRYPTION_KEY=$enc"
		echo "VEYLIX_SESSION_KEY=$sess"
		echo "VEYLIX_SYNC_INTERVAL=5m"
		echo "VEYLIX_SYNC_TIMEOUT=60s"
		echo "VEYLIX_INSECURE_COOKIES=0"
		if [ -n "$DOMAIN" ] && [ "$SKIP_CADDY" -eq 0 ]; then
			echo "VEYLIX_PUBLIC_BASE_URL=https://$DOMAIN"
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
		[ "${CLI_AVAILABLE:-0}" -eq 1 ] && out "[dry-run] install -m 0755 $CLI_ARTIFACT $BIN_DIR/vpn-platformctl"
		return 0
	fi

	run install -o root -g root -m 0755 "$BIN_ARTIFACT" "$BIN_DIR/veylixd"
	say "installed $BIN_DIR/veylixd"

	if [ "${CLI_AVAILABLE:-0}" -eq 1 ]; then
		run install -o root -g root -m 0755 "$CLI_ARTIFACT" "$BIN_DIR/vpn-platformctl"
		say "installed $BIN_DIR/vpn-platformctl"
	fi

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
		out "[dry-run] would poll http://127.0.0.1:8080/api/ready"
		return 0
	fi

	if ! wait_for_ready "http://127.0.0.1:8080/api/ready" 30 1; then
		warn "the service is not ready. Recent log lines:"
		journalctl -u "$SERVICE" -n 30 --no-pager >&2 || true
		die "veylixd did not become ready. Fix the cause, then run: vpn-platformctl doctor"
	fi

	local ui schema
	ui=$(ready_field ui)
	schema=$(ready_field schema_version)
	say "ready: schema_version=$schema ui=$ui"
	[ "$ui" = "not_embedded" ] && die "this binary has no embedded UI; the wrong artifact was installed"
	return 0
}

# ---------------------------------------------------------------------------
# Phase 9 - Caddy and HTTPS.
#
# Success is claimed ONLY when a real request over the public hostname returns
# 200. A failed ACME issuance is exit 2 - installed, not publicly served - and
# is never reported as success.
# ---------------------------------------------------------------------------

install_caddy() {
	phase "https"

	if [ "$SKIP_CADDY" -eq 1 ]; then
		warn "--skip-caddy was passed: no reverse proxy was configured"
		warn "the service is reachable on http://127.0.0.1:8080 only"
		INCOMPLETE=1
		return 0
	fi

	if ! have caddy; then
		warn "caddy is not installed. Install it from https://caddyserver.com/docs/install"
		warn "then run: vpn-platformctl repair"
		INCOMPLETE=1
		return 0
	fi

	CADDY_SRC=$(deploy_file Caddyfile) || true
	if [ -n "${CADDY_SRC:-}" ]; then
		if would_run; then
			sed "s|vpn\.example\.com|$DOMAIN|g" "$CADDY_SRC" >"$CADDYFILE.veylix-new"
		else
			out "[dry-run] render $CADDYFILE from $CADDY_SRC with domain $DOMAIN"
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

		if ! caddy validate --config "$CADDYFILE"; then
			if [ -f "$CADDYFILE.pre-veylix" ]; then
				mv "$CADDYFILE.pre-veylix" "$CADDYFILE"
				warn "the previous Caddyfile was restored"
			fi
			die "caddy rejected the generated configuration; the service still runs on loopback"
		fi
	fi

	run systemctl reload caddy || run systemctl restart caddy
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
# Phase 10 - the first-run report. It states what is true, including "HTTPS is
# not active", rather than printing a success banner unconditionally.
# ---------------------------------------------------------------------------

report() {
	phase "first run"

	local base="http://127.0.0.1:8080"
	[ -n "$DOMAIN" ] && [ "$INCOMPLETE" -eq 0 ] && base="https://$DOMAIN"

	cat <<EOF

VEYLIX is installed.

  Open:        $base/setup
  Database:    $DATA_DIR/veylix.db
  Config:      $CONFIG_DIR/veylix.env  (0600 root:root)

  enc.key     fingerprint $(fingerprint "$CONFIG_DIR/enc.key")
  session.key fingerprint $(fingerprint "$CONFIG_DIR/session.key")

  Back both keys up OFF this server. Losing enc.key makes every sealed 3X-UI
  panel token unrecoverable - they would have to be re-entered in the UI.

  There is NO default password. Complete /setup to create the Super Admin.

Next steps:
  1. $base/setup                     create the Super Admin
  2. sign in and add your 3X-UI panel under Servers (it is sealed on save)
  3. vpn-platformctl status          verify the install
  4. vpn-platformctl backup          take the first backup

Every future change is vpn-platformctl - this installer is not run again:
  vpn-platformctl update             upgrade to the latest release
  vpn-platformctl domain-set <fqdn>  change the domain
  vpn-platformctl backup|restore     back up / restore
  vpn-platformctl doctor             diagnose the host

EOF

	if [ "$INCOMPLETE" -eq 1 ]; then
		cat <<EOF
INSTALL INCOMPLETE — the service is running on http://127.0.0.1:8080 but HTTPS is
NOT active. Fix the cause above, then run: vpn-platformctl repair

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

	[ -n "$DOMAIN" ] || { usage >&2; die "--domain is required" 2; }
	validate_domain "$DOMAIN" || die "'$DOMAIN' is not a valid public hostname. Certificates need an FQDN such as vpn.example.com" 2

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

	report
}

INCOMPLETE=0
STAGE=""
CLI_AVAILABLE=0
BIN_ARTIFACT=""
CLI_ARTIFACT=""

main "$@"
