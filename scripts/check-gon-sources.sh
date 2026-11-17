#!/usr/bin/env bash
# Gon's own sources: every committed .go next to a .gon must be exactly what
# `gon transpile` emits for it (D2), checked with a PINNED gon release (D3):
# a checker under test must not judge itself.
#
#   scripts/check-gon-sources.sh            # uses `gon` from PATH
#   GON=/path/to/gon scripts/check-gon-sources.sh
#
# Steps: find the packages that have .gon sources; `gon transpile` them (the
# directory form checks each package as a whole and writes nothing for one that
# fails); then require the working tree's .go files to be unchanged, including
# no new untracked .go (a .gon whose .go was never committed).
set -euo pipefail
cd "$(dirname "$0")/.."
gon=${GON:-gon}

dirs=$(find cmd internal transpiler -name '*.gon' -not -name '*_test.gon' \
  -not -path '*/testdata/*' -exec dirname {} \; | sort -u)
if [ -z "$dirs" ]; then
  echo "no .gon sources yet: nothing to check"
  exit 0
fi

echo "packages with .gon sources:"
printf '  %s\n' $dirs
# shellcheck disable=SC2086
"$gon" transpile $dirs

status=$(git status --porcelain -- '*.go')
if [ -n "$status" ]; then
  echo
  echo "the committed .go files do not match their .gon sources:"
  echo "$status"
  git --no-pager diff -- '*.go'
  echo "run: gon transpile <package dir> and commit the .go files"
  exit 1
fi
echo "ok: every .go matches its .gon"
