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

<!-- dmrgo:code file=v2/examples/transcoder/main.go symbol=receive -->
```go
func receive(bursts []layer2.RawBurst) (call, error) {
	var c call
	var rx layer2.VoiceReceiver
	b := layer2.NewBurst()
	for i, raw := range bursts {
		if err := b.DecodeFromBytes(raw); err != nil {
			return c, fmt.Errorf("burst %d: %w", i, err)
		}
		_, ok, err := rx.Receive(b, nil)
		if err != nil {
			return c, fmt.Errorf("burst %d: %w", i, err)
		}
		if ok {
			c.embLCs++
		}
		switch b.Kind {
		case layer2.KindData:
			if lc, isLC := b.Data.(*pdu.FullLinkControl); isLC && b.SlotType.DataType == enums.DataTypeVoiceLCHeader {
				l := *lc
				c.lc, c.cc = &l, b.SlotType.ColorCode
			}
		case layer2.KindVoice:
			if s, _, found := layer2.SyncSourceOf(b.SyncPattern); found {
				c.source = s
			}
			c.voice++
			frames, _ := b.AMBEFrames()
			c.frames = append(c.frames, frames[:]...)
			c.slots = append(c.slots, b.VoiceSlots[:]...)
		case layer2.KindUnknown, layer2.KindReverseChannel:
		}
	}
	if c.lc == nil {
		return c, errors.New("no Voice LC Header")
	}
	return c, nil
}
```
<!-- /dmrgo:code -->

### 2. Correct each AMBE+2 frame and encode it again

