//go:build !tinygo

package fsktx

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

// sweepResult is the measurement of one modulator.
type sweepResult struct {
	Name        string    `json:"name"`
	Spec        spec      `json:"spec"`
	NsPerSecond float64   `json:"ns_per_second"`
	Tables      int       `json:"tables_bytes"`
	State       int       `json:"state_bytes"`
	ACP         float64   `json:"acp_dbc"`
	ALT         float64   `json:"alt_dbc"`
	Mask        float64   `json:"mask_margin_db"`
	FSK         fskResult `json:"fsk"`
	VoiceCN90   float64   `json:"voice_cn90"`
	DataCN90    float64   `json:"data_cn90"`
	VoicePen    float64   `json:"voice_penalty_db"`
	DataPen     float64   `json:"data_penalty_db"`
	Points      []point   `json:"points"`
	// The same for the discriminator receiver.
	DiscVoiceCN90 float64 `json:"disc_voice_cn90"`
	DiscDataCN90  float64 `json:"disc_data_cn90"`
	DiscVoicePen  float64 `json:"disc_voice_penalty_db"`
	DiscDataPen   float64 `json:"disc_data_penalty_db"`
	DiscPoints    []point `json:"disc_points"`
	ETSIDev       float64 `json:"etsi_dev_pct"`
	Peak          int     `json:"peak"`
	// PenaltyFrom names the modulator whose receiver runs the result shares.
	PenaltyFrom string `json:"penalty_from,omitempty"`
	// IdealACP and IdealFSK are ACP and FSK of an audio modulator with an
	// ideal DAC, which reconstructs the samples without hold or images.
	IdealACP  float64   `json:"ideal_acp_dbc,omitempty"`
	IdealMask float64   `json:"ideal_mask_margin_db,omitempty"`
	IdealFSK  fskResult `json:"ideal_fsk"`
}

// point is the recovery at one C/N.
type point struct {
	CN      float64 `json:"cn"`
	Voice   int     `json:"voice"`
	VoiceOK int     `json:"voice_ok"`
	Data    int     `json:"data"`
	DataOK  int     `json:"data_ok"`
}

// sweepSeeds is the number of noise seeds at each C/N point.
const sweepSeeds = 16

// output runs the modulator of s over the symbols and returns its signal
// at its analysis rate, and the largest sample magnitude.
func output(s spec, syms []int8, r radio) ([]complex128, float64, int) {
	peak := 0
	if s.Kind == kindAudio {
		k, hz := s.shaper()
		a := make([]int16, 0, len(syms)*hz/SymbolRate+16)
		for _, v := range syms {
			a = k.appendSymbol(a, v)
		}
		for _, v := range a {
			peak = max(peak, int(v), -int(v))
		}
		return dacFM(a, hz, r), dacRate, peak
	}
	p := s.iq()
	b := make([]int16, 0, 2*len(syms)*s.Rate/SymbolRate+16)
	for _, v := range syms {
		b = p.appendSymbol(b, v)
	}
	return fromIQ(b), float64(s.Rate), 0
}

// etsiSymbols returns the deviation test pattern of ETSI TS 102 361-1
// clause 10.2.2.3, +3 +3 -3 -3 repeated.
func etsiSymbols() []int8 {
	out := make([]int8, 4000)
	for i := range out {
		out[i] = 3
		if i%4 >= 2 {
			out[i] = -3
		}
	}
	return out
}

// etsiSignal returns the signal of s for the deviation test at rxRate.
func etsiSignal(s spec, r radio) []complex128 {
	x, rate, _ := output(s, etsiSymbols(), r)
	return resample(x, int(rate), rxRate)
}

