# Follow calls on both slots of a bit stream

This guide shows how to find bursts in a demodulated BS outbound bit stream and follow the voice calls on both time slots.

## Goal

Lock on to a noisy bit stream, split its bursts by time slot, and report each call with its IDs, talker alias and GPS position.

## What you need

- Go and a checkout of dmrgo. The example runs offline.
- A demodulated BS outbound bit stream, packed in bytes, first bit in the most significant bit. The example makes its own stream from the voice capture in `v2/layer2/testdata` on TS1 and a generated group call on TS2, with junk bits in front and random bit errors.
- The packages [layer2/framer](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/framer), [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2) and [layer2/pdu](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu).

Run the example from the top of the repository:

```sh
go run -C v2 ./examples/scanner
```

## Steps

### 1. Make a framer for a stream with a CACH

A BS outbound stream has a CACH in front of each burst, and the TC bit of the CACH gives the time slot of the burst. Set `CACH` in the [framer.Config](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/framer#Config). Give the decoder the color code of the channel in [DecodeOptions](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#DecodeOptions), so that a burst of another color code returns an error.

<!-- dmrgo:code file=v2/examples/scanner/main.go lines=57-65 -->
```go
r := xorshift(2)
f, err := framer.New(&framer.Config{CACH: true})
if err != nil {
	return err
}
chans := [2]*channel{{name: "TS1"}, {name: "TS2"}}
cc := enums.ColorCode(colorCode)
opts := layer2.DecodeOptions{ColorCode: &cc}
var b layer2.Burst
```
<!-- /dmrgo:code -->

### 2. Write the stream and decode each burst

Give the stream to [Framer.Write](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/framer#Framer.Write) in pieces of any size, with the number of bits in each piece. Write returns the bursts that the piece completes, each with its bit offset and its slot. Decode each burst with `DecodeFromBytesWith` and pass it to the state of its slot. The example cuts the stream at random lengths to show that a burst can start at any bit.

<!-- dmrgo:code file=v2/examples/scanner/main.go lines=68-87 -->
```go
for len(stream) > 0 {
	chunk := min(len(stream), 1+int(r.next()%63))
	k := min(8*chunk, left)
	left -= k
	for _, fb := range f.Write(stream[:chunk], k) {
		if first < 0 {
			first = fb.Offset
		}
		n++
		if fb.Slot != enums.Slot1 && fb.Slot != enums.Slot2 {
			continue
		}
		if err := b.DecodeFromBytesWith(fb.Data, &opts); err != nil {
			fmt.Fprintf(w, "%v burst at bit %d: %v\n", fb.Slot, fb.Offset, err)
			continue
		}
		chans[fb.Slot-1].receive(w, &b)
	}
	stream = stream[chunk:]
}
```
<!-- /dmrgo:code -->

### 3. Start and end calls on each slot

Keep one [VoiceReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceReceiver) for each slot. A Voice LC Header starts a call and a Terminator with LC ends it. A voice burst on a slot without a call is a late entry: the scanner started in the middle of the call. `Receive` places each voice burst in its superframe and returns the embedded LC when bursts B to E complete it. `AMBEFrames` returns the AMBE+2 frames and the FEC result of each voice burst, which [fec.BERCalculator](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/fec#BERCalculator) adds up.

<!-- dmrgo:code file=v2/examples/scanner/main.go symbol=channel.receive -->
```go
func (c *channel) receive(w io.Writer, b *layer2.Burst) {
	if b.Kind == layer2.KindData {
		dt := b.SlotType.DataType
		if lc, ok := b.Data.(*pdu.FullLinkControl); ok && !b.FEC.PDU.Uncorrectable {
			if dt == enums.DataTypeVoiceLCHeader {
				c.start(w, lc, dt.String())
			}
			if dt == enums.DataTypeTerminatorWithLC {
				fmt.Fprintf(w, "%s call end: %s, %d voice bursts\n", c.name, describe(lc), c.voice)
				c.inCall = false
				c.ta.Reset()
			}
		}
	}
	lc, ok, err := c.rx.Receive(b, nil)
	if err != nil && !errors.Is(err, layer2.ErrEmbeddedLCSequence) {
		c.lcErrors++
	}
	if b.Kind != layer2.KindVoice {
		return
	}
	if !c.inCall {
		fmt.Fprintf(w, "%s late entry at voice burst %s\n", c.name, b.VoiceBurst)
		c.inCall = true
	}
	c.voice++
	c.positions[b.VoiceBurst]++
	_, res := b.AMBEFrames()
	c.ambe.Add(res)
	if ok {
		c.embedded(w, &lc)
	}
}
```
<!-- /dmrgo:code -->

### 4. Read the embedded LC

The embedded LC carries the call, a GPS position or a talker alias. After a late entry, the call IDs come from the embedded LC. A talker alias comes in a header LC and block LCs, in any order, so add each one to a [pdu.TalkerAliasAssembler](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#TalkerAliasAssembler) until it returns the alias. Reset the assembler at the end of each call. (ETSI TS 102 361-2 clause 5.4.3)

<!-- dmrgo:code file=v2/examples/scanner/main.go symbol=channel.embedded -->
```go
func (c *channel) embedded(w io.Writer, lc *pdu.FullLinkControl) {
	switch lc.FLCO {
	case enums.FLCOGroupVoiceChannelUser, enums.FLCOUnitToUnitVoiceChannelUser:
		if d := describe(lc); d != c.call {
			fmt.Fprintf(w, "%s call: %s (embedded LC)\n", c.name, d)
			c.call = d
		}
	case enums.FLCOGPSInfo:
		g, ok := lc.AsGPSInfo()
		if !ok {
			return
		}
		if pos := fmt.Sprintf("lat %.4f lon %.4f (%s)", g.Latitude, g.Longitude, g.PositionError.String()); pos != c.gps {
			fmt.Fprintf(w, "%s GPS: %s\n", c.name, pos)
			c.gps = pos
		}
	case enums.FLCOTalkerAliasHeader, enums.FLCOTalkerAliasBlock1, enums.FLCOTalkerAliasBlock2, enums.FLCOTalkerAliasBlock3:
		alias, ok, err := c.ta.Add(lc)
		switch {
		case err != nil:
			fmt.Fprintf(w, "%s talker alias: %v\n", c.name, err)
		case ok:
			fmt.Fprintf(w, "%s talker alias: %q\n", c.name, alias)
		}
	case enums.FLCOTerminatorDataLinkControl:
	}
}
```
<!-- /dmrgo:code -->

### How the example makes its input

The TS2 call comes from a [VoiceTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceTransmitter). Its `EmbeddedLC` callback returns the LC of each superframe, here a rotation of the talker alias blocks, a GPS position and the call LC.

<!-- dmrgo:code file=v2/examples/scanner/main.go symbol=generatedCall -->
```go
func generatedCall() ([]layer2.RawBurst, error) {
	rotation, err := pdu.EncodeTalkerAlias("KF5XYZ Jacob", layer3.TalkerAliasSevenBit)
	if err != nil {
		return nil, err
	}
	gps, err := pdu.FLCGPSInfo{PositionError: layer3.PositionLessThan20M, Longitude: -88.4321, Latitude: 27.1234}.FullLinkControl()
	if err != nil {
		return nil, err
	}
	lc, err := pdu.FLCGroupVoice{GroupAddress: 3100, SourceAddress: 3141592}.FullLinkControl()
	if err != nil {
		return nil, err
	}
	rotation = append(rotation, gps)
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{
		LC:        lc,
		ColorCode: colorCode,
		Source:    layer2.SyncSourceBS,
		Silence:   vocoder.AMBESilenceSlot(),
		EmbeddedLC: func(sf int) *pdu.FullLinkControl {
			if i := sf % (len(rotation) + 1); i > 0 {
				return &rotation[i-1]
			}
			return nil
		},
	})
	if err != nil {
		return nil, err
	}
	out := tx.Headers()
	frames := make([]vocoder.AMBEFrame, 200)
	for i := range frames {
		frames[i] = vocoder.AMBESilence()
	}
	voice, err := tx.AddFrames(frames...)
	if err != nil {
		return nil, err
	}
	tail, err := tx.Terminate()
	if err != nil {
		return nil, err
	}
	return append(append(out, voice...), tail...), nil
}
```
<!-- /dmrgo:code -->

## Output

The first line describes the stream. Then come the events of both slots in the order of the stream, and a summary for each slot with the count of each voice burst position and the AMBE+2 bit error rate.

<!-- dmrgo:output example=scanner -->
```text
stream: 43813 bits, 37 junk bits, 149 bits flipped
TS2 call: group 3141592 -> 3100 (Voice LC Header)
TS1 late entry at voice burst Unknown
TS1 call: private 3191868 -> 9990 (embedded LC)
TS2 talker alias: "KF5XYZ Jacob"
TS2 GPS: lat 27.1234 lon -88.4321 (PositionLessThan20M)
TS2 call end: group 3141592 -> 3100, 72 voice bursts
TS1 call end: private 3191868 -> 9990, 74 voice bursts
framer: first burst at bit 349, 151 bursts
TS1: 74 voice bursts (A=11 B=12 C=12 D=12 E=13 F=13 unknown=1), 0 embedded LC errors, AMBE BER 0.33%
TS2: 72 voice bursts (A=12 B=12 C=12 D=12 E=12 F=12 unknown=0), 0 embedded LC errors, AMBE BER 0.37%
```
<!-- /dmrgo:output -->

The TS1 call starts with a late entry, because the stream skips the first bursts of the capture. Its first voice burst is burst C. Burst C has no voice SYNC, and its LCSS (Continuation) fits burst C and burst D, so the first burst decodes at `Unknown`. The call IDs come from the embedded LC of the first full superframe.

## How it works

The framer searches the stream for a SYNC pattern at any bit offset. After it locks, it returns a burst every burst period, with or without a SYNC, so that voice bursts B to F come out too. It searches again after a number of bursts without a SYNC at the expected place. (ETSI TS 102 361-1 clause 6.1, clause 9.1.1, clause 6.3, Table 9.24)

A voice burst B to F has no SYNC, so the decode of one burst cannot place it. The VoiceReceiver places it from the voice SYNC of burst A and the LCSS of the EMB, and assembles the embedded LC of bursts B to E. (ETSI TS 102 361-1 clause 5.1.2, clause 7.1.3)

A real RF stream also loses bursts and has noise that looks like a burst. The [voice Receiver](../machines/voice-receiver.md) is a state machine for that case: it scores the evidence that a burst is voice, reports lost and duplicate bursts, and ends a call on a timeout. Use it in a scanner that reads a radio. The receivers of package [rf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/rf) recover bursts from audio or I/Q samples with their own SYNC search and slot timing, so they need no framer.

## Related guides

- [Decode and re-encode captures](decode-captures.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
- [Transcode voice: AMBE+2 FEC, Codec2, DVSI](transcode-voice.md)
- [Report and request location: LIP, LRRP, NMEA](location.md)
