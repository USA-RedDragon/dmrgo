package csbk_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

const (
	maxFuzzOps = 512
	maxScript  = 128
)

type rig struct {
	q    *csbk.Requester
	r    *csbk.Responder
	p    *csbk.Preambles
	w    *csbk.Wakeup
	pool []layer2.RawBurst
	out  []csbk.Output
	now  time.Duration
}

func newRig(t testing.TB) *rig {
	t.Helper()
	scratch := layer2.NewBurst()
	q, err := csbk.NewRequester(&csbk.RequesterConfig{ColorCode: 7, Source: layer2.SyncSourceMS, Scratch: scratch, HoldoffMax: 200 * ms})
	if err != nil {
		t.Fatal(err)
	}
	r, err := csbk.NewResponder(&csbk.ResponderConfig{Address: anytone, ColorCode: 7, Scratch: scratch})
	if err != nil {
		t.Fatal(err)
	}
	w, err := csbk.NewWakeup(&csbk.WakeupConfig{ColorCode: 7, Scratch: scratch})
	if err != nil {
		t.Fatal(err)
	}
	p, err := csbk.NewPreambles(&csbk.PreamblesConfig{Scratch: scratch})
	if err != nil {
		t.Fatal(err)
	}
	g := &rig{q: q, r: r, p: p, w: w, out: make([]csbk.Output, 0, csbk.MaxOutputs)}
	for _, s := range []string{injectedRadioCheck, anytoneRadioAnswer, injectedCallAlert, anytoneCallAlertAc, anytoneRadioCheck[0], anytoneRadioCheck[5]} {
		b, _ := hex.DecodeString(s)
		for _, src := range []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS} {
			raw, err := layer2.BuildDataBurst(pdu.CSBKBytes(b), enums.DataTypeCSBK, 7, src)
			if err != nil {
				t.Fatal(err)
			}
			g.pool = append(g.pool, raw)
		}
	}
	idle, err := layer2.BuildBurst(&pdu.PRFill{}, 7, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	g.pool = append(g.pool, idle)
	return g
}

func (g *rig) check(t testing.TB, op byte) {
	t.Helper()
	if len(g.out) > csbk.MaxOutputs {
		t.Fatalf("op %d appended %d outputs", op, len(g.out))
	}
}

func (g *rig) step(t testing.TB, op, arg byte) {
	t.Helper()
	g.now += time.Duration(arg&0x0F) * 15 * ms
	h := layer2.Hint{}
	var raw layer2.RawBurst
	if len(g.pool) > 0 {
		raw = g.pool[int(arg>>4)%len(g.pool)]
	}
	if op&0x80 != 0 {
		raw[int(arg)%len(raw)] ^= 1 << (op & 7)
	}
	switch op & 0x0F {
	case 0:
		g.out = g.q.Burst(g.now, &raw, h, g.out[:0])
	case 1:
		g.out = g.r.Burst(g.now, &raw, h, g.out[:0])
	case 2:
		g.out = g.p.Burst(g.now, &raw, h, g.out[:0])
	case 3:
		g.out = g.w.Burst(g.now, &raw, h, g.out[:0])
	case 4:
		_ = g.q.RequestRadioCheck(g.now, injector, anytone, &csbk.RequestOptions{Preambles: int(arg & 3), Access: enums.Access(arg >> 6)})
	case 5:
		_ = g.q.RequestOACSU(g.now, injector, anytone, layer3.ServiceOptions{}, &csbk.RequestOptions{TimeCritical: arg&1 != 0})
	case 6:
		g.q.Channel(g.now, arg&1 != 0)
		g.r.Channel(g.now, arg&2 != 0)
	case 7:
		_ = g.w.Wake(g.now, xpr, anytone)
	case 8:
		g.out = g.q.Cancel(g.now, g.out[:0])
	case 9:
		g.out = g.w.Cancel(g.now, g.out[:0])
	case 10:
		g.out = g.q.Lost(g.now, g.out[:0])
	case 11:
		g.out = g.r.Lost(g.now, g.out[:0])
	default:
		for _, m := range []machine{g.q, g.r, g.p, g.w} {
			if at, ok := m.Deadline(); ok && at <= g.now {
				g.out = m.Tick(g.now, g.out[:0])
				g.check(t, op)
			}
		}
	}
	g.check(t, op)
}

func runScript(t testing.TB, g *rig, in []byte, from, to, reps uint8) {
	t.Helper()
	in = in[:min(len(in), maxScript)]
	n := len(in) / 2
	lo, hi, total := fuzztest.Loop(n, from, to, reps, maxFuzzOps)
	for s := range total {
		i := 2 * fuzztest.Index(s, lo, hi, reps)
		g.step(t, in[i], in[i+1])
	}
}

func fuzzSeeds() [][]byte {
	return [][]byte{
		{4, 0x01, 15, 0x02, 15, 0x0F, 0, 0x1F, 15, 0x0F},
		{5, 0x00, 15, 0x00, 1, 0x00, 15, 0x05, 0, 0x30, 15, 0x0F},
		{7, 0x00, 15, 0x00, 3, 0xC0, 15, 0x0F, 15, 0x0F},
		{6, 0x03, 4, 0x02, 15, 0x0F, 15, 0x0F, 6, 0x00, 15, 0x01},
		{2, 0x80, 2, 0x81, 2, 0x81, 2, 0x8F, 15, 0x0F},
		{0x84, 0x05, 0x81, 0x22, 8, 0, 9, 0, 10, 0, 11, 0},
	}
}

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()
	for _, in := range fuzzSeeds() {
		runScript(t, newRig(t), in, 0, 0, 0)
	}
}

func FuzzMachines(f *testing.F) {
	for _, in := range fuzzSeeds() {
		f.Add(in, uint8(0), uint8(0), uint8(0))
	}
	f.Add([]byte{4, 0x01, 1, 0x21, 15, 0x0F}, uint8(1), uint8(3), uint8(40))
	f.Fuzz(func(t *testing.T, in []byte, from, to, reps uint8) {
		runScript(t, newRig(t), in, from, to, reps)
	})
}
