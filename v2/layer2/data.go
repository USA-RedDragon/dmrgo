package layer2

import "github.com/USA-RedDragon/dmrgo/v2/enums"

// Data is the PDU that a data or control burst carries. The types of
// package pdu that DecodeFromBytes returns implement it. Burst.Encode and
// BuildBurst accept any Data with an Encode method that returns the
// information bits of the PDU, before the BPTC(196,96), Rate 3/4 trellis or
// Rate 1 coding of the burst, packed, the first bit in the most significant
// bit of byte 0: a [12]byte for the BPTC data types, an [18]byte for Rate
// 3/4 or a [24]byte for Rate 1, and an error. A Full LC applies the RS(12,9)
// mask of its BurstDataType.
type Data interface {
	// String returns the fields of the PDU as text.
	String() string

	// DataType returns the data type of the bursts that carry the PDU.
	DataType() enums.DataType
}
