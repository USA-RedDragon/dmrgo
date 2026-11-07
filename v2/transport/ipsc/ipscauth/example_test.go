package ipscauth_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth"
)

// ExampleKey_Sign signs the registration request of an XPR 8400 with the
// key 123456789abcdef1234 of its configuration, and checks the trailer.
func ExampleKey_Sign() {
	k, err := ipscauth.ParseKey("123456789abcdef1234")
	if err != nil {
		fmt.Println(err)
		return
	}
	key := ipscauth.NewKey(k)
	p, _ := hex.DecodeString("900004977e6a0000805c04080400")
	signed, _ := key.Sign(p)
	fmt.Printf("%x\n", signed[len(signed)-ipscauth.TrailerLen:])

	body, ok := key.Verify(signed)
	fmt.Printf("%x %v\n", body, ok)
	// Output:
	// 728dcee3d5b8009d4a07
	// 900004977e6a0000805c04080400 true
}
