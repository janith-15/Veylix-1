#!/usr/bin/env bash
# t17-render.sh - render the Caddyfile using install.sh's OWN install_caddy().
#
# WHY THIS FILE EXISTS
#
# The selftest used to render the template with its own copy of the sed. A test
# that restates the decision cannot detect the decision changing, so it stayed
# green while install.sh shipped a site address of `http://0.0.0.0:80`. Caddy
# turns the host part of a site address into a Host matcher - `caddy adapt`
# showed {"match":[{"host":["0.0.0.0"]}]} - and a browser sends Host:
# 140.245.243.28, which never matches. The result was a listening site that
# answered every request with Content-Length: 0.
#
# So this harness stubs ONLY the environment (root, systemd, the caddy binary)
# and lets the real function do the render. If install.sh changes the
# substitution, this output changes with it and the assertions fail.
#
# Inputs (env): VEYLIX_T17_MODE=http|domain   VEYLIX_T17_OUT=<path prefix>
# Output: the rendered Caddyfile on stdout.
set -uo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
MODE=${VEYLIX_T17_MODE:-http}
OUT=${VEYLIX_T17_OUT:-$(mktemp -u)}

# install.sh is an executable script, not a library, so it is sourced with the
# side-effect-free tail removed: everything up to (but not including) the
# `main "$@"` dispatch. Sourcing the whole file would run an install.
SRC=$(sed '/^main "\$@"/,$d' "$ROOT/install.sh")

# shellcheck disable=SC1090,SC2086
eval "$SRC" 2>/dev/null || true

# The environment install.sh would have: no privilege checks, no systemd, and a
# `caddy` that reports success for validate without touching the real daemon.
need_root() { :; }
acquire_lock() { :; }
have_flock() { :; }
systemctl() { return 0; }
caddy() { return 0; }
have() { return 0; }
is_dry_run() { return 1; }
say() { :; }
warn() { :; }
skip() { :; }
phase() { :; }
out() { :; }

DRY_RUN=0
SKIP_CADDY=0
INCOMPLETE=0
HERE="$ROOT"
DEPLOY_SRC="$ROOT/deploy"
CADDYFILE="$OUT"
BIN_DIR=${VEYLIX_TEST_BIN_DIR:-/tmp}
HTTP_PORT=80
HTTPS_PORT=443
APP_PORT=8080
VEYLIX_CADDYFILE="$OUT"

if [ "$MODE" = http ]; then
	HTTP_MODE=1
	HTTP_BIND=0.0.0.0
	DOMAIN=""
else
	HTTP_MODE=0
	HTTP_BIND=127.0.0.1
	DOMAIN=vpn.acme.com
fi

# Render only. install_caddy continues past the render into validate + reload;
# every command it would run is stubbed above, so this is safe to call whole.
install_caddy >/dev/null 2>&1

# install_caddy writes "$CADDYFILE.veylix-new" and mvs it into place, but our
# stubbed validate/reload means the mv may or may not have happened depending on
# where it returns. Print whichever file exists - both contain the same bytes.
if [ -f "$OUT.veylix-new" ]; then
	cat "$OUT.veylix-new"
elif [ -f "$OUT" ]; then
	cat "$OUT"
fi
