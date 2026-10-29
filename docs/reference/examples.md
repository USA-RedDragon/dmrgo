# Examples

This page lists the example programs in `v2/examples` and how to run each one.

## Offline examples

These programs run offline on the captures in `v2/layer2/testdata`. Run each one from the root of a clone with `go run -C v2 ./examples/<name>`.

### hotspot

[v2/examples/hotspot](../../v2/examples/hotspot/main.go)

<!-- dmrgo:doc pkg=examples/hotspot -->
<!-- /dmrgo:doc -->

### bridge

[v2/examples/bridge](../../v2/examples/bridge/main.go)

<!-- dmrgo:doc pkg=examples/bridge -->
<!-- /dmrgo:doc -->

### scanner

[v2/examples/scanner](../../v2/examples/scanner/main.go)

<!-- dmrgo:doc pkg=examples/scanner -->
<!-- /dmrgo:doc -->

### trunking

[v2/examples/trunking](../../v2/examples/trunking/main.go)

<!-- dmrgo:doc pkg=examples/trunking -->
<!-- /dmrgo:doc -->

### data

[v2/examples/data](../../v2/examples/data/main.go)

<!-- dmrgo:doc pkg=examples/data -->
<!-- /dmrgo:doc -->

### transcoder

[v2/examples/transcoder](../../v2/examples/transcoder/main.go)

<!-- dmrgo:doc pkg=examples/transcoder -->
<!-- /dmrgo:doc -->

### conformance

[v2/examples/conformance](../../v2/examples/conformance/main.go)

<!-- dmrgo:doc pkg=examples/conformance -->
<!-- /dmrgo:doc -->

## Microcontroller examples

These programs build with TinyGo for a board. See [Microcontrollers](../targets/mcu.md).

### tinygo

[v2/examples/tinygo](../../v2/examples/tinygo/main.go)

<!-- dmrgo:doc pkg=examples/tinygo -->
<!-- /dmrgo:doc -->

### rxaudio

[v2/examples/rxaudio](../../v2/examples/rxaudio/main.go)

<!-- dmrgo:doc pkg=examples/rxaudio -->
<!-- /dmrgo:doc -->

### txaudio

[v2/examples/txaudio](../../v2/examples/txaudio/main.go)

<!-- dmrgo:doc pkg=examples/txaudio -->
<!-- /dmrgo:doc -->

### txiq

[v2/examples/txiq](../../v2/examples/txiq/main.go)

<!-- dmrgo:doc pkg=examples/txiq -->
<!-- /dmrgo:doc -->

## Browser examples

These programs build to WebAssembly and run in a web page. See [Browser](../targets/browser.md).

### wasm/inspector

[v2/examples/wasm/inspector](../../v2/examples/wasm/inspector/README.md)

<!-- dmrgo:doc pkg=examples/wasm/inspector -->
<!-- /dmrgo:doc -->

### wasm/decoder

[v2/examples/wasm/decoder](../../v2/examples/wasm/decoder/README.md)

<!-- dmrgo:doc pkg=examples/wasm/decoder -->
<!-- /dmrgo:doc -->

## Network examples

The [udp](../../v2/examples/udp) and [tcp](../../v2/examples/tcp) examples are modules of their own, because they import packages that the `v2` module does not. Run each one with `go run -C v2/examples/<name> .`.

- udp sends filled forms as UDP datagrams over DMR confirmed data, between two simulated radios on a channel with bit errors. It prints the size of each form encoding and the time from a form to its acknowledgement at rate 1/2, 3/4 and 1.
- tcp runs a TN3270 session over TCP/IPv4 over DMR confirmed data, between two simulated radios on a channel with bit errors, with the TCP of gVisor netstack at each end. It prints the time of each screen.
