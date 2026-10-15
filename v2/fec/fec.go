// Package fec provides shared types for Forward Error Correction across all FEC codecs.
package fec

// Result tracks the outcome of a single FEC decode/check operation.
//
// BitsChecked and ErrorsCorrected count bits of an error correcting code
// at this step, so ErrorsCorrected over BitsChecked is a bit error rate. A
// step that only detects errors, such as a CRC or checksum, reports
// BitsChecked 0 and ErrorsCorrected 0, and sets Uncorrectable when its
// check fails.
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

// HasErrors returns true if any errors were corrected or if the data is uncorrectable.
func (r Result) HasErrors() bool {
	return r.ErrorsCorrected > 0 || r.Uncorrectable
}

// BERCalculator accumulates FEC results to compute an overall Bit Error Rate.
//
// MMDVM computes BER as: BER% = (totalErrors * 100) / totalBitsChecked
// Consumers can aggregate per-call, per-N-bursts, sliding window, etc.
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

// BER returns the current Bit Error Rate as a percentage (0.0-100.0).
// Returns 0 if no bits have been checked.
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

// BurstFECStats holds per-layer FEC results for a single DMR burst. It has
// no voice layer: the vocoder FEC is codec specific, and a caller that
// decodes AMBE+2 frames adds their result (layer2.Burst.AMBEFrames).
type BurstFECStats struct {
	SlotType Result // Golay(20,8,7) — 20 bits
	EMB      Result // QR(16,7,6) — 16 bits (voice bursts only)
	Payload  Result // BPTC/Trellis/Rate1 — 196/196/0 bits
	// PDU is the RS(12,9) correction (96 bits) or the CRC check (0 bits) of
	// the information bits that the Payload code returned (FLC/CSBK/DataHeader).
	PDU Result
	RC  Result // Single Burst BPTC, 32 bits, + CRC-7 check (Reverse Channel, voice bursts only)
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
