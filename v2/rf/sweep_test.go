//go:build !tinygo

package rf

import (
	"bytes"
	"encoding/json"
	"os"
	"runtime"
	"slices"
	"sync"
	"testing"
)

type sweepJob struct {
	Tier        int             `json:"tier"`
	Opt         json.RawMessage `json:"opt,omitempty"`
	Lookback    int             `json:"lookback"`
	Hz          int             `json:"hz,omitempty"`
	Superframes int             `json:"superframes"`
	Offset      float64         `json:"offset"`
	Cases       []sweepCase     `json:"cases"`
	Cost        bool            `json:"cost,omitempty"`
	Disc        bool            `json:"disc,omitempty"`
	Stream      string          `json:"stream,omitempty"`
	Radio       string          `json:"radio,omitempty"`
	PreEmphasis bool            `json:"pre_emphasis,omitempty"`
	Public      bool            `json:"public,omitempty"`
}

type sweepCase struct {
	CN   float64 `json:"cn"`
	Seed uint64  `json:"seed"`
}

type sweepCaseResult struct {
	sweepCase
	Payloads   map[string]*benchScore `json:"payloads"`
	CPUSeconds float64                `json:"cpu_seconds"`
	Seconds    float64                `json:"seconds"`
}

type sweepCost struct {
	RAMBytes          int     `json:"ram_bytes"`
	Allocs            float64 `json:"allocs"`
	FalsePerHour      float64 `json:"false_per_hour"`
	SearchNsPerSecond float64 `json:"search_ns_per_second"`
}

type sweepResult struct {
	Cases []sweepCaseResult `json:"cases"`
	Cost  *sweepCost        `json:"cost,omitempty"`
}

func (j *sweepJob) options() (options, error) {
	tier := Tier0 + Tier(j.Tier)
	var o options
	switch {
	case j.Disc:
		o = discTierOptions(tier)
	case j.Tier > 0:
		o = tierOptions(tier)
		if tier == Tier1 || tier == Tier2 {
			o.ExtLookback = 8
		}
	default:
		o = discOptions()
	}
	if len(j.Opt) > 0 {
		d := json.NewDecoder(bytes.NewReader(j.Opt))
		d.DisallowUnknownFields()
		if err := d.Decode(&o); err != nil {
			return o, err
		}
	}
	return o, nil
}

func (j *sweepJob) config(t testing.TB) (RxConfig, *options) {
	t.Helper()
	tier := Tier0 + Tier(j.Tier)
	o, err := j.options()
	if err != nil {
		t.Fatalf("opt: %v", err)
	}
	hz := j.hz()
	if j.Disc {
		hz = basebandRate
	}
	return RxConfig{Rate: hz, Format: IQFormatCF32, Tier: tier, LookbackSlots: j.Lookback}, &o
}

func (j *sweepJob) hz() int {
	if j.Hz == 0 {
		return basebandRate
	}
	return j.Hz
}

func (j *sweepJob) input(x []complex64) []complex64 {
	if j.hz() == basebandRate {
		return x
	}
	y := make([]complex64, len(x)/2)
	for i := range y {
		y[i] = (x[2*i] + x[2*i+1]) / 2
	}
	return y
}

type sweepRx struct {
	r *Receiver
	d *AudioReceiver
}

type sweepInput struct {
	x []complex64
	a []int16
}

func (j *sweepJob) newRx(cfg RxConfig, o *options) sweepRx {
	if j.Disc {
		c := AudioRxConfig{Rate: j.audioHz(), Tier: cfg.Tier, OnBursts: cfg.OnBursts}
		if j.PreEmphasis {
			c.PreEmphasis = NewRxPreEmphasis()
		}
		if j.Public {
			d, err := NewAudioReceiver(&c)
			if err != nil {
				panic(err)
			}
			return sweepRx{d: d}
		}
		d, err := newAudioReceiver(c, o)
		if err != nil {
			panic(err)
		}
		if o.SoftExt {
			if err := d.addSoft(o); err != nil {
				panic(err)
			}
			d.attachFix(c.Tier != Tier0)
		}
		return sweepRx{d: d}
	}
	return sweepRx{r: mustNewOpt(cfg, *o)}
}

func (s sweepRx) convert(j *sweepJob, x []complex64) sweepInput {
	if s.d != nil {
		return sweepInput{a: radioAudio(x, j.audioHz(), j.Radio == "deemph")}
	}
	return sweepInput{x: j.input(x)}
}

func (in sweepInput) len() int { return len(in.x) + len(in.a) }

func (s sweepRx) write(in sweepInput, lo, hi int) {
	for k := lo; k < hi; k += 4096 {
		e := min(k+4096, hi)
		if s.d != nil {
			must(s.d.writeAll(in.a[k:e]))
		} else {
			must(s.r.writeBB(in.x[k:e]))
		}
	}
}

func (s sweepRx) flush() {
	if s.d != nil {
		s.d.flushAll()
	} else {
		s.r.flushAll()
	}
}

func (j *sweepJob) rate() int {
	if j.Disc {
		return j.audioHz()
	}
	return j.hz()
}

func (j *sweepJob) audioHz() int {
	if j.Hz == 0 {
		return discRate
	}
	return j.Hz
}

