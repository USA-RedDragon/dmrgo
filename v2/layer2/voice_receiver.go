package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// VoiceReceiver tracks the superframe position of the voice bursts of one
// logical channel and reassembles the embedded Full LC of bursts B to E. The
// zero VoiceReceiver is ready to use. It is not safe for concurrent use.
// (ETSI TS 102 361-1 clause 5.1.2.1, clause 7.1.3)
type VoiceReceiver struct {
	// Strict selects the strict decode of DecodeEmbeddedLC for the embedded
	// LC. Reset keeps it.
	Strict bool

	position enums.VoiceBurstType
	lc       EmbeddedLCAssembler
}

// Reset forgets the superframe position and any partial embedded LC.
func (r *VoiceReceiver) Reset() {
	*r = VoiceReceiver{Strict: r.Strict}
}

// Receive takes the next decoded burst of the logical channel, in order, and
// sets b.VoiceBurst to its superframe position as the package doc describes.
// When burst E completes an embedded LC, Receive returns it with ok set, or
// with ErrEmbeddedLCUncorrectable. It returns ErrEmbeddedLCSequence when the
// burst discards a partial embedded LC. It allocates the payload struct of a
// completed LC; ReceiveInto does not.
func (r *VoiceReceiver) Receive(b *Burst) (lc pdu.FullLinkControl, ok bool, err error) {
	bits, res, done, err := r.receive(b)
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

// ReceiveInto takes the next burst as Receive does. When burst E completes
// an embedded LC, ReceiveInto decodes it into dst, with the payload struct
// of dst in p as pdu.DecodeFullLinkControlInto places it, and returns ok or
// ErrEmbeddedLCUncorrectable as Receive does. Otherwise it leaves dst as it
// is. It does not allocate.
func (r *VoiceReceiver) ReceiveInto(dst *pdu.FullLinkControl, p *pdu.FullLinkControlPayloads, b *Burst) (ok bool, err error) {
	bits, res, done, err := r.receive(b)
	if !done {
		return false, err
	}
	pdu.DecodeFullLinkControlInto(dst, p, embeddedFullLCCodeword(bits), enums.DataTypeVoiceLCHeader)
	dst.FEC = combineEmbeddedFEC(res, dst.FEC)
	if dst.FEC.Uncorrectable {
		return false, ErrEmbeddedLCUncorrectable
	}
	return true, nil
}

func (r *VoiceReceiver) receive(b *Burst) (lc [9]byte, res fec.Result, done bool, err error) {
	switch {
	case b.Kind == KindReverseChannel:
		b.VoiceBurst = enums.VoiceBurstUnknown
		return lc, res, false, nil
	case b.Kind != KindVoice || !b.HasEmbeddedSignalling() && !isVoiceSync(b.SyncPattern):
		b.VoiceBurst = enums.VoiceBurstUnknown
		err = r.lc.discard()
		r.Reset()
		return lc, res, false, err
	case !b.HasEmbeddedSignalling():
		b.VoiceBurst = enums.VoiceBurstA
		r.position = enums.VoiceBurstA
		return lc, res, false, r.lc.discard()
	}

	r.position = r.embeddedPosition(b)
	b.VoiceBurst = r.position
	lcss, ok := r.position.LCSS()
	if !ok || r.position == enums.VoiceBurstF {
		return lc, res, false, r.lc.discard()
	}
	r.lc.Strict = r.Strict
	return r.lc.add(fragmentWord(b.EmbeddedSignallingData), lcss)
}

func (r *VoiceReceiver) embeddedPosition(b *Burst) enums.VoiceBurstType {
	next := enums.VoiceBurstUnknown
	if r.position >= enums.VoiceBurstA && r.position < enums.VoiceBurstF {
		next = r.position + 1
	}
	if b.EmbeddedSignalling.FEC.Uncorrectable {
		return next
	}
	switch b.EmbeddedSignalling.LCSS {
	case enums.LCSSFirstFragment:
		return enums.VoiceBurstB
	case enums.LCSSLastFragment:
		return enums.VoiceBurstE
	case enums.LCSSContinuationFragment:
		if next == enums.VoiceBurstC || next == enums.VoiceBurstD {
			return next
		}
	case enums.LCSSSingleFragment:
		if next == enums.VoiceBurstF {
			return next
		}
	}
	return enums.VoiceBurstUnknown
}
