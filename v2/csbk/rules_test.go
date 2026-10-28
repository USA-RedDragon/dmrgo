package csbk_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func loop(from []csbk.Output, to machine) []csbk.Output {
	var all []csbk.Output
	for i := range from {
		if from[i].Kind == csbk.OutputBurst {
			all = append(all, to.Burst(from[i].Due, &from[i].Burst, layer2.Hint{}, nil)...)
		}
	}
	return all
}

func TestOACSU(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		answer csbk.OACSUAnswer
		result csbk.Result
	}{
		{csbk.OACSUProceed, csbk.ResultAnswered},
		{csbk.OACSUDeny, csbk.ResultRefused},
		{csbk.OACSUNotSupported, csbk.ResultNACK},
	} {
		q := mustRequester(t, &csbk.RequesterConfig{Slot: enums.Slot1, ColorCode: 1, Source: layer2.SyncSourceMS})
		r := mustResponder(t, &csbk.ResponderConfig{Slot: enums.Slot1, Address: 1002, ColorCode: 1, Source: layer2.SyncSourceMS, OACSU: tt.answer})
		if err := q.RequestOACSU(0, 1002, 1001, layer3.ServiceOptions{PriorityLevel: 2}, nil); err != nil {
			t.Fatal(err)
		}
		reqs := loop(run(t, q, 0), r)
		if len(reqs) != 1 || reqs[0].Kind != csbk.OutputRequest {
			t.Fatalf("%v: responder gave %v", tt.answer, kinds(reqs))
		}
		wantSvc := csbk.ServiceOACSU
		if tt.answer == csbk.OACSUNotSupported {
			wantSvc = csbk.ServiceUnsupported
		}
		if reqs[0].Service != wantSvc || reqs[0].Call.Source != 1001 || reqs[0].Call.Target != 1002 {
			t.Errorf("%v: request %+v", tt.answer, reqs[0])
		}
		done := loop(run(t, r, time.Second), q)
		if len(done) != 1 || done[0].Result != tt.result || done[0].Service != csbk.ServiceOACSU {
			t.Fatalf("%v: requester gave %+v", tt.answer, done)
		}
		c := pdu.DecodeCSBK(done[0].CSBK)
		switch tt.answer {
		case csbk.OACSUNotSupported:
			n, ok := c.AsNegativeAcknowledgement()
			if !ok || n.ServiceType != pdu.CSBKUnitToUnitVoiceServiceRequest || csbk.Reason(n.ReasonCode) != csbk.ReasonNotSupported ||
				n.SourceType != layer3.SourceTypeMS || n.TargetAddress != 1001 || n.SourceAddress != 1002 || done[0].Reason != csbk.ReasonNotSupported {
				t.Errorf("NACK_Rsp %+v", n)
			}
		case csbk.OACSUProceed, csbk.OACSUDeny:
			a, ok := c.AsUnitToUnitVoiceServiceAnswerResponse()
			if !ok || a.TargetAddress != 1001 || a.SourceAddress != 1002 || a.ServiceOptions.PriorityLevel != 2 {
				t.Errorf("UU_Ans_Rsp %+v", a)
			}
		}
	}
}

func TestRequesterRetries(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		retries, tries int
	}{
		{0, 1 + constants.NCSBKRetry},
		{-1, 1},
		{3, 4},
	} {
		q := mustRequester(t, &csbk.RequesterConfig{Slot: enums.Slot1, Retries: tt.retries})
		if err := q.RequestOACSU(0, 2, 1, layer3.ServiceOptions{}, nil); err != nil {
			t.Fatal(err)
		}
		outs := run(t, q, time.Hour)
		if len(outs) != tt.tries+1 {
			t.Fatalf("retries %d: outputs %v", tt.retries, kinds(outs))
		}
		for i := 1; i < tt.tries; i++ {
			if d := outs[i].Due - outs[i-1].Due; d != constants.TAckWait {
				t.Errorf("retries %d: try %d after %v", tt.retries, i, d)
			}
		}
		if end := outs[tt.tries]; end.Result != csbk.ResultNoAnswer || end.Due != 0 {
			t.Errorf("retries %d: end %+v", tt.retries, end)
		}
	}
}

