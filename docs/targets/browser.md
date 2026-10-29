# Browser

This page shows how to build dmrgo to WebAssembly and run it in a web page, with the two browser examples in `v2/examples/wasm`.

## What you need

- Go, at the version of `v2/go.mod`. See [PC](pc.md). TinyGo is optional; see [Microcontrollers](mcu.md) for its versions.
- A current browser. A live RTL-SDR needs WebUSB, which Chrome and Edge have and Firefox and Safari do not.
- A clone of the repository. The examples serve its captures.

The whole `v2` module builds for `GOOS=js GOARCH=wasm`, and CI runs the Codec2 tests on the WebAssembly engine of Node:

<!-- dmrgo:code file=.github/workflows/test.yaml lines=58-62 -->
```yaml
- name: Build for js/wasm
  run: GOOS=js GOARCH=wasm go build -C v2 ./...

- name: Codec2 tests on the browser engine in Node
  run: GOOS=js GOARCH=wasm go test -C v2 -exec "$(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./vocoder/codec2/
```
<!-- /dmrgo:code -->

## Build and run the decoder

[v2/examples/wasm/decoder](../../v2/examples/wasm/decoder/README.md) runs the receivers of [rf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf) in a page. It decodes discriminator audio from a microphone or a WAV file, I/Q files, and a live RTL-SDR over WebUSB.

From the `v2` directory:

```sh
go run ./examples/wasm/decoder/serve
```

The command builds `decoder.wasm` and serves the page. Open http://localhost:8080/. Its flags:

<!-- dmrgo:code file=v2/examples/wasm/decoder/serve/main.go lines=21-24 -->
```go
addr := flag.String("addr", "localhost:8080", "listen address; WebUSB and the microphone need localhost or HTTPS")
build := flag.Bool("build", true, "build the decoder before serving")
tinygo := flag.Bool("tinygo", false, "build and serve the decoder with TinyGo instead of Go")
files := flag.String("files", "", "a directory of recordings to serve under /files/, for the src parameter of the page")
```
<!-- /dmrgo:code -->

## Build and run the packet inspector

[v2/examples/wasm/inspector](../../v2/examples/wasm/inspector/README.md) shows the field tree of IPSC, DMRD, Hytera, MMDVM serial and bare burst packets, from pasted hex or a pcap file. Its README gives the build commands: a `GOOS=js GOARCH=wasm go build`, the `wasm_exec.js` of the same Go release, and any static web server.

## Builds with TinyGo

The TinyGo job of CI builds the receiver and modulator examples for `wasm` and `wasip1`. See [Microcontrollers](mcu.md#boards-and-builds-in-ci). The `-tinygo` flag of the decoder server builds the page with TinyGo.

## Limits

- [vocoder/codec2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2) runs libcodec2 in the WebAssembly engine of the browser. On js/wasm each call into that module allocates JavaScript values.
- The microphone and WebUSB work only on localhost or HTTPS.
- The decoder page supports RTL-SDR dongles with an R820T or R820T2 tuner. Its README gives the driver setup for Linux and Windows.
- The decoder page does not decode AMBE+2 to audio.
- Tier 3 of `rf` is for offline use. The decoder README gives the real-time factor of each tier in Node and Chromium.
