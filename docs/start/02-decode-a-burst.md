# Decode a burst

This step reads a capture of a voice call, decodes each 33-byte burst with layer2, checks its FEC and reads the addresses of the call.

## What a burst is

A DMR burst is the 264 bits that one 30 ms time slot carries. dmrgo holds a burst as a `[33]byte`, packed in transmit order: the first bit on air is the most significant bit of byte 0. A burst holds a SYNC pattern or an EMB in its middle, and a payload on each side. The SYNC pattern marks a burst as data or voice. A data burst also holds a slot type, which gives its color code and data type. (ETSI TS 102 361-1 clause 6 and clause 9)

## The capture

`v2/layer2/testdata/parrot_kerchunk.bin` is a short group call to talkgroup 9990, a parrot, recorded as 33-byte bursts of one time slot, one after the other. It has no time stamps and no transport header.

## The code

[layer2.NewBurstFromBytes](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#NewBurstFromBytes) decodes one burst into a [layer2.Burst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Burst). This example decodes each burst of the capture:

<!-- dmrgo:code file=v2/layer2/example_test.go lines=159-188 -->
<!-- /dmrgo:code -->

Run it from the `v2` directory:

```sh
go test -run ExampleNewBurstFromBytes_capture -v ./layer2
```

It prints:

<!-- dmrgo:output test=layer2.ExampleNewBurstFromBytes_capture -->
<!-- /dmrgo:output -->

## What the code does

- `NewBurstFromBytes` returns an error for a burst that it cannot read, such as a burst with a reserved data type ([layer2.ErrReservedDataType](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#ErrReservedDataType)). An FEC or CRC failure is not an error of `NewBurstFromBytes`.
- `burst.FEC` holds the result of each FEC and CRC check of the burst. `burst.FEC.Aggregate().Err()` returns [fec.ErrUncorrectable](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/fec#ErrUncorrectable) when a check failed. `burst.Data` still holds the decoded PDU in that case, so you can use it or drop it.
- `burst.Kind` is `layer2.KindData` or `layer2.KindVoice`. A data burst has a `SlotType`, and its `Data` holds the PDU of its data type.
- The Voice LC Header and the Terminator with LC each carry a Full LC: a [pdu.FullLinkControl](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#FullLinkControl). `Addresses` returns its target and source addresses, and ok false for an LC that is not a call to a group or a radio. (ETSI TS 102 361-2 Tables 7.1 and 7.2)
- The voice bursts between them carry the AMBE+2 frames of the speech. This step only counts them. The next step follows them as a call.

A `layer2.Burst` also encodes back to 33 bytes. `burst.Encode()` returns the same bytes until you change a field of `burst.Data`, and then writes new FEC for it. [layer2.RawBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#RawBurst) reads and writes single fields of a burst in place, without a full decode.

Next: [Follow a voice call](03-follow-a-voice-call.md).