// cpuNs returns the least CPU time over five runs that s takes to
// modulate ten seconds of random symbols, per second.
func cpuNs(s spec) float64 {
	rng := rand.New(rand.NewPCG(5, 6))
	syms := make([]int8, 10*SymbolRate)
	for i := range syms {
		syms[i] = int8(2*rng.IntN(4) - 3)
	}
	best := math.Inf(1)
	for range 5 {
		var ns float64
		if s.Kind == kindAudio {
			k, _ := s.shaper()
			dst := make([]int16, 0, 64)
			ns = threadCPU(func() {
				for _, v := range syms {
					dst = k.appendSymbol(dst[:0], v)
				}
			})
		} else {
			p := s.iq()
			dst := make([]int16, 0, 2048)
			ns = threadCPU(func() {
				for _, v := range syms {
					dst = p.appendSymbol(dst[:0], v)
				}
			})
		}
		best = min(best, ns/10)
	}
	return best
}

// penaltyJob receives one noisy run.
type penaltyJob struct {
	x     []complex128
	shift int
	disc  bool
	cn    float64
	seed  uint64
	out   *score
	wg    *sync.WaitGroup
}

// sweepRunner runs the receivers on a pool of goroutines.
type sweepRunner struct {
	jobs chan penaltyJob
	ts   *testStream
	n    int
}

func newSweepRunner(ts *testStream, n int) *sweepRunner {
	r := &sweepRunner{jobs: make(chan penaltyJob), ts: ts, n: n}
	for range runtime.NumCPU() {
		go func() {
			for j := range r.jobs {
				x := noisy(j.x, j.shift, n, j.cn, j.seed)
				if j.disc {
					*j.out = ts.score(receiveDisc(x), rxPad)
				} else {
					*j.out = ts.score(receive(x, rf.Tier3), rxPad)
				}
				j.wg.Done()
			}
		}()
	}
	return r
}

// curve measures the recovery of the aligned signal x from C/N lo upwards
// in steps of 0.5 dB until both payloads recover 98.5%, the top of the
// fit of probit, with Tier3 or with the discriminator receiver.
func (r *sweepRunner) curve(x []complex128, shift int, lo float64, disc bool) []point {
	var pts []point
	for cn := lo; cn <= 14; cn += 0.5 {
		sc := make([]score, sweepSeeds)
		var wg sync.WaitGroup
		for i := range sc {
			wg.Add(1)
			r.jobs <- penaltyJob{x: x, shift: shift, disc: disc, cn: cn, seed: uint64(100 + i), out: &sc[i], wg: &wg}
		}
		wg.Wait()
		p := point{CN: cn}
		for _, s := range sc {
			p.Voice += s.voice
			p.VoiceOK += s.voiceOK
			p.Data += s.data
			p.DataOK += s.dataOK
		}
		pts = append(pts, p)
		if float64(p.VoiceOK) >= 0.985*float64(p.Voice) && float64(p.DataOK) >= 0.985*float64(p.Data) {
			break
		}
	}
	return pts
}

// cn90 returns the C/N of 90% recovery of the voice frames and the data
// blocks of pts.
func cn90(pts []point) (voice, data float64) {
	cns := make([]float64, len(pts))
	vo, vn, do, dn := make([]int, len(pts)), make([]int, len(pts)), make([]int, len(pts)), make([]int, len(pts))
	for i, p := range pts {
		cns[i] = p.CN
		vo[i], vn[i], do[i], dn[i] = p.VoiceOK, p.Voice, p.DataOK, p.Data
	}
	return probit(cns, vo, vn), probit(cns, do, dn)
}

