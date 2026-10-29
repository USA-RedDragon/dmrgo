# Build and decode trunking messages

This guide shows how to build and decode the control messages of a DMR Tier III trunked site, and how to follow the calls of a site with the trunk machines.

## Goal

Build the CSBKs, multiple block CSBKs (MBC) and CACH Short LCs that a Tier III control channel sends, decode them back, and follow a site to the channel of a call.

## What you need

- Go and a checkout of dmrgo. The examples run offline.
- The packages [layer2](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2), [layer2/pdu](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu) and [trunk](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk).

Run the example from the top of the repository:

```sh
go run -C v2 ./examples/trunking
```

## Steps

### 1. Build a CSBK and put it in a burst

Each Tier III CSBK has a PDU type in package pdu, such as [pdu.AlohaPDU](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#AlohaPDU) for C_ALOHA or [pdu.TalkgroupVoiceGrantPDU](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#TalkgroupVoiceGrantPDU) for TV_GRANT. Its `CSBK` method returns the [pdu.CSBK](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CSBK). [pdu.SysCode](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#SysCode) packs the network model, network and site into the system identity code of a C_ALOHA.

<!-- dmrgo:code file=v2/examples/trunking/main.go lines=44-64 -->
<!-- /dmrgo:code -->

[layer2.BuildBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#BuildBurst) codes the CSBK into a burst with the color code and the SYNC source: `layer2.SyncSourceBS` for the outbound of the control channel, `layer2.SyncSourceMS` for a radio.

<!-- dmrgo:code file=v2/examples/trunking/main.go symbol=roundTripCSBK -->
<!-- /dmrgo:code -->

### 2. Decode a burst

[layer2.Burst.DecodeFromBytesWith](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Burst.DecodeFromBytesWith) decodes the burst. Check the FEC result, then read the PDU with the `As` method of its type, such as [pdu.CSBK.AsAloha](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CSBK.AsAloha).

<!-- dmrgo:code file=v2/examples/trunking/main.go symbol=decodeAs -->
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/examples/trunking/main.go lines=84-96 -->
<!-- /dmrgo:code -->

Opcode 0x38 means a different PDU inbound and outbound. A decode of a BS-sourced burst sets `Hint0x38`, and [pdu.CSBK.AsTalkgroupDataGrantMultiItem](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CSBK.AsTalkgroupDataGrantMultiItem) reads it.

### 3. Send a grant with its channel frequencies as an MBC

A grant to a channel that the site has not announced carries the frequencies in a CG_AP block. `MBCHeader` of the grant returns the MBC header, and a [pdu.CGAPContinuation](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CGAPContinuation) holds the last block with the [layer3.CdefParms](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer3#CdefParms).

<!-- dmrgo:code file=v2/examples/trunking/main.go symbol=mbcGrant -->
<!-- /dmrgo:code -->

### 4. Send the system parameters in the CACH

A Short LC of C_SYS_Parms or P_SYS_Parms carries the system identity code in the CACH of a control or payload channel. [layer2.EncodeShortLCToCACH](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#EncodeShortLCToCACH) splits it into four CACH fields, and a [layer2.ShortLCAssembler](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#ShortLCAssembler) joins them again.

<!-- dmrgo:code file=v2/examples/trunking/main.go symbol=cachRoundTrip -->
<!-- /dmrgo:code -->

### 5. Follow a site

A [trunk.Follower](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#Follower) follows the calls of a site without transmitting. Give it each burst of the control channel. It returns `OutputSite` when it decodes the site, `OutputGrant` for each grant and `OutputTune` with the channel to tune to. A [trunk.ChannelPlan](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#ChannelPlan) maps channel numbers to frequencies; add the plan of your site with `Add` or `AddRule`.

<!-- dmrgo:code file=v2/trunk/example_test.go symbol=ExampleFollower -->
<!-- /dmrgo:code -->

To decode the voice of a grant, pass `Grant.Call` to [voice.Receiver.Expect](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Receiver.Expect) and feed the bursts of the granted slot to that Receiver, because trunked voice can start without a Voice LC Header.

## Output

<!-- dmrgo:output example=trunking -->
<!-- /dmrgo:output -->

The Follower:

<!-- dmrgo:output test=trunk.ExampleFollower -->
<!-- /dmrgo:output -->

<!-- dmrgo:config pkg=trunk type=FollowerConfig -->
<!-- /dmrgo:config -->

## How it works

A Tier III site has a control channel (TSCC) and payload channels. The TSCC sends C_ALOHA to invite random access and carries the system identity code, C_BCAST for broadcasts such as the channel frequencies, and the grants that move the radios of a call to a payload channel. Radios send their requests as C_RAND by random access. A CSBK is one burst. An MBC is a header and continuation blocks, and its last block ends with a CRC. The CACH of each outbound burst carries one quarter of a Short LC. (ETSI TS 102 361-4 clauses 6.2 to 6.6)

On the slot grid, inbound time slot 1 or 2 starts 30 ms after outbound time slot 1 or 2. A radio sends its answer in the next TDMA frame after the PDU that requests it, and the AT bit of the CACH shows when an inbound slot is withdrawn from random access. (ETSI TS 102 361-1 clauses 4.2, 4.5 and 5.1.1, ETSI TS 102 361-4 clause 6.2.1.1.3)

The trunk machines do no I/O. A [trunk.MS](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#MS) registers and makes calls by random access, a [trunk.TSCC](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#TSCC) runs a single site with its payload channels, and a [trunk.CapacityPlusFollower](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#CapacityPlusFollower) follows a MOTOTRBO Capacity Plus or Linked Capacity Plus site from its rest channel.

The state machines: [trunk Follower](../machines/trunk-follower.md), [trunk MS](../machines/trunk-ms.md), [trunk TSCC](../machines/trunk-tscc.md) and [trunk CapacityPlusFollower](../machines/trunk-capacityplusfollower.md).

## Related guides

- [Radio check, call alert and other CSBK services](csbk-services.md)
- [Receive with an SDR or an FM radio](receive-sdr-fm.md)
- [Follow calls on both slots of a bit stream](follow-both-slots.md)
- [Decode and re-encode captures](decode-captures.md)