func TestRequesterChannelAccess(t *testing.T) {
	t.Parallel()

	t.Run("time critical denied", func(t *testing.T) {
		t.Parallel()
		q := mustRequester(t, nil)
		q.Channel(0, true)
		if err := q.RequestOACSU(0, 2, 1, layer3.ServiceOptions{}, nil); err != nil {
			t.Fatal(err)
		}
		if at, _ := q.Deadline(); at != csbk.DefaultIdleSearch {
			t.Errorf("deadline %v, want T_IdleSrch", at)
		}
		outs := run(t, q, time.Hour)
		if len(outs) != 1 || outs[0].Result != csbk.ResultDenied {
			t.Errorf("outputs %+v", outs)
		}
	})
	t.Run("idle in time", func(t *testing.T) {
		t.Parallel()
		q := mustRequester(t, nil)
		q.Channel(0, true)
		if err := q.RequestOACSU(0, 2, 1, layer3.ServiceOptions{}, nil); err != nil {
			t.Fatal(err)
		}
		q.Channel(100*ms, false)
		out := q.Tick(100*ms, nil)
		if len(out) != 1 || out[0].Kind != csbk.OutputBurst || out[0].Due != 100*ms {
			t.Errorf("outputs %+v", out)
		}
	})
	t.Run("holdoff", func(t *testing.T) {
		t.Parallel()
		q := mustRequester(t, &csbk.RequesterConfig{Slot: enums.Slot1, Seed: 7})
		q.Channel(0, true)
		if err := q.RequestCallAlert(0, 2, 1, nil); err != nil {
			t.Fatal(err)
		}
		if out := run(t, q, 10*time.Second); len(out) != 0 {
			t.Fatalf("busy channel gave %v", kinds(out))
		}
		if q.State() != csbk.RequesterAccess {
			t.Fatalf("state %v", q.State())
		}
		at, _ := q.Deadline()
		q.Channel(at-ms, false)
		if out := q.Tick(at-ms, nil); len(out) != 0 {
			t.Fatal("sent before the end of the holdoff")
		}
		out := run(t, q, at)
		if len(out) != 1 || out[0].Kind != csbk.OutputBurst || out[0].Due != at {
			t.Errorf("outputs %+v at %v", out, at)
		}
	})
	t.Run("impolite", func(t *testing.T) {
		t.Parallel()
		q := mustRequester(t, nil)
		q.Channel(0, true)
		if err := q.RequestRadioCheck(0, 2, 1, &csbk.RequestOptions{Access: enums.AccessImpolite}); err != nil {
			t.Fatal(err)
		}
		if out := q.Tick(0, nil); len(out) != 1 || out[0].Kind != csbk.OutputBurst {
			t.Errorf("outputs %+v", out)
		}
	})
}

func TestRequesterErrors(t *testing.T) {
	t.Parallel()

	q := mustRequester(t, nil)
	var ce *dmrgo.ConfigError
	if err := q.RequestOACSU(0, 2, 1, layer3.ServiceOptions{}, &csbk.RequestOptions{Access: enums.AccessImpolite}); !errors.As(err, &ce) || ce.Field != "Access" {
		t.Errorf("impolite OACSU: %v", err)
	}
	if err := q.RequestRadioCheck(0, 1<<24, 1, nil); !errors.As(err, &ce) || ce.Field != "Target" {
		t.Errorf("target: %v", err)
	}
	if err := q.RequestCallAlert(0, 2, 1, &csbk.RequestOptions{Preambles: 256}); !errors.As(err, &ce) || ce.Field != "Preambles" {
		t.Errorf("preambles: %v", err)
	}
	if err := q.Request(0, &pdu.CSBK{}, csbk.Match{}, nil); !errors.As(err, &ce) || ce.Field != "Match" {
		t.Errorf("match: %v", err)
	}
	if err := q.Request(0, &pdu.CSBK{Opcode: pdu.CSBKAloha}, csbk.Match{N: 1}, nil); !errors.Is(err, csbk.ErrNoAddresses) {
		t.Errorf("C_ALOHA: %v", err)
	}
	if err := q.Request(0, nil, csbk.Match{N: 1}, nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil CSBK: %v", err)
	}
	if err := q.Request(0, &pdu.CSBK{Opcode: 64}, csbk.Match{N: 1}, nil); err == nil {
		t.Error("opcode above 6 bits")
	}
	if err := q.RequestRadioCheck(0, 2, 1, nil); err != nil {
		t.Fatal(err)
	}
	if err := q.RequestRadioCheck(0, 2, 1, nil); !errors.Is(err, csbk.ErrBusy) {
		t.Errorf("second request: %v", err)
	}
	out := q.Cancel(0, nil)
	if len(out) != 1 || out[0].Result != csbk.ResultCanceled || q.State() != csbk.RequesterIdle {
		t.Errorf("cancel %+v", out)
	}
	for _, cfg := range []*csbk.RequesterConfig{{Slot: enums.Slot1, ColorCode: 16}, {Slot: enums.Slot1, Source: 9}, {Slot: enums.Slot1, Retries: csbk.MaxRetries + 1}} {
		if _, err := csbk.NewRequester(cfg); !errors.As(err, &ce) {
			t.Errorf("%+v: %v", cfg, err)
		}
	}
	for _, cfg := range []*csbk.ResponderConfig{nil, {Slot: enums.Slot1}, {Slot: enums.Slot1, Address: 1 << 24}, {Slot: enums.Slot1, Address: 1, ColorCode: 16}, {Slot: enums.Slot1, Address: 1, Source: 9},
		{Slot: enums.Slot1, Address: 1, Access: 3}, {Slot: enums.Slot1, Address: 1, OACSU: 3}} {
		if _, err := csbk.NewResponder(cfg); !errors.As(err, &ce) {
			t.Errorf("%+v: %v", cfg, err)
		}
	}
	for _, cfg := range []*csbk.WakeupConfig{{Slot: enums.Slot1, ColorCode: 16}, {Slot: enums.Slot1, Attempts: -1}, {Slot: enums.Slot1, Attempts: csbk.MaxRetries + 1}} {
		if _, err := csbk.NewWakeup(cfg); !errors.As(err, &ce) {
			t.Errorf("%+v: %v", cfg, err)
		}
	}
}

