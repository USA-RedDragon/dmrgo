package csbk_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func TestStrings(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ got, want string }{
		{csbk.ServiceCustom.String(), "Custom"},
		{csbk.ServiceOACSU.String(), "OACSU"},
		{csbk.ServiceRadioCheck.String(), "RadioCheck"},
		{csbk.ServiceCallAlert.String(), "CallAlert"},
		{csbk.ServiceUnsupported.String(), "Unsupported"},
		{csbk.ServiceWakeup.String(), "Wakeup"},
		{csbk.Service(99).String(), "Service(99)"},
		{csbk.ResultAnswered.String(), "Answered"},
		{csbk.ResultRefused.String(), "Refused"},
		{csbk.ResultNACK.String(), "NACK"},
		{csbk.ResultNoAnswer.String(), "NoAnswer"},
		{csbk.ResultDenied.String(), "Denied"},
		{csbk.ResultCanceled.String(), "Canceled"},
		{csbk.Result(0).String(), "Result(0)"},
		{csbk.OutputBurst.String(), "Burst"},
		{csbk.OutputDone.String(), "Done"},
		{csbk.OutputRequest.String(), "Request"},
		{csbk.OutputAnnounced.String(), "Announced"},
		{csbk.OutputJoined.String(), "Joined"},
		{csbk.OutputExpired.String(), "Expired"},
		{csbk.OutputKind(0).String(), "OutputKind(0)"},
		{csbk.ReasonNotSupported.String(), "NotSupported"},
		{csbk.Reason(1).String(), "Reason(1)"},
		{csbk.OACSUProceed.String(), "Proceed"},
		{csbk.OACSUDeny.String(), "Deny"},
		{csbk.OACSUNotSupported.String(), "NotSupported"},
		{csbk.OACSUAnswer(9).String(), "OACSUAnswer(9)"},
		{csbk.RequesterIdle.String(), "Idle"},
		{csbk.RequesterAccess.String(), "Access"},
		{csbk.RequesterSending.String(), "Sending"},
		{csbk.RequesterWaiting.String(), "Waiting"},
		{csbk.RequesterState(9).String(), "RequesterState(9)"},
		{csbk.PreamblesIdle.String(), "Idle"},
		{csbk.PreamblesAnnounced.String(), "Announced"},
		{csbk.PreamblesState(9).String(), "PreamblesState(9)"},
		{csbk.WakeupIdle.String(), "Idle"},
		{csbk.WakeupSending.String(), "Sending"},
		{csbk.WakeupWaiting.String(), "Waiting"},
		{csbk.WakeupState(9).String(), "WakeupState(9)"},
		{csbk.ResponderIdle.String(), "Idle"},
		{csbk.ResponderAccess.String(), "Access"},
		{csbk.ResponderSending.String(), "Sending"},
		{csbk.ResponderState(9).String(), "ResponderState(9)"},
		{(&dmrgo.ConfigError{Type: "csbk", Field: "Retries"}).Error(), "csbk: Retries is out of range"},
	} {
		if tt.got != tt.want {
			t.Errorf("%q, want %q", tt.got, tt.want)
		}
	}
}

