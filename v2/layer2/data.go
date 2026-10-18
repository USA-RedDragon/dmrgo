package layer2

import "github.com/USA-RedDragon/dmrgo/v2/enums"

// Data is the PDU that a data or control burst carries. The types of
// package pdu that DecodeFromBytes returns implement it. Burst.Encode and
// BuildBurst accept only these types (see BuildBurst) and return an error
// for a Data of another type. A Full LC applies the RS(12,9) mask of its
// BurstDataType.
type Data interface {
	// String returns the fields of the PDU as text.
	String() string

	// DataType returns the data type of the bursts that carry the PDU.
	DataType() enums.DataType
}
