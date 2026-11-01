package rf

import (
	"math"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestIQDiscBurstyOffset(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.ColorCode, cfg.Superframes, cfg.Wakeups, cfg.Slot = 1, 2, 0, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	const calls = 3
	gap := make([]float64, int(0.3*basebandRate))
	var fmAll, envAll []float64
	want := make([]uint64, 0, calls*18*cfg.Superframes)
	for c := range calls {
		if c > 0 {
			fmAll, envAll = append(fmAll, gap...), append(envAll, gap...)
		}
		fmAll, envAll = append(fmAll, fm...), append(envAll, env...)
		for i := range 18 * cfg.Superframes {
			want = append(want, rftx.AMBEValue(rftx.TestAMBEFrame(i)))
		}
	}
	for _, tier := range []Tier{Tier0, Tier1} {
		for seed := range uint64(4) {
			t.Run(strconv.Itoa(int(tier-Tier0))+"/"+strconv.Itoa(int(seed)), func(t *testing.T) {
				t.Parallel()
				x := msBaseband(fmAll, envAll, 900, 30, 100+seed)
				if n := c0c1InOrder(want, voiceFrames(receiveBaseband(x, tier))); n < len(want) {
					t.Errorf("C0 and C1 of %d of %d voice frames exact", n, len(want))
				}
			})
		}
	}
}

func TestIQDiscDataBias(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.ColorCode, cfg.Wakeups, cfg.Slot, cfg.Preambles = 1, 0, 1, 2
	cfg.Message = "DMRGO SIMCMP CALL 11 THE QUICK BROWN FOX JUMPS OVER THE LAZY DOG 0123456789"
	built, err := rftx.Sequence("data1", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := map[[24]byte]bool{}
	wantN := 0
	for _, b := range built.Bursts {
		if x, err := layer2.NewBurstFromBytes(b); err == nil && x.Kind == layer2.KindData && x.SlotType.DataType == enums.DataTypeRate1 {
			p, _ := b.Rate1Payload()
			want[p] = true
			wantN++
		}
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	const calls = 4
	gap := make([]float64, int(0.3*basebandRate))
	fmAll, envAll := make([]float64, 0, calls*(len(gap)+len(fm))), make([]float64, 0, calls*(len(gap)+len(env)))
	for range calls {
		fmAll, envAll = append(fmAll, gap...), append(envAll, gap...)
		fmAll, envAll = append(fmAll, fm...), append(envAll, env...)
	}
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		t.Run(strconv.Itoa(int(tier-Tier0)), func(t *testing.T) {
			t.Parallel()
			x := msBaseband(fmAll, envAll, 0, 30, 7)
			r := mustNew(RxConfig{Rate: basebandRate, Tier: tier, Format: IQFormatCU8})
			var got []Burst
			var worst float32
			for k := 0; k < len(x); k += 960 {
				got = append(got, must(r.writeBB(x[k:min(k+960, len(x))]))...)
				worst = max(worst, abs32(r.host().iqd.conv.hz))
			}
			got = append(got, r.flushAll()...)
			if worst > iqAFCLockHz {
				t.Errorf("oscillator %.0f Hz off the carrier", worst)
			}
			n := 0
			for i := range got {
				d, _ := got[i].Decoded()
				x, err := layer2.NewBurstFromBytes(d)
				if err != nil || x.Kind != layer2.KindData || x.SlotType.DataType != enums.DataTypeRate1 {
					continue
				}
				if p, _ := d.Rate1Payload(); want[p] {
					n++
				}
			}
			if total := calls * wantN; n != total {
				t.Errorf("%d of %d rate 1 blocks exact", n, total)
			}
		})
	}
}

func TestIQDiscAdjacent(t *testing.T) {
	t.Parallel()
	for _, held := range []bool{false, true} {
		o := discOptions()
		o.IQDisc, o.IQHz = true, 12000
		if held {
			o.IQTuneMaxHz, o.IQTuneSyncOnly = 2500, true
		}
		n, want, off := adjacentReceive(t, Tier0, &o, 300)
		if n < want {
			t.Errorf("held %v: C0 and C1 of %d of %d voice frames exact", held, n, want)
		}
		switch {
		case held && off > iqAFCLockHz:
			t.Errorf("held: oscillator %.0f Hz off the carrier while locked", off)
		case !held && off <= iqAFCLockHz:
			t.Errorf("not held: oscillator at most %.0f Hz off the carrier; the test no longer shows the pull", off)
		}
	}
}

func TestTier3Adjacent(t *testing.T) {
	t.Parallel()
	for _, held := range []bool{false, true} {
		o := tierOptions(Tier3)
		if !held {
			o.IQTuneMaxHz, o.IQCutHz, o.IQKaiser = 0, 0, 0
		}
		n, want, _ := adjacentReceive(t, Tier3, &o, 0)
		switch {
		case held && n < want:
			t.Errorf("held: C0 and C1 of %d of %d voice frames exact", n, want)
		case !held && n == want:
			t.Errorf("not held: all %d voice frames exact; the test no longer shows the other channel", n)
		}
	}
}

func adjacentReceive(t *testing.T, tier Tier, o *options, offsetHz float64) (n, want int, off float32) {
	t.Helper()
	return adjacentReceiveSeed(t, tier, o, offsetHz, 7)
}

func adjacentReceiveSeed(t *testing.T, tier Tier, o *options, offsetHz float64, seed uint64) (n, want int, off float32) {
	t.Helper()
	cfg := testConfig()
	cfg.ColorCode, cfg.Superframes, cfg.Wakeups, cfg.Slot = 1, 2, 0, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	const calls = 3
	gap := make([]float64, int(0.3*basebandRate))
	var fmAll, envAll []float64
	frames := make([]uint64, 0, calls*18*cfg.Superframes)
	for c := range calls {
		if c > 0 {
			fmAll, envAll = append(fmAll, gap...), append(envAll, gap...)
		}
		fmAll, envAll = append(fmAll, fm...), append(envAll, env...)
		for i := range 18 * cfg.Superframes {
			frames = append(frames, rftx.AMBEValue(rftx.TestAMBEFrame(i)))
		}
	}
	ic := testConfig()
	ic.ColorCode, ic.Superframes, ic.Wakeups, ic.Slot, ic.BSSourced = 5, 4, 0, 0, true
	ib, err := rftx.Sequence("voice", ic)
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := rftx.ModulateBS(ib.Slots)
	x := msBaseband(fmAll, envAll, offsetHz, 20, seed)
	var ph float64
	for i := range x {
		ph += 2 * math.Pi * (fi[i%len(fi)] + 12500) / basebandRate
		x[i] += complex(float32(100*math.Cos(ph)), float32(100*math.Sin(ph)))
	}
	var out []Burst
	r := mustNewOpt(RxConfig{Rate: basebandRate, Tier: tier, OnBursts: each(func(b *Burst) { out = append(out, *b) }), Format: IQFormatCU8}, *o)
	for k := 0; k < len(x); k += 960 {
		must(r.writeBB(x[k:min(k+960, len(x))]))
		if r.host().iqd != nil && r.host().iqd.d.locked {
			off = max(off, abs32(r.host().iqd.conv.hz-float32(offsetHz)))
		}
	}
	r.flushAll()
	return c0c1InOrder(frames, voiceFrames(out)), len(frames), off
}

func iqDiscReceive(x []complex64, hz int) []Burst {
	o := discOptions()
	o.IQDisc, o.IQHz = true, hz
	var out []Burst
	r := mustNewOpt(RxConfig{Rate: basebandRate, Tier: Tier0, OnBursts: each(func(b *Burst) { out = append(out, *b) }), Format: IQFormatCU8}, o)
	for k := 0; k < len(x); k += 4096 {
		must(r.writeBB(x[k:min(k+4096, len(x))]))
	}
	r.flushAll()
	return out
}

func TestIQDiscLoopback(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 30, 3)
	for _, hz := range []int{discRate, discRate / 2} {
		sc := scoreBench(truth, iqDiscReceive(x, hz))
		for _, k := range []string{"ambe", "rate12", "rate34", "csbk", "emb", "slottype"} {
			if s := sc[k]; s.Truth-s.Exact > 3 {
				t.Errorf("%d Hz: %s %d of %d exact", hz, k, s.Exact, s.Truth)
			}
		}
	}
	o := discOptions()
	o.IQDisc = true
	r := mustNewOpt(RxConfig{Rate: basebandRate, Tier: Tier0, OnBursts: each(func(*Burst) {}), Format: IQFormatCU8}, o)
	must(r.writeBB(x))
	if got := float64(r.host().iqd.conv.hz); got < 1300 || got > 1500 {
		t.Errorf("oscillator at %.0f Hz, want about 1400", got)
	}
}

func TestIQSpecHunt(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 6)
	for _, off := range []float64{-6000, -5000, 5000, 6000} {
		x := noisyBaseband(slots, off, 10, 4)
		for _, tier := range []Tier{Tier0, Tier1, Tier2} {
			if exact, total := exactFrames(slots, receiveBaseband(x, tier)); exact < total*8/10 {
				t.Errorf("tier %d at %.0f Hz: %d of %d voice frames exact", tier-Tier0, off, exact, total)
			}
		}
	}
}

