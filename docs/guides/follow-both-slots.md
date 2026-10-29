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
<!-- /dmrgo:code -->

### 2. Write the stream and decode each burst

Give the stream to [Framer.Write](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/framer#Framer.Write) in pieces of any size, with the number of bits in each piece. Write returns the bursts that the piece completes, each with its bit offset and its slot. Decode each burst with `DecodeFromBytesWith` and pass it to the state of its slot. The example cuts the stream at random lengths to show that a burst can start at any bit.

<!-- dmrgo:code file=v2/examples/scanner/main.go lines=68-87 -->
<!-- /dmrgo:code -->

### 3. Start and end calls on each slot

Keep one [VoiceReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceReceiver) for each slot. A Voice LC Header starts a call and a Terminator with LC ends it. A voice burst on a slot without a call is a late entry: the scanner started in the middle of the call. `Receive` places each voice burst in its superframe and returns the embedded LC when bursts B to E complete it. `AMBEFrames` returns the AMBE+2 frames and the FEC result of each voice burst, which [fec.BERCalculator](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/fec#BERCalculator) adds up.

<!-- dmrgo:code file=v2/examples/scanner/main.go symbol=channel.receive -->
<!-- /dmrgo:code -->

### 4. Read the embedded LC

The embedded LC carries the call, a GPS position or a talker alias. After a late entry, the call IDs come from the embedded LC. A talker alias comes in a header LC and block LCs, in any order, so add each one to a [pdu.TalkerAliasAssembler](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#TalkerAliasAssembler) until it returns the alias. Reset the assembler at the end of each call. (ETSI TS 102 361-2 clause 5.4.3)

<!-- dmrgo:code file=v2/examples/scanner/main.go symbol=channel.embedded -->
<!-- /dmrgo:code -->

### How the example makes its input

The TS2 call comes from a [VoiceTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceTransmitter). Its `EmbeddedLC` callback returns the LC of each superframe, here a rotation of the talker alias blocks, a GPS position and the call LC.

<!-- dmrgo:code file=v2/examples/scanner/main.go symbol=generatedCall -->
<!-- /dmrgo:code -->

## Output

The first line describes the stream. Then come the events of both slots in the order of the stream, and a summary for each slot with the count of each voice burst position and the AMBE+2 bit error rate.

<!-- dmrgo:output example=scanner -->
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
