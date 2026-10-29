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

<!-- dmrgo:code file=v2/examples/bridge/main.go symbol=receive -->
```go
func receive(bursts []layer2.RawBurst) (inbound, error) {
	var in inbound
	var rx layer2.VoiceReceiver
	haveLC := false
	var b layer2.Burst
	for i, p := range bursts {
		if err := b.DecodeFromBytes(p); err != nil {
			return in, fmt.Errorf("burst %d: %w", i, err)
		}
		if _, _, err := rx.Receive(&b, nil); err != nil {
			return in, fmt.Errorf("burst %d: %w", i, err)
		}
		switch b.Kind {
		case layer2.KindVoice:
			in.slots = append(in.slots, b.VoiceSlots[:]...)
		case layer2.KindData:
			lc, ok := b.Data.(*pdu.FullLinkControl)
			if ok && !haveLC && b.SlotType.DataType == enums.DataTypeVoiceLCHeader && !b.FEC.PDU.Uncorrectable {
				in.lc, in.colorCode, haveLC = *lc, b.SlotType.ColorCode, true
			}
		case layer2.KindUnknown, layer2.KindReverseChannel:
		}
	}
	if !haveLC {
		return in, errors.New("no usable Voice LC Header")
	}
	return in, nil
}
```
<!-- /dmrgo:code -->

### 2. Rewrite the LC

