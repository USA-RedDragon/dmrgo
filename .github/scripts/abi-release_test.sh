#!/usr/bin/env bash
# usage: abi-release_test.sh
# Runs abi-release.sh on each kind of tag, with and without the prerelease
# line, and fails when one gives the wrong result.
set -uo pipefail
dir=$(mktemp -d "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/abi-release.XXXXXX")
trap 'rm -rf "$dir"' EXIT
printf 'abi 1\nprerelease\ncode X -100\n' > "$dir/pre.txt"
printf 'abi 1\ncode X -100\n' > "$dir/rel.txt"
script=$(dirname "$0")/abi-release.sh
fail=0
check() {
  local tag=$1 lock=$2 want=$3 got=0
  bash "$script" "$tag" "$dir/$lock" >/dev/null 2>&1 || got=1
  if [[ $got != "$want" ]]; then
    echo "abi-release.sh $tag $lock: exit $got, want $want"
    fail=1
  fi
}
check v2.0.0-rc.1 pre.txt 0
check v2.0.0-beta.2 pre.txt 0
check v2.1.0-alpha pre.txt 0
check v2.0.0-rc.1 rel.txt 0
check v2.0.0 pre.txt 1
check v2.0.0 rel.txt 0
check v2.0.0- pre.txt 1
check v2 pre.txt 1
exit $fail
