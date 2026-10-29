# Decode and re-encode captures

This guide shows how to decode captured DMR bursts, check their FEC and CRCs, and encode them again bit for bit.

## Goal

Decode every burst of a capture file and prove that dmrgo writes the same bits back.

## What you need

- Go and a checkout of dmrgo. The example runs offline.
- Captures of raw bursts, one burst of <!-- dmrgo:const pkg=layer2 name=RawBurstSize --><!-- /dmrgo:const --> octets after the other. The example reads the captures in `v2/layer2/testdata`: a voice call, a short call, and SMS captures. Use `-data <dir>` to read your own files with the same names.
- The packages [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2) and [layer2/pdu](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu).

Run the example from the top of the repository:

```sh
go run -C v2 ./examples/conformance
```

## Steps

### 1. Decode each burst and encode it again

Cut the file into [RawBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#RawBurst) values of [RawBurstSize](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#RawBurstSize) octets each. Decode each one into a [Burst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Burst) with `DecodeFromBytes`, read the FEC result with `FEC.Aggregate`, and call `Encode`. A burst without corrected bits must encode to the same octets. A burst with corrected bits encodes to the corrected bits, so it can differ from the capture.

<!-- dmrgo:code file=v2/examples/conformance/main.go lines=36-74 -->
<!-- /dmrgo:code -->

Decoding does not return an error when the FEC or a CRC fails. The `FEC` field of the burst and of each PDU holds that result, and the decoded fields are still set. An error from `DecodeFromBytes` means that the bits cannot be decoded at all.

### 2. Rebuild each data burst from its payload

For a data burst, read the slot type and the payload from the raw bits, and build a new burst from them. [SyncSourceOf](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#SyncSourceOf) returns the source (BS or MS) of the SYNC pattern. The builder takes the payload of the data type: [BuildRate34DataBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#BuildRate34DataBurst) for rate 3/4, [BuildRate1DataBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#BuildRate1DataBurst) for rate 1, and [BuildDataBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#BuildDataBurst) for every BPTC data type.

<!-- dmrgo:code file=v2/examples/conformance/main.go symbol=checkData -->
<!-- /dmrgo:code -->

### 3. Check and write the CRC or RS(12,9) parity

[pdu.CRCMask](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CRCMask) returns the width of the check of a data type. A Full LC (Voice LC Header and Terminator with LC) has RS(12,9) parity; check it with `FullLCParityOK` and write it with [LCBytes.Codeword](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#LCBytes.Codeword). The other BPTC types have a CRC-CCITT; check it with `BlockCRCOK` and write it with [pdu.Seal](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#Seal). A resealed payload must equal the payload on air. (ETSI TS 102 361-1 clause B.3.12, Table B.21)

<!-- dmrgo:code file=v2/examples/conformance/main.go symbol=checkSeal -->
<!-- /dmrgo:code -->

### 4. Re-encode the embedded LC of each superframe

Bursts B to E of a voice superframe carry the Full LC in fragments. A [VoiceReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceReceiver) places each voice burst and returns the LC when a superframe completes it. Keep the embedded data of bursts B to E, and compare it with [EncodeEmbeddedFullLinkControl](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#EncodeEmbeddedFullLinkControl) of the LC. (ETSI TS 102 361-1 clause 7.1.3)

<!-- dmrgo:code file=v2/examples/conformance/main.go symbol=checkVoice -->
<!-- /dmrgo:code -->

## Output

Each row is one capture. `re-encoded` counts the bursts that `Encode` wrote back unchanged, `rebuilt` the data bursts that a builder wrote back unchanged, `sealed` the payloads whose check bits dmrgo wrote again, `emb LC` the embedded LCs that re-encode, and `FEC fixed` the bursts with corrected bits.

<!-- dmrgo:output example=conformance -->
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
