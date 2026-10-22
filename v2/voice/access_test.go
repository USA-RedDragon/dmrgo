package voice_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func shape(out []voice.Output) string {
	var pos []byte
	for _, o := range out {
		switch {
		case o.Kind == voice.OutputEnded:
			pos = append(pos, '.')
		case o.Hint.VoiceBurst != enums.VoiceBurstUnknown:
			pos = append(pos, o.Hint.VoiceBurst.String()[0])
		case o.Hint.DataType == enums.DataTypeVoiceLCHeader:
			pos = append(pos, 'h')
		case o.Hint.DataType == enums.DataTypeTerminatorWithLC:
			pos = append(pos, 't')
		}
	}
	return string(pos)
}

// ETSI TS 102 361-1 clauses 5.2.1.6 and 5.2.2
func TestTransmitter_ChannelAccess(t *testing.T) {
	t.Parallel()

	call := voice.Call{Group: true, Target: 9, Source: 3120001}
	idle := 540 * time.Millisecond
	tests := []struct {
		name      string
		access    enums.Access
		search    time.Duration
		busy      bool
		inCall    bool
		freeAt    time.Duration
		state     voice.TransmitterState
		deadline  time.Duration
		firstDue  time.Duration
		end       voice.EndReason
		endDue    time.Duration
		noOutputs bool
	}{
		{name: "idle polite", state: voice.TransmitterHeader, deadline: time.Second, firstDue: time.Second, end: voice.EndTerminator},
		{name: "busy impolite", access: enums.AccessImpolite, busy: true, state: voice.TransmitterHeader, deadline: time.Second, firstDue: time.Second, end: voice.EndTerminator},
		{name: "busy in call", busy: true, inCall: true, state: voice.TransmitterHeader, deadline: time.Second, firstDue: time.Second, end: voice.EndTerminator},
		{name: "busy polite to all frees", access: enums.AccessPoliteAll, busy: true, freeAt: 1300 * time.Millisecond, state: voice.TransmitterAccess, deadline: time.Second + idle, firstDue: 1300 * time.Millisecond, end: voice.EndTerminator},
		{name: "busy polite to own CC frees early", busy: true, freeAt: 900 * time.Millisecond, state: voice.TransmitterAccess, deadline: time.Second + idle, firstDue: time.Second, end: voice.EndTerminator},
		{name: "busy polite denied", busy: true, state: voice.TransmitterAccess, deadline: time.Second + idle, end: voice.EndDenied, endDue: time.Second + idle},
		{name: "busy polite no search", busy: true, search: -1, state: voice.TransmitterAccess, deadline: time.Second, end: voice.EndDenied, endDue: time.Second},
		{name: "busy polite short search", busy: true, search: 120 * time.Millisecond, state: voice.TransmitterAccess, deadline: 1120 * time.Millisecond, end: voice.EndDenied, endDue: 1120 * time.Millisecond},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot(), Access: tc.access, IdleSearch: tc.search})
			tx.Channel(0, tc.busy)
			if err := tx.Start(time.Second, call, &voice.CallOptions{InCall: tc.inCall}); err != nil {
				t.Fatal(err)
			}
			if tx.State() != tc.state {
				t.Fatalf("state %v, want %v", tx.State(), tc.state)
			}
			if at, ok := tx.Deadline(); !ok || at != tc.deadline {
				t.Fatalf("deadline %v %v, want %v", at, ok, tc.deadline)
			}
			if err := tx.Frames(vocoder.AMBESilence(), vocoder.AMBESilence(), vocoder.AMBESilence()); err != nil {
				t.Fatal(err)
			}
			if tc.freeAt > 0 {
				tx.Channel(tc.freeAt, false)
				if tx.State() != voice.TransmitterHeader {
					t.Fatalf("state after idle %v", tx.State())
				}
			}
			if tc.end == voice.EndTerminator {
				tx.Stop()
				tx.Channel(0, true)
			}
			out := drain(t, tx, 20)
			last := out[len(out)-1]
			if last.Kind != voice.OutputEnded || last.Reason != tc.end || tx.State() != voice.TransmitterIdle {
				t.Fatalf("outputs %s, end %v, state %v", shape(out), last.Reason, tx.State())
			}
			if tc.end == voice.EndDenied {
				if len(out) != 1 || last.Due != tc.endDue || tx.Room() != voice.DefaultQueue {
					t.Fatalf("denied: %s due %v room %d", shape(out), last.Due, tx.Room())
				}
				return
			}
			if shape(out) != "hABCDEFt." || out[0].Due != tc.firstDue {
				t.Fatalf("outputs %s, first due %v", shape(out), out[0].Due)
			}
		})
	}
}

