package rf

import (
	"encoding/json"
	"os"
	"runtime"
	"sync"
	"testing"
)

// sweepJob is the input of TestSweepWorker: one receiver variant and the
// cases to receive with it. cmd/rxsweep writes it.
type sweepJob struct {
	// Tier is the base tier, and Opt the JSON of the options fields that
	// replace the settings of the tier.
	Tier int             `json:"tier"`
	Opt  json.RawMessage `json:"opt,omitempty"`
	// Lookback is Config.LookbackSlots.
	Lookback int `json:"lookback"`
	// Hz is Config.BasebandHz: 48000, or 24000 for the mean of each pair
	// of samples, or with Disc DiscConfig.Hz. 0 means the default rate.
	Hz int `json:"hz,omitempty"`
	// Superframes is the length of the voice call of the stream.
	Superframes int `json:"superframes"`
	// Offset is the carrier offset in Hz.
	Offset float64 `json:"offset"`
	// Cases are the C/N and noise seeds to receive.
	Cases []sweepCase `json:"cases"`
	// Cost also measures the RAM, the allocations and the false bursts.
	Cost bool `json:"cost,omitempty"`
	// Disc receives the stream through discAudio with a DiscReceiver.
	Disc bool `json:"disc,omitempty"`
}

// sweepCase is one stream of a job.
type sweepCase struct {
	CN   float64 `json:"cn"`
	Seed uint64  `json:"seed"`
}

// sweepCaseResult is the score and CPU time of one case.
type sweepCaseResult struct {
	sweepCase
	Payloads map[string]*benchScore `json:"payloads"`
	// CPUSeconds is the thread CPU time of the receiver over the stream of
	// Seconds seconds.
	CPUSeconds float64 `json:"cpu_seconds"`
	Seconds    float64 `json:"seconds"`
}

// sweepCost is the cost of a variant apart from its CPU time.
type sweepCost struct {
	RAMBytes     int     `json:"ram_bytes"`
	Allocs       float64 `json:"allocs"`
	FalsePerHour float64 `json:"false_per_hour"`
	// SearchNsPerSecond is the CPU time per second of noise.
	SearchNsPerSecond float64 `json:"search_ns_per_second"`
}

// sweepResult is the output of TestSweepWorker.
type sweepResult struct {
	Cases []sweepCaseResult `json:"cases"`
	Cost  *sweepCost        `json:"cost,omitempty"`
}

// config returns the Config of the job.
func (j *sweepJob) config(t testing.TB) Config {
	t.Helper()
	o := tierOptions(Tier(j.Tier))
	if len(j.Opt) > 0 {
		if err := json.Unmarshal(j.Opt, &o); err != nil {
			t.Fatalf("opt: %v", err)
		}
	}
	hz := j.hz()
	if j.Disc {
		hz = basebandRate
	}
	return Config{Rate: basebandRate, Tier: Tier(j.Tier), LookbackSlots: j.Lookback, BasebandHz: hz, opt: &o}
}

func (j *sweepJob) hz() int {
	if j.Hz == 0 {
		return basebandRate
	}
	return j.Hz
}

// input returns x at the input rate of the job.
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

// sweepRx is the receiver of a job: a Receiver that takes baseband, or a
// DiscReceiver that takes its discAudio.
type sweepRx struct {
	r *Receiver
	d *DiscReceiver
}

// sweepInput is a stream converted for a sweepRx.
type sweepInput struct {
	x []complex64
	a []int16
}

// newRx returns the receiver of the job with the settings of cfg.
func (j *sweepJob) newRx(cfg Config) sweepRx {
	if j.Disc {
		d, err := NewDisc(DiscConfig{Hz: j.audioHz(), OnBurst: cfg.OnBurst, opt: cfg.opt})
		if err != nil {
			panic(err)
		}
		return sweepRx{d: d}
	}
	return sweepRx{r: mustNew(cfg)}
}

// convert returns the input of the receiver for the baseband x of the job.
func (s sweepRx) convert(j *sweepJob, x []complex64) sweepInput {
	if s.d != nil {
		return sweepInput{a: discAudio(x, j.audioHz())}
	}
	return sweepInput{x: j.input(x)}
}

// len returns the number of samples of in.
func (in sweepInput) len() int { return len(in.x) + len(in.a) }

// write feeds samples lo to hi of in to the receiver in writes of 4096.
func (s sweepRx) write(in sweepInput, lo, hi int) {
	for k := lo; k < hi; k += 4096 {
		e := min(k+4096, hi)
		if s.d != nil {
			s.d.Write(in.a[k:e])
		} else {
			s.r.WriteBaseband(in.x[k:e])
		}
	}
}

func (s sweepRx) flush() {
	if s.d != nil {
		s.d.Flush()
	} else {
		s.r.Flush()
	}
}

// rate returns the sample rate of the input of the receiver of the job.
func (j *sweepJob) rate() int {
	if j.Disc {
		return j.audioHz()
	}
	return j.hz()
}

// audioHz returns the sample rate of the discriminator samples of a Disc
// job: Hz, or 24000 when it is 0.
func (j *sweepJob) audioHz() int {
	if j.Hz == 0 {
		return discRate
	}
	return j.Hz
}

// receive runs the baseband x through the receiver of the job and returns
// the bursts, the thread CPU time and the input seconds.
func (j *sweepJob) receive(cfg Config, x []complex64) ([]Burst, float64, float64) {
	var out []Burst
	cfg.OnBurst = func(b Burst) { out = append(out, b) }
	r := j.newRx(cfg)
	in := r.convert(j, x)
	cpu := threadCPUSeconds(func() {
		r.write(in, 0, in.len())
		r.flush()
	})
	return out, cpu, float64(in.len()) / float64(j.rate())
}

// TestSweepWorker runs the job in the file DMRRF_SWEEP_JOB and writes the
// result to DMRRF_SWEEP_OUT. cmd/rxsweep runs it.
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
	cfg := j.config(t)
	if _, err := New(cfg); err != nil {
		t.Fatal(err)
	}
	var res sweepResult
	if j.Cost {
		res.Cost = sweepCosts(t, &j, cfg)
	}
	slots, truth := benchStream(t, j.Superframes)
	res.Cases = make([]sweepCaseResult, len(j.Cases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for i, c := range j.Cases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			got, cpu, sec := j.receive(cfg, noisyBaseband(slots, j.Offset, c.CN, c.Seed))
			res.Cases[i] = sweepCaseResult{sweepCase: c, Payloads: scoreBench(truth, got), CPUSeconds: cpu, Seconds: sec}
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

// sweepCosts measures the RAM of the receiver of the job (the bytes that
// its constructor allocates), its allocations per write on a locked stream
// at 10 dB, and its false bursts and CPU time on 5 minutes of noise. It
// runs before anything else, alone.
func sweepCosts(t testing.TB, j *sweepJob, cfg Config) *sweepCost {
	var c sweepCost
	var a, b runtime.MemStats
	count := 0
	cfg.OnBurst = func(Burst) { count++ }
	runtime.ReadMemStats(&a)
	r := j.newRx(cfg)
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
	ar := j.newRx(cfg)
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
