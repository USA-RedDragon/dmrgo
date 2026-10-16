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
	// of samples. 0 means 48000.
	Hz int `json:"hz,omitempty"`
	// Superframes is the length of the voice call of the stream.
	Superframes int `json:"superframes"`
	// Offset is the carrier offset in Hz.
	Offset float64 `json:"offset"`
	// Cases are the C/N and noise seeds to receive.
	Cases []sweepCase `json:"cases"`
	// Cost also measures the RAM, the allocations and the false bursts.
	Cost bool `json:"cost,omitempty"`
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
	return Config{Rate: basebandRate, Tier: Tier(j.Tier), LookbackSlots: j.Lookback, BasebandHz: j.hz(), opt: &o}
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

// sweepReceive runs x through a receiver of cfg and returns the bursts and the
// thread CPU time.
func sweepReceive(cfg Config, x []complex64) ([]Burst, float64) {
	var out []Burst
	cfg.OnBurst = func(b Burst) { out = append(out, b) }
	r := mustNew(cfg)
	cpu := threadCPUSeconds(func() {
		for k := 0; k < len(x); k += 4096 {
			r.WriteBaseband(x[k:min(k+4096, len(x))])
		}
		r.Flush()
	})
	return out, cpu
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
			x := j.input(noisyBaseband(slots, j.Offset, c.CN, c.Seed))
			got, cpu := sweepReceive(cfg, x)
			res.Cases[i] = sweepCaseResult{sweepCase: c, Payloads: scoreBench(truth, got), CPUSeconds: cpu, Seconds: float64(len(x)) / float64(j.hz())}
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

// sweepCosts measures the RAM of cfg (the bytes that New allocates), its allocations per write on a
// locked stream at 10 dB, and its false bursts and CPU time on 5 minutes
// of noise. It runs before anything else, alone.
func sweepCosts(t testing.TB, j *sweepJob, cfg Config) *sweepCost {
	var c sweepCost
	var a, b runtime.MemStats
	count := 0
	cfg.OnBurst = func(Burst) { count++ }
	runtime.ReadMemStats(&a)
	r := mustNew(cfg)
	runtime.ReadMemStats(&b)
	c.RAMBytes = int(b.TotalAlloc - a.TotalAlloc)
	const seconds = 300
	n := 0
	var cpu float64
	for seed := uint64(1); n < seconds*j.hz(); seed++ {
		x := j.input(noisyBaseband(nil, 0, 0, seed))
		cpu += threadCPUSeconds(func() {
			for k := 0; k < len(x); k += 4096 {
				r.WriteBaseband(x[k:min(k+4096, len(x))])
			}
		})
		n += len(x)
	}
	r.Flush()
	c.FalsePerHour = float64(count) * 3600 * float64(j.hz()) / float64(n)
	c.SearchNsPerSecond = cpu * 1e9 * float64(j.hz()) / float64(n)
	slots, _ := benchStream(t, 6)
	x := j.input(noisyBaseband(slots, j.Offset, 10, 1))
	const chunk = 1 << 12
	ar := mustNew(cfg)
	ar.WriteBaseband(x[:chunk])
	k := len(x)/chunk - 1
	runtime.ReadMemStats(&a)
	for i := 1; i <= k; i++ {
		ar.WriteBaseband(x[i*chunk : (i+1)*chunk])
	}
	runtime.ReadMemStats(&b)
	c.Allocs = float64(b.Mallocs-a.Mallocs) / float64(k)
	return &c
}