func TestRequesterCustom(t *testing.T) {
	t.Parallel()

	var c pdu.CSBK
	if err := c.SetAhoy(pdu.AhoyPDU{TargetAddress: 2, SourceAddress: 1}); err != nil {
		t.Fatal(err)
	}
	ack := pdu.CSBK{LastBlock: true}
	if err := ack.SetMaintenance(pdu.MaintenancePDU{TargetAddress: 1, SourceAddress: 2}); err != nil {
		t.Fatal(err)
	}
	ackOctets, err := ack.Encode()
	if err != nil {
		t.Fatal(err)
	}
	q := mustRequester(t, nil)
	if err := q.Request(0, &c, mustMatch(t, enums.FIDStandardized, pdu.CSBKMaintenance), nil); err != nil {
		t.Fatal(err)
	}
	if c.LastBlock {
		t.Error("Request changed c")
	}
	sent := run(t, q, 0)
	if sent[0].Kind != csbk.OutputBurst || sent[0].Call != (layer2.Call{Target: 2, Source: 1}) {
		t.Errorf("request burst %+v", sent[0])
	}
	if p := pdu.DecodeCSBK([12]byte(bptc(t, &sent[0]))); !p.LastBlock || p.FEC.Uncorrectable {
		t.Errorf("request %+v", p)
	}
	b, err := layer2.BuildDataBurst(ackOctets, enums.DataTypeCSBK, 0, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	out := q.Burst(100*ms, &b, layer2.Hint{}, nil)
	if len(out) != 1 || out[0].Result != csbk.ResultAnswered || out[0].Service != csbk.ServiceCustom {
		t.Errorf("answer %+v", out)
	}
}

func TestResponderFNS(t *testing.T) {
	t.Parallel()

	ahoy := func(target uint32, group bool) layer2.RawBurst {
		c := pdu.CSBK{LastBlock: true}
		if err := c.SetAhoy(pdu.AhoyPDU{Group: group, TargetAddress: target, SourceAddress: 5}); err != nil {
			t.Fatal(err)
		}
		b, err := layer2.BuildBurst(&c, 0, layer2.SyncSourceBS)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	r := mustResponder(t, &csbk.ResponderConfig{Slot: enums.Slot1, Address: 9})
	for _, b := range []layer2.RawBurst{ahoy(9, true), ahoy(10, false)} {
		if out := r.Burst(0, &b, layer2.Hint{}, nil); len(out) != 0 || r.State() != csbk.ResponderIdle {
			t.Errorf("answered %v", kinds(out))
		}
	}
	bad := ahoy(9, false)
	p, _ := bad.BPTCPayload()
	p[10] ^= 1
	bad.SetBPTCPayload(p)
	if out := r.Burst(0, &bad, layer2.Hint{}, nil); len(out) != 0 {
		t.Errorf("bad CRC answered %v", kinds(out))
	}
	good := ahoy(9, false)
	if out := r.Burst(0, &good, layer2.Hint{}, nil); len(out) != 1 || out[0].Service != csbk.ServiceUnsupported {
		t.Fatalf("outputs %+v", out)
	}
	out := run(t, r, time.Second)
	nc := pdu.DecodeCSBK([12]byte(bptc(t, &out[0])))
	n, ok := nc.AsNegativeAcknowledgement()
	if !ok || n.ServiceType != pdu.CSBKAhoy || n.TargetAddress != 5 || n.SourceAddress != 9 {
		t.Errorf("NACK_Rsp %+v", n)
	}
	if out[0].Kind != csbk.OutputBurst || out[0].Call != (layer2.Call{Target: 5, Source: 9}) {
		t.Errorf("NACK_Rsp burst %+v", out[0])
	}

	quiet := mustResponder(t, &csbk.ResponderConfig{Slot: enums.Slot1, Address: 9, NoFNS: true})
	if out := quiet.Burst(0, &good, layer2.Hint{}, nil); len(out) != 1 || quiet.State() != csbk.ResponderIdle {
		t.Errorf("NoFNS: %v, pending %t", kinds(out), quiet.State() != csbk.ResponderIdle)
	}
}

func TestResponderDenied(t *testing.T) {
	t.Parallel()

	r := mustResponder(t, &csbk.ResponderConfig{Slot: enums.Slot1, Address: anytone, ColorCode: 7})
	r.Channel(0, true)
	r.Burst(0, burstOf(t, injectedRadioCheck, layer2.SyncSourceBS), layer2.Hint{}, nil)
	out := run(t, r, time.Hour)
	if len(out) != 1 || out[0].Kind != csbk.OutputDone || out[0].Result != csbk.ResultDenied || out[0].Service != csbk.ServiceRadioCheck {
		t.Errorf("outputs %+v", out)
	}
	off := mustResponder(t, &csbk.ResponderConfig{Slot: enums.Slot1, Address: anytone, NoRadioCheck: true, NoCallAlert: true})
	for _, s := range []string{injectedRadioCheck, injectedCallAlert, anytoneRadioAnswer} {
		if out := off.Burst(0, burstOf(t, s, layer2.SyncSourceBS), layer2.Hint{}, nil); len(out) != 0 {
			t.Errorf("%s: %v", s, kinds(out))
		}
	}
}

func TestPreamblesData(t *testing.T) {
	t.Parallel()

	pre := func(cbf byte, target uint32) *layer2.RawBurst {
		c := pdu.CSBK{LastBlock: true}
		if err := c.SetPreamble(pdu.PreamblePDU{Data: true, CSBKBlocksToFollow: cbf, TargetAddress: target, SourceAddress: 1}); err != nil {
			t.Fatal(err)
		}
		b, err := layer2.BuildBurst(&c, 1, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		return &b
	}
	var h pdu.DataHeader
	if err := h.SetUnconfirmed(pdu.UnconfirmedDataHeader{LLIDDestination: 2, LLIDSource: 1, BlocksToFollow: 1}); err != nil {
		t.Fatal(err)
	}
	header, err := layer2.BuildBurst(&h, 1, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	p := mustPreambles(t, nil)
	out := p.Burst(0, pre(3, 2), layer2.Hint{}, nil)
	out = append(out, p.Burst(60*ms, pre(2, 2), layer2.Hint{}, nil)...)
	out = append(out, p.Burst(120*ms, &header, layer2.Hint{}, nil)...)
	if !slices.Equal(kinds(out), []csbk.OutputKind{csbk.OutputAnnounced, csbk.OutputJoined}) || !out[1].Data || !out[0].Data || out[0].Until != 180*ms {
		t.Fatalf("outputs %+v", out)
	}

	out = p.Burst(0, pre(2, 2), layer2.Hint{}, nil)
	out = append(out, p.Burst(60*ms, pre(5, 3), layer2.Hint{}, nil)...)
	out = append(out, run(t, p, time.Hour)...)
	if !slices.Equal(kinds(out), []csbk.OutputKind{csbk.OutputAnnounced, csbk.OutputExpired, csbk.OutputAnnounced, csbk.OutputExpired}) {
		t.Fatalf("outputs %v", kinds(out))
	}
	if out[1].Call.Target != 2 || out[2].Call.Target != 3 || out[3].Until != 360*ms {
		t.Errorf("outputs %+v", out)
	}
	if p.State() != csbk.PreamblesIdle {
		t.Errorf("state %v", p.State())
	}
}

func TestWakeupOutbound(t *testing.T) {
	t.Parallel()

	idle := func(cc enums.ColorCode) *layer2.RawBurst {
		b, err := layer2.BuildBurst(&pdu.PRFill{}, cc, layer2.SyncSourceBS)
		if err != nil {
			t.Fatal(err)
		}
		return &b
	}
	w, err := csbk.NewWakeup(&csbk.WakeupConfig{Slot: enums.Slot1, ColorCode: 7, Attempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Wake(0, xpr, anytone); err != nil {
		t.Fatal(err)
	}
	if err := w.Wake(0, xpr, anytone); !errors.Is(err, csbk.ErrBusy) {
		t.Errorf("second wake: %v", err)
	}
	if out := w.Tick(0, nil); len(out) != 1 || out[0].Kind != csbk.OutputBurst {
		t.Fatalf("outputs %v", kinds(out))
	}
	if out := w.Burst(100*ms, idle(1), layer2.Hint{}, nil); len(out) != 0 {
		t.Errorf("CC 1 gave %v", kinds(out))
	}
	out := w.Burst(200*ms, idle(7), layer2.Hint{}, nil)
	if len(out) != 1 || out[0].Result != csbk.ResultAnswered || out[0].Service != csbk.ServiceWakeup || w.State() != csbk.WakeupIdle {
		t.Errorf("outputs %+v", out)
	}
}
