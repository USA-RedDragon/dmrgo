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
	x := rftx.Baseband(fm, env, 800)
	rx, err := rf.New(rf.Config{Tier: rf.Tier0, MaxWrite: 2 * len(x)})
	if err != nil {
		fmt.Println(err)
		return
	}
	bursts, err := rx.WriteBaseband(x)
	if err != nil {
		fmt.Println(err)
		return
	}
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

// ExampleMonitor follows a group call on TDMA channel 2 of a repeater.
func ExampleMonitor() {
	built, err := rftx.Sequence("voice", rftx.SeqConfig{ColorCode: 7, Source: 3191868, Dest: 9, BS: 314926, Group: true, Wakeups: 1,
		Superframes: 2, BSSourced: true, Slot: 1})
	if err != nil {
		fmt.Println(err)
		return
	}
	fm, env := rftx.ModulateBS(built.Slots)
	x := rftx.Baseband(fm, env, 800)
	var m rf.Monitor
	lb := layer2.NewBurst()
	voice := 0
	rx, err := rf.New(rf.Config{Tier: rf.Tier0, OnBurst: func(b *rf.Burst) {
		r, err := m.Burst(lb, b)
		if err != nil || r.Hint.Slot != enums.Slot2 {
			return
		}
		if lb.Kind == layer2.KindVoice {
			voice++
		}
		if r.End {
			dst, src, _ := r.LC.Addresses()
			fmt.Printf("call from %d to %d ended after %d voice bursts\n", src, dst, voice)
		}
	}})
	if err != nil {
		fmt.Println(err)
		return
	}
	if _, err := rx.WriteBaseband(x); err != nil {
		fmt.Println(err)
		return
	}
	rx.Flush()
	// Output:
	// call from 3191868 to 9 ended after 12 voice bursts
}
