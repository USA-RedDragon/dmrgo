package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// Data is the PDU that a data or control burst carries. The types of
// package pdu that DecodeFromBytes returns implement it, and Burst.Encode
// accepts any type that does.
type Data interface {
	fmt.Stringer

	// DataType returns the data type of the bursts that carry the PDU.
	DataType() enums.DataType

	// EncodeBits returns the information bits of the PDU, before the
	// BPTC(196,96), Rate 3/4 trellis or Rate 1 coding of the burst: 96
	// bits for the BPTC data types, 144 for Rate 3/4 and 192 for Rate 1.
	// A Full LC applies the RS(12,9) mask of its BurstDataType.
	EncodeBits() ([]bit.Bit, error)
}
