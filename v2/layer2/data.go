package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// Data is the PDU that a data or control burst carries. The types of
// package pdu that DecodeFromBytes returns implement it. Burst.Encode and
// BuildBurst accept any Data with an Encode method that returns the
// information bits of the PDU, before the BPTC(196,96), Rate 3/4 trellis or
// Rate 1 coding of the burst, as a [96]bit.Bit for the BPTC data types, a
// [144]bit.Bit for Rate 3/4 or a [192]bit.Bit for Rate 1, and an error. A
// Full LC applies the RS(12,9) mask of its BurstDataType.
type Data interface {
	fmt.Stringer

	// DataType returns the data type of the bursts that carry the PDU.
	DataType() enums.DataType
}
