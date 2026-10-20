package rf_test

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

func Example() {
	built, err := rftx.Sequence("voice", rftx.SeqConfig{ColorCode: 7, Source: 3191868, Dest: 9, BS: 314926, Group: true, Wakeups: 1,
		Superframes: 2, BSSourced: true, Slot: 1})
	if err != nil {
		fmt.Println(err)
		return
	}
	fm, env := rftx.ModulateBS(built.Slots)
	rx, err := rf.New(rf.Config{Tier: rf.Tier0})
	if err != nil {
		fmt.Println(err)
		return
	}
	bursts := append([]rf.Burst(nil), rx.WriteBaseband(rftx.Baseband(fm, env, 800))...)
	bursts = append(bursts, rx.Flush()...)
	var voice [enums.VoiceBurstF + 1]int
	for _, b := range bursts {
		data, _ := b.Decoded()
		burst, err := layer2.NewBurstFromBytes(data)
		if err != nil || burst.Kind != layer2.KindVoice {
			continue
		}
		pos := b.Voice
		if pos == enums.VoiceBurstUnknown {
			pos = burst.VoiceBurst
		}
		voice[pos]++
	}
	fmt.Println("A to F:", voice[enums.VoiceBurstA:])
	// Output:
	// A to F: [2 2 2 2 2 2]
}
