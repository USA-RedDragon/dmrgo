package fec

import "errors"

// ErrUncorrectable is the error that Result.Err returns for a Result with
// Uncorrectable set. Errors of this module that report an uncorrectable
// FEC or CRC result wrap it, so errors.Is(err, ErrUncorrectable) finds them.
var ErrUncorrectable = errors.New("fec: uncorrectable")

// Result is the outcome of one FEC decode or check step. ErrorsCorrected
// over BitsChecked is a bit error rate. A step that only detects errors,
// such as a CRC or checksum, reports 0 for both counts and sets
// Uncorrectable when its check fails.
type Result struct {
	// BitsChecked is the number of coded bits that an error correcting code
	// covers in this decode step.
	BitsChecked int
	// ErrorsCorrected is the number of bit errors that were corrected.
	ErrorsCorrected int
	// Uncorrectable is true if errors exceeded the correction capability of
	// the code or an error detecting check failed.
	Uncorrectable bool
}

// Combine returns the result of two decode steps over disjoint bits, or of
// a correcting step and a detecting step on its output: the bit counts add
// and Uncorrectable is set if either step set it.
func (r Result) Combine(o Result) Result {
	return Result{
		BitsChecked:     r.BitsChecked + o.BitsChecked,
		ErrorsCorrected: r.ErrorsCorrected + o.ErrorsCorrected,
		Uncorrectable:   r.Uncorrectable || o.Uncorrectable,
	}
}

// Err returns ErrUncorrectable when Uncorrectable is set, and nil
// otherwise. Corrected errors are not an error.
func (r Result) Err() error {
	if r.Uncorrectable {
		return ErrUncorrectable
	}
	return nil
}

// HasErrors returns true if any errors were corrected or if the data is uncorrectable.
func (r Result) HasErrors() bool {
	return r.ErrorsCorrected > 0 || r.Uncorrectable
}

// BERCalculator accumulates Results into an overall bit error rate. The
// zero value is an empty calculator.
type BERCalculator struct {
	totalBitsChecked int
	totalErrors      int
	uncorrectable    int
}

// Add incorporates a single Result into the running BER calculation.
func (b *BERCalculator) Add(result Result) {
	b.totalBitsChecked += result.BitsChecked
	b.totalErrors += result.ErrorsCorrected
	if result.Uncorrectable {
		b.uncorrectable++
	}
}

// AddBurst incorporates the Aggregate of the FEC layers of a burst into the
// running BER calculation.
func (b *BERCalculator) AddBurst(stats BurstFECStats) {
	b.Add(stats.Aggregate())
}

// BER returns the bit error rate as a percentage from 0 to 100, as MMDVM
// computes it. It returns 0 when no bits have been checked.
func (b *BERCalculator) BER() float64 {
	if b.totalBitsChecked == 0 {
		return 0
	}
	return float64(b.totalErrors) * 100.0 / float64(b.totalBitsChecked)
}

// TotalErrors returns the total number of corrected errors accumulated.
func (b *BERCalculator) TotalErrors() int {
	return b.totalErrors
}

// TotalBitsChecked returns the total number of FEC-protected bits accumulated.
func (b *BERCalculator) TotalBitsChecked() int {
	return b.totalBitsChecked
}

// Uncorrectable returns the number of accumulated results with
// Uncorrectable set. The corrected errors and bits of such a result count
// toward the BER, but its uncorrected errors are unknown.
func (b *BERCalculator) Uncorrectable() int {
	return b.uncorrectable
}

// Reset clears the accumulated BER state.
func (b *BERCalculator) Reset() {
	*b = BERCalculator{}
}

// BurstFECStats holds the FEC results of each layer of one DMR burst. It has
// no voice layer, because the vocoder FEC is codec specific. A caller that
// decodes AMBE+2 frames adds their result (layer2.Burst.AMBEFrames).
type BurstFECStats struct {
	SlotType Result // Golay (20,8,7), 20 bits
	EMB      Result // QR (16,7,6), 16 bits, bursts with embedded signalling only
	Payload  Result // BPTC (196 bits), trellis (196 bits) or rate 1 (0 bits)
	// PDU is the RS(12,9) correction (96 bits) or the CRC check (0 bits) of
	// the information bits that the Payload code returned (FLC/CSBK/DataHeader).
	PDU Result
	RC  Result // single burst BPTC (32 bits) and CRC-7 check, Reverse Channel only
}

// Aggregate returns combined stats across all FEC layers in this burst. The
// PDU bits are a subset of the Payload bits, so Aggregate counts the errors
// that the PDU layer corrected but not its BitsChecked.
func (s BurstFECStats) Aggregate() Result {
	agg := s.SlotType.Combine(s.EMB).Combine(s.Payload).Combine(s.RC)
	agg.ErrorsCorrected += s.PDU.ErrorsCorrected
	agg.Uncorrectable = agg.Uncorrectable || s.PDU.Uncorrectable
	return agg
}