func TestReceiverOffsetHz(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 6)
	for _, c := range []struct{ off, set float64 }{{11400, 11000}, {-11000, -11300}} {
		x := noisyBaseband(slots, c.off, 15, 5)
		for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
			for _, set := range []float64{0, c.set} {
				r := mustNew(RxConfig{Tier: tier, OffsetHz: set, Rate: basebandRate, Format: IQFormatCU8})
				var got []Burst
				for k := 0; k < len(x); k += 4096 {
					got = append(got, must(r.writeBB(x[k:min(k+4096, len(x))]))...)
				}
				got = append(got, r.flushAll()...)
				exact, total := exactFrames(slots, got)
				switch {
				case set != 0 && exact < total*9/10:
					t.Errorf("tier %d at %.0f Hz, OffsetHz %.0f: %d of %d voice frames exact", tier-Tier0, c.off, set, exact, total)
				case set != 0 && math.Abs(r.OffsetHz()-c.off) > 100:
					t.Errorf("tier %d at %.0f Hz, OffsetHz %.0f: OffsetHz() %.0f", tier-Tier0, c.off, set, r.OffsetHz())
				case set == 0 && exact > total/10:
					t.Errorf("tier %d at %.0f Hz without OffsetHz: %d of %d voice frames exact; the test no longer shows the next channel", tier-Tier0, c.off, exact, total)
				}
			}
		}
	}
	y := lowIQ(noisyBaseband(slots, 1700, 25, 6), iq0LowHz)
	for _, set := range []float64{0, 1500} {
		var r Receiver
		var got []Burst
		if err := r.Init(RxConfig{OffsetHz: set, Rate: iq0LowHz, Format: IQFormatCF32, OnBursts: func(bs []Burst) { got = append(got, bs...) }}); err != nil {
			t.Fatal(err)
		}
		_, _ = r.Write(cf32(y))
		_ = r.Flush()
		if exact, total := exactFrames(slots, got); exact < total*9/10 {
			t.Errorf("LowIQReceiver at 1700 Hz, OffsetHz %.0f: %d of %d voice frames exact", set, exact, total)
		}
	}
	for _, c := range []RxConfig{{OffsetHz: 24000}, {OffsetHz: math.NaN()}, {Tier: Tier0, Rate: iq0LowHz, OffsetHz: -4800}} {
		if _, err := NewReceiver(&c); err == nil {
			t.Errorf("New took %+v", c)
		}
	}
}

