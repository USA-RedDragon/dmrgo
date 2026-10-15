package dmrgo_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Example decodes a captured Voice LC Header burst, reads the call addresses
// from its Full LC, and encodes the burst back to the same 33 bytes.
func Example() {
	raw := [33]byte{
		68, 75, 3, 135, 36, 66, 12, 240, 21, 240,
		12, 161, 196, 109, 255, 87, 215, 93, 245, 222,
		49, 168, 53, 24, 63, 48, 61, 97, 56, 82,
		151, 134, 91,
	}
	burst, err := layer2.NewBurstFromBytes(raw)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(burst.SlotType.DataType, burst.FEC.Aggregate().Err())
	if lc, ok := burst.Data.(*pdu.FullLinkControl); ok {
		target, source, _ := lc.Addresses()
		fmt.Println("call from", source, "to", target)
	}
	encoded, err := burst.Encode()
	fmt.Println(encoded == raw, err)
	// Output:
	// Voice LC Header <nil>
	// call from 3191868 to 9990
	// true <nil>
}
