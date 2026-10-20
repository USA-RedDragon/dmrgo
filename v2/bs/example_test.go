package bs_test

import (
	"fmt"
	"os"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// ExampleRepeater repeats a real SMS that arrives on slot 2 of a hotspot,
// with a Tick every 30 ms, and counts what goes out on each slot.
func ExampleRepeater() {
	data, err := os.ReadFile("../layer2/testdata/d-sms.bin")
	if err != nil {
		fmt.Println(err)
		return
	}
	r, err := bs.New(&bs.Config{ColorCode: 1, CACH: true})
	if err != nil {
		fmt.Println(err)
		return
	}
	out := make([]bs.Output, 0, bs.MaxOutputs)
	sent := map[enums.Slot]map[enums.DataType]int{enums.Slot1: {}, enums.Slot2: {}}
	handle := func(out []bs.Output) {
		for _, o := range out {
			switch o.Kind {
			case bs.OutputBurst:
				sent[o.Slot][o.Burst.SlotType().DataType]++
			case bs.OutputCallStart:
				fmt.Printf("%v: %v from %d to %d\n", o.Slot, o.Service, o.Call.Source, o.Call.Target)
			default:
				fmt.Println(o.Kind)
			}
		}
	}
	next := 0
	for now := time.Duration(0); now < 6*time.Second; now += bs.SlotTime {
		if next+33 <= len(data) && now >= time.Duration(next/33)*60*time.Millisecond {
			raw := layer2.RawBurst(data[next : next+33])
			next += 33
			out, err = r.Inbound(now, &raw, layer2.Hint{Slot: enums.Slot2}, bs.Source{RF: true}, out[:0])
			if err != nil {
				fmt.Println(err)
				return
			}
			handle(out)
		}
		handle(r.Tick(now, out[:0]))
	}
	fmt.Println("TS1:", sent[enums.Slot1])
	fmt.Println("TS2:", sent[enums.Slot2])
	// Output:
	// Key
	// TS2: Data from 3191868 to 9990
	// CallEnd
	// Dekey
	// TS1: map[Idle:93]
	// TS2: map[CSBK:5 Data Header:1 Rate 1/2:5 Idle:83]
}
