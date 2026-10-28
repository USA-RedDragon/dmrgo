package fsktx

// Peak returns the most that the output of an MCUAudio of t at level
// reaches from its offset, in codes, over every run of symbols.
func (t AudioTable) Peak(level int) int {
	lo, hi := t.bounds(int32((level<<15 + tableUnits/2) / tableUnits))
	return max(-lo, hi)
}

// PeakPerLevel returns the most that the output of a Tier1 or Tier2 Audio
// reaches from its Offset over every run of symbols, in codes per code of
// Level, and 0 for Tier0.
func (a *Audio) PeakPerLevel() float64 {
	if a == nil || a.k == nil {
		return 0
	}
	return a.k.peak / (3 * DeviationHz)
}
