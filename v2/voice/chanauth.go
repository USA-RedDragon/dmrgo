package voice

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const pcgStream = 0x2545F4914F6CDD1D

// Burst decodes the outbound burst raw with what the Hint h says about it,
// runs it through the Transmitter as Decoded does, and appends the outputs
// to dst. It allocates only when dst has no room for MaxOutputs outputs.
// (ETSI TS 102 361-2 clause 5.1.1.6.2)
func (t *Transmitter) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Output) []Output {
	if t == nil {
		return dst
	}

	if t.scratch == nil {
		return t.Decoded(now, nil, h, dst)
	}
	t.dopts = layer2.DecodeOptions{Kind: h.Kind, VoiceBurst: enums.VoiceBurstUnknown}
	if h.Kind == layer2.KindVoice {
		t.dopts.VoiceBurst = h.VoiceBurst
	}
	if h.HasDataType {
		t.dataType = h.DataType
		t.dopts.DataType = &t.dataType
	}
	b := t.scratch
	if b.DecodeFromBytesWith(*raw, &t.dopts) != nil {
		b = nil
	}
	return t.Decoded(now, b, h, dst)
}

// Decoded takes the decoded outbound burst b of the slot, which may be nil,
// while the Transmitter waits in TransmitterChanAuth. A Voice LC Header
// with the source address of the call authorizes it, and its headers and
// voice follow from now. A Voice LC Header with another source address
// denies it, and appends OutputEnded with EndDenied to dst. A Terminator
// with LC or an Idle burst is a hang time burst: when T_MS_ChanAuth runs
// out after one, the Transmitter sends the ChanAuth PDU again after a
// random holdoff, and without one the call starts. It allocates only when
// dst has no room for MaxOutputs outputs. (ETSI TS 102 361-2 clause
// 5.1.1.6.2)
func (t *Transmitter) Decoded(now time.Duration, b *layer2.Burst, _ layer2.Hint, dst []Output) []Output {
	if t == nil {
		return dst
	}

	dst = t.Tick(now, dst)
	if t.state != TransmitterChanAuth || !t.authWaiting || b == nil || b.Kind != layer2.KindData || b.SlotType.FEC.Uncorrectable {
		return dst
	}
	switch b.SlotType.DataType {
	case enums.DataTypeVoiceLCHeader:
		lc, ok := b.Data.(*pdu.FullLinkControl)
		if !ok {
			return dst
		}
		_, source, ok := lc.Addresses()
		if !ok {
			return dst
		}
		if source == t.call.Source {
			t.grant(now)
			return dst
		}
		t.due, t.head, t.length = now, 0, 0
		return t.finish(dst, EndDenied)
	case enums.DataTypeTerminatorWithLC, enums.DataTypeIdle:
		t.hangSeen = true
	case enums.DataTypePIHeader, enums.DataTypeCSBK, enums.DataTypeMBCHeader, enums.DataTypeMBCContinuation,
		enums.DataTypeDataHeader, enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1,
		enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
	}
	return dst
}

func (t *Transmitter) grant(at time.Duration) {
	t.state, t.start, t.due, t.sent, t.authWaiting = TransmitterHeader, at, at, 0, false
}

func (t *Transmitter) authTick(dst []Output) []Output {
	if !t.authWaiting {
		var o *Output
		dst, o = appendOutput(dst, OutputBurst, t.due)
		o.Burst, o.Hint = t.vt.Header(), dataHint(enums.DataTypeVoiceLCHeader)
		t.authWaiting, t.hangSeen, t.waitUntil = true, false, t.due+t.chanAuth
		return dst
	}
	if !t.hangSeen {
		t.grant(t.waitUntil)
		return dst
	}
	t.authWaiting, t.due = false, t.waitUntil
	if t.holdoffMax > 0 {
		t.due += t.rng.Duration(t.holdoffMax)
	}
	return dst
}
