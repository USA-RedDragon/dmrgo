package packet_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ExampleNewPacket splits a payload into a header and data blocks and
// reassembles them.
func ExampleNewPacket() {
	header := pdu.DataHeader{
		Format: pdu.FormatUnconfirmed,
		UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
			SAP:             enums.SAPShortData,
			LLIDDestination: 91,
			LLIDSource:      3120000,
			FullMessageFlag: true,
		},
	}
	p, err := packet.NewPacket(header, []byte("hello, DMR"), enums.DataTypeRate12)
	if err != nil {
		fmt.Println(err)
		return
	}
	h := p.Header.UnconfirmedDataHeader
	fmt.Println(len(p.Blocks), h.BlocksToFollow, h.PadOctetCount)

	a := packet.NewAssembler(127)
	if _, _, err := a.Add(&p.Header); err != nil {
		fmt.Println(err)
		return
	}
	for _, b := range p.Blocks {
		msg, done, err := a.Add(b)
		if err != nil {
			fmt.Println(err)
			return
		}
		if done {
			fmt.Printf("%q\n", msg.Data)
		}
	}
	// Output:
	// 2 2 10
	// "hello, DMR"
}
