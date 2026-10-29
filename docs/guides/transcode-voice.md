# Transcode voice: AMBE+2 FEC, Codec2, DVSI

This guide shows how to correct and re-encode the AMBE+2 frames of a call, send and receive a Codec2 call, and decode AMBE+2 on a DVSI chip, with the reference for the Codec2 backends and the DVSI ports.

## Goal

Take the voice frames out of a DMR call, put them back in a new call, and turn voice slots into audio samples and back.

## What you need

- Go and a checkout of dmrgo. The example runs offline.
- The voice capture in `v2/layer2/testdata`.
- The packages [vocoder](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder), [vocoder/codec2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2) and [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2).
- For DVSI only: a DVSI AMBE-3000R or AMBE-3003 chip, on a USB serial port, a UART or an AMBEserver, and the package [vocoder/dvsi](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi). The example does not use a chip.

Run the example from the top of the repository:

```sh
go run -C v2 ./examples/transcoder
```

## Steps

### 1. Collect the voice of a call

Decode each burst. Keep the LC and the color code of the Voice LC Header, the SYNC source of the voice bursts, the AMBE+2 frames from `AMBEFrames`, and the raw vocoder slots from `VoiceSlots`.

<!-- dmrgo:code file=v2/examples/transcoder/main.go lines=46-82 -->
<!-- /dmrgo:code -->

### 2. Correct each AMBE+2 frame and encode it again

