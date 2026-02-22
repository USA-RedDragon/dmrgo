package elements

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

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

// NewCdefParmsFromBits constructs a CdefParms from a 58-bit array.
// Only valid when Cdeftype=0. Other Cdeftype values are reserved.
func NewCdefParmsFromBits(bits [58]bit.Bit) *CdefParms {
	return &CdefParms{
		Channel: bit.BitsToUint16(bits[:], 0, 12),
		TXMHz:   bit.BitsToUint16(bits[:], 12, 10),
		TXKHz:   bit.BitsToUint16(bits[:], 22, 13),
		RXMHz:   bit.BitsToUint16(bits[:], 35, 10),
		RXKHz:   bit.BitsToUint16(bits[:], 45, 13),
	}
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
