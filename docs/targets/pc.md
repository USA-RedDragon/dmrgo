# PC

This page shows how to build and run dmrgo with the standard Go toolchain on a PC or a server, which platforms CI checks, and which build tags change the build.

## What you need

- Go <!-- dmrgo:gomod field=go -->`1.24.0`<!-- /dmrgo:gomod --> or later, the version of the `go` line of `v2/go.mod`.
- No C compiler. The `v2` module is pure Go, and its only cgo code is the optional system libcodec2 backend of [vocoder/codec2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2).

## Build

Add the module to your program:

```sh
go get github.com/USA-RedDragon/dmrgo/v2
```

Then import the packages that you need, such as `github.com/USA-RedDragon/dmrgo/v2/layer2`. The [Go API](../reference/go-api.md) page lists every package.

## Run

The example programs run offline on the captures in the repository. From the root of a clone, run the conformance example:

```sh
go run -C v2 ./examples/conformance
```

It decodes each capture, encodes each burst again and compares the bytes. It prints:

<!-- dmrgo:output example=conformance -->
```text
                 file  bursts  voice  data  re-encoded  rebuilt  sealed  emb LC  FEC fixed  result
            voice.bin      79     77     2          79        2       2      13          0    PASS
  parrot_kerchunk.bin      15     13     2          15        2       2       2          0    PASS
            d-sms.bin      11      0    11          11       11       6       0          0    PASS
            h-sms.bin       9      0     9           9        9       6       0          0    PASS
            m-sms.bin      12      0    12          12       12       6       0          0    PASS
all captures conform
```
<!-- /dmrgo:output -->

[Examples](../reference/examples.md) lists the other programs.

## Platforms in CI

CI runs the tests of the module at the Go version of `v2/go.mod`, with cgo off:

<!-- dmrgo:code file=.github/workflows/test.yaml symbol=jobs.minimum-go-tests -->
```yaml
minimum-go-tests:
  runs-on: ubuntu-24.04

  steps:
    - name: Checkout repository
      uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1

    - name: Set up Go
      uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
      with:
        go-version-file: v2/go.mod

    - run: CGO_ENABLED=0 go test -C v2 -timeout 60m ./...
```
<!-- /dmrgo:code -->

It also builds and vets the module for 32-bit arm and 386, and tests the Codec2 engine on them:

<!-- dmrgo:code file=.github/workflows/test.yaml symbol=jobs.arch-32bit -->
```yaml
arch-32bit:
  runs-on: ubuntu-24.04

  steps:
    - name: Checkout repository
      uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1

    - name: Set up Go
      uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
      with:
        go-version-file: v2/go.mod

    - name: Build and vet for arm
      run: GOARCH=arm CGO_ENABLED=0 go vet -C v2 ./...

    - name: Build and vet for 386
      run: GOARCH=386 CGO_ENABLED=0 go vet -C v2 ./...

    - name: Codec2 tests on 386
      run: GOARCH=386 CGO_ENABLED=0 go test -C v2 ./vocoder/codec2/...

    - name: Codec2 tests on amd64 with the translated engine
      run: CGO_ENABLED=0 go test -C v2 -tags codec2aot ./vocoder/codec2/...

    - name: Install qemu-user-static
      run: sudo apt-get update && sudo apt-get install -y qemu-user-static

    - name: Codec2 allocation and engine tests on arm
      run: GOARCH=arm CGO_ENABLED=0 go test -C v2 -exec qemu-arm-static -run '^(TestAllocs|TestEnginesMatch|TestAOT)' ./vocoder/codec2/
```
<!-- /dmrgo:code -->

## Build tags

The tags change only the backend of `vocoder/codec2`. The package doc gives each one:

<!-- dmrgo:doc pkg=vocoder/codec2 -->
Package codec2 implements vocoder.Codec with the Codec2 speech codec: it encodes 8 kHz PCM into 72-bit DMR vocoder frames and decodes them, in the DMR framing that QRadioLink uses.

By default, with or without cgo, the package runs libcodec2 compiled to WebAssembly: in wazero, which compiles it to machine code on amd64 and arm64, as Go code that wasm2go translated from the same module on other platforms, such as 386 and arm, where wazero has no compiler, and under GOOS=js GOARCH=wasm in the WebAssembly engine of the browser or Node. -tags codec2aot selects the translated code on every platform, for example for an amd64 CPU without SSE4.1, where wazero interprets. Such a build needs no C compiler and no system libcodec2. With -tags codec2native the package binds the system libcodec2 with cgo instead, so the build needs cgo, a C compiler and libcodec2 with its pkg-config file (pkg-config name codec2), for example the libcodec2-dev package on Debian. Without them the build fails, with "Package 'codec2' not found" when libcodec2 is missing. The tag codec2wasm is accepted and changes nothing. Backend names the backend of a build. Both backends encode the same frames; the decoded samples of the WebAssembly backend can differ from the native ones by small amounts, because its math library differs in the last bit. Build with -tags nocodec2 to remove libcodec2 entirely; New then returns ErrUnavailable. TinyGo builds always use this path.

The WebAssembly backend embeds wasm/codec2.wasm, which wasm/build.sh builds from a pinned libcodec2. NewModule and Config.WASM run another build in its place, which must be a wasm32-wasi reactor module that exports its memory as "memory", "\_initialize", and these functions of the libcodec2 C API, with pointers as offsets into that memory and every value an i32: malloc(size) ptr, free(ptr), codec2\_create(mode) state, codec2\_destroy(state), codec2\_samples\_per\_frame(state) n, codec2\_bits\_per\_frame(state) n, codec2\_encode(state, bits, pcm) and codec2\_decode(state, pcm, bits). mode is the CODEC2\_MODE value of libcodec2: 0 for 3200 bit/s and 1 for 2400 bit/s. PCM is 160 int16 samples, little-endian. The module may import only fd\_close, fd\_seek and fd\_write of wasi\_snapshot\_preview1, which libc uses for its error output. In the browser engine fd\_close returns 0, fd\_seek returns EBADF (8), and fd\_write discards the text and reports it written. On js/wasm each call into the module allocates JavaScript values.

Each 20 ms Codec2 frame (160 samples at 8 kHz) fills one 72-bit DMR vocoder frame, so a 60 ms voice burst carries three. The Codec2 bytes go MSB first at the start of the 9-byte frame and the remaining bits are zero. There is no FEC, scrambling or interleaving. Calls are marked with the Feature set ID 0xC2 in the Full LC. Nothing on the air identifies the Codec2 mode, so by default (ModeAuto) the decoder detects it from the padding: Mode2400 leaves bytes 6 to 8 zero and Mode3200 only byte 8.

libcodec2 is licensed under the GNU LGPL 2.1. The native backend links it dynamically. The WebAssembly backend embeds a build of it, which a user can replace through NewModule as the LGPL requires; wasm/build.sh rebuilds it from the libcodec2 source. A program that links libcodec2 statically must meet the LGPL terms for that copy.
<!-- /dmrgo:doc -->

## Limits

- AMBE+2 voice is not in the library. The library has the AMBE+2 frame FEC, and [vocoder/dvsi](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi) uses a DVSI chip for the codec.
- The library has no I/O of its own for radios and networks. Your program reads and writes the sockets, serial ports and sample streams, and gives the bytes to the packages.
