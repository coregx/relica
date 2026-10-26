#!/usr/bin/env bash
# audit_test.sh — tests for audit.sh. Run: bash audit_test.sh (exit 0 = pass).
#
# Each fixture under testdata/ is copied to a temporary directory (not a git
# work tree), *.go.txt files are renamed to *.go, and audit.sh runs offline on
# it. testdata/bad fires every check a known number of times; testdata/clean
# holds the correct form of each pattern and must produce no finding.

set -u
here=$(cd "$(dirname "$0")" && pwd)
fail=0

run_fixture() { # run_fixture NAME — prints audit output for testdata/NAME
	local tmp
	tmp=$(mktemp -d)
	cp -r "$here/testdata/$1/." "$tmp/"
	# Fixtures carry a .txt suffix so they are neither Go files of this module nor
	# nested modules (a go.mod would cut them out of the module zip).
	find "$tmp" \( -name '*.go.txt' -o -name 'go.mod.txt' \) | while read -r f; do mv "$f" "${f%.txt}"; done
	RELICA_AUDIT_OFFLINE=1 bash "$here/audit.sh" "$tmp"
	rm -rf "$tmp"
}

expect() { # expect OUTPUT SEVERITY ID COUNT
	local out=$1 sev=$2 id=$3 n=$4 got
	got=$(printf '%s\n' "$out" | grep -E "^\[$sev\] $id " | sed -E 's/.*\(([0-9]+)\)$/\1/')
	if [ "$got" != "$n" ]; then
		echo "FAIL: [$sev] $id: got '${got:-none}', want $n"
		fail=1
	fi
}

expect_line() { # expect_line OUTPUT TEXT
	if ! printf '%s\n' "$1" | grep -qF -- "$2"; then
		echo "FAIL: output has no line containing: $2"
		fail=1
	fi
}

bad=$(run_fixture bad)
expect_line "$bad" "go.mod: v0.19.1"
expect_line "$bad" "== Upgrading to v0.20.0"
expect "$bad" HIGH H1 1
expect "$bad" HIGH H2 1
expect "$bad" HIGH H3 1
expect "$bad" MEDIUM M1 1
expect "$bad" MEDIUM M2 1
expect "$bad" MEDIUM M4 1
expect "$bad" MEDIUM M5 1
expect "$bad" MEDIUM M6 1
expect "$bad" MEDIUM M7 1
expect "$bad" HIGH V1 3
expect "$bad" HIGH V2 2
expect "$bad" HIGH V3 1
expect "$bad" HIGH V4 2
expect "$bad" HIGH V5 2
expect "$bad" MEDIUM V6 1
expect "$bad" MEDIUM V7 1
expect "$bad" INFO V8 2
expect "$bad" INFO V9 1
expect "$bad" MEDIUM V10 1
expect "$bad" INFO I5 1
expect "$bad" INFO I6 1
expect "$bad" INFO I7 1
expect "$bad" INFO I8 1
expect "$bad" INFO I1 1
expect "$bad" INFO I2 1
expect "$bad" INFO I3 1
expect "$bad" INFO I4 1
# The chain-style V1/V4 cases (dot at the end of the previous line) are found:
expect_line "$bad" 'OrderBy("a DESC, b")'
expect_line "$bad" 'db.Update("t").'

clean=$(run_fixture clean)
expect_line "$clean" "go.mod: v0.20.0"
expect_line "$clean" "== v0.20.0 rules"
expect_line "$clean" "== Summary: HIGH=0 MEDIUM=0 INFO=0"
if printf '%s\n' "$clean" | grep -qE '^\[(HIGH|MEDIUM|INFO)\]'; then
	echo "FAIL: the clean fixture has findings:"
	printf '%s\n' "$clean" | grep -E '^\[(HIGH|MEDIUM|INFO)\]' | sed 's/^/  /'
	fail=1
fi

# A missing directory is reported, not a crash.
missing=$(RELICA_AUDIT_OFFLINE=1 bash "$here/audit.sh" "$here/testdata/no-such-dir" 2>&1)
expect_line "$missing" "audit: cannot cd to"

if [ "$fail" = 0 ]; then
	echo "PASS: audit.sh"
fi
exit "$fail"
