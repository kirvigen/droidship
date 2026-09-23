#!/usr/bin/env bash
# Read-only checks against the live stores with real credentials. Never
# publishes, releases, rolls out, edits notes or replies.
#
#   SMOKE_PACKAGE=com.example ./scripts/smoke.sh
set -euo pipefail
pkg=${SMOKE_PACKAGE:?set SMOKE_PACKAGE to a package you own}
bin=${DROIDSHIP:-./droidship}

step() { printf '\n== droidship %s\n' "$*"; "$bin" "$@"; }

step auth
step status "$pkg"
step reviews "$pkg" --days 30 --limit 5
step listing "$pkg" --store gplay,appgallery
"$bin" auth --json | python3 -c '
import json, sys
rows = json.load(sys.stdin)
assert rows and all("identity" in r or "error" in r for r in rows), rows
'
printf '\nsmoke: ok\n'
