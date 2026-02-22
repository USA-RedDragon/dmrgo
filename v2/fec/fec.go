// Package fec provides shared types for Forward Error Correction across all FEC codecs.
package fec

// FECResult tracks the outcome of a single FEC decode/check operation.
type FECResult struct {
	// BitsChecked is the total number of FEC-protected bits in this decode step.
	BitsChecked int
	// ErrorsCorrected is the number of bit errors that were corrected.
	ErrorsCorrected int
	// Uncorrectable is true if errors exceeded the correction capability of the code.
	Uncorrectable bool
}

// HasErrors returns true if any errors were corrected or if the data is uncorrectable.
func (r FECResult) HasErrors() bool {
	return r.ErrorsCorrected > 0 || r.Uncorrectable
}

// BERCalculator accumulates FEC results to compute an overall Bit Error Rate.
//
// MMDVM computes BER as: BER% = (totalErrors * 100) / totalBitsChecked
// Consumers can aggregate per-call, per-N-bursts, sliding window, etc.
type BERCalculator struct {
	totalBitsChecked int
	totalErrors      int
}

// Add incorporates a single FECResult into the running BER calculation.
func (b *BERCalculator) Add(result FECResult) {
	b.totalBitsChecked += result.BitsChecked
	b.totalErrors += result.ErrorsCorrected
}

// AddBurst incorporates all FEC layers from a burst into the running BER calculation.
func (b *BERCalculator) AddBurst(stats BurstFECStats) {
	b.Add(stats.SlotType)
	b.Add(stats.EMB)
	b.Add(stats.Payload)
	b.Add(stats.Voice)
	b.Add(stats.PDU)
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

// Reset clears the accumulated BER state.
func (b *BERCalculator) Reset() {
	b.totalBitsChecked = 0
	b.totalErrors = 0
}

// BurstFECStats holds per-layer FEC results for a single DMR burst.
type BurstFECStats struct {
	SlotType FECResult // Golay(20,8,7) — 20 bits
	EMB      FECResult // QR(16,7,6) — 16 bits (voice bursts only)
	Payload  FECResult // BPTC/Trellis/Rate1 — 196/196/0 bits
	Voice    FECResult // Golay(24,12,8)+Golay(23,12,7) summed across 3 frames (voice only)
	PDU      FECResult // RS/CRC on the decoded PDU (FLC/CSBK/DataHeader)
	RC       FECResult // Single Burst BPTC + CRC-7 (Reverse Channel, voice bursts only)
}

// Aggregate returns combined stats across all FEC layers in this burst.
func (s BurstFECStats) Aggregate() FECResult {
	return FECResult{
		BitsChecked:     s.SlotType.BitsChecked + s.EMB.BitsChecked + s.Payload.BitsChecked + s.Voice.BitsChecked + s.PDU.BitsChecked + s.RC.BitsChecked,
		ErrorsCorrected: s.SlotType.ErrorsCorrected + s.EMB.ErrorsCorrected + s.Payload.ErrorsCorrected + s.Voice.ErrorsCorrected + s.PDU.ErrorsCorrected + s.RC.ErrorsCorrected,
		Uncorrectable:   s.SlotType.Uncorrectable || s.EMB.Uncorrectable || s.Payload.Uncorrectable || s.Voice.Uncorrectable || s.PDU.Uncorrectable || s.RC.Uncorrectable,
	}
}
