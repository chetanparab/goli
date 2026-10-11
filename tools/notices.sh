#!/bin/sh
# Writes THIRD_PARTY_NOTICES.md: the licence (and NOTICE, where present) of
# every module compiled into goli, the Go runtime and the bundled fonts.
# Their licences require these notices to travel with binary copies.
# Usage: tools/notices.sh > THIRD_PARTY_NOTICES.md
set -eu
cd "$(dirname "$0")/.."
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
go build -o "$tmp/goli" ./cmd/goli
modcache=$(go env GOMODCACHE)
goroot=$(go env GOROOT)

echo "# Third-party notices"
echo
echo "goli is MIT licensed (see LICENSE). Its binaries include the following"
echo "third-party software, each under its own licence, reproduced below."
echo
section() { printf '\n## %s\n\n```text\n' "$1"; cat "$2"; printf '```\n'; }

golicense="$goroot/LICENSE"
[ -f "$golicense" ] || golicense="$goroot/../LICENSE"   # Homebrew keeps it one level up
section "Go standard library and runtime ($(go env GOVERSION))" "$golicense"
[ -f "$goroot/PATENTS" ] && section "Go — PATENTS" "$goroot/PATENTS"
go version -m "$tmp/goli" | awk '$1=="dep"{print $2, $3}' | sort | while read -r mod ver; do
  # The module cache escapes upper-case letters as !lower.
  esc=$(printf '%s' "$mod" | sed -E 's/([A-Z])/!\1/g' | tr 'A-Z' 'a-z')
  dir="$modcache/$esc@$ver"
  for f in LICENSE LICENSE.md LICENSE.txt COPYING NOTICE; do
    [ -f "$dir/$f" ] && section "$mod $ver — $f" "$dir/$f"
  done
done
section "Fraunces font (bundled in the web UI)" internal/server/web/fonts/OFL-Fraunces.txt
section "Hanken Grotesk font (bundled in the web UI)" internal/server/web/fonts/OFL-Hanken.txt