func TestTransmitter_AccessStop(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot()})
	if _, err := voice.NewTransmitter(&voice.TransmitterConfig{Access: enums.AccessImpolite + 1}); err == nil {
		t.Fatal("NewTransmitter took an unknown Access")
	}
	tx.Channel(0, true)
	if err := tx.Start(0, voice.Call{Target: 2, Source: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Start(0, voice.Call{Target: 2, Source: 1}, nil); !errors.Is(err, voice.ErrBusy) {
		t.Fatalf("Start while waiting: %v", err)
	}
	if tx.ReverseChannel(pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionCommand}) {
		t.Fatal("Interrupt took a call that is not on air")
	}
	tx.Stop()
	if _, ok := tx.Deadline(); ok || tx.State() != voice.TransmitterIdle || tx.Room() != voice.DefaultQueue {
		t.Fatalf("after Stop: state %v", tx.State())
	}
	tx.Reset()
	if err := tx.Start(0, voice.Call{Target: 2, Source: 1}, nil); err != nil || tx.State() != voice.TransmitterHeader {
		t.Fatalf("after Reset: %v, state %v", err, tx.State())
	}
}

// ETSI TS 102 361-2 clauses 6.3.3 and 6.3.4
func TestTransmitter_ReverseChannel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		interruptible bool
		rc            pdu.ReverseChannel
		after         int
		want          bool
		shape         string
	}{
		{name: "command", rc: pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionCommand}, after: 3, want: true, shape: "hABCDEFt."},
		{name: "command interruptible", interruptible: true, rc: pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionCommand}, after: 3, want: true, shape: "hABCDEFt."},
		{name: "request interruptible", interruptible: true, rc: pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionRequest}, after: 3, want: true, shape: "hABCDEFt."},
		{name: "request", rc: pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionRequest}, after: 3, shape: "hABCDEFABCDEFt."},
		{name: "during headers", rc: pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionCommand}, after: 0, want: true, shape: "ht."},
		{name: "reserved", interruptible: true, rc: pdu.ReverseChannel{RCCommand: 0}, after: 3, shape: "hABCDEFABCDEFt."},
		{name: "bad CRC", rc: pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionCommand, FEC: fec.Result{Uncorrectable: true}}, after: 3, shape: "hABCDEFABCDEFt."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot(), Interruptible: tc.interruptible, Queue: 3 * 12})
			if err := tx.Start(0, voice.Call{Target: 2, Source: 1}, nil); err != nil {
				t.Fatal(err)
			}
			for range 3 * 10 {
				if err := tx.Frame(vocoder.AMBESilence()); err != nil {
					t.Fatal(err)
				}
			}
			out := drain(t, tx, 1+tc.after)
			if got := tx.ReverseChannel(tc.rc); got != tc.want {
				t.Fatalf("Interrupt = %v", got)
			}
			if !tc.want {
				tx.Stop()
			}
			out = append(out, drain(t, tx, 30)...)
			want := voice.EndTerminator
			if tc.want {
				want = voice.EndInterrupted
				if tx.ReverseChannel(tc.rc) {
					t.Fatal("Interrupt after the end")
				}
			}
			if shape(out) != tc.shape || out[len(out)-1].Reason != want {
				t.Fatalf("outputs %s, end %v", shape(out), out[len(out)-1].Reason)
			}
		})
	}
}

func TestTransmitter_InterruptEnding(t *testing.T) {
	t.Parallel()

	tx := newTransmitter(t, &voice.TransmitterConfig{Silence: vocoder.AMBESilenceSlot(), Terminators: 3})
	if err := tx.Start(0, voice.Call{Target: 2, Source: 1}, nil); err != nil {
		t.Fatal(err)
	}
	if err := tx.Frames(vocoder.AMBESilence(), vocoder.AMBESilence(), vocoder.AMBESilence()); err != nil {
		t.Fatal(err)
	}
	tx.Stop()
	out := drain(t, tx, 3)
	if tx.State() != voice.TransmitterEnding || !tx.ReverseChannel(pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionCommand}) {
		t.Fatalf("state %v", tx.State())
	}
	out = append(out, drain(t, tx, 10)...)
	if shape(out) != "hABCDEFttt." || out[len(out)-1].Reason != voice.EndInterrupted {
		t.Fatalf("outputs %s, end %v", shape(out), out[len(out)-1].Reason)
	}
	if tx.ReverseChannel(pdu.ReverseChannel{RCCommand: enums.RCCeaseTransmissionCommand}) {
		t.Fatal("Interrupt of an idle Transmitter")
	}
}
