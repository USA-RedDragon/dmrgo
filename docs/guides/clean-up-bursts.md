# Clean up bursts for a hotspot

This guide shows how to correct the bit errors of received bursts, check them as MMDVMHost does, and send clean bursts again.

## Goal

Take bursts with bit errors, correct them, and regenerate bursts that MMDVMHost accepts from the network.

## What you need

- Go and a checkout of dmrgo. The example runs offline.
- Received bursts. The example reads the voice capture in `v2/layer2/testdata` and flips bits in each burst to act as RF errors.
- The packages [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2), [layer2/pdu](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu) and [vocoder](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder).

Run the example from the top of the repository:

```sh
go run -C v2 ./examples/hotspot
```

## Steps

### 1. Decode each burst and add up the FEC results

Decode each burst with the color code of the channel. [fec.Result.Combine](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/fec#Result.Combine) adds the corrected bits of each burst to a total. A [VoiceReceiver](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceReceiver) returns the embedded LC of each superframe, which must match the Voice LC Header of the call.

<!-- dmrgo:code file=v2/examples/hotspot/main.go lines=85-103 -->
<!-- /dmrgo:code -->

### 2. Check each data burst as MMDVMHost does

[RawBurst.MMDVMHostPayload](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#RawBurst.MMDVMHostPayload) returns the BPTC payload and whether MMDVMHost uses a burst of that data type from the network. Keep the Voice LC Header for the next step. From each voice burst, keep the AMBE+2 frames after FEC with `AMBEFrames`.

<!-- dmrgo:code file=v2/examples/hotspot/main.go lines=104-120 -->
<!-- /dmrgo:code -->

### 3. Regenerate the call from the LC and the frames

A [VoiceTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceTransmitter) makes the Voice LC Header, the voice superframes with their embedded LC, and the Terminator with LC. `AddFrames` encodes each AMBE+2 frame with its FEC, so the new bursts carry no bit errors. The transmitter pads the last superframe with the silence slot of [vocoder.AMBESilenceSlot](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/vocoder#AMBESilenceSlot).

<!-- dmrgo:code file=v2/examples/hotspot/main.go symbol=regenerate -->
<!-- /dmrgo:code -->

### 4. Rewrite a header and seal it for MMDVMHost

To change the target of a Voice LC Header in place, read the payload with `BPTCPayload`, set the addresses with [LCBytes.SetAddresses](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#LCBytes.SetAddresses), and write the check bits again with [MMDVMHostSeal](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#MMDVMHostSeal). Then build the burst with [BuildDataBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#BuildDataBurst). Before the seal, MMDVMHost rejects the payload. After the seal, it accepts it.

<!-- dmrgo:code file=v2/examples/hotspot/main.go symbol=rewriteTarget -->
<!-- /dmrgo:code -->

## Output

The first lines describe the capture and the call. Then come the bit errors that the example put in and the bits that the FEC corrected, the MMDVMHost check, the regenerated call, and the rewritten header.

<!-- dmrgo:output example=hotspot -->
<!-- /dmrgo:output -->

The AMBE+2 FEC corrects more bits than the example put in, because the capture itself has a few bit errors in its voice frames. The regenerated call has more bursts than the capture: the transmitter completes the last superframe with silence.

## How it works

MMDVMHost checks each data burst that it gets from the network before it sends the burst on air. [MMDVMHostAccepts](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#MMDVMHostAccepts) runs the same check, and the package doc of [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2) lists it for each data type. Use `MMDVMHostSeal` for a burst that goes to MMDVMHost in a DMRD packet, and [pdu.Seal](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#Seal), the check bits of ETSI TS 102 361-1, for a burst on air or to any other peer. The two write the same bits for the Voice LC Header, the Terminator with LC, the PI Header, the CSBK, the MBC Header and the Data Header. (ETSI TS 102 361-1 clause B.3.12)

The DMRD packets themselves are in package [transport/dmrd](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/transport/dmrd). A hotspot that logs in to a master speaks the MMDVM network protocol (`DialectMMDVM`); the link from MMDVMHost to DMRGateway on one host is `DialectLocal`. The session state is in the [dmrdsession Peer](../machines/dmrdsession-peer.md).

This example rebuilds the whole call after it ends. A hotspot that relays a call while it is on air feeds each burst to a [voice Receiver](../machines/voice-receiver.md) and sends its events through a [voice Transmitter](../machines/voice-transmitter.md). A [voice Jitter](../machines/voice-jitter.md) in front of the Receiver puts network packets back in order.

## Related guides

- [Decode and re-encode captures](decode-captures.md)
- [Rewrite and relay a voice call](relay-voice-call.md)
- [Transcode voice: AMBE+2 FEC, Codec2, DVSI](transcode-voice.md)
