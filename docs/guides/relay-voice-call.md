# Rewrite and relay a voice call

This guide shows how to receive a voice call, change its talkgroup and source ID, and send it again, and how to do the same for the headers of an SMS.

## Goal

Turn a received private call into a group call with new IDs, and forward its voice without a change.

## What you need

- Go and a checkout of dmrgo. The example runs offline.
- The bursts of a call. The example reads the voice capture and an SMS capture in `v2/layer2/testdata`.
- The packages [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2) and [layer2/pdu](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu).

Run the example from the top of the repository:

```sh
go run -C v2 ./examples/bridge
```

## Steps

### 1. Receive the call

Decode each burst. Keep the Full LC of the first Voice LC Header whose FEC passed, with the color code of its slot type, and keep the vocoder slots of each voice burst from `VoiceSlots`. The slots stay as they came: the bridge does not decode the voice.

<!-- dmrgo:code file=v2/examples/bridge/main.go lines=65-92 -->
<!-- /dmrgo:code -->

### 2. Rewrite the LC

[FullLinkControl.AsUnitToUnit](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#FullLinkControl.AsUnitToUnit) reads a private call. `SetGroupVoice` turns the LC into a group call and keeps the service options. `SetAddresses` sets the new target and source. The example rewrites only an LC with the standardized FID and returns an error for any other FID.

<!-- dmrgo:code file=v2/examples/bridge/main.go lines=205-220 -->
<!-- /dmrgo:code -->

### 3. Send the call with the new LC

Give the new LC to a [VoiceTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceTransmitter). `Headers` returns the Voice LC Header bursts, `AddSlots` the voice bursts with the new embedded LC, and `Terminate` the Terminator with LC. `AddSlots` takes the slots of any codec as they are.

<!-- dmrgo:code file=v2/examples/bridge/main.go lines=94-111 -->
<!-- /dmrgo:code -->

### 4. Check the new bursts

Decode the new bursts again. The header and the terminator must carry the new IDs and pass [MMDVMHostAccepts](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#MMDVMHostAccepts), each superframe must carry the new IDs in its embedded LC, and the voice slots must equal the slots that came in.

<!-- dmrgo:code file=v2/examples/bridge/main.go lines=120-158 -->
<!-- /dmrgo:code -->

### 5. Rewrite the headers of an SMS

A data call has no embedded LC. Its IDs are in the CSBK preambles and in the data header. Set them with `SetAddresses` on the [pdu.CSBK](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CSBK) or the [pdu.DataHeader](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#DataHeader), and call `Encode` on the burst. The data blocks after the header go out unchanged.

<!-- dmrgo:code file=v2/examples/bridge/main.go lines=222-260 -->
<!-- /dmrgo:code -->

## Output

<!-- dmrgo:output example=bridge -->
<!-- /dmrgo:output -->

The call out has more voice slots than the call in. The transmitter pads the last superframe with silence slots, and the check counts them.

## How it works

A voice call is a Voice LC Header, superframes of voice bursts A to F, and a Terminator with LC. The header, the terminator and the embedded LC of bursts B to E all carry the LC, so a bridge that changes an ID must write it in all three places. The VoiceTransmitter writes the LC in each place from the one LC that it gets. (ETSI TS 102 361-1 clause 5.1.2, clause 7.1.3)

The voice slots go out as they are, so voice of any codec passes through. For the Voice LC Header and the Terminator with LC, the check bits of ETSI TS 102 361-1 are the same bits that MMDVMHost checks, so the new bursts pass `MMDVMHostAccepts` without a separate seal. See [Clean up bursts for a hotspot](clean-up-bursts.md) for the data types where the two differ.

This example bridges one finished call. A live bridge runs a [voice Receiver](../machines/voice-receiver.md) on the inbound slot and relays its events through a [voice Transmitter](../machines/voice-transmitter.md). A bridge between two masters over OpenBridge runs the [dmrdsession Bridge](../machines/dmrdsession-bridge.md). A bridge from IPSC runs the [ipscsession Peer](../machines/ipscsession-peer.md) on that side.

## Related guides

- [Clean up bursts for a hotspot](clean-up-bursts.md)
- [Follow calls on both slots of a bit stream](follow-both-slots.md)
- [Transcode voice: AMBE+2 FEC, Codec2, DVSI](transcode-voice.md)
- [Send SMS and confirmed data with retries](send-data.md)
