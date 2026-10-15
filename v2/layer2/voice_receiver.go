package layer2

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

var (
	// ErrEmbeddedLCSequence is returned by VoiceReceiver.Receive and
	// EmbeddedLCAssembler.Add when a burst does not continue the partial
	// embedded LC, which is discarded.
	ErrEmbeddedLCSequence = errors.New("embedded LC fragment out of sequence")
	// ErrEmbeddedLCUncorrectable is returned by VoiceReceiver.Receive and
	// EmbeddedLCAssembler.Add when the embedded LC of bursts B to E fails
	// its FEC or its checksum. It wraps fec.ErrUncorrectable.
	ErrEmbeddedLCUncorrectable = fmt.Errorf("embedded LC: %w", fec.ErrUncorrectable)
)

// VoiceReceiver tracks the superframe position of the voice bursts of one
// logical channel and reassembles the embedded Full LC of bursts B to E
// (ETSI TS 102 361-1 §5.1.2.1, §7.1.3). Burst decoding is stateless, so
// DecodeFromBytes leaves bursts B to F at VoiceBurstUnknown; Receive sets
// their position. The zero VoiceReceiver is ready to use. A VoiceReceiver
// is not safe for concurrent use.
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

// Receive takes the next decoded burst of the logical channel and sets
// b.VoiceBurst to its superframe position:
//   - a burst with a voice SYNC pattern is burst A;
//   - an embedded signalling burst with LCSS First is burst B and one with
//     LCSS Last is burst E, so a lost burst A does not lose its superframe;
//   - any other embedded signalling burst takes the position after the
//     previous burst when its LCSS fits that position: Continuation for C
//     and D, Single fragment for F. Otherwise it is VoiceBurstUnknown, as
//     on late entry before the first burst A, B or E;
//   - a data burst or a burst with the reserved SYNC pattern ends the voice
//     transmission and resets the receiver;
//   - a standalone RC burst is not part of the superframe and leaves the
//     receiver unchanged.
//
// When burst E completes the fragments of bursts B to E, Receive returns
// the embedded LC as DecodeEmbeddedFullLinkControl decodes it with ok set,
// or that LC with ErrEmbeddedLCUncorrectable. It returns
// ErrEmbeddedLCSequence when a burst discards a partial embedded LC. A
// burst with an uncorrectable EMB takes the position after the previous
// burst and keeps its fragment for the BPTC and checksum to check. Receive
// expects every burst of the logical channel in order; a lost burst inside
// a superframe can shift the position of the bursts after it until the next
// burst A, B or E. Receive collects the fragments with an
// EmbeddedLCAssembler, with the LCSS of the burst position.
func (r *VoiceReceiver) Receive(b *Burst) (lc pdu.FullLinkControl, ok bool, err error) {
	switch {
	case b.Kind == KindReverseChannel:
		b.VoiceBurst = enums.VoiceBurstUnknown
		return lc, false, nil
	case b.Kind != KindVoice || !b.HasEmbeddedSignalling() && !isVoiceSync(b.SyncPattern):
		b.VoiceBurst = enums.VoiceBurstUnknown
		err = r.lc.discard()
		r.Reset()
		return lc, false, err
	case !b.HasEmbeddedSignalling():
		b.VoiceBurst = enums.VoiceBurstA
		r.position = enums.VoiceBurstA
		return lc, false, r.lc.discard()
	}

	r.position = r.embeddedPosition(b)
	b.VoiceBurst = r.position
	lcss, ok := r.position.LCSS()
	if !ok || r.position == enums.VoiceBurstF {
		return lc, false, r.lc.discard()
	}
	r.lc.Strict = r.Strict
	bits, res, done, err := r.lc.add(b.EmbeddedSignallingData, lcss)
	if !done {
		return lc, false, err
	}
	lc = embeddedFullLinkControl(bits, res)
	if lc.FEC.Uncorrectable {
		return lc, false, ErrEmbeddedLCUncorrectable
	}
	return lc, true, nil
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
