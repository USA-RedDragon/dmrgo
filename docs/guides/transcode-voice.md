# Vocoders

## Codec2

`vocoder/codec2` runs libcodec2 (LGPL 2.1) compiled to WebAssembly by default, with or without cgo. Such a build needs no C compiler and no system libcodec2. `codec2.Backend` names the backend of a build.

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

Nothing on the air identifies the mode, so `ModeAuto` detects it from the padding: `Mode2400` leaves bytes 6 to 8 of each slot zero, and `Mode3200` only byte 8. `ModeAuto` sets the mode after five clean frames (100 ms), and `DecodeMode()` returns it. Call `Reset()` at the start of each call. A decoder in a fixed mode decodes only that mode.

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
