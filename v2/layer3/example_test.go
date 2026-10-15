package layer3_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// ExampleServiceOptionsFromByte reads the Service Options octet of an LC and
// writes it back.
func ExampleServiceOptionsFromByte() {
	so := layer3.ServiceOptionsFromByte(0x83)
	fmt.Println(so.IsEmergency, so.IsBroadcast, so.PriorityLevel)
	fmt.Printf("%#02x\n", so.Byte())
	// Output:
	// true false 3
	// 0x83
}
