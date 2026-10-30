# Decode and re-encode captures

This guide shows how to decode captured DMR bursts, check their FEC and CRCs, and encode them again bit for bit.

## Goal

Decode every burst of a capture file and prove that dmrgo writes the same bits back.

## What you need

- Go and a checkout of dmrgo. The example runs offline.
- Captures of raw bursts, one burst of <!-- dmrgo:const pkg=layer2 name=RawBurstSize -->`33`<!-- /dmrgo:const --> octets after the other. The example reads the captures in `v2/layer2/testdata`: a voice call, a short call, and SMS captures. Use `-data <dir>` to read your own files with the same names.
- The packages [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2) and [layer2/pdu](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu).

Run the example from the top of the repository:

```sh
go run -C v2 ./examples/conformance
```

## Steps

### 1. Decode each burst and encode it again

Cut the file into [RawBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#RawBurst) values of [RawBurstSize](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#RawBurstSize) octets each. Decode each one into a [Burst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Burst) with `DecodeFromBytes`, read the FEC result with `FEC.Aggregate`, and call `Encode`. A burst without corrected bits must encode to the same octets. A burst with corrected bits encodes to the corrected bits, so it can differ from the capture.

<!-- dmrgo:code file=v2/examples/conformance/main.go symbol=checkCapture -->
```go
func checkCapture(name string, raw []byte) *tally {
	t := &tally{file: name}
	if len(raw)%layer2.RawBurstSize != 0 {
		t.failf(0, "length %d is not a multiple of %d", len(raw), layer2.RawBurstSize)
		return t
	}
	var rx layer2.VoiceReceiver
	var frags [4][4]byte
	b := layer2.NewBurst()
	for i := 0; i < len(raw)/layer2.RawBurstSize; i++ {
		in := [layer2.RawBurstSize]byte(raw[i*layer2.RawBurstSize : (i+1)*layer2.RawBurstSize])
		t.bursts++
		if err := b.DecodeFromBytes(in); err != nil {
			t.failf(i, "decode: %v", err)
			continue
		}
		agg := b.FEC.Aggregate()
		if agg.Uncorrectable {
			t.failf(i, "uncorrectable FEC %+v", b.FEC)
		} else if agg.ErrorsCorrected > 0 {
			t.fec++
		}
		out, err := b.Encode()
		switch {
		case err != nil:
			t.failf(i, "encode: %v", err)
		case out != in && agg.ErrorsCorrected == 0:
			t.failf(i, "re-encode differs: %x", out)
		case out == in:
			t.reencoded++
		}

		rb := layer2.RawBurst(in)
		switch b.Kind {
		case layer2.KindData:
			t.data++
			checkData(t, i, b, rb)
			rx.Reset()
		case layer2.KindVoice:
			t.voice++
			checkVoice(t, i, b, rb, &rx, &frags)
		case layer2.KindReverseChannel, layer2.KindUnknown:
			t.failf(i, "unexpected kind %v", b.Kind)
		}
	}
	return t
}
```
<!-- /dmrgo:code -->

Decoding does not return an error when the FEC or a CRC fails. The `FEC` field of the burst and of each PDU holds that result, and the decoded fields are still set. An error from `DecodeFromBytes` means that the bits cannot be decoded at all.

### 2. Rebuild each data burst from its payload

For a data burst, read the slot type and the payload from the raw bits, and build a new burst from them. [SyncSourceOf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#SyncSourceOf) returns the source (BS or MS) of the SYNC pattern. The builder takes the payload of the data type: [BuildRate34DataBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#BuildRate34DataBurst) for rate 3/4, [BuildRate1DataBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#BuildRate1DataBurst) for rate 1, and [BuildDataBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#BuildDataBurst) for every BPTC data type.

<!-- dmrgo:code file=v2/examples/conformance/main.go symbol=checkData -->
```go
func checkData(t *tally, i int, b *layer2.Burst, rb layer2.RawBurst) {
	st := rb.SlotType()
	if st != b.SlotType {
		t.failf(i, "RawBurst slot type %v, Burst %v", st, b.SlotType)
	}
	src, kind, ok := layer2.SyncSourceOf(b.SyncPattern)
	if !ok || kind != layer2.KindData {
		t.failf(i, "no sync source for %v", b.SyncPattern)
		return
	}
	dt := st.DataType
	var rebuilt layer2.RawBurst
	var err error
	switch dt {
	case enums.DataTypeRate34:
		p, _ := rb.Rate34Payload()
		rebuilt, err = layer2.BuildRate34DataBurst(p, st.ColorCode, src)
	case enums.DataTypeRate1:
		p, _ := rb.Rate1Payload()
		rebuilt, err = layer2.BuildRate1DataBurst(p, st.ColorCode, src)
	case enums.DataTypeReserved:
		t.failf(i, "reserved data type")
		return
	case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC,
		enums.DataTypePIHeader, enums.DataTypeCSBK, enums.DataTypeMBCHeader,
		enums.DataTypeDataHeader, enums.DataTypeUnifiedSingleBlock,
		enums.DataTypeMBCContinuation, enums.DataTypeRate12, enums.DataTypeIdle:
		p, _ := rb.BPTCPayload()
		checkSeal(t, i, dt, p)
		rebuilt, err = layer2.BuildDataBurst(p, dt, st.ColorCode, src)
	}
	switch {
	case err != nil:
		t.failf(i, "build %v: %v", dt, err)
	case rebuilt == rb:
		t.rebuilt++
	case b.FEC.Aggregate().ErrorsCorrected == 0:
		t.failf(i, "rebuilt %v burst differs: %x", dt, rebuilt)
	}
}
```
<!-- /dmrgo:code -->

### 3. Check and write the CRC or RS(12,9) parity

[pdu.CRCMask](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CRCMask) returns the width of the check of a data type. A Full LC (Voice LC Header and Terminator with LC) has RS(12,9) parity; check it with `FullLCParityOK` and write it with [LCBytes.Codeword](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#LCBytes.Codeword). The other BPTC types have a CRC-CCITT; check it with `BlockCRCOK` and write it with [pdu.Seal](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#Seal). A resealed payload must equal the payload on air. (ETSI TS 102 361-1 clause B.3.12, Table B.21)

<!-- dmrgo:code file=v2/examples/conformance/main.go symbol=checkSeal -->
```go
func checkSeal(t *tally, i int, dt enums.DataType, p [12]byte) {
	var want [12]byte
	var err error
	_, width, _ := pdu.CRCMask(dt)
	switch width {
	case 24:
		if !pdu.FullLCParityOK(p, dt) {
			t.failf(i, "%v RS(12,9) parity fails", dt)
			return
		}
		lcb := pdu.LCBytes(p[:9])
		want, err = lcb.Codeword(dt)
	case 16:
		if !pdu.BlockCRCOK(p, dt) {
			t.failf(i, "%v CRC-CCITT fails", dt)
			return
		}
		want = p
		err = pdu.Seal(&want, dt)
	default:
		return
	}
	switch {
	case err != nil:
		t.failf(i, "seal %v: %v", dt, err)
	case want != p:
		t.failf(i, "resealed %v payload %x, on air %x", dt, want, p)
	default:
		t.sealed++
	}
}
```
<!-- /dmrgo:code -->

### 4. Re-encode the embedded LC of each superframe

Bursts B to E of a voice superframe carry the Full LC in fragments. A [VoiceReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceReceiver) places each voice burst and returns the LC when a superframe completes it. Keep the embedded data of bursts B to E, and compare it with [EncodeEmbeddedFullLinkControl](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#EncodeEmbeddedFullLinkControl) of the LC. (ETSI TS 102 361-1 clause 7.1.3)

<!-- dmrgo:code file=v2/examples/conformance/main.go symbol=checkVoice -->
```go
func checkVoice(t *tally, i int, b *layer2.Burst, rb layer2.RawBurst, rx *layer2.VoiceReceiver, frags *[4][4]byte) {
	if b.HasEmbeddedSignalling() {
		if emb := rb.EMB(); emb != b.EmbeddedSignalling {
			t.failf(i, "RawBurst EMB %v, Burst %v", emb, b.EmbeddedSignalling)
		}
	}
	lc, ok, err := rx.Receive(b, nil)
	if err != nil {
		t.failf(i, "voice receiver: %v", err)
		return
	}
	if b.VoiceBurst >= enums.VoiceBurstB && b.VoiceBurst <= enums.VoiceBurstE {
		frags[b.VoiceBurst-enums.VoiceBurstB] = rb.EmbeddedData()
	}
	if !ok {
		return
	}
	enc, err := layer2.EncodeEmbeddedFullLinkControl(&lc)
	switch {
	case err != nil:
		t.failf(i, "encode embedded LC: %v", err)
	case enc != *frags:
		t.failf(i, "embedded LC re-encodes to %x, on air %x", enc, *frags)
	default:
		t.embeddedLC++
	}
}
```
<!-- /dmrgo:code -->

## Output

Each row is one capture. `re-encoded` counts the bursts that `Encode` wrote back unchanged, `rebuilt` the data bursts that a builder wrote back unchanged, `sealed` the payloads whose check bits dmrgo wrote again, `emb LC` the embedded LCs that re-encode, and `FEC fixed` the bursts with corrected bits.

<!-- dmrgo:output example=conformance -->
```text
                 file  bursts  voice  data  re-encoded  rebuilt  sealed  emb LC  FEC fixed  result
            voice.bin      79     77     2          79        2       2      13          0    PASS
  parrot_kerchunk.bin      15     13     2          15        2       2       2          0    PASS
            d-sms.bin      11      0    11          11       11       6       0          0    PASS
            h-sms.bin       9      0     9           9        9       6       0          0    PASS
            m-sms.bin      12      0    12          12       12       6       0          0    PASS
all captures conform
```
<!-- /dmrgo:output -->

The test `TestBadCRCFails` in `v2/examples/conformance/main_test.go` flips one bit of a Data Header payload and checks that the example reports the CRC failure.

## How it works

`Burst.DecodeFromBytes` classifies a burst by its SYNC field. A burst without a SYNC pattern is a voice burst B to F, unless its EMB, slot type, BPTC and CRC show a data burst with an EMB in place of the SYNC. The decode is stateless, so a voice burst B to F decodes at `VoiceBurstUnknown` until something places it in the superframe. (ETSI TS 102 361-1 clause 6.4.2)

`Burst.Encode` writes the payload as it was decoded after FEC correction when the `Data` field codes to the same bits. Voice slots and the embedded data field go out as they are. That is why a clean capture re-encodes bit for bit, and why a burst with a failed CRC can still be forwarded. The package doc of [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2) has the full rules. (ETSI TS 102 361-1 clause 6 and clause 9)

This example uses no state machine. To follow calls in a live stream, with lost bursts and late entry, use the voice Receiver: see [voice Receiver](../machines/voice-receiver.md). For data packets, see [data Receiver](../machines/data-receiver.md).

## Related guides

- [Follow calls on both slots of a bit stream](follow-both-slots.md)
- [Clean up bursts for a hotspot](clean-up-bursts.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
- [Send SMS and confirmed data with retries](send-data.md)
