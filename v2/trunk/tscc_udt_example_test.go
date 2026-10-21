package trunk_test

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

// ExampleTSCC_SendShortData sends UDT short data from a gateway: the TSCC
// sends the UDT header and its appended block in TS1 of its carrier, with
// C_ALOHA in TS2.
func ExampleTSCC_SendShortData() {
	ts, _ := trunk.NewTSCC(&trunk.TSCCConfig{SysCode: pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 1, Site: 2}, Channel: 1,
		Payload: [trunk.MaxPayloadChannels]uint16{2}, PayloadLen: 1})
	var a trunk.Appended
	_ = a.SetText(enums.UDTISO8, []byte("gate 4"))
	_ = ts.SendShortData(0, false, 3120001, &a)
	out := make([]trunk.TSCCOutput, 0, trunk.MaxTSCCOutputs)
	for i := range 4 {
		out = ts.Tick(time.Duration(i)*30*time.Millisecond, out[:0])
		for _, o := range out {
			b, _ := layer2.NewBurstFromBytes(o.Burst)
			fmt.Println(o.Slot, b.SlotType.DataType)
		}
	}
	// Output:
	// TS1 Data Header
	// TS2 CSBK
	// TS1 Rate 1/2
	// TS2 CSBK
}