// measure returns the RF metrics of s.
func measure(s spec, ts *testStream, run *sweepRunner, ref sweepResult, r radio, pc *penaltyCache) sweepResult {
	res := sweepResult{Name: s.name(), Spec: s}
	res.Tables, res.State = s.memory()
	x, rate, peak := output(s, ts.syms, r)
	res.Peak = peak
	sp := newSpectrum(x[len(x)/8:len(x)*7/8], rate)
	res.ACP, res.ALT, res.Mask = sp.acp(12500), sp.acp(25000), sp.maskMargin()
	x48 := resample(x, int(rate), rxRate)
	res.FSK = fsk(x48, ts.syms)
	res.ETSIDev = etsiDeviation(etsiSignal(s, r))
	if s.Kind == kindAudio {
		xi, _, _ := output(s, ts.syms, radio{ideal: true})
		spi := newSpectrum(xi[len(xi)/8:len(xi)*7/8], dacRate)
		res.IdealACP, res.IdealMask = spi.acp(12500), spi.maskMargin()
		res.IdealFSK = fsk(resample(xi, dacRate, rxRate), ts.syms)
	}
	if run == nil {
		return res
	}
	lo := -1.0
	if !math.IsNaN(ref.VoiceCN90) {
		lo = math.Floor(min(ref.VoiceCN90, ref.DataCN90)*2)/2 - 1
	}
	shift := int(math.Round(res.FSK.Delay))
	e := pc.entry(s.rfKey())
	e.once.Do(func() {
		e.from = res.Name
		e.points = run.curve(x48, shift, lo, false)
		lo := math.Floor(min(ref.DiscVoiceCN90, ref.DiscDataCN90)*2)/2 - 1
		e.disc = run.curve(x48, shift, lo, true)
	})
	res.PenaltyFrom = e.from
	res.Points = e.points
	res.VoiceCN90, res.DataCN90 = cn90(res.Points)
	res.VoicePen, res.DataPen = res.VoiceCN90-ref.VoiceCN90, res.DataCN90-ref.DataCN90
	res.DiscPoints = e.disc
	res.DiscVoiceCN90, res.DiscDataCN90 = cn90(res.DiscPoints)
	res.DiscVoicePen, res.DiscDataPen = res.DiscVoiceCN90-ref.DiscVoiceCN90, res.DiscDataCN90-ref.DiscDataCN90
	return res
}

// penaltyCache holds the receiver runs of the modulators by rfKey, so that
// modulators that differ only in how they compute the same samples, such
// as the groups of a lookup table, share one run.
type penaltyCache struct {
	mu sync.Mutex
	m  map[string]*penaltyEntry
}

type penaltyEntry struct {
	once         sync.Once
	from         string
	points, disc []point
}

func (c *penaltyCache) entry(key string) *penaltyEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]*penaltyEntry{}
	}
	e := c.m[key]
	if e == nil {
		e = &penaltyEntry{}
		c.m[key] = e
	}
	return e
}

