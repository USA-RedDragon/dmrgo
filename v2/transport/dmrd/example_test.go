package dmrd_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

// ExampleDecodeData decodes a DMRD packet that DMRGateway sent to its
// master, and the hint of its burst for the service machines.
func ExampleDecodeData() {
	p, _ := hex.DecodeString("444d52440030b43c00005b003010afa1506290d020580b4015ec0f6871000d005c4d5d7f77fd757361a416203d52352421818502770000")
	fmt.Println(dmrd.Classify(p, dmrd.DialectMMDVM))
	d, err := dmrd.DecodeData(p)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(d.Source, d.Destination, d.Repeater, d.Slot, d.Private, d.Frame, d.Size)
	h := d.Hint()
	fmt.Println(h.Kind, h.DataType)
	// Output:
	// DMRD
	// 3191868 91 3149999 TS2 false Data Sync 55
	// Data Voice LC Header
}

// ExampleControl_AppendBinary builds the keepalive of repeater 3149999 and
// decodes the login that DMRGateway sent.
func ExampleControl_AppendBinary() {
	ping, _ := dmrd.Control{Command: dmrd.CommandRPTPING, ID: 3149999}.AppendBinary(nil, dmrd.DialectMMDVM)
	fmt.Printf("%x\n", ping)

	login, _ := hex.DecodeString("5250544c003010af")
	c, err := dmrd.DecodeControl(login, dmrd.DialectMMDVM)
	fmt.Println(c.Command, c.ID, err)
	// Output:
	// 52505450494e47003010af
	// RPTL 3149999 <nil>
}
