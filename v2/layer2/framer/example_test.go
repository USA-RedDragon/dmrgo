package framer_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func Example() {
	lc := pdu.LCBytes{0x00, 0x00, 0x00, 0x00, 0x00, 0x5b, 0x00, 0x30, 0x39}
	cw, _ := lc.Codeword(enums.DataTypeTerminatorWithLC)
	raw, _ := layer2.BuildDataBurst(cw, enums.DataTypeTerminatorWithLC, 1, layer2.SyncSourceBS)

	stream := make([]byte, 50)
	for i := range len(raw) * 8 {
		n := 100 + i
		stream[n/8] |= raw[i/8] >> (7 - i%8) & 1 << (7 - n%8)
	}
	f, err := framer.New(&framer.Config{Period: framer.SlotPeriod})
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, b := range f.Write(stream, 8*len(stream)) {
		burst, _ := layer2.NewBurstFromBytes(b.Data)
		fmt.Println(b.Offset, b.Sync, burst.SlotType.DataType)
	}
	// Output:
	// 100 BS-sourced data Terminator With LC
}
