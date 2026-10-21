package voice_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func newInterrupter(t *testing.T, cfg *voice.InterrupterConfig) *voice.Interrupter {
	t.Helper()
	i, err := voice.NewInterrupter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

// ETSI TS 102 361-2 clause 6.3.5
func TestInterrupter_StopsTransmitter(t *testing.T) {
	t.Parallel()

	for _, cmd := range []enums.RCCommand{enums.RCCeaseTransmissionCommand, enums.RCCeaseTransmissionRequest} {
		tx := newTransmitter(t, &voice.TransmitterConfig{ColorCode: 4, Source: layer2.SyncSourceDirectTS1, Silence: vocoder.AMBESilenceSlot(), Interruptible: true, Queue: 3 * 30})
		in := newInterrupter(t, &voice.InterrupterConfig{ColorCode: 4})
		if err := tx.Start(0, voice.Call{Group: true, Target: 9, Source: 1}, nil); err != nil {
			t.Fatal(err)
		}
		for range 3 * 20 {
			if err := tx.Frame(vocoder.AMBESilence()); err != nil {
				t.Fatal(err)
			}
		}
		buf := make([]voice.Output, 0, voice.MaxOutputs)
		var got []voice.Output
		var b layer2.Burst
		for now := time.Duration(0); now < 3*time.Second && (now <= 210*time.Millisecond || in.State() != voice.InterrupterIdle || tx.State() != voice.TransmitterIdle); now += 30 * time.Millisecond {
			if now == 210*time.Millisecond {
				if err := in.Interrupt(now, cmd); err != nil {
					t.Fatal(err)
				}
				if err := in.Interrupt(now, cmd); !errors.Is(err, voice.ErrBusy) {
					t.Fatalf("second Interrupt: %v", err)
				}
			}
			for _, o := range in.Tick(now, buf[:0]) {
				if o.Kind != voice.OutputBurst {
					t.Fatalf("Interrupter output %v", o.Kind)
				}
				if err := b.DecodeFromBytes(o.Burst); err != nil || b.Kind != layer2.KindReverseChannel || b.EmbeddedSignalling.ColorCode != 4 {
					t.Fatalf("RC burst: %v, kind %v", err, b.Kind)
				}
				if !tx.Interrupt(b.ReverseChannel) {
					t.Fatal("Transmitter ignored the RC command")
				}
			}
			for _, o := range tx.Tick(now, buf[:0]) {
				if o.Kind == voice.OutputBurst {
					got = append(got, in.Burst(now, &o.Burst, layer2.Hint{}, nil)...)
				}
			}
		}
		if len(got) != 1 || got[0].Kind != voice.OutputInterrupted || in.State() != voice.InterrupterIdle || tx.State() != voice.TransmitterIdle {
			t.Fatalf("%v: outputs %+v, states %v %v", cmd, got, in.State(), tx.State())
		}
		if got[0].Due > 210*time.Millisecond+360*time.Millisecond+60*time.Millisecond {
			t.Errorf("%v: terminator at %v", cmd, got[0].Due)
		}
	}
}

func TestInterrupter_Retries(t *testing.T) {
	t.Parallel()

	in := newInterrupter(t, &voice.InterrupterConfig{Retries: 2, Repeats: 2})
	if err := in.Interrupt(time.Second, enums.RCCeaseTransmissionRequest); err != nil {
		t.Fatal(err)
	}
	var due []time.Duration
	var end voice.Output
	buf := make([]voice.Output, 0, voice.MaxOutputs)
	for range 20 {
		at, ok := in.Deadline()
		if !ok {
			break
		}
		for _, o := range in.Tick(at, buf[:0]) {
			if o.Kind == voice.OutputBurst {
				due = append(due, o.Due)
			} else {
				end = o
			}
		}
	}
	ms := time.Millisecond
	want := []time.Duration{1000 * ms, 1060 * ms, 1660 * ms, 1720 * ms, 2320 * ms, 2380 * ms}
	if len(due) != len(want) || end.Kind != voice.OutputInterruptFailed || end.Due != 2980*ms {
		t.Fatalf("bursts %v, end %+v", due, end)
	}
	for k := range want {
		if due[k] != want[k] {
			t.Fatalf("bursts %v, want %v", due, want)
		}
	}
}

// ETSI TS 102 361-2 clause 6.3.5
func TestInterrupter_DefaultRetries(t *testing.T) {
	t.Parallel()

	ms := time.Millisecond
	for _, c := range []struct {
		cfg  *voice.InterrupterConfig
		want []time.Duration
		end  time.Duration
	}{
		{nil, []time.Duration{0, 600 * ms}, 1200 * ms},
		{&voice.InterrupterConfig{Retries: -1}, []time.Duration{0}, 600 * ms},
		{&voice.InterrupterConfig{Retries: 2}, []time.Duration{0, 600 * ms, 1200 * ms}, 1800 * ms},
	} {
		in := newInterrupter(t, c.cfg)
		if err := in.Interrupt(0, enums.RCCeaseTransmissionRequest); err != nil {
			t.Fatal(err)
		}
		var due []time.Duration
		var end voice.Output
		for range 20 {
			at, ok := in.Deadline()
			if !ok {
				break
			}
			for _, o := range in.Tick(at, nil) {
				if o.Kind == voice.OutputBurst {
					due = append(due, o.Due)
				} else {
					end = o
				}
			}
		}
		if len(due) != len(c.want) || end.Kind != voice.OutputInterruptFailed || end.Due != c.end {
			t.Fatalf("%+v: bursts %v, end %+v", c.cfg, due, end)
		}
		for k := range c.want {
			if due[k] != c.want[k] {
				t.Fatalf("%+v: bursts %v, want %v", c.cfg, due, c.want)
			}
		}
	}
}

func TestInterrupter_States(t *testing.T) {
	t.Parallel()

	for _, cfg := range []voice.InterrupterConfig{{ColorCode: 16}, {Retries: voice.MaxRetries + 1}, {Repeats: voice.MaxRepeats + 1}, {Repeats: -1}} {
		if _, err := voice.NewInterrupter(&cfg); err == nil {
			t.Errorf("%+v: no error", cfg)
		}
	}
	in := newInterrupter(t, nil)
	if err := in.Interrupt(0, 0); err == nil {
		t.Fatal("Interrupt took a reserved command")
	}
	if _, ok := in.Deadline(); ok || in.State() != voice.InterrupterIdle {
		t.Fatal("idle Interrupter has a deadline")
	}
	term := transmissionEnd(t)
	if out := in.Burst(0, &term, layer2.Hint{}, nil); len(out) != 0 {
		t.Fatalf("idle Interrupter took a terminator: %+v", out)
	}
	if err := in.Interrupt(0, enums.RCCeaseTransmissionCommand); err != nil || in.State() != voice.InterrupterSending {
		t.Fatal(err)
	}
	if out := in.Decoded(0, nil, layer2.Hint{}, nil); len(out) != 1 || out[0].Kind != voice.OutputBurst || in.State() != voice.InterrupterWaiting {
		t.Fatalf("outputs %+v, state %v", out, in.State())
	}
	if at, ok := in.Deadline(); !ok || at != 600*time.Millisecond {
		t.Fatalf("deadline %v", at)
	}
	in.Cancel()
	if in.State() != voice.InterrupterIdle {
		t.Fatal("Cancel")
	}
	wait := newInterrupter(t, &voice.InterrupterConfig{Wait: -1})
	_ = wait.Interrupt(0, enums.RCCeaseTransmissionCommand)
	_ = wait.Tick(0, nil)
	if _, ok := wait.Deadline(); ok || wait.State() != voice.InterrupterWaiting {
		t.Fatal("a negative Wait has a deadline")
	}
	wait.Reset()
	if wait.State() != voice.InterrupterIdle {
		t.Fatal("Reset")
	}
	names := map[voice.InterrupterState]string{voice.InterrupterIdle: "Idle", voice.InterrupterSending: "Sending", voice.InterrupterWaiting: "Waiting", 9: "InterrupterState(9)"}
	for s, want := range names {
		if s.String() != want {
			t.Errorf("%d: %q", s, s.String())
		}
	}
	for k, want := range map[voice.OutputKind]string{voice.OutputInterrupted: "Interrupted", voice.OutputInterruptFailed: "InterruptFailed"} {
		if k.String() != want {
			t.Errorf("%d: %q", k, k.String())
		}
	}
	for r, want := range map[voice.EndReason]string{voice.EndDenied: "Denied", voice.EndInterrupted: "Interrupted"} {
		if r.String() != want {
			t.Errorf("%d: %q", r, r.String())
		}
	}
	if voice.TransmitterAccess.String() != "Access" {
		t.Error(voice.TransmitterAccess.String())
	}
}

func transmissionEnd(t *testing.T) layer2.RawBurst {
	t.Helper()
	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot()})
	if err := tx.Start(0, voice.Call{Target: 2, Source: 1}, nil); err != nil {
		t.Fatal(err)
	}
	tx.Stop()
	out := drain(t, tx, 10)
	return out[len(out)-2].Burst
}
