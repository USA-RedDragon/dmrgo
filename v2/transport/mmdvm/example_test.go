package mmdvm_test

import (
	"encoding/hex"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

// ExampleNext finds the frames in octets that MMDVMHost sent to a modem: a
// get status request and the voice LC header of a call on slot 2.
func ExampleNext() {
	p, _ := hex.DecodeString("e00301" + "e0251a0020580b4015ec0f6871000d005c4d5d7f77fd757361a416203d5235242181850277")
	for len(p) > 0 {
		f, n, st := mmdvm.Next(p)
		if st != mmdvm.StatusFrame {
			break
		}
		p = p[n:]
		fmt.Println(f.Command, len(f.Payload))
		if d, err := mmdvm.DecodeDMRData(f); err == nil {
			sync, _ := d.Burst.Sync()
			fmt.Println(d.Slot, sync, d.Burst.SlotType().DataType)
		}
	}
	// Output:
	// GetStatus 0
	// DMRData2 34
	// TS2 MS-sourced data Voice LC Header
}

// ExampleAppendFrame builds the get version request that a host sends
// first.
func ExampleAppendFrame() {
	out, err := mmdvm.AppendFrame(nil, mmdvm.CommandGetVersion, nil)
	fmt.Printf("%x %v\n", out, err)
	// Output:
	// e00300 <nil>
}
