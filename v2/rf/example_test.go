package rf_test

import (
	"encoding/binary"
	"fmt"
	"math"

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
	iq := cf32(rftx.Baseband(fm, env, 800))
	rx, err := rf.NewReceiver(&rf.RxConfig{Tier: rf.Tier0, Rate: 48000, Format: rf.IQFormatCF32})
	if err != nil {
		fmt.Println(err)
		return
	}
	bursts := rx.AppendBursts(nil, iq)
	bursts = rx.AppendFlush(bursts)
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
	iq := cf32(rftx.Baseband(fm, env, 800))
	var m rf.Monitor
	lb := layer2.NewBurst()
	voice := 0
	onBursts := func(bs []rf.Burst) {
		for i := range bs {
			r, err := m.Burst(lb, &bs[i])
			if err != nil || r.Hint.Slot != enums.Slot2 {
				continue
			}
			if lb.Kind == layer2.KindVoice {
				voice++
			}
			if r.End {
				dst, src, _ := r.LC.Addresses()
				fmt.Printf("call from %d to %d ended after %d voice bursts\n", src, dst, voice)
			}
		}
	}
	rx, err := rf.NewReceiver(&rf.RxConfig{Tier: rf.Tier0, Rate: 48000, Format: rf.IQFormatCF32, OnBursts: onBursts})
	if err != nil {
		fmt.Println(err)
		return
	}
	if _, err := rx.Write(iq); err != nil {
		fmt.Println(err)
		return
	}
	if err := rx.Flush(); err != nil {
		fmt.Println(err)
	}
	// Output:
	// call from 3191868 to 9 ended after 12 voice bursts
}

func cf32(x []complex64) []byte {
	b := make([]byte, 8*len(x))
	for i, v := range x {
		binary.LittleEndian.PutUint32(b[8*i:], math.Float32bits(real(v)))
		binary.LittleEndian.PutUint32(b[8*i+4:], math.Float32bits(imag(v)))
	}
	return b
}
