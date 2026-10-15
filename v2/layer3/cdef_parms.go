// Package layer3 defines the layer 3 information elements of ETSI TS 102
// 361-2 and 361-4 that the PDUs of package pdu carry, such as service
// options, source type and channel definitions.
package layer3

// CdefParms is the Channel Definition Parameters (Cdeftype 0) information
// element, which carries the absolute TX and RX frequencies in 58 bits (ETSI TS
// 102 361-4 clause 7.2.42, Table 7.103).
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
func (s *CdefParms) TXFrequencyHz() uint64 {
	return uint64(s.TXMHz)*1_000_000 + uint64(s.TXKHz)*125
}

// RXFrequencyHz returns the receive frequency in Hz, RXMHz MHz plus RXKHz
// steps of 125 Hz.
func (s *CdefParms) RXFrequencyHz() uint64 {
	return uint64(s.RXMHz)*1_000_000 + uint64(s.RXKHz)*125
}

// TXFrequencyMHz returns the transmit frequency in MHz.
// Calculated as TXMHz + TXKHz × 0.000125.
// ETSI TS 102 361-4 — Annex C
func (s *CdefParms) TXFrequencyMHz() float64 {
	return float64(s.TXMHz) + float64(s.TXKHz)*0.000125
}

// RXFrequencyMHz returns the receive frequency in MHz.
// Calculated as RXMHz + RXKHz × 0.000125.
// ETSI TS 102 361-4 — Annex C
func (s *CdefParms) RXFrequencyMHz() float64 {
	return float64(s.RXMHz) + float64(s.RXKHz)*0.000125
}
