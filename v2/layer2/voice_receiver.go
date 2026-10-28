package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// VoiceReceiver places the voice bursts of one logical channel in their
// superframe with a PositionTracker and reassembles the embedded Full LC of
// bursts B to E with an EmbeddedLCAssembler, by the rules of the package
// doc. The zero VoiceReceiver is ready to use. It is not safe for
// concurrent use. (ETSI TS 102 361-1 clause 5.1.2.1, clause 7.1.3)
type VoiceReceiver struct {
	pos PositionTracker
	lc  EmbeddedLCAssembler
}

// Reset clears the call and any partial embedded LC.
func (r *VoiceReceiver) Reset() {
	if r == nil {
		return
	}
	*r = VoiceReceiver{}
}

// Receive takes the next decoded burst b of the logical channel with its
// Hint h, and sets b.VoiceBurst to its superframe position, as
// PositionTracker.PlaceDecoded places it with one step per burst. A nil h
// means the zero Hint, as for a burst from an RF receiver. When burst E
// completes an embedded LC, Receive returns it with ok set, or with
// ErrEmbeddedLCUncorrectable. It returns ErrEmbeddedLCSequence when the
// burst discards a partial embedded LC. A duplicate or late burst gets its
// position but does not go to the LC. A nil b applies only the stream,
// SyncLost and End of h, as PositionTracker.Place does for a nil raw. It
// does not allocate.
func (r *VoiceReceiver) Receive(b *Burst, h *Hint) (lc pdu.FullLinkControl, ok bool, err error) {
	if r == nil {
		return pdu.FullLinkControl{}, false, dmrgo.ErrNilReceiver
	}
	bits, res, done, err := r.receive(b, h)
	if !done {
		return lc, false, err
	}
	lc = pdu.DecodeFullLinkControl(embeddedFullLCCodeword(bits), enums.DataTypeVoiceLCHeader)
	lc.FEC = combineEmbeddedFEC(res, lc.FEC)
	if lc.FEC.Uncorrectable {
		return lc, false, ErrEmbeddedLCUncorrectable
	}
	return lc, true, nil
}

func (r *VoiceReceiver) receive(b *Burst, hp *Hint) (lc [9]byte, res fec.Result, done bool, err error) {
	var h Hint
	if hp != nil {
		h = *hp
	}
	if r.pos.hint(&h) {
		err = r.lc.discard()
	}
	if b != nil {
		lc, res, done, err = r.place(b, &h, err)
	}
	if h.End == EndStrong {
		r.pos.end()
		if e := r.lc.discard(); err == nil {
			err = e
		}
	}
	return lc, res, done, err
}

func (r *VoiceReceiver) place(b *Burst, h *Hint, err error) (lc [9]byte, res fec.Result, done bool, _ error) {
	switch b.Kind {
	case KindVoice:
	case KindData, KindUnknown:
		b.VoiceBurst = enums.VoiceBurstUnknown
		if e := r.lc.discard(); err == nil {
			err = e
		}
		r.pos.end()
		if b.Kind == KindData && (b.SlotType.DataType == enums.DataTypeVoiceLCHeader || b.SlotType.DataType == enums.DataTypePIHeader) {
			r.pos.Header(h)
		}
		return lc, res, false, err
	case KindReverseChannel:
		b.VoiceBurst = enums.VoiceBurstUnknown
		return lc, res, false, err
	}
	if _, p := r.pos.PlaceDecoded(b, h, 1); p != PlacementInOrder {
		b.VoiceBurst = h.VoiceBurst
		return lc, res, false, err
	}
	b.VoiceBurst = h.VoiceBurst
	if h.End == EndWeak {
		r.pos.known = false
	}
	lcss, ok := b.VoiceBurst.LCSS()
	if !ok || b.VoiceBurst == enums.VoiceBurstF || !b.HasEmbeddedSignalling() {
		if e := r.lc.discard(); err == nil {
			err = e
		}
		return lc, res, false, err
	}
	lc, res, done, e := r.lc.add(fragmentWord(b.EmbeddedSignallingData), lcss)
	if err == nil {
		err = e
	}
	return lc, res, done, err
}
