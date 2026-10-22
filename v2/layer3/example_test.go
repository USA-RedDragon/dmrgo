package layer3_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// ExampleDecodeServiceOptions decodes a service options octet and
// encodes it back.
func ExampleDecodeServiceOptions() {
	so := layer3.DecodeServiceOptions([1]byte{0x83})
	fmt.Println(so.IsEmergency, so.IsBroadcast, so.PriorityLevel)
	b, err := so.Field()
	fmt.Printf("%#02x %v\n", b, err)
	// Output:
	// true false 3
	// 0x83 <nil>
}
