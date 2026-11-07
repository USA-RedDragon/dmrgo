package dmrd_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

// ExampleDecodeData decodes a DMRD packet that DMRGateway sent to its
// master, and the hint of its burst for the service machines.
func ExampleDecodeData() {
	p, _ := hex.DecodeString("444d5244002dedcf00005b002dedc6a1506290d021f40d58182015f8599012205c4d5d7f77fd757362300e202b9229044141c303910000")
	fmt.Println(dmrd.Classify(p, dmrd.DialectMMDVM))
	d, err := dmrd.DecodeData(p)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(d.Source, d.Destination, d.Repeater, d.Slot, d.Group, d.Frame, d.Size)
	h := d.Hint(0)
	fmt.Println(h.Kind, h.DataType)
	// Output:
	// DMRD
	// 3009999 91 3009990 TS2 true Data Sync 55
	// Data Voice LC Header
}

// ExampleControl_AppendBinary builds the keepalive of repeater 3003299 and
// decodes the login that DMRGateway sent.
func ExampleControl_AppendBinary() {
	ping, _ := (&dmrd.Control{Command: dmrd.CommandRPTPING, ID: 3003299}).AppendBinary(nil, dmrd.DialectMMDVM)
	fmt.Printf("%x\n", ping)

	login, _ := hex.DecodeString("5250544c002dedc6")
	c, err := dmrd.DecodeControl(login, dmrd.DialectMMDVM)
	fmt.Println(c.Command, c.ID, err)
	// Output:
	// 52505450494e47002dd3a3
	// RPTL 3009990 <nil>
}
