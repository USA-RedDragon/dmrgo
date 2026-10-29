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
```go
code, err := sc.Pack()
if err != nil {
	return err
}
builds := []func() (pdu.CSBK, error){
	pdu.AlohaPDU{TSCCAS: true, SiteTSSync: true, Version: 1, NRandWait: 5, Reg: true, Backoff: 3, SysIdentCode: code}.CSBK,
	pdu.BroadcastPDU{Reg: true, Backoff: 2, SysIdentCode: code}.CSBK,
	pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 42, LogicalChannel: true, TargetAddress: 9, SourceAddress: 3120001}.CSBK,
	pdu.PrivateVoiceGrantPDU{PhysicalChannel: 43, TargetAddress: 3120002, SourceAddress: 3120001}.CSBK,
	pdu.AhoyPDU{TargetAddress: 3120002, SourceAddress: 3120001}.CSBK,
	pdu.AckOutboundPDU{ReasonCode: 0x62, TargetAddress: 3120002}.CSBK,
	pdu.MovePDU{Mask: 3, Reg: true, Backoff: 4, PhysicalChannel: 12, MSAddress: 3120002}.CSBK,
}
csbks := make([]*pdu.CSBK, len(builds))
for i, build := range builds {
	c, err := build()
	if err != nil {
		return err
	}
	csbks[i] = &c
}
```
<!-- /dmrgo:code -->

