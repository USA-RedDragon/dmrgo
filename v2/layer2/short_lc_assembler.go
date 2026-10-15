package layer2

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ETSI TS 102 361-1 §7.1.4 — Short Link Control in CACH
//
// A Short LC PDU is assembled from 4 consecutive CACH signalling payloads.
// The LCSS field in each CACH's TACT indicates the fragment sequence:
//   - LCSS=01: First fragment
//   - LCSS=11: Continuation fragment (×2)
//   - LCSS=10: Last fragment
//
// LCSS=00 is not used: there is no single fragment LC for CACH signalling
// (Table 9.20 note).
//
// The 4 × 17-bit payloads form the BPTC transmit matrix, which is
// deinterleaved and FEC-decoded to recover the 36-bit Short LC PDU.

var (
	// ErrShortLCSequence is returned by ShortLCAssembler.AddFragment when a
	// CACH fragment arrives out of sequence or has an LCSS that is not valid
	// for Short LC. The partial Short LC is discarded.
	ErrShortLCSequence = errors.New("short LC fragment out of sequence")
	// ErrShortLCIncomplete is returned by ShortLCAssembler.Complete when the
	// assembler does not hold 4 fragments in sequence.
	ErrShortLCIncomplete = errors.New("short LC needs 4 fragments in sequence")
)

// ShortLCAssembler accumulates 4 CACH signalling payloads and decodes
// the assembled Short LC PDU.
type ShortLCAssembler struct {
	fragments [4][CACHPayloadBits]bit.Bit
	count     int
}

// Reset clears the assembler state for reuse.
func (a *ShortLCAssembler) Reset() {
	a.fragments = [4][CACHPayloadBits]bit.Bit{}
	a.count = 0
}

// Count returns the number of fragments accumulated so far.
func (a *ShortLCAssembler) Count() int {
	return a.count
}

// AddFragment adds a 17-bit CACH signalling payload with the LCSS from the
// same CACH burst's TACT. A First fragment always starts a new Short LC.
// Continuation fragments must follow the First or the first Continuation,
// and a Last fragment must follow two Continuations. Any other LCSS or order
// discards the partial Short LC and returns ErrShortLCSequence.
//
// Returns true when the Last fragment completes the sequence and the
// assembler is ready for Complete().
func (a *ShortLCAssembler) AddFragment(lcss enums.LCSS, payload [CACHPayloadBits]bit.Bit) (bool, error) {
	switch lcss {
	case enums.LCSSFirstFragment:
		a.Reset()
	case enums.LCSSContinuationFragment:
		if a.count != 1 && a.count != 2 {
			a.Reset()
			return false, ErrShortLCSequence
		}
	case enums.LCSSLastFragment:
		if a.count != 3 {
			a.Reset()
			return false, ErrShortLCSequence
		}
	case enums.LCSSSingleFragment:
		a.Reset()
		return false, ErrShortLCSequence
	default:
		a.Reset()
		return false, ErrShortLCSequence
	}
	a.fragments[a.count] = payload
	a.count++
	return a.count == 4, nil
}

// Complete decodes the assembled Short LC PDU from the 4 accumulated
// fragments and resets the assembler. The FEC field of the ShortLC is the
// combined result of the BPTC FEC and the CRC-8 check. Complete returns
// ErrShortLCIncomplete if the assembler does not hold a full sequence.
func (a *ShortLCAssembler) Complete() (pdu.ShortLC, error) {
	if a.count != 4 {
		return pdu.ShortLC{}, ErrShortLCIncomplete
	}
	defer a.Reset()

	// Decode BPTC to recover the 36-bit Short LC PDU
	info, bptcResult := bptc.DecodeCACH(a.fragments)

	// Decode the Short LC PDU (CRC-8 check + SLCO dispatch)
	var slcBits [36]bit.Bit
	copy(slcBits[:], info[:])
	slc := pdu.DecodeShortLC(slcBits)
	slc.FEC = bptcResult.Combine(slc.FEC)
	return slc, nil
}

// DecodeShortLCFromFragments is a convenience function that decodes a
// Short LC PDU directly from 4 × 17-bit CACH signalling payloads, given in
// the order First, two Continuations, Last. The FEC field is set as
// Complete sets it.
func DecodeShortLCFromFragments(fragments [4][CACHPayloadBits]bit.Bit) pdu.ShortLC {
	var a ShortLCAssembler
	a.fragments = fragments
	a.count = len(fragments)
	slc, _ := a.Complete()
	return slc
}
