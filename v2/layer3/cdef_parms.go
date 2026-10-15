package layer3

// CdefParms represents the Channel Definition Parameters (Cdeftype=0).
// This carries absolute TX/RX frequency information in 58 bits.
// ETSI TS 102 361-4 — §7.2.42, Table 7.103
//
// The field names are the IE names of the spec. TXKHz and RXKHz are not in
// kHz: they count 125 Hz steps above TXMHz and RXMHz.
type CdefParms struct {
	Channel uint16 `dmr:"bits:0-11"`
	TXMHz   uint16 `dmr:"bits:12-21"`
	TXKHz   uint16 `dmr:"bits:22-34"`
	RXMHz   uint16 `dmr:"bits:35-44"`
	RXKHz   uint16 `dmr:"bits:45-57"`
}

// TXFrequencyHz returns the transmit frequency in Hz, TXMHz MHz plus TXKHz
// steps of 125 Hz.
func (c *CdefParms) TXFrequencyHz() uint64 {
	return uint64(c.TXMHz)*1_000_000 + uint64(c.TXKHz)*125
}

// RXFrequencyHz returns the receive frequency in Hz, RXMHz MHz plus RXKHz
// steps of 125 Hz.
func (c *CdefParms) RXFrequencyHz() uint64 {
	return uint64(c.RXMHz)*1_000_000 + uint64(c.RXKHz)*125
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