func TestIQAFCAvgNoise(t *testing.T) {
	t.Parallel()
	for _, m := range []int{2, 3, 5} {
		o := tierOptions(Tier0)
		o.IQAFCAvg = m
		r := mustNewOpt(RxConfig{Tier: Tier0, Rate: basebandRate, Format: IQFormatCU8}, o)
		for seed := uint64(1); seed <= 20; seed++ {
			must(r.writeBB(noisyBaseband(nil, 0, 0, seed)))
		}
		if hz := r.host().iqd.conv.hz; hz != 0 {
			t.Errorf("IQAFCAvg %d: oscillator at %.0f Hz after noise alone", m, hz)
		}
	}
}

func TestIQAFCAvgChunks(t *testing.T) {
	t.Parallel()
	x := fuzzSamples(fuzzSeedBaseband(t), 12, false)
	for _, m := range []int{3, 5} {
		o := tierOptions(Tier0)
		o.IQAFCAvg = m
		run := func(seed uint64) []Burst {
			r := mustNewOpt(RxConfig{Tier: Tier0, Rate: basebandRate, Format: IQFormatCU8}, o)
			var out []Burst
			p := 0
			for _, k := range fuzzChunks(len(x), 1<<14, seed) {
				out = append(out, must(r.writeBB(x[p:p+k]))...)
				p += k
			}
			return append(out, r.flushAll()...)
		}
		a, b := run(0), run(1)
		if len(a) == 0 || len(a) != len(b) {
			t.Fatalf("IQAFCAvg %d: %d bursts in one chunk, %d in chunks", m, len(a), len(b))
		}
		for i := range a {
			if a[i] != b[i] {
				t.Errorf("IQAFCAvg %d: burst %d differs in chunks", m, i)
			}
		}
	}
}

