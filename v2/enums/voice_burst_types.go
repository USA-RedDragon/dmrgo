package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// VoiceBurstType is the position of a voice burst in a voice superframe. The
// zero value is VoiceBurstUnknown. (ETSI TS 102 361-1 clause 5.1.2.1)
type VoiceBurstType uint8

// The VoiceBurstType values.
const (
	// VoiceBurstUnknown is a burst whose superframe position is not known.
	VoiceBurstUnknown VoiceBurstType = iota

	VoiceBurstA // burst A, which carries the voice SYNC
	VoiceBurstB // burst B
	VoiceBurstC // burst C
	VoiceBurstD // burst D
	VoiceBurstE // burst E
	VoiceBurstF // burst F
)

// VoiceBurstAt returns the voice burst at index i of a superframe, 0 for
// burst A to 5 for burst F. ok is false when i is outside 0 to 5.
func VoiceBurstAt(i int) (v VoiceBurstType, ok bool) {
	bursts := [...]VoiceBurstType{VoiceBurstA, VoiceBurstB, VoiceBurstC, VoiceBurstD, VoiceBurstE, VoiceBurstF}
	if i < 0 || i >= len(bursts) {
		return VoiceBurstUnknown, false
	}
	return bursts[i], true
}

// Index returns the index of v in a superframe, 0 for burst A to 5 for
// burst F. ok is false for VoiceBurstUnknown and values above VoiceBurstF.
func (v VoiceBurstType) Index() (i int, ok bool) {
	if v < VoiceBurstA || v > VoiceBurstF {
		return 0, false
	}
	return int(v - VoiceBurstA), true
}

// LCSS returns the LCSS in the EMB of voice burst v in a superframe with an
// embedded LC: First for B, Continuation for C and D, Last for E and Single
// for F. ok is false for burst A, which has a SYNC pattern instead of an EMB,
// and for an unknown position. (ETSI TS 102 361-1 clause 7.1.3)
func (v VoiceBurstType) LCSS() (lcss LCSS, ok bool) {
	switch v {
	case VoiceBurstB:
		return LCSSFirstFragment, true
	case VoiceBurstC, VoiceBurstD:
		return LCSSContinuationFragment, true
	case VoiceBurstE:
		return LCSSLastFragment, true
	case VoiceBurstF:
		return LCSSSingleFragment, true
	case VoiceBurstUnknown, VoiceBurstA:
	}
	return 0, false
}

// String returns the name of a voice burst type. It allocates only for a
// value over VoiceBurstF.
func (v VoiceBurstType) String() string {
	switch v {
	case VoiceBurstUnknown:
		return unknownName
	case VoiceBurstA:
		return "A"
	case VoiceBurstB:
		return "B"
	case VoiceBurstC:
		return "C"
	case VoiceBurstD:
		return "D"
	case VoiceBurstE:
		return "E"
	case VoiceBurstF:
		return "F"
	}
	return "VoiceBurstType(" + text.Int(int64(v)) + ")"
}
