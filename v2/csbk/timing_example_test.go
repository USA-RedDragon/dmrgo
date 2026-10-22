package csbk_test

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ExampleTiming joins the channel timing of a leader from a
// CT_CSBK beacon and counts the CT_CSBK_Prop bursts that it then sends.
func ExampleTiming() {
	m, err := csbk.NewTiming(&csbk.TimingConfig{ID: 100, DynamicID: 2, Slot: enums.Slot1, Seed: 1})
	if err != nil {
		fmt.Println(err)
		return
	}
	if err := m.Join(0); err != nil {
		fmt.Println(err)
		return
	}
	beacon := pdu.ChannelTimingPDU{LeaderDynamicIdentifier: 3, LeaderIdentifier: 7, ChannelTimingOp1: true, ChannelTimingOp0: true, SourceDynamicIdentifier: 3, SourceIdentifier: 7}
	c := pdu.CSBK{LastBlock: true}
	if err := c.SetChannelTiming(beacon); err != nil {
		fmt.Println(err)
		return
	}
	raw, _ := layer2.BuildBurst(&c, csbk.AllSiteColorCode, layer2.SyncSourceDirectTS2)
	out := make([]csbk.Output, 0, csbk.MaxOutputs)
	for _, o := range m.Burst(time.Minute, &raw, layer2.Hint{}, out[:0]) {
		fmt.Println(o.Kind, o.Service)
	}
	di, id, _ := m.Leader()
	fmt.Println(m.State(), di, id)
	bursts := 0
	for {
		at, ok := m.Deadline()
		if !ok || at > 2*time.Minute {
			break
		}
		bursts += len(m.Tick(at, out[:0]))
	}
	fmt.Println("CT_CSBK_Prop bursts:", bursts)
	// Output:
	// Timing Timing
	// LeaderAndTimingKnown 3 7
	// CT_CSBK_Prop bursts: 10
}
