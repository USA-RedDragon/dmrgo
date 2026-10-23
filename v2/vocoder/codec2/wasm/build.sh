#!/usr/bin/env bash
# Builds codec2.wasm, libcodec2 for WebAssembly (wasm32-wasi), from a pinned
# commit of https://github.com/drowe67/codec2 with a pinned toolchain.
#
# Usage: build.sh [workdir]
#
# The workdir (default: a new directory from mktemp -d) receives the codec2
# checkout and the objects. The script writes codec2.wasm next to itself.
# Set CODEC2_SRC to the path of an existing checkout to skip the clone; its
# HEAD must be CODEC2_COMMIT. Set DMRGO_ANY_TOOLCHAIN=1 to build with other
# versions of the tools; the output can then differ from the checked-in file.
#
# Toolchain (Ubuntu 24.04 packages): clang-18 and lld-18 1:18.1.3-1ubuntu1,
# wasi-libc 0.0~git20230113.4362b18-3, libclang-rt-18-dev-wasm32
# 1:18.1.3-1ubuntu1.
#
#   apt-get install clang-18 lld-18 wasi-libc libclang-rt-18-dev-wasm32 git
#
# The output is the work of libcodec2 compiled, and is licensed under the
# GNU LGPL 2.1 like libcodec2 (see LICENSE next to this script).
set -euo pipefail

CODEC2_REPO=${CODEC2_REPO:-https://github.com/drowe67/codec2}
CODEC2_COMMIT=310777b1c6f1af0bc7c72f5b32f80f6fd9136962
CLANG_VERSION="Ubuntu clang version 18.1.3 (1ubuntu1)"
LLD_VERSION="Ubuntu LLD 18.1.3"
WASI_LIBC_VERSION="0.0~git20230113.4362b18-3"
CLANG_RT_VERSION="1:18.1.3-1ubuntu1"

out_dir=$(cd "$(dirname "$0")" && pwd)
work=${1:-$(mktemp -d)}
mkdir -p "$work"
work=$(cd "$work" && pwd)

CC=${CC:-clang-18}
WASM_LD=${WASM_LD:-wasm-ld-18}
command -v "$CC" >/dev/null || CC=clang
command -v "$WASM_LD" >/dev/null || WASM_LD=wasm-ld

pkg_version() {
	dpkg-query -W -f='${Version}' "$1" 2>/dev/null || echo missing
}

check() {
	if [ "$2" != "$3" ]; then
		if [ "${DMRGO_ANY_TOOLCHAIN:-}" = 1 ]; then
			echo "warning: $1 is $2, want $3" >&2
		else
			echo "error: $1 is $2, want $3 (set DMRGO_ANY_TOOLCHAIN=1 to build anyway)" >&2
			exit 1
		fi
	fi
}

check clang "$("$CC" --version | head -n 1)" "$CLANG_VERSION"
check wasm-ld "$("$WASM_LD" --version | head -n 1)" "$LLD_VERSION"
check wasi-libc "$(pkg_version wasi-libc)" "$WASI_LIBC_VERSION"
check libclang-rt-18-dev-wasm32 "$(pkg_version libclang-rt-18-dev-wasm32)" "$CLANG_RT_VERSION"

src=${CODEC2_SRC:-$work/codec2}
if [ -z "${CODEC2_SRC:-}" ] && [ ! -d "$src/.git" ]; then
	git clone --quiet "$CODEC2_REPO" "$src"
fi
git -C "$src" -c advice.detachedHead=false checkout --quiet "$CODEC2_COMMIT"
check "codec2 commit" "$(git -C "$src" rev-parse HEAD)" "$CODEC2_COMMIT"

obj=$work/obj
rm -rf "$obj"
mkdir -p "$obj"

d=$src/src/codebook
"$CC" -O2 -o "$obj/generate_codebook" "$src/src/generate_codebook.c" -lm
"$obj/generate_codebook" lsp_cb "$d"/lsp{1,2,3,4,5,6,7,8,9,10}.txt >"$obj/codebook.c"
"$obj/generate_codebook" lsp_cbd "$d"/dlsp{1,2,3,4,5,6,7,8,9,10}.txt >"$obj/codebookd.c"
"$obj/generate_codebook" lsp_cbjmv "$d"/lspjmv{1,2,3}.txt >"$obj/codebookjmv.c"
"$obj/generate_codebook" ge_cb "$d/gecb.txt" >"$obj/codebookge.c"
"$obj/generate_codebook" newamp1vq_cb "$d"/train_120_{1,2}.txt >"$obj/codebooknewamp1.c"
"$obj/generate_codebook" newamp1_energy_cb "$d/newamp1_energy_q.txt" >"$obj/codebooknewamp1_energy.c"

sources=(
	"$src"/src/{codec2,codec2_fft,interp,kiss_fft,kiss_fftr,lpc,lsp,mbest,newamp1,nlp,pack,phase,postfilter,quantise,sine}.c
	"$obj"/{codebook,codebookd,codebookjmv,codebookge,codebooknewamp1,codebooknewamp1_energy}.c
)

cflags=(
	--target=wasm32-wasi --sysroot=/usr
	-std=gnu11 -O2 -Wall -Wno-strict-overflow
	-ffunction-sections -fdata-sections
	"-ffile-prefix-map=$src=codec2" "-ffile-prefix-map=$obj=codec2/build"
	-DNDEBUG -DGIT_HASH=\"310777b\"
	-DCODEC2_MODE_EN_DEFAULT=0 -DCODEC2_MODE_3200_EN=1 -DCODEC2_MODE_2400_EN=1
	-I"$src/src" -I"$obj"
)

objects=()
for f in "${sources[@]}"; do
	o=$obj/$(basename "$f" .c).o
	"$CC" "${cflags[@]}" -c -o "$o" "$f"
	objects+=("$o")
done

"$CC" --target=wasm32-wasi --sysroot=/usr -mexec-model=reactor \
	-Wl,--strip-all -Wl,--gc-sections \
	-Wl,-z,stack-size=65536 -Wl,--initial-memory=262144 -Wl,--max-memory=16777216 \
	-Wl,--export=codec2_create -Wl,--export=codec2_destroy \
	-Wl,--export=codec2_encode -Wl,--export=codec2_decode \
	-Wl,--export=codec2_samples_per_frame -Wl,--export=codec2_bits_per_frame \
	-Wl,--export=malloc -Wl,--export=free \
	-o "$obj/codec2.wasm" "${objects[@]}" -lm

cp "$obj/codec2.wasm" "$out_dir/codec2.wasm"
sha256sum "$out_dir/codec2.wasm"
