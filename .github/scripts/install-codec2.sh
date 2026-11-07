#!/usr/bin/env bash
# usage: install-codec2.sh
# Installs libcodec2 and the c2enc and c2dec tools from the baseline amd64
# build. The amd64v3 build lets GCC fuse multiplies and adds into FMA, so its
# c2dec output differs from the unfused libcodec2 by up to 4 LSBs, and
# TestMatchesC2Dec compares the WebAssembly build of libcodec2 against it.
set -euo pipefail
apt_get() {
  sudo apt-get -o APT::Architecture-Variants= "$@"
}
apt_get update
apt_get install -y libcodec2-dev pkg-config codec2
variants=$(dpkg-query -W -f='${Package} ${Architecture-Variant}\n' 'libcodec2-[0-9]*' codec2 | awk 'NF > 1' | paste -sd ' ')
if [ -n "$variants" ]; then
  echo "::error::codec2 is not the baseline amd64 build: $variants"
  exit 1
fi