[layer2.BuildBurst](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#BuildBurst) codes the CSBK into a burst with the color code and the SYNC source: `layer2.SyncSourceBS` for the outbound of the control channel, `layer2.SyncSourceMS` for a radio.

<!-- dmrgo:code file=v2/examples/trunking/main.go symbol=roundTripCSBK -->
```go
func roundTripCSBK(c *pdu.CSBK, src layer2.SyncSource) (*pdu.CSBK, *layer2.Burst, error) {
	raw, err := layer2.BuildBurst(c, colorCode, src)
	if err != nil {
		return nil, nil, err
	}
	return decodeAs[*pdu.CSBK](raw)
}
```
<!-- /dmrgo:code -->

### 2. Decode a burst

[layer2.Burst.DecodeFromBytesWith](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#Burst.DecodeFromBytesWith) decodes the burst. Check the FEC result, then read the PDU with the `As` method of its type, such as [pdu.CSBK.AsAloha](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CSBK.AsAloha).

<!-- dmrgo:code file=v2/examples/trunking/main.go symbol=decodeAs -->
```go
func decodeAs[T layer2.Data](raw layer2.RawBurst) (T, *layer2.Burst, error) {
	var zero T
	cc := enums.ColorCode(colorCode)
	var b layer2.Burst
	if err := b.DecodeFromBytesWith(raw, &layer2.DecodeOptions{ColorCode: &cc}); err != nil {
		return zero, nil, err
	}
	d, ok := b.Data.(T)
	if !ok {
		return zero, nil, fmt.Errorf("decoded %T, want %T", b.Data, d)
	}
	return d, &b, nil
}
```
<!-- /dmrgo:code -->

<!-- dmrgo:code file=v2/examples/trunking/main.go lines=84-96 -->
```go
aloha, _, err := roundTripCSBK(csbks[0], layer2.SyncSourceBS)
if err != nil {
	return err
}
ap, ok := aloha.AsAloha()
if !ok {
	return fmt.Errorf("C_ALOHA decoded as %s", aloha.Opcode)
}
var got pdu.SysCode
if got.Unpack(ap.SysIdentCode); got != sc {
	return fmt.Errorf("C_ALOHA SYScode %+v, want %+v", got, sc)
}
fmt.Fprintf(w, "C_ALOHA SYScode 0x%04x: model %d net %d site %d par %d\n", code, got.Model, got.Net, got.Site, got.Par)
```
<!-- /dmrgo:code -->

Opcode 0x38 means a different PDU inbound and outbound. A decode of a BS-sourced burst sets `Hint0x38`, and [pdu.CSBK.AsTalkgroupDataGrantMultiItem](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CSBK.AsTalkgroupDataGrantMultiItem) reads it.

### 3. Send a grant with its channel frequencies as an MBC

A grant to a channel that the site has not announced carries the frequencies in a CG_AP block. `MBCHeader` of the grant returns the MBC header, and a [pdu.CGAPContinuation](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CGAPContinuation) holds the last block with the [layer3.CdefParms](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer3#CdefParms).

<!-- dmrgo:code file=v2/examples/trunking/main.go symbol=mbcGrant -->
```go
func mbcGrant(w io.Writer) error {
	grant := pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 0xFFF, TargetAddress: 9, SourceAddress: 3120001}
	hdr, err := grant.MBCHeader()
	if err != nil {
		return err
	}
	cgap := pdu.CGAPContinuation{LastBlock: true, Opcode: pdu.CSBKTalkgroupVoiceGrant, ColorCode: colorCode,
		CdefParms: layer3.CdefParms{Channel: 0xFFF, TXMHz: 451, TXKHz: 1000, RXMHz: 456, RXKHz: 1000}}
	hraw, err := layer2.BuildBurst(&hdr, colorCode, layer2.SyncSourceBS)
	if err != nil {
		return err
	}
	craw, err := layer2.BuildBurst(&cgap, colorCode, layer2.SyncSourceBS)
	if err != nil {
		return err
	}

	gotHdr, _, err := decodeAs[*pdu.MBCHeader](hraw)
	if err != nil {
		return err
	}
	gotGrant, ok := gotHdr.AsTalkgroupVoiceGrant()
	if gotHdr.FEC.Uncorrectable || !ok || gotGrant != grant {
		return fmt.Errorf("MBC header grant %+v, want %+v", gotGrant, grant)
	}

	gotCont, _, err := decodeAs[*pdu.MBCContinuation](craw)
	if err != nil {
		return err
	}
	gotCGAP := gotCont.CGAP()
	if err := gotCGAP.FEC.Err(); err != nil {
		return fmt.Errorf("MBC last block CRC: %w", err)
	}
	if gotCGAP.CdefParms != cgap.CdefParms {
		return fmt.Errorf("CGAP %+v, want %+v", gotCGAP.CdefParms, cgap.CdefParms)
	}
	fmt.Fprintf(w, "MBC %s ch 0x%X tg %d: TX %.4f MHz RX %.4f MHz\n", gotHdr.Opcode, gotGrant.PhysicalChannel,
		gotGrant.TargetAddress, gotCGAP.CdefParms.TXFrequencyMHz(), gotCGAP.CdefParms.RXFrequencyMHz())
	return nil
}
```
<!-- /dmrgo:code -->

### 4. Send the system parameters in the CACH

A Short LC of C_SYS_Parms or P_SYS_Parms carries the system identity code in the CACH of a control or payload channel. [layer2.EncodeShortLCToCACH](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#EncodeShortLCToCACH) splits it into four CACH fields, and a [layer2.ShortLCAssembler](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#ShortLCAssembler) joins them again.

<!-- dmrgo:code file=v2/examples/trunking/main.go symbol=cachRoundTrip -->
```go
func cachRoundTrip(asm *layer2.ShortLCAssembler, slc *pdu.ShortLC) (pdu.ShortLC, error) {
	fields, err := layer2.EncodeShortLCToCACH(slc, pdu.TACT{AccessType: true})
	if err != nil {
		return pdu.ShortLC{}, err
	}
	var got pdu.ShortLC
	done := false
	for i, f := range fields {
		c := layer2.DecodeCACHField(f)
		if err := c.TACT.FEC.Err(); err != nil {
			return pdu.ShortLC{}, fmt.Errorf("CACH %d TACT: %w", i, err)
		}
		if got, done, err = asm.Add(c); err != nil {
			return pdu.ShortLC{}, err
		}
	}
	if !done {
		return pdu.ShortLC{}, errors.New("short LC incomplete after 4 CACH fields")
	}
	if err := got.FEC.Err(); err != nil {
		return got, fmt.Errorf("short LC %s: %w", got.SLCO, err)
	}
	if got.SLCO != slc.SLCO {
		return got, fmt.Errorf("short LC %s, want %s", got.SLCO, slc.SLCO)
	}
	return got, nil
}
```
<!-- /dmrgo:code -->

### 5. Follow a site

A [trunk.Follower](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#Follower) follows the calls of a site without transmitting. Give it each burst of the control channel. It returns `OutputSite` when it decodes the site, `OutputGrant` for each grant and `OutputTune` with the channel to tune to. A [trunk.ChannelPlan](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/trunk#ChannelPlan) maps channel numbers to frequencies; add the plan of your site with `Add` or `AddRule`.

<!-- dmrgo:code file=v2/trunk/example_test.go symbol=ExampleFollower -->
```go
func ExampleFollower() {
	var plan trunk.ChannelPlan
	_ = plan.AddRule(trunk.ChannelRule{First: 1, Last: 200, RXHz: 451_000_000, StepHz: 12_500})
	f, _ := trunk.NewFollower(&trunk.FollowerConfig{Plan: &plan})
	out := make([]trunk.Output, 0, trunk.MaxOutputs)

	code, _ := pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 1, Site: 2}.Pack()
	alohaCSBK := pdu.CSBK{LastBlock: true}
	_ = alohaCSBK.SetAloha(pdu.AlohaPDU{SysIdentCode: code})
	grantCSBK := pdu.CSBK{LastBlock: true}
	_ = grantCSBK.SetTalkgroupVoiceGrant(pdu.TalkgroupVoiceGrantPDU{PhysicalChannel: 17, LogicalChannel: true, TargetAddress: 9, SourceAddress: 3120001})
	aloha, _ := layer2.BuildBurst(&alohaCSBK, 1, layer2.SyncSourceBS)
	grant, _ := layer2.BuildBurst(&grantCSBK, 1, layer2.SyncSourceBS)

	now := time.Duration(0)
	for _, raw := range []layer2.RawBurst{aloha, grant} {
		out = f.Burst(now, &raw, layer2.Hint{}, nil, out[:0])
		for _, o := range out {
			switch o.Kind {
			case trunk.OutputSite:
				s := f.Site()
				fmt.Println("site", s.SysCode.Net, s.SysCode.Site)
			case trunk.OutputGrant:
				fmt.Println("grant to talkgroup", o.Grant.Target, "from", o.Grant.Source)
			case trunk.OutputTune:
				fmt.Println("tune to channel", o.Tune.Channel, o.Tune.Slot, o.Tune.RXHz, "Hz")
			default:
			}
		}
		now += 30 * time.Millisecond
	}
}
```
<!-- /dmrgo:code -->

To decode the voice of a grant, pass `Grant.Call` to [voice.Receiver.Expect](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/voice#Receiver.Expect) and feed the bursts of the granted slot to that Receiver, because trunked voice can start without a Voice LC Header.

## Output

<!-- dmrgo:output example=trunking -->
```text
outbound CSBKs: 7 round trips ok
C_ALOHA SYScode 0x6acd: model 1 net 85 site 19 par 1
Burst.Encode matches BuildBurst for TV_GRANT PDU
opcode 0x38 from BS: TD_GRANT_MI ch 77 tg 9 src 3120001
inbound C_RAND PDU ok, sync MS-sourced data
MBC TV_GRANT PDU ch 0xFFF tg 9: TX 451.1250 MHz RX 456.1250 MHz
CACH Short LC: 8 C_SYS_Parms/P_SYS_Parms round trips ok over 4 models
all checks passed
```
<!-- /dmrgo:output -->

The Follower:

<!-- dmrgo:output test=trunk.ExampleFollower -->
```text
site 1 2
grant to talkgroup 9 from 3120001
tune to channel 17 TS2 451200000 Hz
```
<!-- /dmrgo:output -->

<!-- dmrgo:config pkg=trunk type=FollowerConfig -->
| Field | Type | Default | Description |
|---|---|---|---|
| `SysCode` | `*pdu.SysCode` | zero value | SysCode, when set, is the system to follow: a TSCC with another MODEL, NET or SITE is skipped. Nil follows the first TSCC found. |
| `Hunt` | `*HuntList` | zero value | Hunt, when set, is the hunt list. The Follower copies it, requests each channel in turn with OutputTune until it finds a TSCC, and adds and withdraws channels on an Ann_WD_TSCC broadcast. With a nil or empty Hunt the consumer keeps the receiver on one TSCC, which is then channel CHNULL. |
| `Plan` | `*ChannelPlan` | zero value | Plan, when set, is the channel plan to start from. The Follower copies it and adds the channels that CG_AP and Chan_Freq announce. |
| `Talkgroups` | `[32]uint32` | zero value | Talkgroups holds the first TalkgroupsLen 24-bit talkgroups to follow. TalkgroupsLen 0 follows every talkgroup. |
| `TalkgroupsLen` | `int` | zero value | TalkgroupsLen is the number of entries of Talkgroups in use. |
| `Individual` | `bool` | zero value | Individual follows individual voice calls too. |
| `Data` | `bool` | zero value | Data follows packet data grants too. |
| `NoFollow` | `bool` | zero value | NoFollow reports grants with OutputGrant but never leaves the TSCC, for a receiver that cannot tune, such as the replay of a recording. |
| `NoSignal` | `time.Duration` | `constants.TNoSignalDefault` (5s) | NoSignal is T_Nosig, the time without a TSCC burst after which the Follower hunts again, and the time it stays on each hunt list channel. 0 means constants.TNoSignalDefault, and a negative value turns the timer off (ETSI TS 102 361-4 Table A.1). |
| `Inactive` | `time.Duration` | `constants.TVInactiveDefault` (5s) | Inactive is TV_Inactive, the time without a burst on the payload channel after which the Follower returns to the TSCC. 0 means constants.TVInactiveDefault, and a negative value turns the timer off. |
| `Hangtime` | `time.Duration` | `constants.TVHangtimeDefault` (4s) | Hangtime is the time after a Terminator with LC in which the Follower waits on the payload channel for more voice. 0 means constants.TVHangtimeDefault, and a negative value turns the timer off. |
| `SysErrors` | `int` | `constants.NSysErrDefault` (2) | SysErrors is NSYSerr, the number of C_SYScodes from another system after which the Follower leaves the TSCC. 0 means constants.NSysErrDefault; a negative value is an error. |
| `AcceptRAS` | `bool` | zero value | AcceptRAS takes a CSBK whose CRC fails as valid when its BPTC needed no correction and its BPTC reserved bit R(2) flags RAS. The CRC of a Motorola Capacity Max system with RAS is a MAC with the key of the system, which the Follower cannot check. |
| `Scratch` | `*layer2.Burst` | `layer2.NewBurst()` | Scratch, when set, is the Burst that the Follower decodes into, as for voice.ReceiverConfig.Scratch. Nil means the Follower makes its own. |
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
