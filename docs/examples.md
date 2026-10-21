# Examples

The programs in [v2/examples](../v2/examples) run offline on the captures in `v2/layer2/testdata` (`go run -C v2 ./examples/<name>`):

- [hotspot](../v2/examples/hotspot/main.go): the MMDVMHost check and regenerated bursts.
- [bridge](../v2/examples/bridge/main.go): rewrite the talkgroup and source of a call.
- [scanner](../v2/examples/scanner/main.go): find bursts in a bit stream.
- [trunking](../v2/examples/trunking/main.go): Tier III CSBKs, MBC and CACH.
- [data](../v2/examples/data/main.go): packet data and selective retry.
- [transcoder](../v2/examples/transcoder/main.go): AMBE+2 FEC and Codec2.
- [conformance](../v2/examples/conformance/main.go): byte-for-byte re-encode of every capture.
- [tinygo](../v2/examples/tinygo/main.go): decode, follow and relay a call on a microcontroller (see [TinyGo](tinygo.md)).
- [wasm/inspector](../v2/examples/wasm/inspector/README.md): a packet inspector in the browser, built to WebAssembly.
- [wasm/decoder](../v2/examples/wasm/decoder/README.md): DMR from audio, an I/Q file or a live RTL-SDR, decoded in the browser by the receivers of `rf` built to WebAssembly.

The [udp](../examples/udp) and [tcp](../examples/tcp) examples are their own modules, run with `go run -C examples/<name> .`. udp sends filled forms as UDP datagrams over DMR confirmed data, between two simulated radios on a channel with bit errors, and measures the size of each form encoding and the time per form at each rate. tcp runs a TN3270 session over TCP/IPv4 over DMR, with the TCP of gVisor netstack, on a simplex and a duplex channel.
