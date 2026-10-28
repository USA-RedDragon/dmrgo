package rf

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func msBursts(t testing.TB, name string) []Burst {
	t.Helper()
	cfg := testConfig()
	cfg.Superframes = 1
	b, err := rftx.Sequence(name, cfg)
	if err != nil {
		return nil
	}
	out := make([]Burst, len(b.Bursts))
	for i, x := range b.Bursts {
		out[i] = Burst{Data: x, Stream: 1, Slot: int64(2 * i), Seconds: -1, Distance: 9, Sync: enums.SyncBSSourcedData}
	}
	return out
}

func bsBursts(t testing.TB, name string) []Burst {
	t.Helper()
	cfg := testConfig()
	cfg.Superframes, cfg.BSSourced, cfg.Slot = 1, true, 1
	b, err := rftx.Sequence(name, cfg)
	if err != nil {
		return nil
	}
	out := make([]Burst, len(b.Slots))
	for i, s := range b.Slots {
		out[i] = Burst{Data: s.Burst, CACH: s.CACH, HasCACH: true, BS: true, Stream: 1, Slot: int64(i)}
	}
	return out
}

const gapLag = tier0IQLag

func gapRun(t *testing.T, b []Burst) []float64 {
	t.Helper()
	tx, err := newTransmitter(usIQ(Tier0, 96000, IQFormatCS16), openGate)
	if err != nil {
		t.Fatal(err)
	}
	out := tx.AppendSamples(nil, b)
	if len(out) > tx.MaxOutput(len(b))*4 {
		t.Fatalf("%d bytes over MaxOutput %d", len(out), tx.MaxOutput(len(b))*4)
	}
	if !tx.Active() {
		t.Fatal("not active after the bursts")
	}
	out = tx.AppendFlush(out)
	if tx.Active() {
		t.Fatal("active after AppendFlush")
	}
	return envelope(out, tier0IQSamples)
}

func keyedOver(t *testing.T, name string, env []float64, from, to int) {
	t.Helper()
	for k := from; k < to; k++ {
		if env[k] < 0.98 {
			t.Fatalf("%s: carrier down at symbol %d (%.3f), want up from %d to %d", name, k, env[k], from, to)
		}
	}
}

func downOver(t *testing.T, name string, env []float64, from, to int) {
	t.Helper()
	for k := from; k < to; k++ {
		if env[k] > 0.01 {
			t.Fatalf("%s: carrier up at symbol %d (%.3f), want down from %d to %d", name, k, env[k], from, to)
		}
	}
}

func TestTxGapMS(t *testing.T) {
	t.Parallel()
	for _, name := range rftx.Sequences() {
		b := msBursts(t, name)
		if b == nil {
			continue
		}
		env := gapRun(t, b)
		want := (2*len(b)-1)*txSlotSyms + gapLag + txTail
		if len(env) != want {
			t.Fatalf("%s: %d symbols, want %d", name, len(env), want)
		}
		for i := range b {
			s := 2 * i * txSlotSyms
			keyedOver(t, name, env, s+txGuard+gapLag, s+txSlotSyms+gapLag)
			if i+1 < len(b) {
				downOver(t, name, env, s+txSlotSyms+gapLag+txTail, s+2*txSlotSyms+gapLag)
			}
		}
		downOver(t, name, env, len(env)-1, len(env))
	}
}

func TestTxGapBS(t *testing.T) {
	t.Parallel()
	for _, name := range rftx.Sequences() {
		b := bsBursts(t, name)
		if b == nil {
			continue
		}
		env := gapRun(t, b)
		want := txLead + len(b)*txSlotSyms + gapLag + txTail
		if len(env) != want {
			t.Fatalf("%s: %d symbols, want %d", name, len(env), want)
		}
		keyedOver(t, name, env, txLead+gapLag, txLead+len(b)*txSlotSyms+gapLag)
		downOver(t, name, env, len(env)-1, len(env))
	}
}

