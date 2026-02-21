package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 7.1 Voice channel coding
type Vocoder struct {
	Frames [3]vocoder.VocoderFrame `dmr:"bits:0-215,delegate,noptr,stride:72"`
}

// CorrectedErrors returns the total number of corrected bit errors in the vocoder frames.
func (vc *Vocoder) CorrectedErrors() int {
	var count int
	for _, f := range vc.Frames {
		count += f.FEC.ErrorsCorrected
	}
	return count
}

// Uncorrectable returns true if any of the vocoder frames were uncorrectable.
func (vc *Vocoder) Uncorrectable() bool {
	for _, f := range vc.Frames {
		if f.FEC.Uncorrectable {
			return true
		}
	}
	return false
}

// HasError returns true if any error correction was performed or if the data is uncorrectable.
func (vc *Vocoder) HasError() bool {
	return vc.CorrectedErrors() > 0 || vc.Uncorrectable()
}

// FECResult returns the aggregated FEC result across all 3 vocoder frames.
func (vc *Vocoder) FECResult() fec.FECResult {
	result := fec.FECResult{}
	for _, f := range vc.Frames {
		result.BitsChecked += f.FEC.BitsChecked
		result.ErrorsCorrected += f.FEC.ErrorsCorrected
		if f.FEC.Uncorrectable {
			result.Uncorrectable = true
		}
	}
	return result
}
