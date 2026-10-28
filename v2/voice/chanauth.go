package voice

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const pcgStream = 0x2545F4914F6CDD1D

// Burst decodes the outbound burst raw with the data in the Hint h,
// runs it through the Transmitter as Decoded does, and appends the outputs
// to dst. A nil raw, a transport frame without a burst, only runs the
// timers, as Lost does. It allocates only when dst has no room for
// MaxOutputs outputs. (ETSI TS 102 361-2 clause 5.1.1.6.2)
func (t *Transmitter) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Output) []Output {
	if t == nil {
		return dst
	}

	if raw == nil {
		return t.Tick(now, dst)
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

// Decoded takes the decoded outbound burst b of the slot, which may be nil
// for a burst that did not decode, while the Transmitter waits in
// TransmitterChanAuth. A Voice LC Header with the source address of the
// call authorizes it, and its headers and voice follow from now. A Voice
// LC Header with another source address denies it, and appends
// OutputEnded with EndDenied to dst. Every other outbound burst, whatever
// its kind and FEC, shows a busy channel: when T_MS_ChanAuth runs out
// after one, the Transmitter sends the ChanAuth PDU again after a random
// holdoff. When it runs out with no outbound burst, the call does not
// start: the Transmitter appends OutputEnded with EndNoOutbound. It
// allocates only when dst has no room for MaxOutputs outputs. (ETSI TS 102
// 361-2 clause 5.1.1.6.2)
func (t *Transmitter) Decoded(now time.Duration, b *layer2.Burst, _ layer2.Hint, dst []Output) []Output {
	if t == nil {
		return dst
	}

	dst = t.Tick(now, dst)
	if t.state != TransmitterChanAuth || !t.authWaiting {
		return dst
	}
	if b != nil && b.Kind == layer2.KindData && !b.SlotType.FEC.Uncorrectable && b.SlotType.DataType == enums.DataTypeVoiceLCHeader {
		if lc, ok := b.Data.(*pdu.FullLinkControl); ok {
			if _, source, ok := lc.Addresses(); ok {
				if source == t.call.Source {
					t.grant(now + t.lead)
					return dst
				}
				t.due, t.head, t.length = now, 0, 0
				return t.finish(dst, EndDenied)
			}
		}
	}
	t.busySeen = true
	return dst
}

// Lost takes a transport event without a burst, such as an MMDVM
// TAG_LOST. The Transmitter only sends, so Lost only runs the timers, as
// Tick does. It allocates only when dst has no room for MaxOutputs outputs.
func (t *Transmitter) Lost(now time.Duration, dst []Output) []Output {
	return t.Tick(now, dst)
}

func (t *Transmitter) grant(at time.Duration) {
	t.state, t.start, t.due, t.sent, t.authWaiting = TransmitterHeader, at, at, 0, false
}

func (t *Transmitter) authTick(dst []Output) []Output {
	if !t.authWaiting {
		var o *Output
		dst, o = t.emit(dst, OutputBurst)
		o.Burst, o.Hint = t.vt.Header(), t.dataHint(enums.DataTypeVoiceLCHeader)
		t.authWaiting, t.busySeen, t.waitUntil = true, false, t.due+t.chanAuth
		return dst
	}
	if !t.busySeen {
		t.due, t.head, t.length = t.waitUntil, 0, 0
		return t.finish(dst, EndNoOutbound)
	}
	t.authWaiting, t.due = false, t.waitUntil+t.lead
	if t.holdoffMax > 0 {
		t.due += t.rng.Duration(t.holdoffMax)
	}
	return dst
}
