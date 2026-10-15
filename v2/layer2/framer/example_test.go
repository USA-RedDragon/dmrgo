package framer_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Example finds a Terminator with LC burst that follows 100 bits of other
// data in a bit stream, and decodes it.
func Example() {
	lc := pdu.LCBytes{0x00, 0x00, 0x00, 0x00, 0x00, 0x5b, 0x00, 0x30, 0x39}
	cw, _ := lc.Codeword(enums.DataTypeTerminatorWithLC)
	raw, _ := layer2.BuildDataBurst(cw, enums.DataTypeTerminatorWithLC, 1, layer2.SyncSourceBS)

	stream := append(make([]bit.Bit, 100), bit.Unpack(raw[:])...)
	f := framer.New(framer.Config{Threshold: -1, Period: framer.SlotPeriod})
	for _, b := range f.Write(stream...) {
		burst, _ := layer2.NewBurstFromBytes(b.Data)
		fmt.Println(b.Offset, b.Sync, burst.SlotType.DataType)
	}
	// Output:
	// 100 BS-sourced data Terminator With LC
}
