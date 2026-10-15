package layer3

// CdefParms represents the Channel Definition Parameters (Cdeftype=0).
// This carries absolute TX/RX frequency information in 58 bits.
// ETSI TS 102 361-4 — §7.2.42, Table 7.103
type CdefParms struct {
	Channel uint16 `dmr:"bits:0-11"`
	TXMHz   uint16 `dmr:"bits:12-21"`
	TXKHz   uint16 `dmr:"bits:22-34"`
	RXMHz   uint16 `dmr:"bits:35-44"`
	RXKHz   uint16 `dmr:"bits:45-57"`
}

// TXFrequencyMHz returns the transmit frequency in MHz.
// Calculated as TXMHz + TXKHz × 0.000125.
// ETSI TS 102 361-4 — Annex C
func (c *CdefParms) TXFrequencyMHz() float64 {
	return float64(c.TXMHz) + float64(c.TXKHz)*0.000125
}

// RXFrequencyMHz returns the receive frequency in MHz.
// Calculated as RXMHz + RXKHz × 0.000125.
// ETSI TS 102 361-4 — Annex C
func (c *CdefParms) RXFrequencyMHz() float64 {
	return float64(c.RXMHz) + float64(c.RXKHz)*0.000125
}