func TestReceiverOffsetHzTransmitters(t *testing.T) {
	t.Parallel()
	const rate = 2400000
	bound := 1 / (8 * burstSeconds)
	slots, _ := benchStream(t, 2)
	bs := make([]Burst, len(slots))
	for i, s := range slots {
		bs[i] = Burst{Data: s.Burst, CACH: s.CACH, HasCACH: true, BS: true, Stream: 1, Slot: int64(i)}
	}
	cfg := testConfig()
	cfg.BSSourced = false
	built, err := rftx.Sequence("data12", cfg)
	if err != nil {
		t.Fatal(err)
	}
	ms := make([]Burst, len(built.Bursts))
	for i, raw := range built.Bursts {
		ms[i] = Burst{Data: raw, Stream: 1, Slot: 2 * int64(i)}
	}
	signal := func(src string, base bool, hz float64) []byte {
		if src == "rrc" {
			x := cleanBaseband(slots, hz).noisy(60, 1)
			if !base {
				f, e := rftx.Modulate(built.Bursts, rftx.Schedule{})
				x = noisyFM(f, e, hz, 60, 1)
			}
			return iqBytes(dgUpsample(x, rate), IQFormatCF32)
		}
		var out []byte
		c := usIQ(Tier1, rate, IQFormatCF32)
		if src == "tx2" {
			c.Tier = Tier2
		}
		c.OffsetHz, c.OnSamples = hz, func(b []byte) { out = append(out, b...) }
		tx, err := NewTransmitter(&c)
		if err != nil {
			t.Fatal(err)
		}
		in := ms
		if base {
			in = bs
		}
		if _, err := tx.Write(in); err != nil {
			t.Fatal(err)
		}
		_ = tx.Flush()
		pad := make([]byte, 8*rate/10)
		return append(append(pad, out...), pad...)
	}
	for _, src := range []string{"rrc", "tx1", "tx2"} {
		for _, base := range []bool{false, true} {
			for _, hz := range []float64{0, -4460, 3000} {
				b := signal(src, base, hz)
				r := mustNew(RxConfig{Tier: Tier3, Rate: rate, Format: IQFormatCF32})
				n := 0
				for k := 0; k < len(b); k += 1 << 16 {
					n += len(must(r.writeIQ(b[k:min(k+1<<16, len(b))])))
				}
				n += len(r.flushAll())
				if e := r.OffsetHz() - hz; n == 0 || math.Abs(e) > bound {
					t.Errorf("%s BS %v at %+g Hz: OffsetHz %+.1f Hz off with %d bursts, bound %.1f Hz", src, base, hz, e, n, bound)
				}
			}
		}
	}
}

