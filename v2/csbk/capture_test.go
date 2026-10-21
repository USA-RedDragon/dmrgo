package csbk_test

import (
	"encoding/hex"
	"slices"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

var (
	anytoneRadioCheck = []string{
		"bd0000052febf630b43c9df6",
		"bd0000042febf630b43c2597",
		"bd0000032febf630b43c3cd3",
		"bd0000022febf630b43c84b2",
		"bd0000012febf630b43c5c30",
		"a410000030b43c2febf67ba1",
	}
	anytoneRetryPreambles = []string{
		"bd00000f2febf630b43c6eb8", "bd00000e2febf630b43cd6d9", "bd00000d2febf630b43c0e5b",
		"bd00000c2febf630b43cb63a", "bd00000b2febf630b43caf7e", "bd00000a2febf630b43c171f",
		"bd0000092febf630b43ccf9d", "bd0000082febf630b43c77fc", "bd0000072febf630b43cfd15",
		"bd0000062febf630b43c4574",
	}
)

const (
	injectedRadioCheck = "a41000002febf630b43c0637"
	anytoneRadioAnswer = "a41000802febf630b43cadce"
	injectedCallAlert  = "9f10000030b43c2febf62dbe"
	anytoneCallAlertAc = "a0109f002febf630b43cb2f5"
)

func TestRequesterRadioCheckMatchesAnyTone(t *testing.T) {
	t.Parallel()

	q := mustRequester(t, &csbk.RequesterConfig{ColorCode: 7, Source: layer2.SyncSourceMS})
	if err := q.RequestRadioCheck(0, injector, anytone, &csbk.RequestOptions{Preambles: 5, RetryPreambles: 15}); err != nil {
		t.Fatal(err)
	}
	outs := run(t, q, time.Hour)
	want := slices.Concat(anytoneRadioCheck, anytoneRetryPreambles, anytoneRadioCheck)
	if len(outs) != len(want)+1 {
		t.Fatalf("outputs %v, want %d bursts and the end", kinds(outs), len(want))
	}
	for i, w := range want {
		if got := payload(t, &outs[i]); got != w {
			t.Errorf("burst %d: %s, the radio sent %s", i, got, w)
		}
	}
	if d := outs[1].Due - outs[0].Due; d != csbk.BurstInterval {
		t.Errorf("burst spacing %v", d)
	}
	if d := outs[6].Due - outs[5].Due; d != csbk.DefaultMotorolaAckWait {
		t.Errorf("retry %v after the request, want T_AckWait", d)
	}
	end := outs[len(outs)-1]
	if end.Kind != csbk.OutputDone || end.Result != csbk.ResultNoAnswer || end.Service != csbk.ServiceRadioCheck ||
		end.Target != injector || end.Source != anytone {
		t.Errorf("end %+v", end)
	}
}

func TestRequesterTakesAnyToneAnswers(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name            string
		alert           bool
		request, answer string
		svc             csbk.Service
	}{
		{"radio check", false, injectedRadioCheck, anytoneRadioAnswer, csbk.ServiceRadioCheck},
		{"call alert", true, injectedCallAlert, anytoneCallAlertAc, csbk.ServiceCallAlert},
	} {
		q := mustRequester(t, &csbk.RequesterConfig{ColorCode: 7})
		var err error
		if tt.alert {
			err = q.RequestCallAlert(0, anytone, injector, nil)
		} else {
			err = q.RequestRadioCheck(0, anytone, injector, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		out := q.Tick(0, nil)
		if len(out) != 1 || payload(t, &out[0]) != tt.request {
			t.Fatalf("%s: request %v, injected %s", tt.name, out, tt.request)
		}
		if out = q.Burst(240*ms, burstOf(t, tt.request, layer2.SyncSourceBS), layer2.Hint{}, nil); len(out) != 0 {
			t.Fatalf("%s: the echo of the request gave %v", tt.name, kinds(out))
		}
		out = q.Burst(430*ms, burstOf(t, tt.answer, layer2.SyncSourceBS), layer2.Hint{}, nil)
		if len(out) != 1 || out[0].Kind != csbk.OutputDone || out[0].Result != csbk.ResultAnswered || out[0].Service != tt.svc ||
			hex.EncodeToString(out[0].CSBK[:]) != tt.answer {
			t.Fatalf("%s: answer gave %+v", tt.name, out)
		}
		if q.State() != csbk.RequesterIdle {
			t.Errorf("%s: state %v", tt.name, q.State())
		}
	}
}

func TestResponderAnswersLikeAnyTone(t *testing.T) {
	t.Parallel()

	r := mustResponder(t, &csbk.ResponderConfig{Address: anytone, ColorCode: 7, Source: layer2.SyncSourceMS})
	for _, tt := range []struct {
		request, answer string
		svc             csbk.Service
	}{
		{injectedRadioCheck, anytoneRadioAnswer, csbk.ServiceRadioCheck},
		{injectedCallAlert, anytoneCallAlertAc, csbk.ServiceCallAlert},
	} {
		out := r.Burst(0, burstOf(t, tt.request, layer2.SyncSourceBS), layer2.Hint{}, nil)
		if len(out) != 1 || out[0].Kind != csbk.OutputRequest || out[0].Service != tt.svc || out[0].Target != anytone || out[0].Source != injector {
			t.Fatalf("request %s gave %+v", tt.request, out)
		}
		out = run(t, r, time.Second)
		if len(out) != 1 || payload(t, &out[0]) != tt.answer {
			t.Errorf("answer to %s: %v, the radio sent %s", tt.request, out, tt.answer)
		}
	}
}

func TestPreamblesJoinXPROutbound(t *testing.T) {
	t.Parallel()

	p := mustPreambles(t, nil)
	pre := "bd00000130b43c2febf621a6"
	all := make([]csbk.Output, 0, 2)
	for _, at := range []int{624, 684, 744, 804, 864} {
		all = append(all, p.Burst(time.Duration(at)*ms, burstOf(t, pre, layer2.SyncSourceBS), layer2.Hint{}, nil)...)
	}
	all = append(all, p.Burst(924*ms, burstOf(t, injectedRadioCheck, layer2.SyncSourceBS), layer2.Hint{}, nil)...)
	if !slices.Equal(kinds(all), []csbk.OutputKind{csbk.OutputAnnounced, csbk.OutputJoined}) {
		t.Fatalf("outputs %v", kinds(all))
	}
	if a := all[0]; a.Target != anytone || a.Source != injector || a.Data || a.Until != 684*ms {
		t.Errorf("announced %+v", a)
	}
	if j := all[1]; j.Data || hex.EncodeToString(j.CSBK[:]) != injectedRadioCheck || j.Target != anytone {
		t.Errorf("joined %+v", j)
	}
}

func TestPreamblesJoinAnyToneInbound(t *testing.T) {
	t.Parallel()

	p := mustPreambles(t, nil)
	all := make([]csbk.Output, 0, 2)
	for i, s := range anytoneRadioCheck {
		all = append(all, p.Burst(time.Duration(i)*csbk.BurstInterval, burstOf(t, s, layer2.SyncSourceMS), layer2.Hint{}, nil)...)
	}
	if !slices.Equal(kinds(all), []csbk.OutputKind{csbk.OutputAnnounced, csbk.OutputJoined}) {
		t.Fatalf("outputs %v", kinds(all))
	}
	if all[0].Until != 5*csbk.BurstInterval || all[0].Target != injector || all[0].Source != anytone {
		t.Errorf("announced %+v", all[0])
	}
}

func TestWakeupMatchesXPRGolden(t *testing.T) {
	t.Parallel()

	const golden = "06548e2409d808e846084ab09ced5d7f77fd7577988a04f87788be603ac028a6e5"
	w, err := csbk.NewWakeup(&csbk.WakeupConfig{ColorCode: 7})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Wake(0, xpr, anytone); err != nil {
		t.Fatal(err)
	}
	outs := run(t, w, time.Hour)
	if !slices.Equal(kinds(outs), []csbk.OutputKind{csbk.OutputBurst, csbk.OutputBurst, csbk.OutputDone}) {
		t.Fatalf("outputs %v", kinds(outs))
	}
	if got := hex.EncodeToString(outs[0].Burst[:]); got != golden {
		t.Errorf("wakeup %s, the repeater accepted %s", got, golden)
	}
	if outs[1].Due != csbk.DefaultSyncWait || outs[2].Result != csbk.ResultNoAnswer || outs[2].Target != xpr {
		t.Errorf("retry at %v, end %+v", outs[1].Due, outs[2])
	}
}
