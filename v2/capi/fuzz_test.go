package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

const fuzzAlias = "dmrgo test alias"

func goOutputs(t *testing.T, frames int) ([]string, [][33]byte) {
	t.Helper()
	tx, err := voice.NewTransmitter(&voice.TransmitterConfig{Slot: enums.Slot1, ColorCode: 1, Source: layer2.SyncSourceMS, Queue: 192})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Start(0, voice.Call{Group: true, Target: 9990, Source: 3191868}, &voice.CallOptions{TalkerAlias: fuzzAlias}); err != nil {
		t.Fatal(err)
	}
	for i := range frames {
		f := vocoder.AMBESilence()
		f[0] = byte(i)
		if err := tx.Frame(f); err != nil {
			t.Fatal(err)
		}
	}
	tx.Stop(0)
	var keys []string
	var bursts [][33]byte
	out := make([]voice.Output, 0, voice.MaxOutputs)
	for now := time.Duration(0); now < 30*time.Second; now += 10 * time.Millisecond {
		for _, o := range tx.Tick(now, out[:0]) {
			keys = append(keys, fmt.Sprintf("%d %x %d %d %d %t %d", o.Kind, o.Burst[:], int64(o.Due), o.Hint.VoiceBurst, o.Hint.DataType, o.Fill, o.Reason))
			if o.Kind == voice.OutputBurst {
				bursts = append(bursts, o.Burst)
			}
		}
	}
	return keys, bursts
}

func goEvents(t *testing.T, bursts [][33]byte) []string {
	t.Helper()
	rx, err := voice.NewReceiver(nil)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	ev := make([]voice.Event, 0, voice.MaxEvents)
	for i, b := range bursts {
		raw := layer2.RawBurst(b)
		for _, e := range rx.Burst(time.Duration(i+1)*60*time.Millisecond, &raw, layer2.Hint{}, ev[:0]) {
			keys = append(keys, fmt.Sprintf("%d %d %t %d %d %d %q %d %d %d", e.Kind, e.Position, e.Call.Group, e.Call.Target,
				e.Call.Source, e.Call.Origin, string(e.Text), e.Reason, e.Lost, e.Stats.Bursts))
		}
	}
	return keys
}

func TestTransmitterAsGo(t *testing.T) {
	t.Parallel()

	want, _ := goOutputs(t, 180)
	for seed := range uint64(8) {
		got, shorts, err := transmitterFuzzC(fuzzAlias, 180, seed)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("seed %d: C outputs differ from Go (%d and %d outputs)", seed, len(got), len(want))
		}
		if shorts == 0 {
			t.Errorf("seed %d: no call was short", seed)
		}
	}
}

func TestReceiverAsGo(t *testing.T) {
	t.Parallel()

	_, bursts := goOutputs(t, 180)
	parrot, err := os.ReadFile("../layer2/testdata/parrot_kerchunk.bin")
	if err != nil {
		t.Fatal(err)
	}
	for off := 0; off+33 <= len(parrot); off += 33 {
		bursts = append(bursts, [33]byte(parrot[off:off+33]))
	}
	want := goEvents(t, bursts)
	for seed := range uint64(8) {
		got, shorts, err := receiverFuzzC(bursts, seed)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			for i := range min(len(got), len(want)) {
				if got[i] != want[i] {
					t.Fatalf("seed %d: event %d is %s, want %s", seed, i, got[i], want[i])
				}
			}
			t.Fatalf("seed %d: %d events, want %d", seed, len(got), len(want))
		}
		if shorts == 0 {
			t.Errorf("seed %d: no call was short", seed)
		}
	}
	text := 0
	for _, k := range want {
		if strings.Contains(k, fmt.Sprintf("%q", fuzzAlias)) {
			text++
		}
	}
	if text == 0 {
		t.Error("the Go receiver returned no talker alias, so the test does not cover Text")
	}
}