A vocoder slot holds one AMBE+2 frame with its FEC. [vocoder.DecodeAMBE72](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder#DecodeAMBE72) corrects the slot and returns the frame and a [fec.Result](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/fec#Result). [vocoder.EncodeAMBE72](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder#EncodeAMBE72) writes the FEC again. A slot that had bit errors re-encodes to the corrected bits.

<!-- dmrgo:code file=v2/examples/transcoder/main.go lines=114-136 -->
<!-- /dmrgo:code -->

### 3. Build a new call from the frames

A [VoiceTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceTransmitter) takes AMBE+2 frames with `AddFrames`, or vocoder slots of any codec with `AddSlots`. Give it the silence slot of the codec, to pad the last superframe.

<!-- dmrgo:code file=v2/examples/transcoder/main.go lines=84-99 -->
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/examples/transcoder/main.go lines=138-151 -->
<!-- /dmrgo:code -->

### 4. Encode audio with Codec2

[codec2.New](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2#New) returns a [vocoder.Codec](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder#Codec). It returns `codec2.ErrUnavailable` in a build without libcodec2, so check for it. `Encode` takes `FrameSamples` samples of 8 kHz audio and returns one vocoder slot.

<!-- dmrgo:code file=v2/examples/transcoder/main.go lines=173-192 -->
<!-- /dmrgo:code -->

### 5. Send the Codec2 slots in a call

A Codec2 call carries the FID of the codec in its Full LC, so a receiver can separate it from an AMBE+2 call. Set the FID from `FID()`, and use `Silence()` as the silence slot.

<!-- dmrgo:code file=v2/examples/transcoder/main.go lines=194-203 -->
<!-- /dmrgo:code -->

### 6. Decode the slots to audio

`Decode` appends the samples of one slot and returns a [vocoder.FrameReport](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder#FrameReport) for it.

<!-- dmrgo:code file=v2/examples/transcoder/main.go lines=217-233 -->
<!-- /dmrgo:code -->

### 7. Decode AMBE+2 on a DVSI chip

[dvsi.New](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi#New) takes the port that you opened and sets up the chip. The Codec that it returns decodes and encodes like any other `vocoder.Codec`. The chip does the AMBE+2 FEC, so give `Decode` the raw slot from `VoiceSlots`, not a frame from `AMBEFrames`.

<!-- dmrgo:code file=v2/vocoder/dvsi/example_test.go lines=15-28 -->
<!-- /dmrgo:code -->

## Output

<!-- dmrgo:output example=transcoder -->
<!-- /dmrgo:output -->

A build with `-tags nocodec2` prints `codec2 unavailable, skipping` in place of the Codec2 lines.

## How it works

Each voice burst carries vocoder slots in fixed places around the SYNC or embedded signalling field. The burst codec treats the slots as opaque bits. An AMBE+2 slot is a frame after `EncodeAMBE72`, and another codec supplies its own slots. That is how one VoiceTransmitter sends both AMBE+2 and Codec2. (ETSI TS 102 361-1 clause 5.1.2)

A codec implements [vocoder.Codec](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder#Codec), with `Encode`, `Decode`, and `DecodeLost` for a frame that never arrived. A transcoder is a Decode on one codec and an Encode on the other, with the frame timing of the call.

To transcode a live call, feed the bursts to a [voice Receiver](../machines/voice-receiver.md). Each voice event carries the burst with its slots, the FEC result of each slot, and the count of voice bursts lost before it. Give `DecodeLost` one call for each lost frame. Send the new slots with a [voice Transmitter](../machines/voice-transmitter.md).

## Codec2

`vocoder/codec2` runs libcodec2 (LGPL 2.1) compiled to WebAssembly by default, with or without cgo. Such a build needs no C compiler and no system libcodec2. [codec2.Backend](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2#Backend) names the backend of a build.

| Build | Backend |
| --- | --- |
| Default, with or without cgo | `wasm`: the embedded WebAssembly build of libcodec2 |
| `-tags codec2native` | `native`: the system libcodec2 with cgo. The build needs cgo, a C compiler, libcodec2 and pkg-config, for example `apt-get install libcodec2-dev pkg-config`. |
| `-tags nocodec2`, and every TinyGo build | `stub`: `codec2.New` returns `codec2.ErrUnavailable` |

The WebAssembly backend runs in one of these engines:

| Platform | Engine |
| --- | --- |
| amd64 and arm64 on Linux, macOS, Windows, FreeBSD and NetBSD, and amd64 on DragonFly BSD, Solaris and illumos | wazero, compiled to machine code |
| Every other platform, such as 386 and arm, and every platform with `-tags codec2aot` | Go code that wasm2go translates from the same module (`go generate`) |
| `GOOS=js GOARCH=wasm` | The WebAssembly engine of the browser or Node |

`Encode` and `Decode` do not allocate with the native backend, wazero or the translated code. A replacement module from `codec2.NewModule` always runs in wazero, which allocates on each call where it has no compiler, and so does the embedded module on an amd64 CPU without SSE4.1 unless the build uses `-tags codec2aot`.

Earlier versions used the native backend in every cgo build, and the tag `codec2wasm` selected the WebAssembly backend there. The tag is still accepted and changes nothing. libdmrgo follows the same rule, so build it with `GOFLAGS=-tags=codec2native make -C v2/capi` for the native backend. Its header comes from the native build, so `DMRGO_CODEC2_BACKEND` is `"native"` in every build. The decoded samples of the WebAssembly backend can differ from the native ones by small amounts; the encoded frames are the same.

The config of `codec2.New`:

<!-- dmrgo:config pkg=vocoder/codec2 type=Config -->
<!-- /dmrgo:config -->

| Item | Value |
| --- | --- |
| Modes | `Mode2400` (<!-- dmrgo:const pkg=vocoder/codec2 name=Mode2400 --><!-- /dmrgo:const -->) and `Mode3200` (<!-- dmrgo:const pkg=vocoder/codec2 name=Mode3200 --><!-- /dmrgo:const -->), one 20 ms frame per voice slot, MSB first, zero padded, no FEC |
| Samples per frame | <!-- dmrgo:const pkg=vocoder/codec2 name=FrameSamples --><!-- /dmrgo:const --> |
| Default mode | `ModeAuto`: decode detects the mode of each call, encode uses `Mode2400` |
| Feature set ID | <!-- dmrgo:const pkg=vocoder/codec2 name=DefaultFID --><!-- /dmrgo:const --> in the Full LC (self-assigned, compatible with QRadioLink) |
| Silence | `Silence()` is 20 ms of silence in the encode mode |
| Frame report | `CountsKnown` is true and no bits are corrected. `FrameInvalid` when a bit after the Codec2 frame is set. |
| Lost frames | `DecodeLost` outputs 20 ms of zero samples (`OutputSilence`): libcodec2 has no concealment for these modes |

Nothing on the air identifies the mode, so `ModeAuto` detects it from the padding: `Mode2400` leaves bytes 6 to 8 of each slot zero, and `Mode3200` only byte 8. `ModeAuto` sets the mode after five clean frames (100 ms), and `DecodeMode()` returns it. Call `Reset()` at the start of each call. A decoder in a fixed mode decodes only that mode.

The errors of `codec2.New`, `Encode` and `Decode`:

<!-- dmrgo:errors pkg=vocoder/codec2 type=Codec -->
<!-- /dmrgo:errors -->

## DVSI

`vocoder/dvsi` builds and parses the packets of the DVSI AMBE-3000R and AMBE-3003 vocoder chips, and `dvsi.New` returns a `vocoder.Codec` that decodes and encodes DMR AMBE+2 frames on such a chip. The package does no I/O: the caller opens the port, which can be any `io.ReadWriter`.

| Port | Connection |
| --- | --- |
| DVSI USB-3000 (DVstick 30), ThumbDV | Serial, 8N1, RTS/CTS |
| DVSI USB-3003 (DVstick 33) | Serial, 8N1, RTS/CTS; channels 0 to <!-- dmrgo:const pkg=vocoder/dvsi name=MaxChannel --><!-- /dmrgo:const --> |
| AMBEserver | UDP, one packet per datagram |
| Microcontroller UART | Any type with `Read` and `Write`; the package builds with TinyGo |

The package doc of [vocoder/dvsi](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi) gives the baud rate of each serial device and the UDP port of an AMBEserver.

The config of `dvsi.New`:

<!-- dmrgo:config pkg=vocoder/dvsi type=Config -->
<!-- /dmrgo:config -->

`New` resets the chip, turns on the parity field, reads the product name and version, sets the DMR rate of 3600 bit/s with 2450 bit/s of speech, has the chip put its decoder status in each speech packet (`PKT_SPCHFMT`), and clears the decoder flags of its channel (`PKT_DCMODE`). `New` returns `dvsi.ErrSpeechFormat` when the chip rejects `PKT_SPCHFMT` and `dvsi.ErrStatus` when it rejects another field. The chip does the AMBE+2 FEC, so `Decode` takes the <!-- dmrgo:const pkg=vocoder/dvsi name=FrameBits --><!-- /dmrgo:const --> on-air bits of a vocoder slot, as `layer2.RawBurst.VoiceSlots` returns them.

`Decode` reports the decoder status of the chip in its `FrameReport`:

| DCMODE_OUT flags | `Frame` | `Output` |
| --- | --- | --- |
| `TONE_FRAME`, `DATA_INVALID` 0 | `FrameGood` | `OutputTone` |
| `VOICE_ACTIVE` 0, `DATA_INVALID` 0 | `FrameGood` | `OutputSilence` |
| `VOICE_ACTIVE` 1, `DATA_INVALID` 1 | `FrameErased` | `OutputRepeat` |
| `VOICE_ACTIVE` 0, `DATA_INVALID` 1 | `FrameErased` | `OutputSilence` |
| `VOICE_ACTIVE` 1, `DATA_INVALID` 0 | `FrameGood` | `OutputVoice` |

The chip reports no bit counts, so `CountsKnown` is false. `DecodeLost` sets `LOST_FRAME`: the chip repeats the frame before, and after 2 repeats outputs comfort noise. The next `Decode` clears `LOST_FRAME`, and so does `Close` after a `DecodeLost`. The chip keeps `LOST_FRAME` until it is changed, so `New` clears it in case an earlier user of the channel left it set.

[dvsi.AppendChannel](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi#AppendChannel) and [dvsi.Parse](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi#Parse) build and read the packets without a Codec, for a program that runs its own protocol loop:

<!-- dmrgo:code file=v2/vocoder/dvsi/example_test.go lines=34-47 -->
<!-- /dmrgo:code -->

<!-- dmrgo:output test=vocoder/dvsi.ExampleAppendChannel -->
<!-- /dmrgo:output -->

The errors of the Codec:

<!-- dmrgo:errors pkg=vocoder/dvsi type=Codec -->
<!-- /dmrgo:errors -->

## Related guides

- [Clean up bursts for a hotspot](clean-up-bursts.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
- [Follow calls on both slots of a bit stream](follow-both-slots.md)
