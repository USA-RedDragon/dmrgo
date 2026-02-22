package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
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
// The 4 × 17-bit payloads form the BPTC transmit matrix, which is
// deinterleaved and FEC-decoded to recover the 36-bit Short LC PDU.

// ShortLCAssembler accumulates 4 CACH signalling payloads and decodes
// the assembled Short LC PDU.
type ShortLCAssembler struct {
	fragments [4][CACHPayloadBits]bit.Bit
	count     int
}

// Reset clears the assembler state for reuse.
func (a *ShortLCAssembler) Reset() {
	a.count = 0
}

// Count returns the number of fragments accumulated so far.
func (a *ShortLCAssembler) Count() int {
	return a.count
}

// AddFragment appends a 17-bit CACH signalling payload to the assembler.
// Returns true when 4 fragments have been collected and the assembler
// is ready for Complete().
func (a *ShortLCAssembler) AddFragment(payload [CACHPayloadBits]bit.Bit) bool {
	if a.count >= 4 {
		// Already full — caller should have called Complete() or Reset()
		return true
	}
	a.fragments[a.count] = payload
	a.count++
	return a.count >= 4
}

// Complete decodes the assembled Short LC PDU from the 4 accumulated
// fragments. Returns the decoded ShortLC and a combined FEC result
// covering both BPTC FEC and CRC-8 verification.
//
// The assembler is NOT automatically reset — call Reset() to reuse.
func (a *ShortLCAssembler) Complete() (pdu.ShortLC, fec.FECResult) {
	// Decode BPTC to recover the 36-bit Short LC PDU
	info, bptcResult := bptc.DecodeCACHBPTC(a.fragments)

	// Decode the Short LC PDU (CRC-8 check + SLCO dispatch)
	var slcBits [36]bit.Bit
	copy(slcBits[:], info[:])
	slc, crcResult := pdu.DecodeShortLC(slcBits)

	// Combine FEC results
	combined := fec.FECResult{
		BitsChecked:     bptcResult.BitsChecked + crcResult.BitsChecked,
		ErrorsCorrected: bptcResult.ErrorsCorrected + crcResult.ErrorsCorrected,
		Uncorrectable:   bptcResult.Uncorrectable || crcResult.Uncorrectable,
	}
	slc.FEC = combined

	return slc, combined
}

// DecodeShortLCFromFragments is a convenience function that decodes a
// Short LC PDU directly from 4 × 17-bit CACH signalling payloads.
func DecodeShortLCFromFragments(fragments [4][CACHPayloadBits]bit.Bit) (pdu.ShortLC, fec.FECResult) {
	var a ShortLCAssembler
	for i := range fragments {
		a.AddFragment(fragments[i])
	}
	return a.Complete()
}
