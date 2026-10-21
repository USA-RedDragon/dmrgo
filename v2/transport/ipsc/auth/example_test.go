package auth_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/auth"
)

// ExampleKey_Sign signs the registration request of an XPR 8400 with the
// key 123456789abcdef1234 of its configuration, and checks the trailer.
func ExampleKey_Sign() {
	k, err := auth.ParseKey("123456789abcdef1234")
	if err != nil {
		fmt.Println(err)
		return
	}
	key := auth.NewKey(k)
	p, _ := hex.DecodeString("900004ce2e6a0000805c04080400")
	signed, _ := key.Sign(p)
	fmt.Printf("%x\n", signed[len(signed)-auth.TrailerLen:])

	body, ok := key.Verify(signed)
	fmt.Printf("%x %v\n", body, ok)
	// Output:
	// 5a09decc4db02fba02dc
	// 900004ce2e6a0000805c04080400 true
}