func TestTxGapRules(t *testing.T) {
	t.Parallel()
	ms := msBursts(t, "data12")
	bsb := bsBursts(t, "voice")[:4]
	resp := msBursts(t, "ack")
	for _, c := range []struct {
		name string
		b    []Burst
		syms int
		up   [][2]int
		down [][2]int
	}{
		{"MS on consecutive slots", []Burst{
			{Data: ms[0].Data, Stream: 1, Slot: 0}, {Data: ms[1].Data, Stream: 1, Slot: 1}},
			2*txSlotSyms + gapLag + txTail,
			[][2]int{{txGuard, txSlotSyms}, {txSlotSyms + txGuard, 2 * txSlotSyms}}, nil},
		{"data then a response after a gap", append(append([]Burst(nil), ms...),
			Burst{Data: resp[0].Data, Stream: 1, Slot: ms[len(ms)-1].Slot + 3}),
			(2*len(ms)-1)*txSlotSyms + gapLag + txTail + txSlotSyms + gapLag + txTail,
			nil, [][2]int{{(2*len(ms)-1)*txSlotSyms + txTail, (2*len(ms)-1)*txSlotSyms + gapLag + txTail + txGuard/2}}},
		{"new stream", []Burst{
			{Data: ms[0].Data, Stream: 1, Slot: 0}, {Data: ms[1].Data, Stream: 2, Slot: 1}},
			2 * (txSlotSyms + gapLag + txTail), nil, [][2]int{{txSlotSyms + txTail, txSlotSyms + gapLag + txTail + txGuard/2}}},
		{"BS after an idle slot", []Burst{bsb[0], {Data: bsb[2].Data, CACH: bsb[2].CACH, HasCACH: true, BS: true, Stream: 1, Slot: 2}},
			txLead + 3*txSlotSyms + gapLag + txTail,
			[][2]int{{txLead, txLead + txSlotSyms}, {txLead + 2*txSlotSyms, txLead + 3*txSlotSyms}},
			[][2]int{{txLead + txSlotSyms + txTail, txLead + 2*txSlotSyms - txLead}}},
		{"MS then BS on the next slot", []Burst{{Data: ms[0].Data, Stream: 1, Slot: 0}, bsb[1]},
			2*txSlotSyms + gapLag + txTail, [][2]int{{txGuard, 2 * txSlotSyms}}, nil},
		{"BS then MS on the next slot", []Burst{bsb[0], {Data: ms[1].Data, Stream: 1, Slot: 1}},
			txLead + 2*txSlotSyms + gapLag + txTail,
			[][2]int{{txLead, txLead + txSlotSyms}, {txLead + txSlotSyms + txGuard, txLead + 2*txSlotSyms}}, nil},
	} {
		env := gapRun(t, c.b)
		if len(env) != c.syms {
			t.Errorf("%s: %d symbols, want %d", c.name, len(env), c.syms)
			continue
		}
		for _, u := range c.up {
			keyedOver(t, c.name, env, u[0]+gapLag, u[1]+gapLag)
		}
		for _, d := range c.down {
			downOver(t, c.name, env, d[0]+gapLag, d[1]+gapLag)
		}
	}
}

func repeaterBursts(t *testing.T) []Burst {
	t.Helper()
	r, err := bs.New(&bs.Config{Mode: bs.ModeRF, ColorCode: 1, CACH: true})
	if err != nil {
		t.Fatal(err)
	}
	var in []layer2.RawBurst
	for _, name := range []string{"voice", "data12"} {
		cfg := testConfig()
		cfg.Superframes = 2
		b, err := rftx.Sequence(name, cfg)
		if err != nil {
			t.Fatal(err)
		}
		in = append(in, b.Bursts...)
	}
	out := make([]bs.Output, 0, bs.MaxOutputs)
	var got []Burst
	keep := func() {
		for _, o := range out {
			if o.Kind != bs.OutputBurst || o.To&bs.ToRF == 0 {
				continue
			}
			got = append(got, Burst{Data: o.Burst, CACH: o.CACH, HasCACH: o.HasCACH, BS: true, Stream: 1, Slot: int64(o.Due / (30 * time.Millisecond))})
		}
	}
	now := time.Duration(0)
	for i := 0; now < time.Duration(len(in)+200)*60*time.Millisecond; now += 10 * time.Millisecond {
		if i < len(in) && now >= time.Duration(i)*60*time.Millisecond {
			out = r.Inbound(now, &in[i], layer2.Hint{Slot: enums.Slot1}, bs.Source{RF: true}, out[:0])
			keep()
			i++
		}
		for {
			at, ok := r.Deadline()
			if !ok || at > now {
				break
			}
			out = r.Tick(now, out[:0])
			keep()
			if len(out) == 0 {
				break
			}
		}
	}
	return got
}

func TestTxGapRepeater(t *testing.T) {
	t.Parallel()
	b := repeaterBursts(t)
	if len(b) < 20 {
		t.Fatalf("%d repeater bursts", len(b))
	}
	runs := 0
	for i := range b {
		if i == 0 || b[i].Slot != b[i-1].Slot+1 {
			runs++
			if i > 0 && b[i].Slot <= b[i-1].Slot {
				t.Fatalf("repeater slot %d after %d", b[i].Slot, b[i-1].Slot)
			}
		}
	}
	c := usIQ(Tier0, 96000, IQFormatCS16)
	var out []byte
	c.OnSamples = func(x []byte) { out = append(out, x...) }
	var tx Transmitter
	if err := tx.init(c, openGate); err != nil {
		t.Fatal(err)
	}
	if n, err := tx.Write(b); n != len(b) || err != nil {
		t.Fatalf("%d of %d: %v", n, len(b), err)
	}
	if err := tx.Flush(); err != nil {
		t.Fatal(err)
	}
	env := envelope(out, tier0IQSamples)
	if runs == 1 {
		keyedOver(t, "repeater", env, txLead+gapLag, txLead+len(b)*txSlotSyms+gapLag)
	}
	t.Logf("%d repeater bursts in %d runs of consecutive slots, %d symbols", len(b), runs, len(env))
}
