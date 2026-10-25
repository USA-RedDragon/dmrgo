// Package codec2 implements vocoder.Codec with the Codec2 speech codec: it
// encodes 8 kHz PCM into 72-bit DMR vocoder frames and decodes them, in the
// DMR framing that QRadioLink uses.
//
// With cgo the package binds the system libcodec2, so a build that imports it
// needs a C compiler and libcodec2 with its pkg-config file (pkg-config name
// codec2), for example the libcodec2-dev package on Debian. Without them the
// build fails with "Package 'codec2' not found". With CGO_ENABLED=0, or -tags
// codec2wasm, the package runs libcodec2 compiled to WebAssembly instead: in
// wazero, which compiles it to machine code on amd64 and arm64, as Go code
// that wasm2go translated from the same module on other platforms, such as
// 386 and arm, where wazero has no compiler, and under GOOS=js GOARCH=wasm in
// the WebAssembly engine of the browser or Node. -tags codec2aot selects the
// translated code on every platform, for example for an amd64 CPU without
// SSE4.1, where wazero interprets. Backend names the backend of a build. Both
// backends encode the same frames; the decoded samples of the WebAssembly
// backend can differ from the native ones by small amounts, because its math
// library differs in the last bit. Build with -tags nocodec2 to remove
// libcodec2 entirely; New then returns ErrUnavailable. TinyGo builds always
// use this path.
//
// The WebAssembly backend embeds wasm/codec2.wasm, which wasm/build.sh builds
// from a pinned libcodec2. NewModule and Config.WASM run another build in its
// place, which must be a wasm32-wasi reactor module that exports its memory
// as "memory", "_initialize", and these functions of the libcodec2 C API,
// with pointers as offsets into that memory and every value an i32:
// malloc(size) ptr, free(ptr), codec2_create(mode) state,
// codec2_destroy(state), codec2_samples_per_frame(state) n,
// codec2_bits_per_frame(state) n, codec2_encode(state, bits, pcm) and
// codec2_decode(state, pcm, bits). mode is the CODEC2_MODE value of
// libcodec2: 0 for 3200 bit/s and 1 for 2400 bit/s. PCM is 160 int16
// samples, little-endian. The module may import only fd_close, fd_seek and
// fd_write of wasi_snapshot_preview1, which libc uses for its error output.
// In the browser engine fd_close returns 0, fd_seek returns EBADF (8), and
// fd_write discards the text and reports it written. On js/wasm each call
// into the module allocates JavaScript values.
//
// Each 20 ms Codec2 frame (160 samples at 8 kHz) fills one 72-bit DMR
// vocoder frame, so a 60 ms voice burst carries three. The Codec2 bytes go
// MSB first at the start of the 9-byte frame and the remaining bits are zero.
// There is no FEC, scrambling or interleaving. Calls are marked with the
// Feature set ID 0xC2 in the Full LC. Nothing on the air identifies the
// Codec2 mode, so by default (ModeAuto) the decoder detects it from the
// padding: Mode2400 leaves bytes 6 to 8 zero and Mode3200 only byte 8.
//
// libcodec2 is licensed under the GNU LGPL 2.1. The cgo backend links it
// dynamically. The WebAssembly backend embeds a build of it, which a user can
// replace through NewModule as the LGPL requires; wasm/build.sh rebuilds it
// from the libcodec2 source. A program that links libcodec2 statically must
// meet the LGPL terms for that copy.
package codec2
