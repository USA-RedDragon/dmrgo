package fsktx

// Peak returns the most that the output of an MCUAudio of t at level
// reaches from its offset, in codes, over every run of symbols.
func (t AudioTable) Peak(level int) int {
	lo, hi := t.bounds(int32((level<<15 + tableUnits/2) / tableUnits))
	return max(-lo, hi)
}

// Peak returns the most that the output of a Tier1 or Tier2 Audio of
// Offset 0 reaches from 0 over every run of symbols, in codes, and 0 for
// Tier0.
func (a *Audio) Peak() float64 {
	if a == nil || a.k == nil {
		return 0
	}
	lo, hi := a.bounds()
	return max(-lo, hi)
}
