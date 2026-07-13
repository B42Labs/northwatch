#!/usr/bin/env bash
# covcheck.sh enforces a per-package statement-coverage floor for the core
# packages, on top of the global gate CI already applies.
#
# It reads a merged Go coverage profile (produced with `go test -coverpkg=...`)
# and computes statement coverage per package. Because a merged profile repeats
# a block once per test package that exercised it, blocks are deduplicated by
# file:range and their hit counts OR-ed, so a block covered from any test
# package counts as covered. Package membership is an exact directory match, so
# internal/api and internal/api/handler are scored separately.
#
# Usage: scripts/covcheck.sh <coverage.out>
set -euo pipefail

PROFILE="${1:-coverage.out}"
THRESHOLD="${COVCHECK_THRESHOLD:-85}"

# The designated core packages. Named explicitly (rather than derived) so the
# enforced set is reviewed like any other change.
CORE_PACKAGES=(
	"github.com/b42labs/northwatch/internal/write"
	"github.com/b42labs/northwatch/internal/debug"
	"github.com/b42labs/northwatch/internal/api"
	"github.com/b42labs/northwatch/internal/api/handler"
)

if [[ ! -f "$PROFILE" ]]; then
	echo "covcheck: coverage profile not found: $PROFILE" >&2
	exit 2
fi

awk -v threshold="$THRESHOLD" -v pkgs="${CORE_PACKAGES[*]}" '
BEGIN {
	n = split(pkgs, arr, " ")
	for (i = 1; i <= n; i++) core[arr[i]] = 1
}
NR > 1 {
	key = $1
	stmts = $2
	cnt = $3
	if (!(key in seenStmts)) {
		seenStmts[key] = stmts
		maxCnt[key] = cnt
		# Package = file path with the trailing "/<file>:<range>" removed.
		file = $1
		sub(/:[0-9].*$/, "", file)
		sub("/[^/]+$", "", file)
		pkgOf[key] = file
	} else if (cnt > maxCnt[key]) {
		maxCnt[key] = cnt
	}
}
END {
	for (key in seenStmts) {
		p = pkgOf[key]
		if (!(p in core)) continue
		total[p] += seenStmts[key]
		if (maxCnt[key] > 0) covered[p] += seenStmts[key]
	}
	fail = 0
	for (i = 1; i <= n; i++) {
		p = arr[i]
		if (!(p in total) || total[p] == 0) {
			printf "  %-55s   no statements found\n", p
			printf "covcheck: %s produced no coverage data\n", p > "/dev/stderr"
			fail = 1
			continue
		}
		pct = 100 * covered[p] / total[p]
		status = (pct + 1e-9 >= threshold) ? "OK  " : "FAIL"
		printf "  [%s] %-50s %5.1f%% (%d/%d)\n", status, p, pct, covered[p], total[p]
		if (pct + 1e-9 < threshold) fail = 1
	}
	printf "covcheck: per-package threshold %d%%\n", threshold
	if (fail) {
		print "covcheck: one or more core packages are below the threshold" > "/dev/stderr"
		exit 1
	}
}
' "$PROFILE"
