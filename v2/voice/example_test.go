package voice_test

import (
	"fmt"
	"os"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// ExampleReceiver decodes a recorded voice call and reports its start,
// its end and the end of the session.
func ExampleReceiver() {
	data, err := os.ReadFile("../layer2/testdata/parrot_kerchunk.bin")
	if err != nil {
		fmt.Println(err)
		return
	}
	rx, err := voice.NewReceiver(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	events := make([]voice.Event, 0, voice.MaxEvents)
	var now time.Duration
	voiceBursts := 0
	for off := 0; off+33 <= len(data); off += 33 {
		raw := layer2.RawBurst(data[off : off+33])
		now += 60 * time.Millisecond
		events = rx.Burst(now, &raw, layer2.Hint{}, events[:0])
		for _, e := range events {
			switch e.Kind {
			case voice.EventCallStart:
				fmt.Printf("call from %d to %d (%v)\n", e.Call.Source, e.Call.Target, e.Call.Origin)
			case voice.EventVoice:
				voiceBursts++
			case voice.EventCallEnd:
				fmt.Printf("end: %v after %v, %d voice bursts\n", e.Reason, e.Stats.Duration, voiceBursts)
			default:
			}
		}
	}
	if at, ok := rx.Deadline(); ok {
		for _, e := range rx.Tick(at, events[:0]) {
			fmt.Println(e.Kind, "at", at)
		}
	}
	// Output:
	// call from 3191868 to 9990 (Header)
	// end: Terminator after 840ms, 13 voice bursts
	// SessionEnd at 3.9s
}

// ExampleJitter takes the packets of a network stream out of order and
// plays them in the order of their sequence numbers, a fixed delay after
// the first one arrived.
func ExampleJitter() {
	j, err := voice.NewJitter(nil)
	if err != nil {
		fmt.Println(err)
		return
	}
	arrivals := []struct {
		at  time.Duration
		seq uint16
	}{
		{0, 0},
		{120 * time.Millisecond, 2},
		{150 * time.Millisecond, 1},
		{180 * time.Millisecond, 3},
		{190 * time.Millisecond, 3},
	}
	for _, a := range arrivals {
		var raw layer2.RawBurst
		h := layer2.Hint{Slot: enums.Slot1, Kind: layer2.KindVoice, Seq: a.seq, HasSeq: true, SeqBits: 8}
		fmt.Println("push", a.seq, "at", a.at, j.Push(a.at, &raw, h, 3120001))
	}
	var p voice.JitterPacket
	for {
		at, ok := j.Deadline()
		if !ok {
			break
		}
		if j.Pop(at, &p) {
			fmt.Println("pop", p.Hint.Seq, "due", p.Due)
		}
	}
	fmt.Printf("%+v\n", j.Stats())
	// Output:
	// push 0 at 0s Buffered
	// push 2 at 120ms Buffered
	// push 1 at 150ms Buffered
	// push 3 at 180ms Buffered
	// push 3 at 190ms Duplicate
	// pop 0 due 360ms
	// pop 1 due 420ms
	// pop 2 due 480ms
	// pop 3 due 540ms
	// {Late:0 Duplicates:1 Overflows:0 Resyncs:0}
}

// ExampleTransmitter sends a short call as its source: one Voice LC
// Header, the voice bursts of the queued vocoder slots with silence to the
// end of the superframe, and a Terminator with LC.
func ExampleTransmitter() {
	tx, err := voice.NewTransmitter(&voice.TransmitterConfig{Slot: enums.Slot1, ColorCode: 1, Source: layer2.SyncSourceMS})
	if err != nil {
		fmt.Println(err)
		return
	}
	if err := tx.Start(0, voice.Call{Group: true, Target: 9, Source: 3120001}, nil); err != nil {
		fmt.Println(err)
		return
	}
	silence := vocoder.AMBESilenceSlot()
	if err := tx.VocoderSlots(silence, silence, silence, silence, silence, silence); err != nil {
		fmt.Println(err)
		return
	}
	tx.Stop(0)
	out := make([]voice.Output, 0, voice.MaxOutputs)
	for {
		at, ok := tx.Deadline()
		if !ok {
			break
		}
		for _, o := range tx.Tick(at, out[:0]) {
			switch {
			case o.Kind == voice.OutputEnded:
				fmt.Println(o.Due, o.Kind, o.Reason)
			case o.Hint.Kind == layer2.KindVoice:
				fmt.Println(o.Due, "voice", o.Hint.VoiceBurst, "fill", o.Fill)
			default:
				fmt.Println(o.Due, o.Hint.DataType)
			}
		}
	}
	// Output:
	// 0s Voice LC Header
	// 60ms voice A fill false
	// 120ms voice B fill false
	// 180ms voice C fill true
	// 240ms voice D fill true
	// 300ms voice E fill true
	// 360ms voice F fill true
	// 420ms Terminator With LC
	// 420ms Ended Terminator
}