// TestTxSweep measures every modulator of sweepSpecs when DMRGO_TXSWEEP
// names an output directory, and writes sweep.json there.
// DMRGO_TXSWEEP_MATCH selects modulators by a regular expression of their
// names, DMRGO_TXSWEEP_SKIP names a sweep.json whose modulators it
// skips, DMRGO_TXSWEEP_NORX=1 skips the receiver runs, and
// DMRGO_TXSWEEP_CPUONLY=1 measures only the CPU time, into cpu.json.
func TestTxSweep(t *testing.T) {
	t.Parallel()
	dir := os.Getenv("DMRGO_TXSWEEP")
	if dir == "" {
		t.Skip("set DMRGO_TXSWEEP to an output directory")
	}
	match := regexp.MustCompile(os.Getenv("DMRGO_TXSWEEP_MATCH"))
	ts := newTestStream(t, 6, 40)
	n := int((2*rxPad + float64(len(ts.syms))/SymbolRate) * rxRate)
	var run *sweepRunner
	if os.Getenv("DMRGO_TXSWEEP_NORX") == "" {
		run = newSweepRunner(ts, n)
	}
	skip := map[string]bool{}
	if f := os.Getenv("DMRGO_TXSWEEP_SKIP"); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var old struct {
			Variants []sweepResult `json:"variants"`
		}
		if err := json.Unmarshal(b, &old); err != nil {
			t.Fatal(err)
		}
		for _, v := range old.Variants {
			skip[v.Name] = true
		}
	}
	var specs []spec
	for _, s := range sweepSpecs() {
		if match.MatchString(s.name()) && !skip[s.name()] {
			specs = append(specs, s)
		}
	}
	cpu := make([]float64, len(specs))
	for i, s := range specs {
		cpu[i] = cpuNs(s)
	}
	if os.Getenv("DMRGO_TXSWEEP_CPUONLY") != "" {
		m := map[string]float64{}
		for i, s := range specs {
			m[s.name()] = cpu[i]
		}
		b, err := json.MarshalIndent(m, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "cpu.json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	refFreq := refDeviation(ts.syms)
	refX := fm(refFreq)
	ref := sweepResult{Name: "reference"}
	sp := newSpectrum(refX[len(refX)/8:len(refX)*7/8], dacRate)
	ref.ACP, ref.ALT, ref.Mask = sp.acp(12500), sp.acp(25000), sp.maskMargin()
	ref48 := resample(refX, dacRate, rxRate)
	ref.FSK = fsk(ref48, ts.syms)
	ref.VoiceCN90, ref.DataCN90 = math.NaN(), math.NaN()
	ref.ETSIDev = etsiDeviation(resample(fm(refDeviation(etsiSymbols())), dacRate, rxRate))
	if run != nil {
		ref.Points = run.curve(ref48, int(math.Round(ref.FSK.Delay)), -1.5, false)
		ref.VoiceCN90, ref.DataCN90 = cn90(ref.Points)
		ref.DiscPoints = run.curve(ref48, int(math.Round(ref.FSK.Delay)), 3, true)
		ref.DiscVoiceCN90, ref.DiscDataCN90 = cn90(ref.DiscPoints)
	}
	t.Logf("reference: %+v", ref)
	{
		r := ref
		clean(&r)
		if b, err := json.Marshal(r); err == nil {
			_ = os.WriteFile(filepath.Join(dir, "reference.json"), b, 0o600)
		}
	}
	res := make([]sweepResult, len(specs))
	var wg sync.WaitGroup
	var pc penaltyCache
	var mu sync.Mutex
	f, err := os.OpenFile(filepath.Join(dir, "results.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sem := make(chan struct{}, 8)
	for i, s := range specs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res[i] = measure(s, ts, run, ref, radio{}, &pc)
			res[i].NsPerSecond = cpu[i]
			r := res[i]
			clean(&r)
			mu.Lock()
			if b, err := json.Marshal(r); err == nil {
				_, _ = f.Write(append(b, '\n'))
			}
			mu.Unlock()
			t.Logf("%-60s cpu %10.0f ns/s acp %6.1f alt %6.1f mask %6.1f fsk %5.2f%% dev %+5.2f%% etsi %+5.1f%% pen %5.2f %5.2f disc %5.2f %5.2f", r.Name, r.NsPerSecond, r.ACP, r.ALT, r.Mask, r.FSK.FSKError, r.FSK.DevError, r.ETSIDev, r.VoicePen, r.DataPen, r.DiscVoicePen, r.DiscDataPen)
		}()
	}
	wg.Wait()
	clean(&ref)
	for i := range res {
		clean(&res[i])
	}
	if b, err := json.Marshal(ref); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "reference.json"), b, 0o600)
	}
	b, err := json.MarshalIndent(map[string]any{"reference": ref, "variants": res}, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sweep.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// clean replaces the NaN values of r, which JSON cannot hold, by -999.
func clean(r *sweepResult) {
	for _, p := range []*float64{&r.ACP, &r.ALT, &r.Mask, &r.VoiceCN90, &r.DataCN90, &r.VoicePen, &r.DataPen, &r.DiscVoiceCN90, &r.DiscDataCN90, &r.DiscVoicePen, &r.DiscDataPen} {
		if math.IsNaN(*p) || math.IsInf(*p, 0) {
			*p = -999
		}
	}
}
