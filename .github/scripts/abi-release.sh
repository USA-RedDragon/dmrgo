#!/usr/bin/env bash
# usage: abi-release.sh TAG ABI_LOCK
# Fails a release tag vX.Y.Z while ABI_LOCK has the prerelease line. A tag
# with a prerelease suffix, such as v2.0.0-rc.1, passes either way.
set -euo pipefail
tag=$1
lock=$2
if [[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+-[0-9A-Za-z.-]+$ ]]; then
  exit 0
fi
if grep -qx prerelease "$lock"; then
  echo "::error file=$lock::$lock still has the prerelease line; remove it in the release commit of $tag"
  exit 1
fi
