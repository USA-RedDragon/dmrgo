# dmrgo

[![go.mod version](https://img.shields.io/github/go-mod/go-version/USA-RedDragon/dmrgo.svg?filename=v2/go.mod)](https://github.com/USA-RedDragon/dmrgo) [![coverage](https://raw.githubusercontent.com/USA-RedDragon/dmrgo/main/.github/badges/coverage-v2.svg)](https://github.com/USA-RedDragon/dmrgo/actions/workflows/test.yaml) [![License](https://badgen.net/github/license/USA-RedDragon/dmrgo)](https://github.com/USA-RedDragon/dmrgo/blob/main/LICENSE)

Decode and encode DMR (Digital Mobile Radio) bursts and PDUs as defined in [ETSI TS 102 361](https://www.etsi.org/standards) parts 1 to 4.

[DMRHub](https://github.com/USA-RedDragon/DMRHub) and [ipsc2mmdvm](https://github.com/USA-RedDragon/ipsc2mmdvm) use it.

## Installation

Requires Go 1.24 or later.

```sh
go get github.com/USA-RedDragon/dmrgo/v2
```

## Getting started

```go
package main

import (
	"fmt"
	"log"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func main() {
	// A Voice LC Header burst as received from a repeater.
	raw := [33]byte{
		68, 75, 3, 135, 36, 66, 12, 240, 21, 240,
		12, 161, 196, 109, 255, 87, 215, 93, 245, 222,
		49, 168, 53, 24, 63, 48, 61, 97, 56, 82,
		151, 134, 91,
	}

	burst, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		log.Fatal(err)
	}
	if err := burst.FEC.Aggregate().Err(); err != nil {
		log.Fatal(err)
	}
	if lc, ok := burst.Data.(*pdu.FullLinkControl); ok {
		target, source, _ := lc.Addresses()
		fmt.Printf("%s: call from %d to %d\n", burst.SlotType.DataType, source, target)
	}

	encoded, err := burst.Encode()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("re-encoded unchanged:", encoded == raw)
}
```

The program prints `Voice LC Header: call from 3191868 to 9990` and `re-encoded unchanged: true`.

An FEC or CRC error does not make `NewBurstFromBytes` fail. `burst.FEC` reports it, and `burst.Data` still holds the decoded PDU. Change a field of `burst.Data` and `Encode` writes a new burst with new parity. The [package documentation](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2) has the package map and the errors to match.

Fields of bits are packed in bytes in transmit order, with the first bit on air in the most significant bit of byte 0. A burst is a `[33]byte`, a CSBK a `[12]byte` and a vocoder slot a `[9]byte`.

The programs in [v2/examples](v2/examples) run offline on the captures in `v2/layer2/testdata` (`go run -C v2 ./examples/<name>`):

- [hotspot](v2/examples/hotspot/main.go): the MMDVMHost check and regenerated bursts.
- [bridge](v2/examples/bridge/main.go): rewrite the talkgroup and source of a call.
- [scanner](v2/examples/scanner/main.go): find bursts in a bit stream.
- [trunking](v2/examples/trunking/main.go): Tier III CSBKs, MBC and CACH.
- [data](v2/examples/data/main.go): packet data and selective retry.
- [transcoder](v2/examples/transcoder/main.go): AMBE+2 FEC and Codec2.
- [conformance](v2/examples/conformance/main.go): byte-for-byte re-encode of every capture.
- [tinygo](v2/examples/tinygo/main.go): decode, follow and relay a call on a microcontroller (see [TinyGo](#tinygo)).

The [udp](examples/udp) and [tcp](examples/tcp) examples are their own modules, run with `go run -C examples/<name> .`. udp sends filled forms as UDP datagrams over DMR confirmed data, between two simulated radios on a channel with bit errors, and measures the size of each form encoding and the time per form at each rate. tcp runs a TN3270 session over TCP/IPv4 over DMR, with the TCP of gVisor netstack, on a simplex and a duplex channel.

## Features

- Bursts (`layer2`):
  - Decode and encode voice, data and standalone Reverse Channel bursts, including data bursts that carry an EMB in place of the SYNC.
  - Read and write single fields of a 33-byte burst in place with `RawBurst`.
  - Build data bursts from a PDU or a 12-byte payload, and voice superframes with `VoiceTransmitter`.
  - Receive calls with `VoiceReceiver`, the embedded LC assembler and the CACH Short LC assembler, and place the voice bursts of a network stream with `PositionTracker`.
  - Check and reseal bursts the way MMDVMHost does.
- PDUs (`layer2/pdu`), generated from struct tags by `dmrgen`:
  - CSBK and MBC, with the Tier II and Tier III payloads, and the Motorola radio check and call alert.
  - Full LC and Short LC, with their payloads.
  - A CSBK, MBC header, Full LC, Short LC or data header keeps its payload as bits, and its opcode or format selects their layout. `AsX` reads the payload X, and `SetX` writes it and sets the opcode.
  - Data headers, data blocks at rate 1/2, 3/4 and 1, USBD and the compressed UDP/IPv4 header.
  - Slot type, EMB, TACT, PI header, Reverse Channel and PR FILL.
  - Byte-level editors for LC, CSBK and data header octets, and `Seal` to recompute parity or CRC.
- Packet data (`layer2/packet`): segmentation and reassembly of confirmed and unconfirmed packets, with response packets and selective retry:
  - `Builder.Retry` builds the later try that a SACK asks for.
  - The `Assembler` counts a block burst lost on air with `Lost`, reads the Full Message Flag, takes later tries of confirmed and short data packets, and with `AcceptBadCRC` delivers a packet whose CRC failed, as the "skip CRC" setting of some radios does.
  - `Message.UDPIPv4` returns the compressed UDP/IPv4 header of a packet and the application data after it.
- Applications in data packets:
  - `ip`: the IPv4 and UDP headers of a data packet and the compressed UDP/IPv4 header, with their checksums.
  - `sms`: text messages in every format that radios use: MOTOTRBO TMS, the ETSI IP text, the DMR Standard text of AnyTone radios, ETSI defined data and the AnyTone H-SMS text.
  - `shortdata`: the text of an ETSI defined data packet in the coding that its DD format names, and the AnyTone H-SMS text.
  - `location`: positions in every format that radios report: the NMEA RMC sentence of AnyTone radios and the LRRP reports of MOTOTRBO and Hytera radios.
  - `lip`: the ETSI Location Information Protocol that a UDT carries in the LIP format: the long location report, the report acknowledgement, the immediate location report request, the location reporting enable/disable PDUs and the type 5 elements of every PDU. `trunk.DecodeLIPShortReport` reads the short location report.
  - `nmea`: `ParseRMC` decodes the NMEA 0183 RMC sentence that radios such as AnyTone send in a UDP datagram, and checks its checksum.
  - `mototrbo/ars`, `mototrbo/lrrp`, `mototrbo/tms`: the MOTOTRBO registration (UDP port 4005), location (port 4001) and text message (port 4007) services.
  - `hytera/hdap`: the Hytera DMR Application Protocol (HDAP): the frame with its checksum, and the radio registration, location and text message services.
- Voice calls (`voice`), without I/O or goroutines:
  - `Receiver` turns the bursts of one logical channel into call events: call start with where the call came from, voice bursts with their superframe position, lost count and frame quality, embedded GPS and talker alias, and call end with a reason. It handles late entry, duplicate and lost bursts, transport hints (`layer2.Hint`) and the call hang time.
  - `Transmitter` sends a call one burst per 60 ms slot: from AMBE+2 frames with the headers, embedded talker alias and GPS, padding, terminators and the transmit timeout, or as a relay of the events of a `Receiver`, with the local color code and SYNC. A call follows the polite or impolite channel access and the transmit admit criteria, waits for the BS to authorize it in hang time, and ends on a Tier 2 RC cease command.
  - `Interrupter` sends a Tier 2 RC cease command in standalone RC bursts and waits T_RCtimer for the Terminator with LC of the call, with retries.
- CSBK services (`csbk`), without I/O or goroutines:
  - `Requester` sends a CSBK request, after Pre_CSBKs when asked, with polite or impolite channel access, the random holdoff, T_AckWait and N_CSBKRetry, and reports the answer, a NACK_Rsp, or why none came. It knows the OACSU presence check (UU_V_Req) and the Motorola radio check and call alert.
  - `Responder` answers the CSBKs that address a radio: UU_Ans_Rsp, the Motorola radio check answer and call alert acknowledgement, and the Feature Not Supported NACK_Rsp.
  - `Preambles` joins Pre_CSBKs with the CSBK or data that they announce, and `Wakeup` sends BS_Dwn_Act until the BS outbound starts.
  - `Timing` runs the TDMA direct mode wide area timing with CT_CSBKs: it follows, elects and appoints the timing leader, propagates its beacon, answers requests, corrects radios with old timing, and beacons as the leader.
- Base station (`bs`), without I/O or goroutines:
  - `Repeater` is a two-slot repeater or hotspot, or the slot arbiter of a network server. It gives each slot to one source and queues or refuses the others, holds the call hang time, regenerates every repeated burst with new FEC and the local color code, and in RF mode sends one burst per 30 ms with the CACH, the AT bit, the Activity Update, Idle bursts, Terminators in the call hang time and TD_LC in the data hang time. It wakes up on BS_Dwn_Act or any burst and hibernates after T_MSInactiv. With channel authorization it repeats the ChanAuth PDU of one MS that starts in hang time, single site or multi-site. Its outbound stream for a voice call matches a Motorola XPR 8400 bit for bit.
  - `CACH` makes the CACH and Short LC fragments alone.
- Tier III trunking (`trunk`), without I/O or goroutines:
  - Channel plans, hunt lists and site tables, from the configuration and from CG_AP, Chan_Freq, Ann_WD_TSCC and Adjacent_Site.
  - `Follower` hunts for a TSCC and follows its voice and data grants to the payload channels, as a scanner does, and returns on P_CLEAR, after the hang time or when the channel goes quiet. It reads UDT deliveries, and with `AcceptRAS` the CSBKs of Capacity Max sites whose CRC is a keyed MAC.
  - `MS` registers and de-registers, makes voice and packet data calls by random access with backoff, also as emergency, answers C_AHOY and P_AHOY radio checks, follows C_MOVE, P_PROTECT and P_CLEAR, keeps a denied registration list, leaves a weak TSCC, and ends its calls with P_MAINT.
  - `MS` also sends UDT short data, polls an MS for UDT short data, sets and clears call diversion, assigns DGNA talkgroups, makes full-duplex voice and data calls and Ambient Listening Service calls, answers UDT and USBD polls, and follows the DGNA talkgroups that it receives.
  - `Appended` reads and writes the ten appended data formats of a UDT: binary, addresses, BCD digits, ISO 7 bit and ISO 8 bit text, NMEA, IP, UTF-16BE text, mixed and LIP.
  - `TSCC` runs a single site: C_ALOHA and C_SYS_Parms, AT reservation, registration, talkgroup and individual (OACSU) voice calls with grants, late entry grants, relay on the payload channels and P_CLEAR.
  - `TSCC` also carries UDT short data between MSs and from a gateway, polls MSs for UDT short data and with USBD Poll Requests, keeps the call diversions of MSs and answers calls to a diverted MS with the diverted address, relays DGNA talkgroups, and sets up full-duplex and Ambient Listening Service calls.
- Data calls (`data`), without I/O or goroutines:
  - `Receiver` turns the bursts of one logical channel into data events: call start at the preamble or header, each complete packet with its CRCs checked, the responses heard, the ACK, NACK or SACK bursts to send, and call end with a reason. It takes confirmed, unconfirmed, short data and UDT packets at rate 1/2, 3/4 and 1, counts a block lost on air from the transport sequence number or the slot time, drops a repeated packet, keeps a confirmed packet for its retry, and joins a confirmed message from its fragments (FSN).
  - `Sender` sends a message one burst per 60 ms slot, with preamble CSBKs, polite or impolite channel access and a seeded random holdoff. For confirmed data it waits for the response, resends the blocks of a SACK or the whole packet after a NACK or no response, and keeps N(S) per link. With `Window` it sends up to 8 IP packets in one sliding window that one response confirms, and the `Receiver` answers such a window. `Output.ICMP` gives the ICMP error message of a message that failed, which `ip.AppendICMPPacket` builds.
- Network protocols, without I/O, with codecs generated by `dmrgen`:
  - `transport/dmrd`: the DMRD packet and the login, keepalive and other control messages of the MMDVM network protocol of MMDVMHost and DMRGateway, of the 2015 HomeBrew Repeater Protocol and of the link between MMDVMHost and DMRGateway; the RPTC configuration, talker alias and GPS messages; the DMR+ startup options; and OpenBridge, with the DMRE and control packets of FreeDMR.
  - `transport/dmrd/dmrdauth`: the login token (SHA-256) and the OpenBridge signatures (HMAC-SHA1 and keyed BLAKE2b), without allocation. `transport/dmrd` does not link this code.
  - `transport/dmrd/dmrdsession`: session machines without IO for the repeater and master sides of the MMDVM network protocol and of HBRP (login with the salted token, options, keepalive, timeout, retry with backoff, close), and the signatures and BCKA keepalive of OpenBridge.
  - `transport/ipsc`: the peer control and the voice and data packets of Motorola IP Site Connect, with the RTP header. A data packet gives its burst as a `layer2.RawBurst` and a `layer2.Hint`.
  - `transport/ipsc/ipscauth`: the HMAC-SHA1 trailer of authenticated IPSC and the key text of a MOTOTRBO or DMRlink configuration, without allocation. `transport/ipsc` does not link this code.
  - `transport/ipsc/ipscsession`: IPSC peer and master machines without IO: registration, peer list, peer to peer registration, keepalives, peer expiry and the authentication trailer.
  - `transport/hytera`: the 72-octet DMR frame, the heartbeat and keepalive, and the registration, start and redirect commands of Hytera IP Multi-Site Connect. A frame gives its burst as a `layer2.RawBurst` and a `layer2.Hint`.
  - `transport/mmdvm`: the serial protocol between MMDVMHost and an MMDVM or MMDVM_HS modem, for both roles: a stateless frame finder, the DMR data and lost frames with their `layer2.Hint`, version, status, configuration, frequency, mode, ACK and NAK, DMR start, abort, Short LC and ALOHA, and debug frames.
- FEC: BPTC(196,96), embedded LC BPTC, rate 3/4 trellis, RS(12,9), Golay(20,8), Golay(23,12), Golay(24,12), QR(16,7), Hamming codes and the CRCs. Each decoder reports its result in one `fec.Result`.
- Burst finding in a demodulated bit stream (`layer2/framer`).
- Vocoders (`vocoder`):
  - The AMBE+2 frame FEC. AMBE+2 itself is not included.
  - A `Codec` interface and a Codec2 codec (`vocoder/codec2`). `Decode` returns a `FrameReport` for each frame: whether the frame was good, lost, erased or invalid, whether the samples are voice, a tone, a repeat or silence, and the bits that the FEC corrected. `DecodeLost` fills a frame that never arrived.
  - AMBE+2 on a DVSI AMBE-3000R or AMBE-3003 chip (`vocoder/dvsi`), such as a ThumbDV, a DVstick or an AMBEserver. See [DVSI](#dvsi).
- A C API (`v2/capi`): libdmrgo, a shared library with the C form of the whole library (`dmrgo.h` and `dmrgo_gen.h`). It is a module of its own, so its cgo stays out of `v2`. `make -C v2/capi` builds it, and `make -C v2/capi static` builds `libdmrgo.a`, a static archive whose only global symbols are the C API. The C ABI has its own number, `DMRGO_ABI_VERSION`, which is independent of the Go module version and changes only when a C declaration breaks programs built against the older header; the soname `libdmrgo.so.N` follows it. Config, options and vtable structs start with `size_t size`, so they can get members at the end without a new ABI version. The library has no version of its own: `dmrgo_version` gives the v2 tag that it was built from, and the capi module is never tagged.
- No cgo in the `v2` module outside `vocoder/codec2`.
- No allocation on the hot path: see [Allocation](#allocation).

## Allocation

The hot path does not allocate. After construction, decoding, encoding, building, receiving and assembling bursts and packets make no heap allocations, with gc (Go 1.24 or later) and with TinyGo. Errors are static values, so a failed call does not allocate either.

On TinyGo this holds only when the values that you pass by pointer are not local variables of your function. TinyGo moves such a local to the heap, 12 to 96 bytes per call, for `layer2.BuildBurst(&lc, ...)`, `packet.AppendBursts(dst, &p, ...)`, `layer2.VoiceTransmitter.Reset(&opts)` and `packet.Assembler.Add` of a local block. Keep those values in package-level variables or in a struct that you allocate once:

```go
var lc pdu.FullLinkControl // package level, not a local of send

func send() (layer2.RawBurst, error) {
	return layer2.BuildBurst(&lc, 1, layer2.SyncSourceBS)
}
```

The table lists every place where the library can allocate. [v2/internal/allocgate](v2/internal/allocgate) generates it and fails when the README differs.

<!-- allocgate:begin -->

| What may allocate | When | Functions |
| --- | --- | ---: |
| The initialization of a package-level variable, such as a lookup table or a joined sentinel error | Once, when the program starts. | 6 |
| A String, GoString or Error method | When the text is built, for a person to read. | 292 |
| A function of internal/text, which formats the values of String and Error methods | When the text is built, for a person to read. | 7 |
| An exported package-level New function | Once, at construction: it allocates what it returns, sized up front. | 33 |
| An exported function or method that takes a slice dst and returns a slice of the same type | Only to grow dst when dst has no room. The caller sizes dst. A machine built from other machines may also pass each of them an unexported field sliced to [:0] that its New makes with capacity MaxX of that machine's package, whose fuzz test checks that bound. | 259 |
| An exported X with an exported twin AppendX or XInto that does not allocate | X allocates the slice, string or struct it returns; the twin writes into storage of the caller. | 9 |
| An exported X whose whole body is return Twin(nil, its parameters), where Twin allocates only to grow dst | X allocates the slice it returns; Twin with room in dst does not. | 3 |
| A type assertion or type switch to an interface type, and a goroutine that blocks | Once for each call site and dynamic type, as the runtime fills the cache of the assertion or switch, and when no sudog is free for a goroutine that blocks on a mutex or a channel. The runtime keeps both. | 5 |
| A method of an interface that the library declares for the caller to implement, such as the Read and Write of a dvsi.Port | When the method of the caller allocates. The code of the library on the path does not. | 2 |
| The runtime print functions, which the runtime calls on its debug paths | Never: they format into fixed buffers that the gate cannot prove are large enough. | 1 |

The counts are of exported functions and methods, for linux/amd64 with cgo. `DMRGO_README=write go test -run TestHeapAllocations ./internal/allocgate` in v2 writes the table.

<!-- allocgate:end -->

- Each exported function that a rule covers says in its doc when it allocates.
- Do not copy a machine or a `layer2.Burst` after first use. They keep pointers into their own storage, and `go vet` reports a copy.
- A `layer2.Burst` keeps its decoded `Data` until the next decode into it. `packet.Builder` and `packet.Assembler` keep their results the same way.
- Three checks keep this true, with no list of functions to maintain:
  - [v2/internal/allocgate](v2/internal/allocgate) follows the machine code of each exported function to the heap allocator.
  - The same package adds each allocation site that `tinygo build -print-allocs` reports for the pico and for `riscv-qemu`.
  - [v2/internal/allocs](v2/internal/allocs) measures allocations of the main calls with `testing.AllocsPerRun`, and under TinyGo with `runtime.MemStats`.

## Codec2

`vocoder/codec2` links the system libcodec2 (LGPL 2.1) with cgo. A program that imports it needs a C compiler, libcodec2 and pkg-config, for example `apt-get install libcodec2-dev pkg-config`. With `CGO_ENABLED=0` or `-tags codec2wasm` it runs an embedded WebAssembly build of libcodec2 instead, with no C toolchain:

| Platform | Engine |
| --- | --- |
| amd64 and arm64 on Linux, macOS, Windows, FreeBSD and NetBSD, and amd64 on DragonFly BSD, Solaris and illumos | wazero, compiled to machine code |
| Every other platform, such as 386 and arm, and every platform with `-tags codec2aot` | Go code that wasm2go translates from the same module (`go generate`) |
| `GOOS=js GOARCH=wasm` | The WebAssembly engine of the browser or Node |

`Encode` and `Decode` do not allocate with cgo, wazero or the translated code. A replacement module from `codec2.NewModule` always runs in wazero, which allocates on each call where it has no compiler, and so does the embedded module on an amd64 CPU without SSE4.1 unless the build uses `-tags codec2aot`. Build with `-tags nocodec2` to leave Codec2 out: `codec2.New` then returns `codec2.ErrUnavailable`. TinyGo builds always leave it out.

Create a codec with `codec2.New`:

```go
enc, err := codec2.New(nil)
```

| Item | Value |
| --- | --- |
| Modes | `Mode2400`, `Mode3200`, one 20 ms frame per 72-bit voice slot, MSB first, zero padded, no FEC |
| Default mode | `ModeAuto`: decode detects the mode of each call, encode uses `Mode2400` |
| Feature set ID | `0xC2` in the Full LC (self-assigned, compatible with QRadioLink) |
| Silence | `Silence()` is 20 ms of silence in the encode mode |
| Frame report | `CountsKnown` is true and no bits are corrected. `FrameInvalid` when a bit after the Codec2 frame is set. |
| Lost frames | `DecodeLost` outputs 20 ms of zero samples (`OutputSilence`): libcodec2 has no concealment for these modes |

Nothing on the air identifies the mode, so `ModeAuto` detects it from the padding: `Mode2400` leaves bytes 6 to 8 of each slot zero, and `Mode3200` only byte 8. Five clean frames (100 ms) decide the mode, and `DecodeMode()` reports it. Call `Reset()` at the start of each call. A decoder in a fixed mode decodes only that mode.

## DVSI

`vocoder/dvsi` builds and parses the packets of the DVSI AMBE-3000R and AMBE-3003 vocoder chips, and `dvsi.New` returns a `vocoder.Codec` that decodes and encodes DMR AMBE+2 frames on such a chip. The package does no I/O: the caller opens the port, which can be any `io.ReadWriter`.

```go
conn, err := (&net.Dialer{}).DialContext(ctx, "udp", "ambeserver.local:2460")
if err != nil {
	return err
}
codec, err := dvsi.New(conn, nil)
```

| Port | Settings |
| --- | --- |
| DVSI USB-3000 (DVstick 30), ThumbDV | Serial, 460800 baud (230400 for early ThumbDV units), 8N1, RTS/CTS |
| DVSI USB-3003 (DVstick 33) | Serial, 921600 baud, 8N1, RTS/CTS; channels 0 to 2 |
| AMBEserver | UDP, port 2460, one packet per datagram |
| Microcontroller UART | Any type with `Read` and `Write`; the package builds with TinyGo |

`New` resets the chip, turns on the parity field, reads the product name and version, sets the rate 3600 bit/s with 2450 bit/s of speech (rate index 33), has the chip put its decoder status in each speech packet (`PKT_SPCHFMT`), and clears the decoder flags of its channel (`PKT_DCMODE` 0x0000). `New` returns `dvsi.ErrSpeechFormat` when the chip rejects `PKT_SPCHFMT` and `dvsi.ErrStatus` when it rejects another field. The chip does the AMBE+2 FEC, so `Decode` takes the 72 on-air bits of a vocoder slot, as `layer2.RawBurst.VoiceSlots` returns them.

`Decode` reports the decoder status of the chip in its `FrameReport`:

| DCMODE_OUT flags | `Frame` | `Output` |
| --- | --- | --- |
| `TONE_FRAME`, `DATA_INVALID` 0 | `FrameGood` | `OutputTone` |
| `VOICE_ACTIVE` 0, `DATA_INVALID` 0 | `FrameGood` | `OutputSilence` |
| `VOICE_ACTIVE` 1, `DATA_INVALID` 1 | `FrameErased` | `OutputRepeat` |
| `VOICE_ACTIVE` 0, `DATA_INVALID` 1 | `FrameErased` | `OutputSilence` |
| `VOICE_ACTIVE` 1, `DATA_INVALID` 0 | `FrameGood` | `OutputVoice` |

The chip reports no bit counts, so `CountsKnown` is false. `DecodeLost` sets `LOST_FRAME`: the chip repeats the frame before, and after 2 repeats outputs comfort noise. The next `Decode` clears `LOST_FRAME`, and so does `Close` after a `DecodeLost`. The chip keeps `LOST_FRAME` until it is changed, so `New` clears it in case an earlier user of the channel left it set.

## TinyGo

The library builds with TinyGo 0.41 and 0.43 for microcontrollers (for example RP2040, RP2350, ESP32-C3) and for WebAssembly. CI uses TinyGo 0.43, which needs Go 1.25 to 1.27 installed. TinyGo 0.41 needs Go 1.24 to 1.26.

```sh
cd v2
tinygo flash -target=pico ./examples/tinygo
```

The example prints its counts on the serial console once a second.

- CI builds [v2/examples/tinygo](v2/examples/tinygo/main.go) for the Raspberry Pi Pico and runs the library tests with TinyGo.
- Decode and encode make no heap garbage at the default stack size of these targets (8 KB). TinyGo puts each local variable larger than 256 bytes on the heap, and the library has no such local on those paths. TinyGo also puts on the heap a local of the caller whose address goes into the library, so keep such values in package-level or heap storage, as [Allocation](#allocation) shows.
- `vocoder/codec2` always uses its stub, so `codec2.New` returns `codec2.ErrUnavailable`.

## Versioning

- The `v2` module follows semantic versioning. A new package, function, method, struct field, constant or enum value comes in a minor release. A change that breaks code that compiles comes only in a new major version.
- A minor release can add a value to an enum type, so give a switch over an enum a default case.
- The minimum Go version rises only in a minor release, and the release notes say so.
- libdmrgo, the C API, has its own ABI number (`DMRGO_ABI_VERSION`) and soname (`libdmrgo.so.N`). It is built from the `v2` tag, and `v2/capi` has no tags of its own. A change that breaks C code that compiles against `dmrgo.h`, or a program that links an older libdmrgo, gives a new ABI number and soname. v2.0.0 has ABI 1.
- A config, options or vtable struct of the C API whose first member is `size_t size` is sized. The caller sets `size` to `sizeof` the struct, and the library reads only the members that `size` covers and takes the rest as zero. Thus a minor release can add members at the end of such a struct without a new ABI number, and a program built against older headers works with the newer library. Other structs have a fixed layout.

## Tooling

- `go generate ./...` from the repository root runs four generators. Do not edit the files they write.
  - `cmd/tablegen` writes the FEC and CRC lookup tables (`tables_gen.go`).
  - `cmd/dmrgen` writes the decoder and encoder of each struct with `dmr` field tags (`*_decode_gen.go`).
  - `v2/internal/allocgate/apigen` writes the program that calls every export for the allocation gate (`allapi_gen.go`).
  - `v2/capi/internal/capigen` writes the C API of libdmrgo: the `*_gen*` files, `libdmrgo.map` and `abi.txt` in `v2/capi`.
- `env CGO_ENABLED=0 go run -C cmd/speccheck . -update-readme` runs the tests that `spec_matrix.yaml` maps to each ETSI clause and writes the table below. `-check-readme` fails when the table is out of date.

## ETSI TS 102 361 Specification Coverage

Each row is a clause or a subclause. A clause that is only partly implemented is split into subclause rows, so a procedure without code shows as unimplemented. A clause that is not in the table is not claimed.

<!-- BEGIN ETSI COVERAGE -->

*Generated by `cmd/speccheck -update-readme`. The status of each section comes from its tests.*

### Summary

| Metric | Count |
| --- | --- |
| Total sections | 233 |
| Verified (tests pass) | 199 |
| Failing (tests fail) | 0 |
| Untested (has source, no tests) | 3 |
| Unimplemented | 31 |

**Overall: 199/233 verified (85.4%)**

### ETSI TS 102 361-1: Air Interface Protocol

73 sections: 70 verified, 0 failing, 0 untested, 3 unimplemented.

| Section | Title | Status | Tests | Source Files |
| --- | --- | --- | --- | --- |
| 5.1.1 | Channel timing relationship | verified | 5/5 passed | `v2/trunk/ms.go`, `v2/trunk/ms_access.go`, `v2/csbk/sender.go`, `v2/csbk/responder.go` |
| 5.2.1.6 | Transmit admit criteria | verified | 2/2 passed | `v2/voice/transmitter.go`, `v2/enums/access.go` |
| 5.2.2.1 | Direct mode Channel Access | unimplemented | - | - |
| 5.2.2.2 | Repeater mode channel access, other than 5.2.2.2.4, 5.2.2.2.5 and 5.2.2.2.7 | unimplemented | - | - |
| 5.2.2.2.4 | MS TX_Wakeup_Message | verified | 2/2 passed | `v2/csbk/wakeup.go` |
| 5.2.2.2.5 | MS Not_In_Call channel access | verified | 1/1 passed | `v2/voice/transmitter.go` |
| 5.2.2.2.7 | MS My_Call channel access | verified | 1/1 passed | `v2/voice/transmitter.go` |
| 5.2.2.3 | Non-time critical CSBK ACK/NACK channel access | verified | 2/2 passed | `v2/csbk/sender.go` |
| 5.2.2.4 | TDMA direct mode channel access | unimplemented | - | - |
| 6.1 | Vocoder socket | verified | 21/21 passed | `v2/layer2/burst.go`, `v2/vocoder/frame.go`, `v2/internal/fec/ambe/ambe.go`, `v2/vocoder/codec.go`, `v2/layer2/voice_superframe.go`, `v2/internal/fec/golay/golay_23_12_7.go`, `v2/internal/fec/golay/golay_24_12_8.go`, `v2/enums/voice_burst_types.go` |
| 6.2 | Data and control | verified | 6/6 passed | `v2/layer2/burst.go`, `v2/layer2/pdu/pi_header.go`, `v2/layer2/voice_transmitter_options.go` |
| 6.3 | Common Announcement Channel burst | verified | 10/10 passed | `v2/layer2/cach.go`, `v2/layer2/pdu/tact.go`, `v2/bs/cach.go`, `v2/bs/repeater.go` |
| 6.4 | Reverse Channel | verified | 17/17 passed | `v2/layer2/reverse_channel.go`, `v2/layer2/burst.go` |
| 7.1.1 | Voice LC header | verified | 10/10 passed | `v2/layer2/burst.go`, `v2/layer2/pdu/full_link_control.go`, `v2/layer2/voice_superframe.go`, `v2/layer2/voice_transmitter_options.go` |
| 7.1.2 | Terminator with LC | verified | 7/7 passed | `v2/layer2/burst.go`, `v2/layer2/pdu/full_link_control.go`, `v2/layer2/voice_superframe.go` |
| 7.1.3 | Embedded signalling | verified | 26/26 passed | `v2/layer2/burst.go`, `v2/layer2/pdu/embedded_signalling.go`, `v2/layer2/embedded_lc.go`, `v2/layer2/embedded_lc_bytes.go`, `v2/layer2/voice_superframe.go`, `v2/layer2/voice_receiver.go`, `v2/layer2/position_tracker.go`, `v2/layer2/voice_transmitter_options.go` |
| 7.1.4 | Short Link Control in CACH | verified | 12/12 passed | `v2/layer2/short_lc_assembler.go`, `v2/layer2/short_lc_cach.go`, `v2/internal/fec/bptc/bptc_cach.go` |
| 7.2.1 | Control Signalling BlocK (CSBK) | verified | 4/4 passed | `v2/layer2/pdu/csbk.go` |
| 7.3 | Idle message | verified | 4/4 passed | `v2/layer2/pdu/pr_fill.go`, `v2/layer2/burst.go` |
| 7.4.1 | Multi Block Control (MBC) | verified | 17/17 passed | `v2/layer2/pdu/mbc.go`, `v2/layer2/burst.go` |
| 8.2.1 | Header block structure | verified | 7/7 passed | `v2/layer2/pdu/data_header.go`, `v2/layer2/pdu/data_header_bytes.go` |
| 8.2.2 | Data block structure | verified | 13/13 passed | `v2/layer2/packet/packet.go`, `v2/layer2/packet/assembler.go`, `v2/layer2/packet/bursts.go` |
| 8.2.2.3 | Response packet format | verified | 3/3 passed | `v2/layer2/packet/response.go` |
| 8.2.3.1 | Unified Single Block Data block structure | verified | 10/10 passed | `v2/layer2/pdu/usbd.go`, `v2/layer2/burst.go` |
| 9.1.1 | Synchronization (SYNC) PDU | verified | 19/19 passed | `v2/enums/sync_patterns.go`, `v2/layer2/burst.go`, `v2/layer2/decode_options.go`, `v2/layer2/framer/framer.go` |
| 9.1.2 | Embedded signalling (EMB) PDU | verified | 9/9 passed | `v2/layer2/pdu/embedded_signalling.go`, `v2/layer2/pdu/emb_info.go` |
| 9.1.3 | Slot Type (SLOT) PDU | verified | 9/9 passed | `v2/layer2/pdu/slot_type.go`, `v2/layer2/pdu/slot_type_info.go` |
| 9.1.4 | TACT PDU | verified | 6/6 passed | `v2/layer2/pdu/tact.go`, `v2/internal/fec/hamming/hamming_7_4_3.go` |
| 9.1.5 | Reverse Channel (RC) PDU | verified | 6/6 passed | `v2/layer2/pdu/reverse_channel.go` |
| 9.1.6 | Full Link Control (FULL LC) PDU | verified | 10/10 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/full_link_control_bytes.go` |
| 9.1.7 | Short Link Control (SHORT LC) PDU | verified | 14/14 passed | `v2/layer2/pdu/short_link_control.go`, `v2/enums/slco.go`, `v2/internal/crc/crc8.go` |
| 9.1.8 | Control Signalling Block (CSBK) PDU | verified | 7/7 passed | `v2/layer2/pdu/csbk.go`, `v2/layer2/pdu/csbk_bytes.go` |
| 9.1.9 | Pseudo Random Fill Bit (PR FILL) PDU | verified | 7/7 passed | `v2/layer2/pdu/pr_fill.go` |
| 9.2.2 | Rate ¾ coded packet Data (R_3_4_DATA) PDU | verified | 10/10 passed | `v2/layer2/pdu/rate34_data.go`, `v2/layer2/pdu/confirmed_block.go` |
| 9.2.6 | Unconfirmed data packet Header (U_HEAD) PDU | verified | 3/3 passed | `v2/layer2/pdu/data_header.go` |
| 9.2.7 | Rate ½ coded packet Data (R_1_2_DATA) PDU | verified | 7/7 passed | `v2/layer2/pdu/rate12_data.go`, `v2/layer2/pdu/confirmed_block.go` |
| 9.2.9 | Proprietary Header (P_HEAD) PDU | verified | 3/3 passed | `v2/layer2/pdu/data_header.go` |
| 9.2.13 | Unified Data Transport Header (UDT_HEAD) PDU | verified | 3/3 passed | `v2/layer2/pdu/data_header.go` |
| 9.2.15 | Rate 1 coded packet Data (R_1_DATA) PDU | verified | 9/9 passed | `v2/layer2/pdu/rate1_data.go`, `v2/layer2/pdu/confirmed_block.go` |
| 9.3.1 | Colour Code (CC) | verified | 4/4 passed | `v2/enums/color_code.go`, `v2/layer2/pdu/embedded_signalling.go`, `v2/layer2/pdu/slot_type.go` |
| 9.3.2 | Pre-emption and power control Indicator (PI) | verified | 1/1 passed | `v2/layer2/pdu/embedded_signalling.go` |
| 9.3.3 | LC Start/Stop (LCSS) | verified | 3/3 passed | `v2/enums/lcss.go` |
| 9.3.4 | EMB parity | verified | 4/4 passed | `v2/internal/fec/quadraticresidue/quadratic_residue_16_7_6.go`, `v2/layer2/pdu/embedded_signalling.go` |
| 9.3.5 | Feature set ID (FID) | verified | 9/9 passed | `v2/enums/fid.go`, `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/csbk.go` |
| 9.3.6 | Data Type | verified | 6/6 passed | `v2/enums/data_type.go`, `v2/layer2/data.go` |
| 9.3.10 | Protect Flag (PF) | verified | 2/2 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/csbk.go` |
| 9.3.11 | Full Link Control Opcode (FLCO) | verified | 1/1 passed | `v2/enums/flco.go` |
| 9.3.17 | Data Packet Format (DPF) | verified | 1/1 passed | `v2/layer2/pdu/data_header.go` |
| 9.3.18 | SAP identifier (SAP) | verified | 2/2 passed | `v2/layer2/pdu/data_header.go`, `v2/enums/sap.go` |
| 9.3.19 | Logical Link ID (LLID) | verified | 3/3 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/csbk.go` |
| 9.3.32 | Control Signalling BlocK Opcode (CSBKO) | verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| B.1.1 | BPTC (196,96) | verified | 11/11 passed | `v2/internal/fec/bptc/bptc_196_96.go` |
| B.2.1 | Variable length BPTC for embedded signalling | verified | 15/15 passed | `v2/internal/fec/bptc/bptc_embedded_lc.go` |
| B.2.2 | Single Burst Variable length BPTC | verified | 14/14 passed | `v2/internal/fec/bptc/bptc_single_burst.go` |
| B.2.3 | Variable length BPTC for CACH signalling | verified | 13/13 passed | `v2/internal/fec/bptc/bptc_cach.go` |
| B.2.4 | Rate ¾ Trellis code | verified | 12/12 passed | `v2/internal/fec/trellis/trellis_3_4.go` |
| B.2.5 | Rate 1 coded data | verified | 1/1 passed | `v2/layer2/burst.go` |
| B.3.1 | Golay (20,8) | verified | 3/3 passed | `v2/internal/fec/golay/golay_20_8_7.go` |
| B.3.2 | Quadratic residue (16,7,6) | verified | 2/2 passed | `v2/internal/fec/quadraticresidue/quadratic_residue_16_7_6.go` |
| B.3.3 | Hamming (17,12,3) | verified | 1/1 passed | `v2/internal/fec/bptc/bptc_196_96.go` |
| B.3.4 | Hamming (13,9,3), Hamming (15,11,3), and Hamming (16,11,4) | verified | 4/4 passed | `v2/internal/fec/bptc/bptc_196_96.go`, `v2/internal/fec/bptc/bptc_embedded_lc.go` |
| B.3.5 | Hamming (7,4,3) | verified | 2/2 passed | `v2/internal/fec/hamming/hamming_7_4_3.go` |
| B.3.6 | Reed-Solomon (12,9) | verified | 22/22 passed | `v2/internal/fec/reedsolomon/reed_solomon_12_9_4.go` |
| B.3.7 | 8-bit CRC calculation | verified | 7/7 passed | `v2/internal/crc/crc8.go` |
| B.3.8 | CRC-CCITT calculation | verified | 5/5 passed | `v2/internal/crc/crc_ccitt.go` |
| B.3.9 | 32-bit CRC calculation | verified | 9/9 passed | `v2/internal/crc/crc32.go` |
| B.3.10 | CRC-9 calculation | verified | 9/9 passed | `v2/internal/crc/crc9.go` |
| B.3.11 | 5-bit Checksum (CS) calculation | verified | 9/9 passed | `v2/internal/crc/checksum5.go`, `v2/layer2/embedded_lc.go` |
| B.3.12 | Data Type CRC Mask | verified | 26/26 passed | `v2/internal/crc/crc_ccitt.go`, `v2/layer2/pdu/full_link_control.go`, `v2/layer2/mmdvmhost.go`, `v2/layer2/pdu/codeword_bytes.go`, `v2/layer2/mmdvmhost_seal.go` |
| B.3.13 | 7-bit CRC calculation | verified | 7/7 passed | `v2/internal/crc/crc7.go` |
| B.4 | Interleaving | verified | 8/8 passed | `v2/layer2/cach_interleave.go` |
| D | Idle and Null message bit definition | verified | 5/5 passed | `v2/layer2/pdu/pr_fill.go`, `v2/layer2/voice_superframe.go` |
| E | Transmit bit order | verified | 14/14 passed | `v2/layer2/transmit_order.go`, `v2/layer2/raw_burst.go` |

### ETSI TS 102 361-2: DMR Voice and Generic Services and Facilities

45 sections: 43 verified, 0 failing, 0 untested, 2 unimplemented.

| Section | Title | Status | Tests | Source Files |
| --- | --- | --- | --- | --- |
| 5.1.1.6.1 | BS Channel Authorization Procedures | verified | 6/6 passed | `v2/bs/chanauth.go`, `v2/bs/types.go` |
| 5.1.1.6.2 | MS Channel Authorization Procedures | verified | 4/4 passed | `v2/voice/chanauth.go` |
| 5.1.2 | Feature Not Supported (FNS) signalling | verified | 2/2 passed | `v2/csbk/responder.go` |
| 5.2.2.3 | MS Individual call channel access | verified | 3/3 passed | `v2/csbk/requester.go` |
| 5.4.2 | Inband positioning data service | verified | 1/1 passed | `v2/layer2/pdu/full_link_control.go` |
| 5.4.3 | Inband talker alias data service | verified | 5/5 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer3/talker_alias_data_format.go`, `v2/layer2/pdu/talker_alias.go` |
| 6.1 | Transmit timeout | verified | 2/2 passed | `v2/constants/constants.go` |
| 6.2 | TDMA direct mode wide area timing | verified | 18/18 passed | `v2/csbk/timing.go` |
| 6.3.1 | Reverse Channel Transmitter Interrupt: Repeater Mode | unimplemented | - | - |
| 6.3.2 | Reverse Channel Transmitter Interrupt: Direct Mode | unimplemented | - | - |
| 6.3.3 | Voice Interrupt | verified | 2/2 passed | `v2/voice/transmitter.go` |
| 6.3.4 | Emergency Interrupt | verified | 1/1 passed | `v2/voice/transmitter.go` |
| 6.3.5 | Sending Interrupt Request and Retry Mechanism | verified | 5/5 passed | `v2/voice/interrupter.go` |
| 7.1.1 | Full Link Control PDUs | verified | 3/3 passed | `v2/layer2/pdu/full_link_control.go` |
| 7.1.1.1 | Group Voice Channel User LC PDU | verified | 6/6 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/lc_bytes.go` |
| 7.1.1.2 | Unit to Unit Voice Channel User LC PDU | verified | 6/6 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/lc_bytes.go` |
| 7.1.1.3 | GPS Info LC PDU | verified | 1/1 passed | `v2/layer2/pdu/full_link_control.go` |
| 7.1.1.4 | Talker Alias header LC PDU | verified | 3/3 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer3/talker_alias_data_format.go` |
| 7.1.1.5 | Talker Alias block LC PDU | verified | 2/2 passed | `v2/layer2/pdu/full_link_control.go` |
| 7.1.2 | Control Signalling BlocK (CSBK) PDUs | verified | 6/6 passed | `v2/layer2/pdu/csbk.go`, `v2/layer2/pdu/csbk_addresses.go`, `v2/layer2/pdu/csbk_bytes.go` |
| 7.1.2.1 | BS Outbound Activation CSBK PDU | verified | 2/2 passed | `v2/layer2/pdu/csbk.go` |
| 7.1.2.2 | Unit to Unit Voice Service Request CSBK PDU | verified | 2/2 passed | `v2/layer2/pdu/csbk.go` |
| 7.1.2.3 | Unit to Unit Voice Service Answer Response CSBK PDU | verified | 2/2 passed | `v2/layer2/pdu/csbk.go` |
| 7.1.2.4 | Negative Acknowledge Response CSBK PDU | verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| 7.1.2.5 | Preamble CSBK PDU | verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| 7.1.2.6 | Channel Timing CSBK PDU | verified | 2/2 passed | `v2/layer2/pdu/csbk.go` |
| 7.1.3 | Short Link Control PDUs | verified | 18/18 passed | `v2/layer2/pdu/short_link_control.go`, `v2/enums/slco.go`, `v2/enums/activity_id.go`, `v2/layer2/pdu/address_hash.go` |
| 7.2.1 | Service Options | verified | 4/4 passed | `v2/layer3/service_options.go` |
| 7.2.2 | Answer Response | verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| 7.2.3 | Reason Code | verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| 7.2.4 | Service Type | verified | 2/2 passed | `v2/layer2/pdu/csbk.go` |
| 7.2.5 | Source Type | verified | 5/5 passed | `v2/layer3/source_type.go`, `v2/layer2/pdu/csbk.go` |
| 7.2.6 | Additional Information Field | verified | 5/5 passed | `v2/layer3/additional_information_field.go`, `v2/layer2/pdu/csbk.go` |
| 7.2.7 | CSBK Blocks to Follow (CBF) | verified | 4/4 passed | `v2/layer2/pdu/csbk.go`, `v2/csbk/preambles.go` |
| 7.2.8 | Identifier (ID) | verified | 3/3 passed | `v2/layer2/pdu/full_link_control.go`, `v2/layer2/pdu/csbk.go` |
| 7.2.15 | Position Error | verified | 3/3 passed | `v2/layer3/position_accuracy.go` |
| 7.2.16 | Longitude | verified | 5/5 passed | `v2/layer2/pdu/full_link_control.go` |
| 7.2.17 | Latitude | verified | 5/5 passed | `v2/layer2/pdu/full_link_control.go` |
| 7.2.18 | Talker Alias Data Format | verified | 3/3 passed | `v2/layer3/talker_alias_data_format.go` |
| 7.2.19 | Talker Alias Data Length | verified | 1/1 passed | `v2/layer2/pdu/full_link_control.go` |
| 7.2.20 | Tier 2 Reverse Channel (RC) Command | verified | 9/9 passed | `v2/enums/rc_command.go`, `v2/layer2/pdu/reverse_channel.go` |
| A | Timers and constants in DMR | verified | 4/4 passed | `v2/constants/constants.go` |
| B.1 | Full Link Control Opcode List | verified | 2/2 passed | `v2/layer2/pdu/full_link_control.go` |
| B.2 | CSBK Opcode List | verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| B.3 | Short Link Control Opcode List | verified | 8/8 passed | `v2/enums/slco.go`, `v2/enums/activity_id.go`, `v2/layer2/pdu/short_link_control.go` |

### ETSI TS 102 361-3: DMR Data Protocol

17 sections: 17 verified, 0 failing, 0 untested, 0 unimplemented.

| Section | Title | Status | Tests | Source Files |
| --- | --- | --- | --- | --- |
| 5.2 | IP error messages | verified | 6/6 passed | `v2/ip/icmp.go`, `v2/ip/errors.go`, `v2/data/sender.go` |
| 5.3 | Unconfirmed data DLL bearer service | verified | 6/6 passed | `v2/layer2/pdu/data_header.go`, `v2/data/event.go`, `v2/data/message.go`, `v2/data/receiver.go`, `v2/data/sender.go` |
| 5.4.1 | Confirmed IP Data Types/PDUs | verified | 3/3 passed | `v2/layer2/pdu/data_header.go` |
| 5.4.2 | Confirmed IP Data SDL | verified | 8/8 passed | `v2/data/errors.go`, `v2/data/event.go`, `v2/data/message.go`, `v2/data/receiver.go`, `v2/data/sender.go` |
| 5.4.3 | Confirmed data MSCs | verified | 17/17 passed | `v2/data/receiver.go`, `v2/data/sender.go` |
| 5.4.4 | Sliding window confirmed data | verified | 6/6 passed | `v2/data/errors.go`, `v2/data/receiver.go`, `v2/data/sender.go` |
| 5.5 | UDP/IPv4 data | verified | 9/9 passed | `v2/layer2/pdu/udp_ipv4_compressed.go`, `v2/enums/said.go`, `v2/enums/daid.go`, `v2/enums/spid.go`, `v2/enums/dpid.go`, `v2/ip/datagram.go`, `v2/ip/errors.go`, `v2/ip/ipv4.go`, `v2/ip/port.go` |
| 5.6 | UDP/IPv4 header compression | verified | 6/6 passed | `v2/layer2/pdu/udp_ipv4_compressed.go`, `v2/layer2/packet/assembler.go`, `v2/enums/said.go`, `v2/enums/daid.go`, `v2/enums/spid.go`, `v2/enums/dpid.go`, `v2/ip/port.go` |
| 6.1 | Defined Data | verified | 6/6 passed | `v2/layer2/pdu/data_header.go`, `v2/enums/dd_format.go`, `v2/shortdata/errors.go`, `v2/shortdata/text.go` |
| 6.2 | Raw data | verified | 1/1 passed | `v2/layer2/pdu/data_header.go` |
| 6.3 | Status/precoded data | verified | 1/1 passed | `v2/layer2/pdu/data_header.go` |
| 6.4 | Short data confirmed response | verified | 1/1 passed | `v2/layer2/pdu/data_header.go` |
| 7.1.1 | Full Link Control (FULL LC) PDUs | verified | 1/1 passed | `v2/layer2/pdu/full_link_control.go` |
| 7.1.1.1 | Terminator Data Link Control PDU | verified | 1/1 passed | `v2/layer2/pdu/full_link_control.go`, `v2/enums/flco.go` |
| 7.2 | UDP/IPv4 Compressed Header | verified | 2/2 passed | `v2/layer2/pdu/udp_ipv4_compressed.go` |
| A | PDP timers and constants in DMR | verified | 2/2 passed | `v2/constants/constants.go` |
| B.1 | PDP Full Link Control Opcode list | verified | 1/1 passed | `v2/enums/flco.go`, `v2/layer2/pdu/full_link_control.go` |

### ETSI TS 102 361-4: DMR Trunking Protocol

98 sections: 69 verified, 0 failing, 3 untested, 26 unimplemented.

| Section | Title | Status | Tests | Source Files |
| --- | --- | --- | --- | --- |
| 5.1 | The Use of the CACH | verified | 7/7 passed | `v2/layer2/pdu/short_link_control.go`, `v2/layer2/cach.go`, `v2/layer2/pdu/syscode.go` |
| 5.2 | Tier III Signalling | verified | 4/4 passed | `v2/layer2/pdu/csbk.go`, `v2/layer2/burst.go` |
| 5.3 | Modes of Control Channel | verified | 2/2 passed | `v2/layer2/burst.go` |
| 5.4 | CSBK/MBC/UDT/USBD Block Structure | verified | 4/4 passed | `v2/layer2/pdu/csbk.go`, `v2/layer2/pdu/mbc.go`, `v2/layer2/pdu/data_header.go` |
| 6.2 | Random Access Procedures | verified | 13/13 passed | `v2/layer2/pdu/csbk.go`, `v2/constants/constants.go`, `v2/enums/service_kind.go`, `v2/trunk/ms.go`, `v2/trunk/ms_access.go` |
| 6.3 | Control Channel Acquisition and Retention | verified | 8/8 passed | `v2/trunk/control.go`, `v2/trunk/follower.go`, `v2/trunk/plan.go`, `v2/trunk/site.go` |
| 6.4.1 | Registration | verified | 2/2 passed | `v2/trunk/ms.go`, `v2/trunk/tscc.go` |
| 6.4.2 | MS Parameter Volatility | verified | 2/2 passed | `v2/trunk/ms.go` |
| 6.4.3 | Action on Confirmation of a TSCC | verified | 3/3 passed | `v2/trunk/ms.go` |
| 6.4.4.1 | Registration by Random Access | verified | 6/6 passed | `v2/trunk/ms.go`, `v2/trunk/ms_access.go`, `v2/trunk/tscc.go`, `v2/layer3/random_access_options.go` |
| 6.4.4.1.5 | Challenge and Response Authentication | unimplemented | - | - |
| 6.4.4.1.7 | Registration Demand Received During Random Access Registration | unimplemented | - | - |
| 6.4.4.1.11 | Registration with MS Authentication | unimplemented | - | - |
| 6.4.4.1.13 | Talkgroup Subscription and Talkgroup Attachment | unimplemented | - | - |
| 6.4.5 | Mass Re-registration | verified | 5/5 passed | `v2/trunk/ms_services.go`, `v2/trunk/tscc_services.go`, `v2/layer2/pdu/broadcast_massreg.go` |
| 6.4.6 | De-registration | verified | 1/1 passed | `v2/trunk/ms.go` |
| 6.4.7 | Power Save | verified | 2/2 passed | `v2/trunk/ms_services.go`, `v2/trunk/tscc_services.go` |
| 6.4.8 | Authentication Procedures | verified | 9/9 passed | `v2/trunk/trunkauth/auth.go`, `v2/trunk/ms_access.go`, `v2/trunk/tscc.go` |
| 6.4.9 | MS Stun/Revive | verified | 4/4 passed | `v2/trunk/ms_services.go`, `v2/trunk/tscc_services.go`, `v2/trunk/services.go`, `v2/trunk/trunkauth/auth.go` |
| 6.4.10 | MS Kill | verified | 3/3 passed | `v2/trunk/ms_services.go`, `v2/trunk/tscc_services.go`, `v2/trunk/services.go`, `v2/trunk/trunkauth/auth.go` |
| 6.4.11 | IP Connection Advice | verified | 1/1 passed | `v2/trunk/ms_services.go`, `v2/trunk/tscc_services.go` |
| 6.4.12 | Unsolicited MS Radio Check | verified | 2/2 passed | `v2/trunk/ms_access.go` |
| 6.4.13 | Supplementary_User Data Service | verified | 2/2 passed | `v2/trunk/ms_services.go`, `v2/trunk/tscc_services.go` |
| 6.4.14 | MS Power Control and MS De-key | verified | 5/5 passed | `v2/trunk/ms_services.go`, `v2/trunk/tscc_services.go`, `v2/enums/rc_command.go` |
| 6.4.15 | Transmit Interrupt for MS De-key | verified | 2/2 passed | `v2/trunk/ms_services.go`, `v2/trunk/tscc_services.go` |
| 6.5.1.0 | Format of the Appended Data | unimplemented | - | - |
| 6.5.1.1 | UDT Block Structure | verified | 2/2 passed | `v2/trunk/control.go` |
| 6.5.1.2 | UDT Content for Services Carried on the Outbound Channel | verified | 2/2 passed | `v2/trunk/control.go`, `v2/trunk/follower.go` |
| 6.5.1.3 | UDT Mechanism for the Inbound Channel | unimplemented | - | - |
| 6.6.1.1.1 | Availability of calling MS | unimplemented | - | - |
| 6.6.1.1.2 | Availability of Called Party as part of a Call | verified | 3/3 passed | `v2/trunk/ms_access.go`, `v2/trunk/tscc.go` |
| 6.6.1.1.3 | General MS Radio Check | verified | 1/1 passed | `v2/trunk/ms_access.go` |
| 6.6.1.2 | Call Cancellation | verified | 16/16 passed | `v2/trunk/ms.go`, `v2/trunk/ms_access.go`, `v2/trunk/ms_udt.go`, `v2/trunk/tscc.go`, `v2/trunk/tscc_services.go`, `v2/trunk/tscc_udt.go` |
| 6.6.1.3 | Acknowledgements Sent to Calling MS | verified | 3/3 passed | `v2/trunk/ms_access.go`, `v2/trunk/tscc.go` |
| 6.6.1.4 | Called Party (MS) Answering Mechanism (OACSU) | verified | 2/2 passed | `v2/trunk/ms_access.go`, `v2/trunk/tscc.go` |
| 6.6.1.4.1 | FOACSU - Called Party Accepts the Call | unimplemented | - | - |
| 6.6.1.4.2 | FOACSU - Called Party Rejects the Call | unimplemented | - | - |
| 6.6.1.5 | Maintenance of Call Progress Waiting Timers | verified | 2/2 passed | `v2/trunk/ms.go`, `v2/trunk/tscc.go` |
| 6.6.1.6 | Payload Channel Assignment to a Payload Channel | verified | 5/5 passed | `v2/layer2/pdu/csbk.go`, `v2/trunk/follower.go`, `v2/trunk/tscc.go` |
| 6.6.1.7 | Calls to ALLMSID, ALLMSIDL and ALLMSIDZ | untested | - | `v2/trunk/follower.go` |
| 6.6.2.1 | Voice Call Procedures for the TSCC (single-part set-up) | verified | 2/2 passed | `v2/trunk/tscc.go` |
| 6.6.2.1.2 | TSCC Response to Multi-part Voice Call Set-up | unimplemented | - | - |
| 6.6.2.1.5 | Availability Check for Voice Calls connected through Gateways | unimplemented | - | - |
| 6.6.2.2 | Voice Call Procedures for MS (single-part set-up) | verified | 6/6 passed | `v2/trunk/ms.go`, `v2/trunk/ms_access.go` |
| 6.6.2.2.3 | Initiating a Multi-part Voice Call Service | unimplemented | - | - |
| 6.6.2.2.4 | Response to the Multi-part Voice Service Request | unimplemented | - | - |
| 6.6.2.2.9 | Call Set-up MSC that also Transfers Supplementary_user Data | unimplemented | - | - |
| 6.6.2.3 | Procedures for the Voice Payload Channel | verified | 5/5 passed | `v2/trunk/follower.go`, `v2/trunk/ms_access.go`, `v2/trunk/tscc.go` |
| 6.6.2.4 | Late Entry | untested | - | `v2/trunk/control.go`, `v2/trunk/tscc.go` |
| 6.6.2.5 | Preferential Call Notification | unimplemented | - | - |
| 6.6.2.6 | Ambient Listening Service (ALS) | verified | 2/2 passed | `v2/trunk/tscc_udt.go`, `v2/trunk/tscc.go`, `v2/trunk/ms_udt.go`, `v2/trunk/ms.go`, `v2/trunk/ms_access.go` |
| 6.6.3.1 | Packet Data Call Procedures for the TSCC | unimplemented | - | - |
| 6.6.3.2 | Packet Data Call Procedures for MS (single-part set-up) | verified | 1/1 passed | `v2/trunk/ms.go`, `v2/trunk/ms_access.go` |
| 6.6.3.2.3 | Initiating a Multi-part Packet Data Service | unimplemented | - | - |
| 6.6.3.2.4 | Response to the Multi-part Packet Data Service Request | unimplemented | - | - |
| 6.6.3.2.6 | Availability Check to the Called MS (packet data) | unimplemented | - | - |
| 6.6.3.3.1 | TS Procedures for the Packet Data Payload Channel | unimplemented | - | - |
| 6.6.3.3.2 | MS Procedures for the Packet Data Payload Channel | untested | - | `v2/trunk/ms_access.go` |
| 6.6.4 | UDT Short Data Message Procedure | verified | 4/4 passed | `v2/trunk/tscc_udt.go`, `v2/trunk/tscc.go`, `v2/trunk/ms_udt.go`, `v2/trunk/ms.go`, `v2/trunk/ms_access.go`, `v2/trunk/control.go` |
| 6.6.5 | UDT Short Data Polling Service | verified | 3/3 passed | `v2/trunk/tscc_udt.go`, `v2/trunk/tscc.go`, `v2/trunk/ms_udt.go`, `v2/trunk/ms.go`, `v2/trunk/ms_access.go` |
| 6.6.6 | Status Call Service | verified | 3/3 passed | `v2/trunk/ms_services.go`, `v2/trunk/tscc_services.go`, `v2/trunk/services.go`, `v2/layer3/status_options.go` |
| 6.6.7 | Call Diversion | verified | 4/4 passed | `v2/trunk/tscc_udt.go`, `v2/trunk/tscc.go`, `v2/trunk/ms_udt.go`, `v2/trunk/ms.go`, `v2/trunk/ms_access.go`, `v2/layer3/udt_service_options.go` |
| 6.6.8 | Dynamic Group Numbering Assignment Service | verified | 2/2 passed | `v2/trunk/tscc_udt.go`, `v2/trunk/tscc.go`, `v2/trunk/ms_udt.go`, `v2/trunk/ms.go`, `v2/trunk/ms_access.go`, `v2/trunk/follower.go`, `v2/trunk/control.go` |
| 6.6.9 | Full-Duplex MS to MS Voice Call Procedures | verified | 2/2 passed | `v2/trunk/tscc_udt.go`, `v2/trunk/tscc.go`, `v2/trunk/ms_udt.go`, `v2/trunk/ms.go`, `v2/trunk/ms_access.go`, `v2/trunk/follower.go` |
| 6.6.10 | Full-Duplex MS to MS Packet Data Call Procedures | verified | 2/2 passed | `v2/trunk/tscc_udt.go`, `v2/trunk/tscc.go`, `v2/trunk/ms_udt.go`, `v2/trunk/ms.go`, `v2/trunk/ms_access.go`, `v2/trunk/follower.go` |
| 6.6.11 | Unified Single Block Data Polling Service | verified | 3/3 passed | `v2/trunk/tscc_udt.go`, `v2/trunk/tscc.go`, `v2/trunk/ms_udt.go`, `v2/trunk/ms.go`, `v2/trunk/ms_access.go`, `v2/trunk/appended_formats.go` |
| 7.1.1 | Control Signalling Block (CSBK/MBC/UDT) PDUs | verified | 17/17 passed | `v2/layer2/pdu/csbk.go`, `v2/layer2/pdu/mbc.go`, `v2/layer2/pdu/data_header.go`, `v2/enums/announcement_type.go` |
| 7.1.1.1 | TSCC Outbound channel CSBK/MBC/UDT | verified | 10/10 passed | `v2/layer2/pdu/csbk.go`, `v2/layer2/pdu/mbc.go`, `v2/layer3/cdef_parms.go` |
| 7.1.2 | Short Link Control PDUs | verified | 3/3 passed | `v2/layer2/pdu/short_link_control.go`, `v2/enums/slco.go` |
| 7.2 | Layer 3 Information Element Coding, other than the subclauses below | verified | 15/15 passed | `v2/layer3/cdef_parms.go`, `v2/enums/announcement_type.go`, `v2/enums/ack_reason.go`, `v2/enums/protect_kind.go`, `v2/layer3/random_access_options.go`, `v2/enums/udt_format.go`, `v2/layer2/pdu/broadcast_parms.go` |
| 7.2.9 | Digits | unimplemented | - | - |
| 7.2.13.3 | Service_Options for a Call Diversion Service Request | verified | 1/1 passed | `v2/layer3/udt_service_options.go` |
| 7.2.13.5 | Service_Options for an Include Call Service Request | unimplemented | - | - |
| 7.2.13.6 | Service_Options for a Status Transport Request | verified | 1/1 passed | `v2/layer3/status_options.go` |
| 7.2.13.7 | Service_Options for the UDT Short Data Service | verified | 1/1 passed | `v2/layer3/udt_service_options.go` |
| 7.2.13.8 | Service Options for the Supplementary Data Service | unimplemented | - | - |
| 7.2.13.9 | Service Options for a UDT Short Data Polling Request | verified | 1/1 passed | `v2/layer3/udt_service_options.go` |
| 7.2.14 | Service_Options_Mirror | unimplemented | - | - |
| 7.2.16 | POL_FMT | unimplemented | - | - |
| 7.2.19.2 | Specify Call Timer Parameters (CallTimer_Parms) | verified | 2/2 passed | `v2/layer2/pdu/broadcast_parms.go` |
| 7.2.19.3 | Vote Now Advice (Vote_Now) | verified | 2/2 passed | `v2/layer2/pdu/broadcast_parms.go` |
| 7.2.19.4 | Broadcast Local Time (Local_Time) | verified | 2/2 passed | `v2/layer2/pdu/broadcast_parms.go` |
| 7.2.19.5 | Broadcast Mass Registration (MassReg) | verified | 2/2 passed | `v2/layer2/pdu/broadcast_parms.go` |
| 7.2.19.8 | Broadcast General Site Parameters information | verified | 2/2 passed | `v2/layer2/pdu/broadcast_parms.go` |
| 7.2.31 | STATUS | unimplemented | - | - |
| A | Timers, constants levels and addresses | verified | 5/5 passed | `v2/constants/constants.go` |
| B.1 | CSBK/MBC/UDT Opcode List | verified | 1/1 passed | `v2/layer2/pdu/csbk.go` |
| B.2 | Short Link Control Opcode List | verified | 1/1 passed | `v2/enums/slco.go` |
| B.3.1 | Appended Data Binary Format | verified | 1/1 passed | `v2/trunk/appended.go` |
| B.3.2 | Appended Data Addressing Format | verified | 1/1 passed | `v2/trunk/appended.go` |
| B.3.3 | Appended Data BCD Format | verified | 2/2 passed | `v2/trunk/appended.go` |
| B.3.4 | Appended Data ISO 7 bit character set Format | verified | 2/2 passed | `v2/trunk/appended.go` |
| B.3.5 | Appended Data ISO 8 bit Character Format | verified | 2/2 passed | `v2/trunk/appended.go` |
| B.3.6 | Appended Data NMEA (IEC 61162-1) Format | verified | 1/1 passed | `v2/trunk/appended.go`, `v2/trunk/appended_formats.go` |
| B.3.7 | UDT DMR IP Format | verified | 1/1 passed | `v2/trunk/appended.go` |
| B.3.8 | Appended Data Unicode 16 bit UTF-16BE Character Format | verified | 2/2 passed | `v2/trunk/appended.go` |
| B.3.9 | Appended Data Mixed Format | verified | 2/2 passed | `v2/trunk/appended.go` |
| B.3.10 | Location Information Protocol (LIP) | verified | 9/9 passed | `v2/trunk/appended.go`, `v2/trunk/appended_formats.go`, `v2/lip/control.go`, `v2/lip/element.go`, `v2/lip/errors.go`, `v2/lip/lip.go`, `v2/lip/report.go` |

<!-- END ETSI COVERAGE -->
