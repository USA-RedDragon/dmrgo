package layer3_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// ExampleDecodeServiceOptions reads the Service Options octet of an LC and
// writes it back.
func ExampleDecodeServiceOptions() {
	so := layer3.DecodeServiceOptions([1]byte{0x83})
	fmt.Println(so.IsEmergency, so.IsBroadcast, so.PriorityLevel)
	fmt.Printf("%#02x\n", so.Byte())
	// Output:
	// true false 3
	// 0x83
}