[FullLinkControl.AsUnitToUnit](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#FullLinkControl.AsUnitToUnit) reads a private call. `SetGroupVoice` turns the LC into a group call and keeps the service options. `SetAddresses` sets the new target and source. The example rewrites only an LC with the standardized FID and returns an error for any other FID.

<!-- dmrgo:code file=v2/examples/bridge/main.go symbol=toGroupCall -->
```go
func toGroupCall(lc *pdu.FullLinkControl) error {
	if lc.FID != enums.FIDStandardized {
		return fmt.Errorf("cannot rewrite an LC of FID %s", lc.FID)
	}
	if u, ok := lc.AsUnitToUnit(); ok {
		if err := lc.SetGroupVoice(pdu.FLCGroupVoice{ServiceOptions: u.ServiceOptions}); err != nil {
			return fmt.Errorf("cannot rewrite a %s LC: %w", lc.FLCO, err)
		}
	} else if _, ok := lc.AsGroupVoice(); !ok {
		return fmt.Errorf("cannot rewrite a %s LC", lc.FLCO)
	}
	if err := lc.SetAddresses(newTarget, newSource); err != nil {
		return fmt.Errorf("cannot set addresses of a %s LC: %w", lc.FLCO, err)
	}
	return nil
}
```
<!-- /dmrgo:code -->

### 3. Send the call with the new LC

Give the new LC to a [VoiceTransmitter](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#VoiceTransmitter). `Headers` returns the Voice LC Header bursts, `AddSlots` the voice bursts with the new embedded LC, and `Terminate` the Terminator with LC. `AddSlots` takes the slots of any codec as they are.

<!-- dmrgo:code file=v2/examples/bridge/main.go symbol=transmit -->
```go
func transmit(in inbound) ([]layer2.RawBurst, error) {
	silence := vocoder.AMBESilenceSlot()
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: in.lc, ColorCode: in.colorCode, Source: layer2.SyncSourceBS, Silence: silence})
	if err != nil {
		return nil, err
	}
	out := tx.Headers()
	bursts, err := tx.AddSlots(in.slots...)
	if err != nil {
		return nil, err
	}
	out = append(out, bursts...)
	tail, err := tx.Terminate()
	if err != nil {
		return nil, err
	}
	return append(out, tail...), nil
}
```
<!-- /dmrgo:code -->

### 4. Check the new bursts

Decode the new bursts again. The header and the terminator must carry the new IDs and pass [MMDVMHostAccepts](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2#MMDVMHostAccepts), each superframe must carry the new IDs in its embedded LC, and the voice slots must equal the slots that came in.

<!-- dmrgo:code file=v2/examples/bridge/main.go symbol=verify -->
```go
func verify(bursts []layer2.RawBurst) (verified, error) {
	var v verified
	var rx layer2.VoiceReceiver
	var b layer2.Burst
	for i, p := range bursts {
		if err := b.DecodeFromBytes(p); err != nil {
			return v, fmt.Errorf("rewritten burst %d: %w", i, err)
		}
		lc, ok, err := rx.Receive(&b, nil)
		if err != nil {
			return v, fmt.Errorf("rewritten burst %d: %w", i, err)
		}
		if ok {
			v.embedded++
			if t, s, _ := lc.Addresses(); lc.FLCO == enums.FLCOGroupVoiceChannelUser && t == newTarget && s == newSource {
				v.embeddedNew++
			}
		}
		if b.Kind == layer2.KindVoice {
			v.slots = append(v.slots, b.VoiceSlots[:]...)
			if _, res := b.AMBEFrames(); res.Uncorrectable {
				v.ambeUncorrectable++
			}
			continue
		}
		flc, isLC := b.Data.(*pdu.FullLinkControl)
		if !isLC {
			continue
		}
		v.lcBursts++
		raw := p
		payload, _ := raw.BPTCPayload()
		t, s, _ := flc.Addresses()
		if flc.FLCO == enums.FLCOGroupVoiceChannelUser && t == newTarget && s == newSource && !b.FEC.PDU.HasErrors() && layer2.MMDVMHostAccepts(b.SlotType.DataType, payload) {
			v.lcAccepted++
		}
	}
	return v, nil
}
```
<!-- /dmrgo:code -->

### 5. Rewrite the headers of an SMS

A data call has no embedded LC. Its IDs are in the CSBK preambles and in the data header. Set them with `SetAddresses` on the [pdu.CSBK](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#CSBK) or the [pdu.DataHeader](https://pkg.go.dev/github.com/USA-RedDragon/dmrgo/v2/layer2/pdu#DataHeader), and call `Encode` on the burst. The data blocks after the header go out unchanged.

<!-- dmrgo:code file=v2/examples/bridge/main.go symbol=bridgeSMS -->
```go
func bridgeSMS(w io.Writer, path string) error {
	bursts, err := loadBursts(path)
	if err != nil {
		return err
	}
	var preambles, headers int
	var b, check layer2.Burst
	for i, p := range bursts {
		if err := b.DecodeFromBytes(p); err != nil {
			return fmt.Errorf("sms burst %d: %w", i, err)
		}
		switch d := b.Data.(type) {
		case *pdu.CSBK:
			if _, ok := d.AsPreamble(); !ok || d.SetAddresses(newTarget, newSource) != nil {
				continue
			}
			preambles++
		case *pdu.DataHeader:
			if d.SetAddresses(newTarget, newSource) != nil {
				continue
			}
			headers++
		default:
			continue
		}
		enc, err := b.Encode()
		if err != nil {
			return fmt.Errorf("sms burst %d: %w", i, err)
		}
		if err := checkSMSBurst(&check, enc); err != nil {
			return fmt.Errorf("sms burst %d: %w", i, err)
		}
	}
	if preambles == 0 || headers == 0 {
		return fmt.Errorf("sms: %d CSBK preambles and %d data headers rewritten", preambles, headers)
	}
	fmt.Fprintf(w, "ok sms: %d CSBK preambles and %d data headers rewritten to %d -> %d\n", preambles, headers, newSource, newTarget)
	return nil
}
```
<!-- /dmrgo:code -->

## Output

<!-- dmrgo:output example=bridge -->
```text
voice in: 79 bursts, 231 slots, Unit to Unit Voice Channel User 3191868 -> 9990
voice out: 80 bursts, 234 slots, Group Voice Channel User 3100999 -> 91
ok header and terminator carry the new IDs and pass MMDVMHostAccepts
ok embedded LC carries the new IDs in 13/13 superframes
ok 231 voice slots forwarded unchanged, 3 silence slots of padding
ok sms: 5 CSBK preambles and 1 data headers rewritten to 3100999 -> 91
```
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