func (j *sweepJob) receive(cfg RxConfig, o *options, x []complex64) ([]Burst, float64, float64) {
	var out []Burst
	cfg.OnBursts = func(bs []Burst) { out = append(out, bs...) }
	r := j.newRx(cfg, o)
	in := r.convert(j, x)
	cpu := threadCPUSeconds(func() {
		r.write(in, 0, in.len())
		r.flush()
	})
	return out, cpu, float64(in.len()) / float64(j.rate())
}

func TestSweepWorker(t *testing.T) {
	t.Parallel()
	path := os.Getenv("DMRRF_SWEEP_JOB")
	if path == "" {
		t.Skip("cmd/rxsweep sets DMRRF_SWEEP_JOB")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var j sweepJob
	if err := json.Unmarshal(b, &j); err != nil {
		t.Fatal(err)
	}
	cfg, o := j.config(t)
	if j.Disc {
		if _, err := newAudioReceiver(AudioRxConfig{Rate: j.audioHz(), Tier: cfg.Tier}, o); err != nil {
			t.Fatal(err)
		}
	} else if _, err := newReceiver(cfg, o); err != nil {
		t.Fatal(err)
	}
	var res sweepResult
	if j.Cost {
		res.Cost = sweepCosts(t, &j, cfg, o)
	}
	slots, truth := benchStream(t, j.Superframes)
	var fm, env []float64
	if j.Stream == "ms" {
		fm, env, truth = msStream(t, max(j.Superframes/3, 1), 0)
	}
	res.Cases = make([]sweepCaseResult, len(j.Cases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for i, c := range j.Cases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var x []complex64
			if fm != nil {
				x = noisyFM(fm, env, j.Offset, c.CN, c.Seed)
			} else {
				x = noisyBaseband(slots, j.Offset, c.CN, c.Seed)
			}
			got, cpu, sec := j.receive(cfg, o, x)
			sc := scoreBench(truth, got)
			if fm != nil {
				sc = scoreMS(truth, got)
			}
			res.Cases[i] = sweepCaseResult{sweepCase: c, Payloads: sc, CPUSeconds: cpu, Seconds: sec}
		}()
	}
	wg.Wait()
	b, err = json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("DMRRF_SWEEP_OUT"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSweepAudioShipped(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 2)
	x := noisyBaseband(slots, 1400, 8, 3)
	for _, hz := range []int{9600, 48000} {
		for tier := Tier0; tier <= Tier3; tier++ {
			j := sweepJob{Tier: int(tier - Tier0), Disc: true, Hz: hz}
			cfg, o := j.config(t)
			got, _, _ := j.receive(cfg, o, x)
			var want []Burst
			r, err := NewAudioReceiver(&AudioRxConfig{Rate: hz, Tier: tier, OnBursts: func(bs []Burst) { want = append(want, bs...) }})
			if err != nil {
				t.Fatal(err)
			}
			a := radioAudio(x, hz, false)
			for k := 0; k < len(a); k += 4096 {
				must(r.writeAll(a[k:min(k+4096, len(a))]))
			}
			r.flushAll()
			if len(want) == 0 || !slices.EqualFunc(got, want, func(a, b Burst) bool { return a == b }) {
				t.Errorf("%d Hz tier %d: sweep job gives %d bursts, NewAudioReceiver %d, or they differ", hz, tier-Tier0, len(got), len(want))
			}
		}
	}
}

func TestSweepRejectsUnknownOption(t *testing.T) {
	t.Parallel()
	for _, opt := range []string{`{"CohSPS": 2}`, `{"NoSuchKnob": true}`} {
		j := sweepJob{Disc: true, Hz: 9600, Opt: json.RawMessage(opt)}
		if _, err := j.options(); err == nil {
			t.Errorf("opt %s: no error", opt)
		}
	}
	j := sweepJob{Disc: true, Hz: 9600, Opt: json.RawMessage(`{"SoftExt": true}`)}
	if o, err := j.options(); err != nil || !o.SoftExt {
		t.Errorf("known opt: %v, SoftExt %v", err, o.SoftExt)
	}
}

func sweepCosts(t testing.TB, j *sweepJob, cfg RxConfig, o *options) *sweepCost {
	var c sweepCost
	var a, b runtime.MemStats
	count := 0
	cfg.OnBursts = func(bs []Burst) { count += len(bs) }
	runtime.ReadMemStats(&a)
	r := j.newRx(cfg, o)
	runtime.ReadMemStats(&b)
	c.RAMBytes = int(b.TotalAlloc - a.TotalAlloc)
	const seconds = 300
	n := 0
	var cpu float64
	for seed := uint64(1); n < seconds*j.rate(); seed++ {
		in := r.convert(j, noisyBaseband(nil, 0, 0, seed))
		cpu += threadCPUSeconds(func() { r.write(in, 0, in.len()) })
		n += in.len()
	}
	r.flush()
	c.FalsePerHour = float64(count) * 3600 * float64(j.rate()) / float64(n)
	c.SearchNsPerSecond = cpu * 1e9 * float64(j.rate()) / float64(n)
	slots, _ := benchStream(t, 6)
	ar := j.newRx(cfg, o)
	in := ar.convert(j, noisyBaseband(slots, j.Offset, 10, 1))
	const chunk = 1 << 12
	ar.write(in, 0, chunk)
	k := in.len()/chunk - 1
	runtime.ReadMemStats(&a)
	for i := 1; i <= k; i++ {
		ar.write(in, i*chunk, (i+1)*chunk)
	}
	runtime.ReadMemStats(&b)
	c.Allocs = float64(b.Mallocs-a.Mallocs) / float64(k)
	return &c
}
