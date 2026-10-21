package hytera_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

// ExampleDecodeFrame decodes the DMR frame of a voice LC header on slot 1
// from 3191868 to talkgroup 91, and the hint of its burst for the service
// machines.
func ExampleDecodeFrame() {
	p, _ := hex.DecodeString("c35200502a0300000100050101000000111111111111000040b87900820b4015700f207560056d4457ff5dd7def50031781670382037012f829f0073000000005b0000003cb43000")
	f, err := hytera.DecodeFrame(p)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(f.Slot, f.SlotType, f.ColorCode, f.Source, f.Destination, f.HasBurst())
	h := f.Hint()
	fmt.Println(h.Kind, h.DataType)
	// Output:
	// TS1 Voice LC Header 1 3191868 91 true
	// Data Voice LC Header
}

// ExampleDecodeHeartbeat decodes the heartbeat of a repeater and builds the
// reply of the master.
func ExampleDecodeHeartbeat() {
	p, _ := hex.DecodeString("5a5a5a5a0a000000140000005a5a595a00000000")
	h, err := hytera.DecodeHeartbeat(p)
	fmt.Printf("%#x %#x %v\n", h.Code, h.Seq, err)

	h.Seq = 0x5B
	reply, _ := h.AppendBinary(nil)
	fmt.Printf("%x\n", reply)
	// Output:
	// 0xa 0x5a <nil>
	// 5a5a5a5a0a000000140000005b5a595a00000000
}