func TestResetAndLost(t *testing.T) {
	t.Parallel()

	q := mustRequester(t, nil)
	q.Channel(0, true)
	if err := q.RequestOACSU(0, 2, 1, layer3.ServiceOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	if out := q.Lost(10*ms, nil); len(out) != 1 || out[0].Kind != csbk.OutputBurst || out[0].Due != 10*ms {
		t.Errorf("Lost gave %+v", out)
	}
	q.Reset()
	if _, ok := q.Deadline(); ok || q.State() != csbk.RequesterIdle {
		t.Error("Reset left a request")
	}

	r := mustResponder(t, &csbk.ResponderConfig{Slot: enums.Slot1, Address: anytone})
	r.Channel(0, true)
	r.Burst(0, burstOf(t, injectedRadioCheck, layer2.SyncSourceBS), layer2.Hint{}, nil)
	if out := r.Lost(ms, nil); len(out) != 0 {
		t.Errorf("Lost gave %+v before the next slot", out)
	}
	if out := r.Tick(60*ms, nil); len(out) != 1 || out[0].Kind != csbk.OutputBurst || out[0].Due != 60*ms {
		t.Errorf("Tick after Lost gave %+v", out)
	}
	r.Burst(0, burstOf(t, injectedRadioCheck, layer2.SyncSourceBS), layer2.Hint{}, nil)
	r.Reset()
	if r.State() != csbk.ResponderIdle {
		t.Error("Reset left an answer")
	}

	p := mustPreambles(t, &csbk.PreamblesConfig{Scratch: layer2.NewBurst()})
	p.Burst(0, burstOf(t, anytoneRadioCheck[0], layer2.SyncSourceMS), layer2.Hint{}, nil)
	if out := p.Lost(time.Second, nil); len(out) != 1 || out[0].Kind != csbk.OutputExpired {
		t.Errorf("Lost gave %+v", out)
	}
	p.Burst(0, burstOf(t, anytoneRadioCheck[0], layer2.SyncSourceMS), layer2.Hint{}, nil)
	p.Reset()
	if _, ok := p.Deadline(); ok {
		t.Error("Reset left a transfer")
	}

	w, err := csbk.NewWakeup(&csbk.WakeupConfig{Slot: enums.Slot1, SyncWait: -1})
	if err != nil {
		t.Fatal(err)
	}
	if out := w.Cancel(0, nil); len(out) != 0 {
		t.Error("Cancel without a wakeup gave output")
	}
	if err := w.Wake(0, 1, 2); err != nil {
		t.Fatal(err)
	}
	if out := w.OutboundSync(0, nil); len(out) != 0 {
		t.Error("outbound before the first BS_Dwn_Act ended the wakeup")
	}
	if out := w.Lost(0, nil); len(out) != 1 {
		t.Errorf("Lost gave %+v", out)
	}
	if _, ok := w.Deadline(); ok {
		t.Error("a negative SyncWait has a deadline")
	}
	if out := w.Cancel(0, nil); len(out) != 1 || out[0].Result != csbk.ResultCanceled {
		t.Errorf("Cancel gave %+v", out)
	}
	if err := w.Wake(0, 1<<24, 2); err == nil {
		t.Error("BS address above 24 bits")
	}
	w.Reset()
	if w.State() != csbk.WakeupIdle {
		t.Error("Reset left a wakeup")
	}
}

func TestZeroMachines(t *testing.T) {
	t.Parallel()

	var q csbk.Requester
	for name, err := range map[string]error{
		"RequestOACSU":      q.RequestOACSU(0, 2, 1, layer3.ServiceOptions{}, nil),
		"RequestRadioCheck": q.RequestRadioCheck(0, 2, 1, nil),
		"RequestCallAlert":  q.RequestCallAlert(0, 2, 1, nil),
		"Request":           q.Request(0, octets(t, injectedRadioCheck), csbk.Match{N: 1}, nil),
	} {
		if !errors.Is(err, dmrgo.ErrZeroValue) {
			t.Errorf("zero Requester %s: %v", name, err)
		}
	}
	var w csbk.Wakeup
	if err := w.Wake(0, 1, 2); !errors.Is(err, dmrgo.ErrZeroValue) {
		t.Errorf("zero Wakeup Wake: %v", err)
	}
	var m csbk.Timing
	if err := m.Join(0); !errors.Is(err, dmrgo.ErrZeroValue) {
		t.Errorf("zero Timing Join: %v", err)
	}
	var r csbk.Responder
	var p csbk.Preambles
	b := layer2.NewBurst()
	if err := b.DecodeFromBytes(*burstOf(t, injectedRadioCheck, layer2.SyncSourceBS)); err != nil {
		t.Fatal(err)
	}
	for name, out := range map[string][]csbk.Output{
		"Requester": q.Tick(time.Hour, nil),
		"Wakeup":    w.Tick(time.Hour, nil),
		"Timing":    m.Tick(time.Hour, nil),
		"Responder": append(r.Decoded(0, b, layer2.Hint{}, nil), r.Tick(time.Hour, nil)...),
		"Preambles": p.Decoded(0, b, layer2.Hint{}, nil),
	} {
		if len(out) != 0 {
			t.Errorf("zero %s: %d outputs", name, len(out))
		}
	}
}

func TestLead(t *testing.T) {
	t.Parallel()

	const lead = 20 * ms
	q := mustRequester(t, &csbk.RequesterConfig{Slot: enums.Slot1, Lead: lead})
	if err := q.RequestRadioCheck(time.Second, 2, 1, &csbk.RequestOptions{Preambles: 1}); err != nil {
		t.Fatal(err)
	}
	w, err := csbk.NewWakeup(&csbk.WakeupConfig{Slot: enums.Slot1, Lead: lead})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Wake(time.Second, 1, 2); err != nil {
		t.Fatal(err)
	}
	for name, m := range map[string]machine{"Requester": q, "Wakeup": w} {
		at, ok := m.Deadline()
		if !ok || at != time.Second {
			t.Fatalf("%s: Deadline %v %t", name, at, ok)
		}
		if out := m.Tick(at-1, nil); len(out) != 0 {
			t.Fatalf("%s: burst before Due minus Lead", name)
		}
		out := m.Tick(at, nil)
		if len(out) != 1 || out[0].Kind != csbk.OutputBurst || out[0].Due != time.Second+lead {
			t.Fatalf("%s: %+v", name, out)
		}
	}
	if at, ok := q.Deadline(); !ok || at != time.Second+csbk.BurstInterval {
		t.Errorf("Requester: second burst deadline %v", at)
	}
	if at, ok := w.Deadline(); !ok || at != time.Second+lead+csbk.DefaultSyncWait {
		t.Errorf("Wakeup: T_SyncWu deadline %v, want it from Due", at)
	}
	out := q.Tick(time.Second+csbk.BurstInterval, nil)
	if len(out) != 1 || out[0].Due != time.Second+lead+csbk.BurstInterval {
		t.Fatalf("Requester: second burst %+v", out)
	}
	wait := out[0].Due + csbk.DefaultMotorolaAckWait
	if at, ok := q.Deadline(); !ok || at != wait {
		t.Fatalf("Requester: T_AckWait deadline %v, want %v", at, wait)
	}
	out = q.Tick(wait, nil)
	if len(out) != 1 || out[0].Kind != csbk.OutputBurst || out[0].Due != wait+lead {
		t.Fatalf("Requester: retry %+v, want Due %v", out, wait+lead)
	}
	at, _ := w.Deadline()
	out = w.Tick(at, nil)
	if len(out) != 1 || out[0].Kind != csbk.OutputBurst || out[0].Due != at+lead {
		t.Fatalf("Wakeup: retry %+v, want Due %v", out, at+lead)
	}
	for _, err := range []error{
		func() error {
			_, err := csbk.NewRequester(&csbk.RequesterConfig{Slot: enums.Slot1, Lead: -1})
			return err
		}(),
		func() error {
			_, err := csbk.NewResponder(&csbk.ResponderConfig{Slot: enums.Slot1, Address: 1, Lead: -1})
			return err
		}(),
		func() error { _, err := csbk.NewWakeup(&csbk.WakeupConfig{Slot: enums.Slot1, Lead: -1}); return err }(),
		func() error { _, err := csbk.NewTiming(&csbk.TimingConfig{ID: 1, Slot: 1, Lead: -1}); return err }(),
	} {
		var ce *dmrgo.ConfigError
		if !errors.As(err, &ce) || ce.Field != "Lead" {
			t.Errorf("negative Lead: %v", err)
		}
	}
}

func TestHintSlot(t *testing.T) {
	t.Parallel()

	q := mustRequester(t, &csbk.RequesterConfig{Slot: enums.Slot2})
	if err := q.RequestRadioCheck(0, 2, 1, nil); err != nil {
		t.Fatal(err)
	}
	w, err := csbk.NewWakeup(&csbk.WakeupConfig{Slot: enums.Slot1})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Wake(0, 1, 2); err != nil {
		t.Fatal(err)
	}
	m, err := csbk.NewTiming(&csbk.TimingConfig{ID: 1, Slot: enums.Slot2})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Join(0); err != nil {
		t.Fatal(err)
	}
	for name, tt := range map[string]struct {
		out  []csbk.Output
		slot enums.Slot
	}{
		"Requester":  {q.Tick(0, nil), enums.Slot2},
		"Wakeup":     {w.Tick(0, nil), enums.Slot1},
		"Terminator": {m.Terminator(0, nil), enums.Slot2},
	} {
		if len(tt.out) != 1 {
			t.Fatalf("%s: %d outputs", name, len(tt.out))
		}
		if h := tt.out[0].Hint; h.Slot != tt.slot || h.Kind != layer2.KindData || !h.HasDataType || h.DataType != enums.DataTypeCSBK {
			t.Errorf("%s: Hint %+v", name, h)
		}
	}
	for _, err := range []error{
		func() error { _, err := csbk.NewRequester(&csbk.RequesterConfig{Slot: 3}); return err }(),
		func() error { _, err := csbk.NewResponder(&csbk.ResponderConfig{Address: 1, Slot: 3}); return err }(),
		func() error { _, err := csbk.NewWakeup(&csbk.WakeupConfig{Slot: 3}); return err }(),
	} {
		var ce *dmrgo.ConfigError
		if !errors.As(err, &ce) || ce.Field != "Slot" {
			t.Errorf("Slot 3: %v", err)
		}
	}
}