A vocoder slot holds one AMBE+2 frame with its FEC. [vocoder.DecodeAMBE72](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder#DecodeAMBE72) corrects the slot and returns the frame and a [fec.Result](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/fec#Result). [vocoder.EncodeAMBE72](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder#EncodeAMBE72) writes the FEC again. A slot that had bit errors re-encodes to the corrected bits.

<!-- dmrgo:code file=v2/examples/transcoder/main.go symbol=checkFEC -->
```go
func checkFEC(w io.Writer, c call) error {
	corrected, bad, clean := 0, 0, 0
	for _, slot := range c.slots {
		info, res := vocoder.DecodeAMBE72(slot)
		corrected += res.ErrorsCorrected
		if res.Uncorrectable {
			bad++
		}
		if s, err := vocoder.EncodeAMBE72(info); err == nil && s == slot {
			clean++
		}
	}
	_, _ = fmt.Fprintf(w, "AMBE72 FEC: %d bits corrected, %d uncorrectable frames, %d of %d frames re-encode unchanged\n",
		corrected, bad, clean, len(c.slots))

	slot, err := vocoder.EncodeAMBE72(c.frames[0])
	if err != nil {
		return err
	}
	for _, k := range []int{0, 30, 60} {
		slot[k/8] ^= 0x80 >> (k % 8)
	}
	info, res := vocoder.DecodeAMBE72(slot)
	_, _ = fmt.Fprintf(w, "AMBE72 with 3 flipped bits: %d corrected, frame restored %t\n", res.ErrorsCorrected, info == c.frames[0])
	return nil
}
```
<!-- /dmrgo:code -->

### 3. Build a new call from the frames

A [VoiceTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceTransmitter) takes AMBE+2 frames with `AddFrames`, or vocoder slots of any codec with `AddSlots`. Give it the silence slot of the codec, to pad the last superframe.

<!-- dmrgo:code file=v2/examples/transcoder/main.go symbol=transmit -->
```go
func transmit(lc pdu.FullLinkControl, cc enums.ColorCode, src layer2.SyncSource, silence [9]byte, add func(*layer2.VoiceTransmitter) ([]layer2.RawBurst, error)) ([]layer2.RawBurst, error) {
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: cc, Source: src, Silence: silence})
	if err != nil {
		return nil, err
	}
	out := tx.Headers()
	bursts, err := add(tx)
	if err != nil {
		return nil, err
	}
	tail, err := tx.Terminate()
	if err != nil {
		return nil, err
	}
	return append(append(out, bursts...), tail...), nil
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/examples/transcoder/main.go symbol=rebuild -->
```go
func rebuild(w io.Writer, c call) error {
	rebuilt, err := transmit(*c.lc, c.cc, c.source, vocoder.AMBESilenceSlot(),
		func(tx *layer2.VoiceTransmitter) ([]layer2.RawBurst, error) { return tx.AddFrames(c.frames...) })
	if err != nil {
		return err
	}
	back, err := receive(rebuilt)
	if err != nil {
		return err
	}
	same := len(back.frames) >= len(c.frames)
	for i := 0; same && i < len(c.frames); i++ {
		same = back.frames[i] == c.frames[i]
	}
	_, _ = fmt.Fprintf(w, "rebuilt: %d bursts, %d embedded LCs, AMBE frames equal %t\n", len(rebuilt), back.embLCs, same)
	return nil
}
```
<!-- /dmrgo:code -->

### 4. Encode audio with Codec2

[codec2.New](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2#New) returns a [vocoder.Codec](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder#Codec). It returns `codec2.ErrUnavailable` in a build without libcodec2, so check for it. The `codec2Call` function opens the codec, calls the helpers of steps 4 to 6, and compares the received slots with the sent slots.

<!-- dmrgo:code file=v2/examples/transcoder/main.go symbol=codec2Call -->
```go
func codec2Call(w io.Writer, captured *pdu.FullLinkControl) error {
	enc, err := codec2.New(nil)
	if errors.Is(err, codec2.ErrUnavailable) {
		_, _ = fmt.Fprintln(w, "codec2 unavailable, skipping")
		return nil
	}
	if err != nil {
		return err
	}
	defer func() { _ = enc.Close() }()

	slots, err := encodeSweep(enc, 50)
	if err != nil {
		return err
	}
	bursts, err := sendSlots(enc, captured, slots)
	if err != nil {
		return err
	}

	c, err := receive(bursts)
	if err != nil {
		return err
	}
	same := len(c.slots) >= len(slots)
	for i := 0; same && i < len(slots); i++ {
		same = c.slots[i] == slots[i]
	}
	rdst, rsrc, _ := c.lc.Addresses()
	_, _ = fmt.Fprintf(w, "codec2: %d frames -> %d bursts, rx FID %#x, %d -> %d, %d embedded LCs, slots equal %t\n",
		len(slots), len(bursts), uint8(c.lc.FID), rsrc, rdst, c.embLCs, same)

	return decodeSlots(w, c.slots)
}
```
<!-- /dmrgo:code -->

`Encode` takes `FrameSamples` samples of 8 kHz audio and returns one vocoder slot.

<!-- dmrgo:code file=v2/examples/transcoder/main.go symbol=encodeSweep -->
```go
func encodeSweep(enc *codec2.Codec, frames int) ([][9]byte, error) {
	pcm := sweep(frames * enc.FrameSamples())
	slots := make([][9]byte, 0, frames)
	for off := 0; off < len(pcm); off += enc.FrameSamples() {
		s, err := enc.Encode(pcm[off : off+enc.FrameSamples()])
		if err != nil {
			return nil, err
		}
		slots = append(slots, s)
	}
	return slots, nil
}
```
<!-- /dmrgo:code -->

### 5. Send the Codec2 slots in a call

A Codec2 call carries the FID of the codec in its Full LC, so a receiver can separate it from an AMBE+2 call. Set the FID from `FID()`, and use `Silence()` as the silence slot.

<!-- dmrgo:code file=v2/examples/transcoder/main.go symbol=sendSlots -->
```go
func sendSlots(enc *codec2.Codec, captured *pdu.FullLinkControl, slots [][9]byte) ([]layer2.RawBurst, error) {
	dst, src, _ := captured.Addresses()
	lc := pdu.FullLinkControl{FLCO: enums.FLCOGroupVoiceChannelUser, FID: enc.FID()}
	if err := lc.SetAddresses(dst, src); err != nil {
		return nil, fmt.Errorf("cannot set addresses %d -> %d: %w", src, dst, err)
	}
	return transmit(lc, 1, layer2.SyncSourceMS, enc.Silence(),
		func(tx *layer2.VoiceTransmitter) ([]layer2.RawBurst, error) { return tx.AddSlots(slots...) })
}
```
<!-- /dmrgo:code -->

### 6. Decode the slots to audio

`Decode` appends the samples of one slot and returns a [vocoder.FrameReport](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder#FrameReport) for it.

<!-- dmrgo:code file=v2/examples/transcoder/main.go symbol=decodeSlots -->
```go
func decodeSlots(w io.Writer, slots [][9]byte) error {
	dec, err := codec2.New(nil)
	if err != nil {
		return err
	}
	defer func() { _ = dec.Close() }()
	var out []int16
	good := 0
	for _, s := range slots {
		var rep vocoder.FrameReport
		if out, rep, err = dec.Decode(out, s); err != nil {
			return err
		}
		if rep.Frame == vocoder.FrameGood {
			good++
		}
	}
	_, _ = fmt.Fprintf(w, "codec2 decode: %d samples, %d good frames, audible %t\n", len(out), good, rms(out) > 500)
	return nil
}
```
<!-- /dmrgo:code -->

### 7. Decode AMBE+2 on a DVSI chip

[dvsi.New](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi#New) takes the port that you opened and sets up the chip. The Codec that it returns decodes and encodes like any other `vocoder.Codec`. The chip does the AMBE+2 FEC, so give `Decode` the raw slot from `VoiceSlots`, not a frame from `AMBEFrames`.

<!-- dmrgo:code file=v2/vocoder/dvsi/example_test.go symbol=ExampleNew -->
```go
func ExampleNew() {
	var d net.Dialer
	conn, err := d.DialContext(context.Background(), "udp", "127.0.0.1:2460")
	if err != nil {
		log.Fatal(err)
	}
	c, err := dvsi.New(conn, nil)
	if err != nil {
		_ = conn.Close()
		log.Fatal(err)
	}
	pcm, _, err := c.Decode(nil, vocoder.AMBESilenceSlot())
	fmt.Println(c.Product(), len(pcm), "samples", err)
	_ = c.Close()
	_ = conn.Close()
}
```
<!-- /dmrgo:code -->

## Output

<!-- dmrgo:output example=transcoder -->
```text
capture: 79 bursts, 77 voice bursts, 231 AMBE frames, 13 embedded LCs, 3191868 -> 9990
AMBE72 FEC: 3 bits corrected, 0 uncorrectable frames, 230 of 231 frames re-encode unchanged
AMBE72 with 3 flipped bits: 3 corrected, frame restored true
rebuilt: 80 bursts, 13 embedded LCs, AMBE frames equal true
codec2: 50 frames -> 20 bursts, rx FID 0xc2, 3191868 -> 9990, 3 embedded LCs, slots equal true
codec2 decode: 8640 samples, 54 good frames, audible true
```
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
| Field | Type | Default | Description |
|---|---|---|---|
| `Mode` | `Mode` | `Mode2400` (2400) or `c.det.add(&c.frame)` | Mode is the Codec2 bit rate. Zero is ModeAuto: the decoder detects Mode2400 or Mode3200 from the padding of the frames of each call, and the encoder uses Mode2400. |
| `FID` | `enums.FID` | `DefaultFID` (194) | FID is the Feature set ID to send in the Full LC. Zero selects DefaultFID. FID 0 is the ETSI feature set, which repeaters treat as AMBE, so it cannot be selected. |
| `WASM` | `*Module` | zero value | WASM is a WebAssembly build of libcodec2 from NewModule that the WebAssembly backend runs in place of the module embedded in the package. Nil selects the embedded module. The native backend of -tags codec2native links the system libcodec2 and ignores WASM. |
<!-- /dmrgo:config -->

| Item | Value |
| --- | --- |
| Modes | `Mode2400` (<!-- dmrgo:const pkg=vocoder/codec2 name=Mode2400 -->`2400`<!-- /dmrgo:const -->) and `Mode3200` (<!-- dmrgo:const pkg=vocoder/codec2 name=Mode3200 -->`3200`<!-- /dmrgo:const -->), one 20 ms frame per voice slot, MSB first, zero padded, no FEC |
| Samples per frame | <!-- dmrgo:const pkg=vocoder/codec2 name=FrameSamples -->`160`<!-- /dmrgo:const --> |
| Default mode | `ModeAuto`: decode detects the mode of each call, encode uses `Mode2400` |
| Feature set ID | <!-- dmrgo:const pkg=vocoder/codec2 name=DefaultFID -->`194`<!-- /dmrgo:const --> in the Full LC (self-assigned, compatible with QRadioLink) |
| Silence | `Silence()` is 20 ms of silence in the encode mode |
| Frame report | `CountsKnown` is true and no bits are corrected. `FrameInvalid` when a bit after the Codec2 frame is set. |
| Lost frames | `DecodeLost` outputs 20 ms of zero samples (`OutputSilence`): libcodec2 has no concealment for these modes |

Nothing on the air identifies the mode, so `ModeAuto` detects it from the padding: `Mode2400` leaves bytes 6 to 8 of each slot zero, and `Mode3200` only byte 8. `ModeAuto` sets the mode after five clean frames (100 ms), and `DecodeMode()` returns it. Call `Reset()` at the start of each call. A decoder in a fixed mode decodes only that mode.

The errors of `codec2.New`, `Encode` and `Decode`:

<!-- dmrgo:errors pkg=vocoder/codec2 type=Codec -->
| Error | Returned by | Meaning |
|---|---|---|
| [`codec2.ErrClosed`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2#ErrClosed) | `Codec.Close`, `Codec.Decode`, `Codec.DecodeLost` | ErrClosed is returned when a closed Codec is used, and by New for a closed Module. |
| [`codec2.ErrUnavailable`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2#ErrUnavailable) | `Codec.Decode`, `Codec.Encode` | ErrUnavailable is returned when the package was built without libcodec2 (-tags nocodec2 or TinyGo), and wrapped by New when libcodec2 cannot make a state for a mode or has another frame size for it, and by the WebAssembly backend when its module cannot be compiled or run. |
<!-- /dmrgo:errors -->

## DVSI

`vocoder/dvsi` builds and parses the packets of the DVSI AMBE-3000R and AMBE-3003 vocoder chips, and `dvsi.New` returns a `vocoder.Codec` that decodes and encodes DMR AMBE+2 frames on such a chip. The package does no I/O: the caller opens the port, which can be any `io.ReadWriter`.

| Port | Connection |
| --- | --- |
| DVSI USB-3000 (DVstick 30), ThumbDV | Serial, 8N1, RTS/CTS |
| DVSI USB-3003 (DVstick 33) | Serial, 8N1, RTS/CTS; channels 0 to <!-- dmrgo:const pkg=vocoder/dvsi name=MaxChannel -->`2`<!-- /dmrgo:const --> |
| AMBEserver | UDP, one packet per datagram |
| Microcontroller UART | Any type with `Read` and `Write`; the package builds with TinyGo |

The package doc of [vocoder/dvsi](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi) gives the baud rate of each serial device and the UDP port of an AMBEserver.

The config of `dvsi.New`:

<!-- dmrgo:config pkg=vocoder/dvsi type=Config -->
| Field | Type | Default | Description |
|---|---|---|---|
| `Channel` | `int` | zero value | Channel is the vocoder channel of the chip, 0 to MaxChannel. The AMBE-3000R has only channel 0. |
| `Timeout` | `time.Duration` | `DefaultTimeout` (500ms) | Timeout is the longest wait for each reply. Zero selects DefaultTimeout. |
| `NoReset` | `bool` | zero value | NoReset leaves out the reset of the chip, for a second Codec on another channel of an AMBE-3003 that is already in use. |
| `NoParity` | `bool` | not machine-readable; see the description | NoParity turns the parity fields off. By default every packet carries a parity field and the chip drops packets with a wrong parity byte. |
<!-- /dmrgo:config -->

`New` resets the chip, turns on the parity field, reads the product name and version, sets the DMR rate of 3600 bit/s with 2450 bit/s of speech, has the chip put its decoder status in each speech packet (`PKT_SPCHFMT`), and clears the decoder flags of its channel (`PKT_DCMODE`). `New` returns `dvsi.ErrSpeechFormat` when the chip rejects `PKT_SPCHFMT` and `dvsi.ErrStatus` when it rejects another field. The chip does the AMBE+2 FEC, so `Decode` takes the <!-- dmrgo:const pkg=vocoder/dvsi name=FrameBits -->`72`<!-- /dmrgo:const --> on-air bits of a vocoder slot, as `layer2.RawBurst.VoiceSlots` returns them.

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

<!-- dmrgo:code file=v2/vocoder/dvsi/example_test.go symbol=ExampleAppendChannel -->
```go
func ExampleAppendChannel() {
	slot := vocoder.AMBESilenceSlot()
	pkt, err := dvsi.AppendChannel(nil, 0, slot[:], dvsi.FrameBits, true)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("% x\n", pkt)

	reply := []byte{0x61, 0x00, 0x02, 0x00, 0x3F, 0x00}
	p, n, err := dvsi.Parse(reply, false)
	if err != nil {
		log.Fatal(err)
	}
	status, ok := p.Response(dvsi.FieldParityMode)
	fmt.Println(n, len(status), status[0], ok)
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:output test=vocoder/dvsi.ExampleAppendChannel -->
```text
61 00 0d 01 01 48 b9 e8 81 52 61 73 00 2a 6b 2f bb
6 1 0 true
```
<!-- /dmrgo:output -->

The errors of the Codec:

<!-- dmrgo:errors pkg=vocoder/dvsi type=Codec -->
| Error | Returned by | Meaning |
|---|---|---|
| [`dmrgo.ErrNilArgument`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2#ErrNilArgument) | `New` | ErrNilArgument is returned by a function or method of the library that returns an error when a pointer, func or interface argument it needs is nil. |
| [`dvsi.ErrChannel`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi#ErrChannel) | `New` | ErrChannel is returned for a channel outside 0 to MaxChannel. |
| [`dvsi.ErrClosed`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi#ErrClosed) | `Codec.Close`, `Codec.Decode`, `Codec.Encode` | ErrClosed is returned when a closed Codec is used. |
| [`dvsi.ErrSamples`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi#ErrSamples) | `Codec.Encode` | ErrSamples is returned for a number of speech samples outside MinSamples to MaxSamples, and by Codec.Encode for other than FrameSamples samples. |
| [`dvsi.ErrSpeechFormat`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi#ErrSpeechFormat) | `New` | ErrSpeechFormat is returned by New when the chip rejects the PKT_SPCHFMT field that turns on its decoder status. |
| [`dvsi.ErrStatus`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi#ErrStatus) | `New`, `Codec.Close` | ErrStatus is returned when the chip reports an error in a response field. |
| [`dvsi.ErrTimeout`](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi#ErrTimeout) | `New`, `Codec.Close`, `Codec.Decode`, `Codec.Encode` | ErrTimeout is returned when the chip does not reply within Config.Timeout. |
<!-- /dmrgo:errors -->

## Related guides

- [Clean up bursts for a hotspot](clean-up-bursts.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
- [Follow calls on both slots of a bit stream](follow-both-slots.md)
