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
		"800004ce2e1d30b43c0000090200000d9e2080dd7b5f482e7d3e0000000001c0000a800a006000000000000930b43ce9f8ed0071466f",
		"800004ce2e0130b43c000009020000465e00805d2d490a223107000000000a1440541cbd8ee6d793cd54372cbe82e8c4004a2a78",
	} {
		p, _ := hex.DecodeString(s)
		u, err := ipsc.DecodeUser(p)
		if err != nil {
			fmt.Println(err)
			return
		}
		h := u.Hint()
		fmt.Println(u.Opcode, u.Source, u.Destination, u.Info.Slot(), u.IsVoice(), h.Kind)
	}
	// Output:
	// GroupVoice 3191868 9 TS2 false Data
	// GroupVoice 3191868 9 TS1 true Voice
}

// ExampleDecodeControl decodes the registration request of an XPR 8400 and
// builds the keepalive request of peer 3149999.
func ExampleDecodeControl() {
	p, _ := hex.DecodeString("900004ce2e6a0000805c04080400")
	c, err := ipsc.DecodeControl(p)
	fmt.Println(c.Opcode, c.ID, c.Mode.Digital(), c.Mode.Operational(), err)

	alive, _ := (&ipsc.Control{Opcode: ipsc.OpAliveRequest, ID: 3149999, Mode: c.Mode, Flags: c.Flags, Version: c.Version}).AppendBinary(nil)
	fmt.Printf("%x\n", alive)
	// Output:
	// RegisterRequest 314926 true true <nil>
	// 96003010af6a0000805c04080400
}
