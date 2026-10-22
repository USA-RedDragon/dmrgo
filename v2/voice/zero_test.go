package voice_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestZeroMachines(t *testing.T) {
	t.Parallel()

	var tx voice.Transmitter
	if err := tx.Start(0, voice.Call{Group: true, Target: 9, Source: 1}, nil); !errors.Is(err, dmrgo.ErrZeroValue) {
		t.Errorf("Start: %v", err)
	}
	if err := tx.Relay(0, &voice.Event{Kind: voice.EventCallStart}); !errors.Is(err, dmrgo.ErrZeroValue) {
		t.Errorf("Relay: %v", err)
	}
	if out := tx.Tick(0, nil); len(out) != 0 || tx.State() != voice.TransmitterIdle {
		t.Errorf("Tick: %+v", out)
	}
	var in voice.Interrupter
	if err := in.Interrupt(0, enums.RCCeaseTransmissionRequest); !errors.Is(err, dmrgo.ErrZeroValue) {
		t.Errorf("Interrupt: %v", err)
	}
	var raw layer2.RawBurst
	if out := in.Burst(0, &raw, layer2.Hint{}, nil); len(out) != 0 {
		t.Errorf("Interrupter.Burst: %+v", out)
	}
	var rx voice.Receiver
	if ev := rx.Decoded(0, layer2.NewBurst(), layer2.Hint{SyncLost: true}, nil); len(ev) != 0 || rx.State() != voice.StateIdle {
		t.Errorf("Receiver.Decoded: %+v", ev)
	}
	if err := newTransmitter(t, nil).Relay(0, nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("Relay(nil): %v", err)
	}
}

func TestOutputSlot(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Slot: enums.Slot2})
	if err := tx.Start(0, voice.Call{Group: true, Target: 9, Source: 1}, &voice.CallOptions{}); err != nil {
		t.Fatal(err)
	}
	tx.Channel(0, false)
	tx.Stop()
	n := 0
	for now := time.Duration(0); now < 2*time.Second; now += 60 * time.Millisecond {
		for _, o := range tx.Tick(now, nil) {
			if o.Kind == voice.OutputBurst {
				n++
				if o.Hint.Slot != enums.Slot2 {
					t.Fatalf("%v burst on slot %v", o.Hint.Kind, o.Hint.Slot)
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("no bursts")
	}
	in, err := voice.NewInterrupter(&voice.InterrupterConfig{Slot: enums.Slot1})
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Interrupt(0, enums.RCCeaseTransmissionRequest); err != nil {
		t.Fatal(err)
	}
	if out := in.Tick(0, nil); len(out) != 1 || out[0].Hint.Slot != enums.Slot1 {
		t.Errorf("RC burst %+v", out)
	}
	if _, err := voice.NewTransmitter(&voice.TransmitterConfig{Slot: 3}); err == nil {
		t.Error("Slot 3 accepted")
	}
	if _, err := voice.NewInterrupter(&voice.InterrupterConfig{Slot: 3}); err == nil {
		t.Error("Interrupter Slot 3 accepted")
	}
}
