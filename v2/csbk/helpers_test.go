package csbk_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	anytone  = 3191868
	injector = 3140598
	xpr      = 314926
	ms       = time.Millisecond
)

type machine interface {
	Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []csbk.Output) []csbk.Output
	Lost(now time.Duration, dst []csbk.Output) []csbk.Output
	Tick(now time.Duration, dst []csbk.Output) []csbk.Output
	Deadline() (time.Duration, bool)
}

func octets(t *testing.T, s string) pdu.CSBKBytes {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 12 {
		t.Fatalf("vector %q: %v", s, err)
	}
	return pdu.CSBKBytes(b)
}

func burstOf(t *testing.T, s string, src layer2.SyncSource) *layer2.RawBurst {
	t.Helper()
	b, err := layer2.BuildDataBurst(octets(t, s), enums.DataTypeCSBK, 7, src)
	if err != nil {
		t.Fatal(err)
	}
	return &b
}

func payload(t *testing.T, o *csbk.Output) string {
	t.Helper()
	if o.Kind != csbk.OutputBurst {
		t.Fatalf("output %v, want a burst", o.Kind)
	}
	p, res := o.Burst.BPTCPayload()
	if res.Uncorrectable {
		t.Fatal("burst payload does not decode")
	}
	return hex.EncodeToString(p[:])
}

func run(t *testing.T, m machine, end time.Duration) []csbk.Output {
	t.Helper()
	var all []csbk.Output
	out := make([]csbk.Output, 0, csbk.MaxOutputs)
	for range 10000 {
		at, ok := m.Deadline()
		if !ok || at > end {
			return all
		}
		out = m.Tick(at, out[:0])
		if len(out) > csbk.MaxOutputs {
			t.Fatalf("%d outputs from one Tick", len(out))
		}
		all = append(all, out...)
	}
	t.Fatal("machine does not settle")
	return nil
}

func kinds(outs []csbk.Output) []csbk.OutputKind {
	k := make([]csbk.OutputKind, len(outs))
	for i := range outs {
		k[i] = outs[i].Kind
	}
	return k
}

func mustRequester(t *testing.T, cfg *csbk.RequesterConfig) *csbk.Requester {
	t.Helper()
	q, err := csbk.NewRequester(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func mustMatch(t *testing.T, fid enums.FID, opcodes ...pdu.CSBKOpcode) csbk.Match {
	t.Helper()
	m, err := csbk.NewMatch(fid, opcodes...)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func mustPreambles(t *testing.T, cfg *csbk.PreamblesConfig) *csbk.Preambles {
	t.Helper()
	p, err := csbk.NewPreambles(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func mustResponder(t *testing.T, cfg *csbk.ResponderConfig) *csbk.Responder {
	t.Helper()
	r, err := csbk.NewResponder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func bptc(t *testing.T, o *csbk.Output) pdu.CSBKBytes {
	t.Helper()
	p, res := o.Burst.BPTCPayload()
	if res.Uncorrectable {
		t.Fatal("burst payload does not decode")
	}
	return p
}