func TestReceiverStartLevels(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
	built, err := rftx.Sequence("data12", cfg)
	if err != nil {
		t.Fatal(err)
	}
	slots, _ := benchStream(t, 1)
	bs := make([]layer2.RawBurst, len(slots))
	for i, s := range slots {
		bs[i] = s.Burst
	}
	devs := map[Tier][]float64{Tier0: {0.7, 0.85, 1, 1.15, 1.3}, Tier1: {1, 1.15, 1.3}, Tier2: {1}, Tier3: {0.85, 0.9, 0.925, 0.95, 1, 1.05, 1.1}}
	type job struct {
		ms   bool
		dev  float64
		lead float64
	}
	var jobs []job
	var all []float64
	for _, d := range devs {
		all = append(all, d...)
	}
	slices.Sort(all)
	for _, ms := range []bool{true, false} {
		for _, d := range slices.Compact(all) {
			for _, lead := range []float64{0, 0.1} {
				jobs = append(jobs, job{ms, d, lead})
			}
		}
	}
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var fm, env []float64
			want := built.Bursts
			if j.ms {
				fm, env = rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
			} else {
				want = bs
				fm, env = rftx.ModulateBS(slots)
			}
			n := int(j.lead * 2e6)
			f, e := make([]float64, n+len(fm)), make([]float64, n+len(fm))
			for i := range fm {
				f[n+i], e[n+i] = fm[i]*j.dev, env[i]
			}
			iq := rftx.Channel(rftx.ToIQ(f, e, 2e6, 0, 0.9), 2e6, 2.048e6, 3000, 25, 1)
			for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
				if !slices.Contains(devs[tier], j.dev) {
					continue
				}
				r := mustNew(RxConfig{Rate: 2048000, Format: IQFormatCU8, Tier: tier})
				var got []Burst
				for k := 0; k < len(iq); k += 1 << 16 {
					got = r.AppendBursts(got, iq[k:min(k+1<<16, len(iq))])
				}
				got = r.AppendFlush(got)
				for i, w := range want {
					if !slices.ContainsFunc(got, func(b Burst) bool { return b.Data == w }) {
						t.Errorf("Tier%d ms %v deviation %.2f lead %.1f s: burst %d missing or wrong", tier-Tier0, j.ms, j.dev, j.lead, i)
					}
				}
				if d := r.DeviationHz() / (3 * deviationHz * j.dev); math.Abs(d-1) > 0.05 {
					t.Errorf("Tier%d ms %v deviation %.2f lead %.1f s: DeviationHz %.3f of the true value", tier-Tier0, j.ms, j.dev, j.lead, d)
				}
			}
		}()
	}
	wg.Wait()
}

func TestReceiverClockStart(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE", Superframes: 2}
	type job struct {
		bs   bool
		ppm  float64
		lead float64
	}
	var jobs []job
	for _, bs := range []bool{false, true} {
		for _, ppm := range []float64{-500, -250, 250, 500} {
			for _, lead := range []float64{0, 0.1} {
				jobs = append(jobs, job{bs, ppm, lead})
			}
		}
	}
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := cfg
			c.BSSourced = j.bs
			built, err := rftx.Sequence("voice", c)
			if err != nil {
				t.Error(err)
				return
			}
			want := built.Bursts
			var fm, env []float64
			if j.bs {
				want = want[:0:0]
				for _, s := range built.Slots {
					want = append(want, s.Burst)
				}
				fm, env = rftx.ModulateBS(built.Slots)
			} else {
				fm, env = rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
			}
			n := int(j.lead * 2e6)
			f, e := make([]float64, n+len(fm)), make([]float64, n+len(fm))
			copy(f[n:], fm)
			copy(e[n:], env)
			iq := rftx.Channel(rftx.ToIQ(f, e, 2e6, 0, 0.9), 2e6/(1+j.ppm*1e-6), 2.048e6, 3000, 25, 5)
			r := mustNew(RxConfig{Rate: 2048000, Format: IQFormatCU8, Tier: Tier3})
			got := r.AppendFlush(r.AppendBursts(nil, iq))
			for i, w := range want {
				if !slices.ContainsFunc(got, func(b Burst) bool { return b.Data == w }) {
					t.Errorf("Tier3 bs %v %+g ppm lead %.1f s: burst %d missing or wrong", j.bs, j.ppm, j.lead, i)
				}
			}
		}()
	}
	wg.Wait()
}

