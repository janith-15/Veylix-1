#!/usr/bin/env bash
# D-128 probe: every documented invocation form reaches the CLI.
#
# The lesson from D-127 was that the selftest ran install.sh AS A FILE and
# therefore never exercised the documented form. This probe runs each form
# separately and names the ones that fail, so a future change cannot silently
# break one of them again.
set -uo pipefail
cd "$(dirname "$0")" || exit 1

URL=https://raw.githubusercontent.com/janith-15/Veylix-1/main/install.sh
PASS=0
FAIL=0

ok() { PASS=$((PASS + 1)); printf 'PASS  %s\n' "$1"; }
bad() { FAIL=$((FAIL + 1)); printf 'FAIL  %s\n        %s\n' "$1" "$2"; }

# A form passes when the CLI banner appears and no shell error does.
expect_reaches_cli() {
	local name=$1 out=$2
	if printf '%s' "$out" | grep -q 'VEYLIX installer'; then
		if printf '%s' "$out" | grep -qE 'unbound variable|No such file or directory|command not found'; then
			bad "$name" "banner printed but a shell error came with it"
		else
			ok "$name"
		fi
	else
		bad "$name" "no banner: $(printf '%s' "$out" | head -1)"
	fi
}

echo "--- invocation forms ---"

# 1. as a file (local checkout)
out=$(bash install.sh --help 2>&1 | head -30)
expect_reaches_cli "file:  bash install.sh" "$out"

# 2. process substitution from a local file (the form the owner first ran)
out=$(bash <(cat install.sh) --help 2>&1 | head -30)
expect_reaches_cli "procsub: bash <(cat install.sh)" "$out"

# 3. via stdin pipe (curl ... | bash). BASH_SOURCE[0] is unset here.
out=$(cat install.sh | bash -s -- --help 2>&1 | head -30)
expect_reaches_cli "pipe:  cat install.sh | bash -s --" "$out"

# 4. ./install.sh (executable, the conventional form)
out=$(./install.sh --help 2>&1 | head -30)
expect_reaches_cli "exec:  ./install.sh" "$out"

echo "--- the remote file, same forms ---"
if out=$(curl -fsSL "$URL" 2>/dev/null) && [ -n "$out" ]; then
	o=$(printf '%s' "$out" | bash -s -- --help 2>&1 | head -30)
	expect_reaches_cli "remote pipe: curl | bash -s --" "$o"
	o=$(bash <(curl -fsSL "$URL") --help 2>&1 | head -30)
	expect_reaches_cli "remote procsub: bash <(curl ...)" "$o"
else
	echo "SKIP  remote forms (no network or file missing)"
fi

echo "--- documented-refusal check ---"
# `sudo bash <(curl ...)` cannot work: sudo does not inherit the shell's fd.
# That is a property of sudo, not of this script, so it is documented rather
# than tested. Assert no doc presents it as a command to run. T15e owns this in
# the gate; this is the standalone version.
bad_lines=$(grep -h 'sudo bash <(curl' docs/INSTALL.md docs/PUSH_GUIDE.md 2>/dev/null |
	grep -v -e '\*\*NO\*\*' -e 'Do not use' -e 'You used' -e 'runs a \*\*separate process\*\*' || true)
if [ -n "$bad_lines" ]; then
	bad "INSTALL.md does not recommend sudo with process substitution" "$bad_lines"
else
	ok "INSTALL.md does not recommend sudo with process substitution"
fi

printf '\n--- D-128 probe: %d PASS / %d FAIL ---\n' "$PASS" "$FAIL"
[ "$FAIL" -eq 0 ]
