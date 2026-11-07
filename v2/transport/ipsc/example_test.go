package ipsc_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
)

// ExampleDecodeUser decodes a voice LC header and a voice burst that a
// Motorola XPR 8400 sent, and the kind of each burst for the service
// machines.
func ExampleDecodeUser() {
	for _, s := range []string{
		"800004977e1d2dedcf0000090200000d9e2080dd7b5f482e7d3e0000000001c0000a800a00600000000000092dedcf644e270071466f",
		"800004977e012dedcf000009020000465e00805d2d490a223107000000000a1440541cbd8ee6d793cd54372cbe82e8c4004a2a78",
	} {
		p, _ := hex.DecodeString(s)
		u, err := ipsc.DecodeUser(p)
		if err != nil {
			fmt.Println(err)
			return
		}
		h := u.Hint(0)
		fmt.Println(u.Opcode, u.Source, u.Destination, u.Info.Slot(), u.IsVoice(), h.Kind)
	}
	// Output:
	// GroupVoice 3009999 9 TS2 false Data
	// GroupVoice 3009999 9 TS1 true Voice
}

// ExampleDecodeControl decodes the registration request of an XPR 8400 and
// builds the keepalive request of peer 3003299.
func ExampleDecodeControl() {
	p, _ := hex.DecodeString("900004977e6a0000805c04080400")
	c, err := ipsc.DecodeControl(p)
	fmt.Println(c.Opcode, c.ID, c.Mode.Digital(), c.Mode.Operational(), err)

	alive, _ := (&ipsc.Control{Opcode: ipsc.OpAliveRequest, ID: 3009990, Mode: c.Mode, Flags: c.Flags, Version: c.Version}).AppendBinary(nil)
	fmt.Printf("%x\n", alive)
	// Output:
	// RegisterRequest 300926 true true <nil>
	// 96002dedc66a0000805c04080400
}
