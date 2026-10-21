package csbk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
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
		{(&csbk.ConfigError{Field: "Retries"}).Error(), "csbk: Retries is out of range"},
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

	r := mustResponder(t, &csbk.ResponderConfig{Address: anytone})
	r.Channel(0, true)
	r.Burst(0, burstOf(t, injectedRadioCheck, layer2.SyncSourceBS), layer2.Hint{}, nil)
	if out := r.Lost(ms, nil); len(out) != 1 || out[0].Kind != csbk.OutputBurst {
		t.Errorf("Lost gave %+v", out)
	}
	r.Burst(0, burstOf(t, injectedRadioCheck, layer2.SyncSourceBS), layer2.Hint{}, nil)
	r.Reset()
	if r.Pending() {
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

	w, err := csbk.NewWakeup(&csbk.WakeupConfig{SyncWait: -1})
	if err != nil {
		t.Fatal(err)
	}
	if out := w.Cancel(0, nil); len(out) != 0 {
		t.Error("Cancel without a wakeup gave output")
	}
	if err := w.Wake(0, 1, 2); err != nil {
		t.Fatal(err)
	}
	if out := w.OutboundSync(nil); len(out) != 0 {
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