func TestReceiverDeviationAfterSilence(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
	built, err := rftx.Sequence("data12", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	for _, c := range []struct {
		dev  float64
		seed uint64
	}{{1.10, 24}, {1.15, 5}} {
		n := int(0.1 * 2e6)
		f, e := make([]float64, n+len(fm)), make([]float64, n+len(fm))
		for i := range fm {
			f[n+i], e[n+i] = fm[i]*c.dev, env[i]
		}
		iq := rftx.Channel(rftx.ToIQ(f, e, 2e6, 0, 0.9), 2e6, 2.048e6, 3000, 25, c.seed)
		r := mustNew(RxConfig{Rate: 2048000, Format: IQFormatCU8, Tier: Tier3})
		got := r.AppendFlush(r.AppendBursts(nil, iq))
		if len(got) != len(built.Bursts) {
			t.Errorf("deviation %.2f seed %d: %d bursts out of a call of %d", c.dev, c.seed, len(got), len(built.Bursts))
		}
		for i, w := range built.Bursts {
			if !slices.ContainsFunc(got, func(b Burst) bool { return b.Data == w }) {
				t.Errorf("deviation %.2f seed %d: burst %d missing or wrong", c.dev, c.seed, i)
			}
		}
	}
}

func TestReceiverReadingsAfterLevelStep(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
	built, err := rftx.Sequence("data12", cfg)
	if err != nil {
		t.Fatal(err)
	}
	slots, _ := benchStream(t, 1)
	receive := func(tier Tier, iq []byte) (*Receiver, int) {
		r := mustNew(RxConfig{Rate: 2048000, Format: IQFormatCU8, Tier: tier})
		var got []Burst
		for k := 0; k < len(iq); k += 1 << 16 {
			got = r.AppendBursts(got, iq[k:min(k+1<<16, len(iq))])
		}
		return r, len(r.AppendFlush(got))
	}
	var wg sync.WaitGroup
	for _, ms := range []bool{true, false} {
		for _, tail := range []string{"no carrier", "unmodulated carrier"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var fm, env []float64
				if ms {
					fm, env = rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
				} else {
					fm, env = rftx.ModulateBS(slots)
				}
				n := len(fm)
				head := rftx.Channel(rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6, 2.048e6, 3000, 25, 1)
				fm, env = append(fm, make([]float64, 4*n)...), append(env, make([]float64, 4*n)...)
				if tail == "unmodulated carrier" {
					for i := n; i < len(env); i++ {
						env[i] = 1
					}
				}
				iq := rftx.Channel(rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6, 2.048e6, 3000, 25, 1)
				for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
					c, cn := receive(tier, head)
					r, rn := receive(tier, iq)
					if cn == 0 || rn != cn {
						t.Errorf("Tier%d ms %v, then %s: %d bursts, %d without the tail", tier-Tier0, ms, tail, rn, cn)
						continue
					}
					if d := r.DeviationHz() / (3 * deviationHz); math.Abs(d-1) > 0.05 {
						t.Errorf("Tier%d ms %v, then %s: DeviationHz %.3f of the true value", tier-Tier0, ms, tail, d)
					}
					bound := 0.5 / (iqUnitsPerHz * float64(r.full.(*rxHost).iqd.d.f.gain))
					if e := r.OffsetHz() - c.OffsetHz(); math.Abs(e) > bound {
						t.Errorf("Tier%d ms %v, then %s: OffsetHz moved %+.1f Hz, bound %.2f Hz", tier-Tier0, ms, tail, e, bound)
					}
				}
			}()
		}
	}
	wg.Wait()
}
