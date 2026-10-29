# Examples

This page lists the example programs in `v2/examples` and how to run each one.

## Offline examples

These programs run offline on the captures in `v2/layer2/testdata`. Run each one from the root of a clone with `go run -C v2 ./examples/<name>`.

### hotspot

[v2/examples/hotspot](../../v2/examples/hotspot/main.go)

<!-- dmrgo:doc pkg=examples/hotspot -->
Command hotspot decodes captured bursts with bit errors, applies the MMDVMHost check and regenerates clean bursts.
<!-- /dmrgo:doc -->

### bridge

[v2/examples/bridge](../../v2/examples/bridge/main.go)

<!-- dmrgo:doc pkg=examples/bridge -->
Command bridge receives a voice call, rewrites its talkgroup and source ID, and sends it again.
<!-- /dmrgo:doc -->

### scanner

[v2/examples/scanner](../../v2/examples/scanner/main.go)

<!-- dmrgo:doc pkg=examples/scanner -->
Command scanner finds bursts in a noisy BS outbound bit stream and follows the calls on both time slots.
<!-- /dmrgo:doc -->

### trunking

[v2/examples/trunking](../../v2/examples/trunking/main.go)

<!-- dmrgo:doc pkg=examples/trunking -->
Command trunking builds and decodes Tier III CSBKs, an MBC channel grant and the CACH Short LC system parameters.
<!-- /dmrgo:doc -->

### data

[v2/examples/data](../../v2/examples/data/main.go)

<!-- dmrgo:doc pkg=examples/data -->
Command data reassembles the SMS captures and segments confirmed and unconfirmed packets, with a selective retry.
<!-- /dmrgo:doc -->

### transcoder

[v2/examples/transcoder](../../v2/examples/transcoder/main.go)

<!-- dmrgo:doc pkg=examples/transcoder -->
Command transcoder checks the AMBE+2 FEC of a captured call, rebuilds it, and runs a Codec2 round trip.
<!-- /dmrgo:doc -->

### conformance

[v2/examples/conformance](../../v2/examples/conformance/main.go)

<!-- dmrgo:doc pkg=examples/conformance -->
Command conformance checks that every capture re-encodes byte for byte and that its FEC and CRCs verify.
<!-- /dmrgo:doc -->

## Microcontroller examples

These programs build with TinyGo for a board. See [Microcontrollers](../targets/mcu.md).

### tinygo

[v2/examples/tinygo](../../v2/examples/tinygo/main.go)

<!-- dmrgo:doc pkg=examples/tinygo -->
Command tinygo decodes and re-encodes the bursts of a short voice call on a microcontroller, follows the call with a voice.Receiver and relays it with a voice.Transmitter.
<!-- /dmrgo:doc -->

### rxaudio

[v2/examples/rxaudio](../../v2/examples/rxaudio/main.go)

<!-- dmrgo:doc pkg=examples/rxaudio -->
Command rxaudio runs the Tier 0 receiver, an rf.AudioReceiver, in static memory on a microcontroller, as a hotspot board feeds it the samples of its radio chip's FM discriminator. It allocates nothing: it builds with -gc=none, which fails the link of any program that allocates.
<!-- /dmrgo:doc -->

### txaudio

[v2/examples/txaudio](../../v2/examples/txaudio/main.go)

<!-- dmrgo:doc pkg=examples/txaudio -->
Command txaudio runs the Tier 0 audio transmitter, an rf.AudioTransmitter that Init sets up in static memory, as a hotspot board drives the modulator input of its radio from a 12-bit DAC at 48 kHz and keys the radio by Active. It allocates nothing: it builds with -gc=none, which fails the link of any program that allocates. Init returns an error for a setup that no measurement shows to keep the limits of its region, and then the command prints "setup refused" and stops.
<!-- /dmrgo:doc -->

### txiq

[v2/examples/txiq](../../v2/examples/txiq/main.go)

<!-- dmrgo:doc pkg=examples/txiq -->
Command txiq runs the Tier 0 I/Q transmitter, an rf.Transmitter that Init sets up in static memory, as a microcontroller feeds the I/Q DAC of a software radio at 96 kHz: a few MS bursts on one TDMA channel, with their power ramps. It allocates nothing: it builds with -gc=none, which fails the link of any program that allocates. Init returns an error for a setup that no measurement shows to keep the limits of its region, and then the command prints "setup refused" and stops.
<!-- /dmrgo:doc -->

## Browser examples

These programs build to WebAssembly and run in a web page. See [Browser](../targets/browser.md).

### wasm/inspector

[v2/examples/wasm/inspector](../../v2/examples/wasm/inspector/README.md)

<!-- dmrgo:doc pkg=examples/wasm/inspector -->
Command inspector is a packet inspector for the browser: it decodes the network protocols that dmrgo supports, and the DMR bursts and data inside them, into a field tree with the bit ranges of each field. Built for GOOS=js GOARCH=wasm it serves index.html; built for the host it prints the decode of a file as JSON.
<!-- /dmrgo:doc -->

### wasm/decoder

[v2/examples/wasm/decoder](../../v2/examples/wasm/decoder/README.md)

<!-- dmrgo:doc pkg=examples/wasm/decoder -->
Command decoder decodes DMR in a web browser: the receivers of package rf, compiled to WebAssembly, take FM discriminator audio or I/Q from a file, a microphone or line-in, or an RTL-SDR over WebUSB, and the page shows the bursts, calls, data and text messages that they carry. Built for GOOS=js, it exports the decoder to JavaScript; built natively, it decodes a file and prints the same events, one JSON object per line.
<!-- /dmrgo:doc -->

## Network examples

The [udp](../../v2/examples/udp) and [tcp](../../v2/examples/tcp) examples are modules of their own, because they import packages that the `v2` module does not. Run each one with `go run -C v2/examples/<name> .`.

- udp sends filled forms as UDP datagrams over DMR confirmed data, between two simulated radios on a channel with bit errors. It prints the size of each form encoding and the time from a form to its acknowledgement at rate 1/2, 3/4 and 1.
- tcp runs a TN3270 session over TCP/IPv4 over DMR confirmed data, between two simulated radios on a channel with bit errors, with the TCP of gVisor netstack at each end. It prints the time of each screen.
