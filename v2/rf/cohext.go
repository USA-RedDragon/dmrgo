package rf

import (
	"math"
	"math/cmplx"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const (
	cohSPS   = discRate / symbolRate
	cohHist  = 1 << 13
	cohKnown = 4
	cohLook  = 8
)

type cohStage struct {
	src                         cohRing
	ph                          *discPhase
	trial                       float64
	trialing                    bool
	amp, noise                  [cohFitScales]float64
	easySNR                     float64
	skipSNR                     float64
	locked                      bool
	model                       *cpmModel
	tr                          *trellis
	ex                          *exactStage
	clicks                      bool
	mu                          float32
	llrK                        float32
	buf                         []complex64
	runs                        [cohRuns]cohRunCorr
	runAt                       int
	gen                         int
	symX                        []complex64
	symAt                       []uint32
	symStamp                    uint32
	symKey                      cohSymKey
	known                       [slotSymbols]int8
	isK                         [slotSymbols]bool
	app                         [slotSymbols][4]float32
	dist                        [slotSymbols]int
	g0                          int
	dir                         int
	steps                       int
	t0                          float64
	dt                          float64
	relock                      bool
	budget                      float64
	voiceOnly                   bool
	credit                      float64
	budgetSlot                  int64
	work                        int64
	fitOnly                     bool
	noNext, noCross, crossVoice bool
	verifyScan                  float64
	totalHz                     float64
	freq                        float64
	freqOK                      bool
	nFit                        int
	fitVar                      float64
	scale                       float32
	ampAvg                      float64
	noiseAvg                    float64
	symAvg                      float64
	lastSym                     float64
	easy                        bool
	symNoise                    uint8
	symSums                     [syncSymbols]complex128
	refPh                       []float64
	refX                        []complex64
	refC                        []complex64
	pp48                        phasePulse
	refR                        []complex64
	refIn                       []int
	refSym                      [slotSymbols]int8
	refK                        [slotSymbols]bool
	tfit                        [5]float64
	tfitBase                    int64
	tfitRef                     float64
	slotFit                     bool
	segs                        [2][2]int
	nseg                        int
	prior                       [slotSymbols][4]float32
	hasPrior                    bool
	prevOK                      bool
	cross                       bool
	prevK0                      int
	prevSyms                    [syncSymbols]int8
	lastT0                      float64
	lastSync                    enums.SyncPattern
	bs                          bool
	next                        [cohNext][4]float32
	nextOK                      bool
	mine                        [cohNext][4]float32
	mineOK                      bool
	mineSlot                    int64
	carry                       [cohNext][4]float32
	carryOK                     bool
	qllr                        [extQueue][layer2.BurstBits]int16
	priorK                      bool
	sps                         float64
	div, rate, step             float64
	scanTpl                     *[4][2][cohScanLen]complex64
	scanX                       [cohScanLen]complex64
	nsrc                        int
	peakOK                      bool
	peakAt                      int64
	peakRho                     float64
	peakSrc                     int
	peakData                    bool
	hFit                        float64
	hVar                        float64
	hNew                        bool
	size                        int64
	radios                      [2]cohRadio
	radio                       int8
}

type cohRadio struct {
	dt, freq, totalHz, fitVar float64
	ampAvg, noiseAvg, symAvg  float64
	hFit, hVar                float64
	tfit                      [5]float64
	tfitBase                  int64
	tfitRef                   float64
	amp, noise                [cohFitScales]float64
	nFit                      int
	scale                     float32
	freqOK, hNew, ok          bool
}

func (s *cohStage) saveRadio(r *cohRadio) {
	r.dt, r.freq, r.totalHz, r.fitVar = s.dt, s.freq, s.totalHz, s.fitVar
	r.ampAvg, r.noiseAvg, r.symAvg, r.hFit, r.hVar = s.ampAvg, s.noiseAvg, s.symAvg, s.hFit, s.hVar
	r.amp, r.noise, r.nFit, r.scale, r.freqOK, r.hNew, r.ok = s.amp, s.noise, s.nFit, s.scale, s.freqOK, s.hNew, true
	r.tfit, r.tfitBase, r.tfitRef = s.tfit, s.tfitBase, s.tfitRef
}

func (s *cohStage) loadRadio(r *cohRadio) {
	s.dt, s.freq, s.totalHz, s.fitVar = r.dt, r.freq, r.totalHz, r.fitVar
	s.ampAvg, s.noiseAvg, s.symAvg, s.hFit, s.hVar = r.ampAvg, r.noiseAvg, r.symAvg, r.hFit, r.hVar
	s.amp, s.noise, s.nFit, s.scale, s.freqOK, s.hNew = r.amp, r.noise, r.nFit, r.scale, r.freqOK, r.hNew
	s.tfit, s.tfitBase, s.tfitRef = r.tfit, r.tfitBase, r.tfitRef
}

func (s *cohStage) slotRadio(c *leanCore, slot int64) {
	p := int8(slot & 1)
	if c.bs || p == s.radio {
		return
	}
	if s.radio >= 0 {
		s.saveRadio(&s.radios[s.radio])
	}
	if s.radios[p].ok {
		s.loadRadio(&s.radios[p])
	} else if s.radio >= 0 {
		s.freqOK, s.nFit, s.hNew = false, 0, true
		s.tfit = [5]float64{}
		s.ampAvg, s.noiseAvg, s.symAvg = 0, 0, 0
	}
	s.radio = p
}

const (
	cohIndex      = 0.27
	cohIndexNear  = 0.03
	cohSyncIndex  = 0.977
	cohIndexStart = 0.05 * 0.05
	cohIndexWalk  = 0.03 * 0.03
	cohIndexVar0  = 1e-4
	cohIndexVar   = 2e-3
	cohIndexMove  = 0.002
)

func newCohStage(r *iqRx, opt *options) *cohStage {
	s := newCohStageOf(opt)
	s.src = r
	s.setRing(float64(r.conv.decim))
	if s.div == 2 {
		s.scanTpl = new([4][2][cohScanLen]complex64)
		s.nsrc = min(opt.Sources, len(s.scanTpl))
		for i := range s.nsrc {
			voice, data, _ := sourceAt(i).SyncPatterns()
			s.scanTemplate(voice, &s.scanTpl[i][0])
			s.scanTemplate(data, &s.scanTpl[i][1])
		}
	}
	return s
}

func (s *cohStage) setRing(div float64) {
	s.div, s.rate = div, basebandRate/div
	s.dt = cohDT
	if div == 4 {
		s.dt = cohDT12
	}
	s.sps = samplesPerSy / div
	s.step = s.sps / float64(s.model.sps)
}

func (s *cohStage) ringTime(c *leanCore, k int64) float64 {
	return symTime(c, k) * 2 / s.div
}

func newCohStageOf(opt *options) *cohStage {
	sps := cohSPS
	if opt.CohSPS != 0 {
		sps = opt.CohSPS
	}
	m := newCPMModel(1, sps)
	s := &cohStage{radio: -1, model: m, hFit: cohIndex * cohSyncIndex, hVar: cohIndexStart, hNew: true, mu: opt.PSPGain, llrK: opt.LLRScale, dt: cohDT, easySNR: cohEasySNR, skipSNR: opt.CohSkipSNR, locked: opt.CohLocked, budget: opt.CohBudget,
		noNext: opt.CohNoNext, noCross: opt.CohNoCross, crossVoice: opt.CohCrossVoice, voiceOnly: opt.CohVoiceOnly, verifyScan: opt.CohVerifyScan, symNoise: opt.CohSymNoise, slotFit: opt.CohSlotFit}
	if s.llrK == 0 {
		s.llrK = 1
	}
	win, look := cohMaxRun, 0
	if opt.CohWin > 0 {
		win, look = opt.CohWin, cohLook
	}
	s.tr = newTrellis(m, cohMaxRun, win, look)
	if opt.CohExact {
		s.ex, s.clicks = newExactStage(m, opt, cohMaxRun+2*m.d), opt.CohClicks
	}
	s.size = opt.cohRing()
	s.setRing(2)
	s.buf = make([]complex64, (syncSymbols+2)*sps)
	s.refPh = make([]float64, slotSymbols*samplesPerSy)
	s.refC = make([]complex64, slotSymbols*sps)
	s.pp48 = newPhasePulse(samplesPerSy)
	s.refR = make([]complex64, slotSymbols*samplesPerSy)
	s.refX = make([]complex64, slotSymbols*sps)
	s.refIn = make([]int, slotSymbols)
	s.symX = make([]complex64, cohSymN*sps)
	s.symAt = make([]uint32, cohSymN)
	for i := range s.runs {
		s.runs[i].corr = make([]complex64, cohMaxRun*m.windows)
	}
	return s
}

const (
	cohScanLen   = (syncSymbols - 2) * cohSPS
	cohScanSegs  = 4
	cohScanOff   = 2
	cohScanRho   = 0.47
	cohScanWait  = 3
	cohScanSeg   = cohScanLen / cohScanSegs
	cohScanBound = 1.001
)

func scanPair(x []complex64, tpl *[cohScanLen]complex64, rest, thr float64) (mv, md float64, ok bool) {
	for g := range cohScanSegs {
		if g == cohScanSegs-1 && max(mv, md)+rest < thr {
			return 0, 0, false
		}
		var cv, cd complex64
		for j := g * cohScanSeg; j < (g+1)*cohScanSeg; j++ {
			a, b := float64(real(x[j])), float64(imag(x[j]))
			p, q := float64(real(tpl[j])), float64(imag(tpl[j]))
			ap, bq, aq, bp := a*p, b*q, a*q, b*p
			cv += complex(float32(ap-bq), float32(aq+bp))
			cd += complex(float32(ap+bq), float32(bp-aq))
		}
		mv += float64(real(cv)*real(cv) + imag(cv)*imag(cv))
		md += float64(real(cd)*real(cd) + imag(cd)*imag(cd))
	}
	return mv, md, true
}

func (s *cohStage) scanTemplate(p enums.SyncPattern, tpl *[cohScanLen]complex64) {
	m := s.model
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	ph := 0.0
	for k := 1; k < syncSymbols-1; k++ {
		w := 0
		for i := -1; i <= 1; i++ {
			w = w<<2 | (int(syms[k+i])+3)/2
		}
		rot := complex64(cmplx.Exp(complex(0, -ph)))
		for j := range m.sps {
			tpl[(k-1)*m.sps+j] = m.tpl[w*m.sps+j] * rot
		}
		ph += m.h * math.Pi * float64(syms[k])
	}
}

func (s *cohStage) scan(c *leanCore) (int, bool, int64, bool) {
	hist, n, lo := s.ring()
	if s.scanTpl == nil || n < s.size/2 || s.step != 1 || s.div != 2 {
		return 0, false, 0, false
	}
	sps := float64(samplesPerSy) / 2
	t := symTime(c, c.ns-1) + s.dt - float64(syncSymbols-2)*sps - float64(cohSPS/2)
	b0 := int64(math.Round(t))
	if b0-cohScanOff < lo || b0+cohScanOff+cohScanLen > n {
		return 0, false, 0, false
	}
	best, bestSrc, bestData := 0.0, 0, false
	for o := -cohScanOff; o <= cohScanOff; o++ {
		x := s.scanX[:]
		var e, e3 float64
		for i := range x {
			if i == 3*cohScanSeg {
				e3 = e
			}
			v := hist[(b0+int64(o)+int64(i))&(s.size-1)]
			x[i] = v
			e += float64(real(v)*real(v) + imag(v)*imag(v))
		}
		if e == 0 {
			continue
		}
		rest := (e - e3) * cohScanSeg * cohScanBound
		k := cohScanSegs / (e * cohScanLen)
		ns := -1.0
		for i := range s.nsrc {
			mv, md, ok := scanPair(x, &s.scanTpl[i][0], rest, max(cohScanRho, best)/k)
			if !ok {
				continue
			}
			for d, m := range [2]float64{mv * k, md * k} {
				if m < cohScanRho || m <= best {
					continue
				}
				if ns < 0 {
					ns = noiseShare(x)
				}
				if 1-m <= cohResidual*ns {
					best, bestSrc, bestData = m, i, d == 1
				}
			}
		}
	}
	if s.peakOK && c.ns-1-s.peakAt > 2*cohScanWait {
		s.peakOK = false
	}
	if best >= cohScanRho && (!s.peakOK || best > s.peakRho) {
		s.peakOK, s.peakAt = true, c.ns-1
		s.peakRho, s.peakSrc, s.peakData = best, bestSrc, bestData
	}
	back := c.ns - 1 - s.peakAt
	if !s.peakOK || back < cohScanWait {
		return 0, false, 0, false
	}
	s.peakOK = false
	return s.peakSrc, s.peakData, back, true
}

func (s *cohStage) scanShare(c *leanCore, p enums.SyncPattern) float64 {
	hist, n, lo := s.ring()
	if n < s.size/2 || s.step != 1 || s.div != 2 {
		return 1
	}
	src, data := -1, false
	for i := range s.nsrc {
		voice, dp, _ := sourceAt(i).SyncPatterns()
		if p == voice || p == dp {
			src, data = i, p == dp
			break
		}
	}
	sps := float64(samplesPerSy) / 2
	t := symTime(c, c.ns-1) + s.dt - float64(syncSymbols-2)*sps - float64(cohSPS/2)
	b0 := int64(math.Round(t))
	if src < 0 || b0-cohScanOff < lo || b0+cohScanOff+cohScanLen > n {
		return 1
	}
	best := 0.0
	for o := -cohScanOff; o <= cohScanOff; o++ {
		x := s.scanX[:]
		var e float64
		for i := range x {
			v := hist[(b0+int64(o)+int64(i))&(s.size-1)]
			x[i] = v
			e += float64(real(v)*real(v) + imag(v)*imag(v))
		}
		if e == 0 {
			continue
		}
		mv, md, _ := scanPair(x, &s.scanTpl[src][0], 0, -1)
		m := mv
		if data {
			m = md
		}
		best = max(best, m*cohScanSegs/(e*cohScanLen))
	}
	return best
}

func symTime(c *leanCore, k int64) float64 {
	t := float64(c.mfStep*(c.n-1)+c.center-(c.ns-1-k)*samplesPerSy) + float64(c.mfStep)*float64(c.lastAt)
	return t / 2
}

type cohRing interface {
	cohRing(size int64) ([]complex64, int64, int64)
	cohLazy() bool
	cohPhase(a, b, sign, hz, rate float64) float64
	cohKey() (tunes int, unit, dc float64)
	cohTrack() (float64, bool)
	cohWeight(t0, t1 float64) float32
	cohScale() float32
	cohUnit() float64
	cohResidual() bool
	cohHz() float64
	cohTaps() []float32
}

func (r *iqRx) cohRing(size int64) ([]complex64, int64, int64) {
	return r.yHist, r.yN, r.yN - size
}

func (r *iqRx) cohLazy() bool {
	return false
}

func (r *iqRx) cohPhase(a, b, sign, hz, rate float64) float64 {
	var ph float64
	for a < b {
		n := int64(math.Floor(a))
		f := hz - float64(r.ncoAt(n))
		end := b
		for i := 0; i < min(r.nTunes, iqTunes); i++ {
			if tn := float64(r.tunes[(r.nTunes-1-i)%iqTunes].n); tn > a && tn < end {
				end = tn
			}
		}
		ph += f * (end - a)
		a = end
	}
	return -2 * math.Pi * sign * ph / rate
}

func (r *iqRx) cohKey() (int, float64, float64) {
	return r.nTunes, float64(r.conv.hz), 0
}

func (r *iqRx) cohTrack() (float64, bool) {
	hz := float64(r.conv.hz) + float64(r.conv.hzOf(r.afcAvg))
	if f := &r.d.f; r.d.locked {
		hz = float64(r.conv.hz) + float64((f.dc+r.d.dcHz)/(f.gain*iqUnitsPerHz))
	}
	return hz, r.d.locked
}

func (r *iqRx) cohWeight(_, _ float64) float32 {
	return 1
}

func (r *iqRx) cohScale() float32 {
	return r.d.f.gain
}

func (r *iqRx) cohUnit() float64 {
	return 1
}

func (r *iqRx) cohResidual() bool {
	return true
}

func (r *iqRx) cohTaps() []float32 {
	if r.conv.halfOn || r.conv.decim != 2 {
		return nil
	}
	return r.conv.taps
}

func (p *discPhase) cohTaps() []float32 {
	return nil
}

func (r *iqRx) cohHz() float64 {
	f := &r.d.f
	return float64(r.conv.hz) + float64(f.dc/(f.gain*iqUnitsPerHz))
}

func (p *discPhase) cohRing(size int64) ([]complex64, int64, int64) {
	return p.hist, p.n, max(p.lo, p.n-size)
}

func (p *discPhase) cohLazy() bool {
	return p.lazy
}

func (p *discPhase) cohPhase(a, b, sign, hz, rate float64) float64 {
	return -2 * math.Pi * sign * hz * (b - a) / rate
}

func (p *discPhase) cohKey() (int, float64, float64) {
	return 0, p.unit, p.dcU
}

func (p *discPhase) cohTrack() (float64, bool) {
	return p.offsetHz(), p.c.locked
}

func (p *discPhase) cohWeight(t0, t1 float64) float32 {
	switch {
	case p.clipped(t0, t1):
		return 0
	case p.clicked(t0, t1):
		return discClickWeight
	}
	return 1
}

func (p *discPhase) cohScale() float32 {
	return p.c.audio.gain
}

func (p *discPhase) cohUnit() float64 {
	return p.held
}

func (p *discPhase) cohResidual() bool {
	return false
}

func (p *discPhase) cohHz() float64 {
	return p.offsetHz()
}

func (s *cohStage) ring() ([]complex64, int64, int64) {
	return s.src.cohRing(s.size)
}

func (s *cohStage) ringAt(hist []complex64, k, lo, n int64) complex64 {
	if k < 0 || k < lo || k >= n {
		return 0
	}
	return hist[k&(s.size-1)]
}

func (s *cohStage) sample(t float64) complex64 {
	if s.trialing || s.src.cohLazy() {
		return s.ph.trialAt(t, s.t0, s.trial, s.totalHz)
	}
	hist, n, lo := s.ring()
	i := int64(math.Floor(t))
	mu := float32(t - float64(i))
	p0, p1, p2, p3 := s.ringAt(hist, i-1, lo, n), s.ringAt(hist, i, lo, n), s.ringAt(hist, i+1, lo, n), s.ringAt(hist, i+2, lo, n)
	c := complex(mu, 0)
	return p1 + 0.5*c*(p2-p0+c*(2*p0-5*p1+4*p2-p3+c*(3*(p1-p2)+p3-p0)))
}

func (s *cohStage) phaseAt(t float64) float64 {
	a, b := s.t0, t
	sign := 1.0
	if b < a {
		a, b, sign = b, a, -1
	}
	return s.src.cohPhase(a, b, sign, s.totalHz, s.rate)
}

func (s *cohStage) fill(k0, k1 int, x []complex64) {
	sps := s.model.sps
	if s.dir < 0 || s.trialing || s.src.cohLazy() {
		for i := k0; i < k1; i++ {
			s.fillSym(s.g0+s.dir*i, x[(i-k0)*sps:(i-k0+1)*sps])
		}
		return
	}
	_, n, lo := s.ring()
	key := cohSymKey{gen: s.gen, t0: s.t0, hz: s.totalHz, n: n, lo: lo}
	key.tunes, key.unit, key.dc = s.src.cohKey()
	if key != s.symKey {
		s.symKey = key
		s.symStamp++
	}
	for i := k0; i < k1; i++ {
		k := s.g0 + i
		out := x[(i-k0)*sps : (i-k0+1)*sps]
		j := k + slotSymbols
		if j < 0 || j >= len(s.symAt) {
			s.fillSym(k, out)
			continue
		}
		c := s.symX[j*sps : (j+1)*sps]
		if s.symAt[j] != s.symStamp {
			s.fillSym(k, c)
			s.symAt[j] = s.symStamp
		}
		copy(out, c)
	}
}

type cohSymKey struct {
	gen      int
	t0, hz   float64
	n, lo    int64
	unit, dc float64
	tunes    int
}

func (s *cohStage) fillSym(k int, x []complex64) {
	sps := s.model.sps
	tc := s.t0 + float64(k)*s.sps
	step := s.step
	start := tc - float64(sps/2)*step
	if s.dir < 0 {
		start = tc + float64(sps/2)*step - 2/s.div
		step = -step
	}
	ph0 := s.phaseAt(start)
	dph := (s.phaseAt(start+float64(sps-1)*step) - ph0) / float64(sps-1)
	sn, cs := math.Sincos(ph0)
	rot := complex64(complex(cs, sn))
	sn, cs = math.Sincos(dph)
	rs := complex64(complex(cs, sn))
	for j := range sps {
		v := s.sample(start+float64(j)*step) * rot
		rot *= rs
		if s.dir < 0 {
			v = complex(real(v), -imag(v))
		}
		x[j] = v
	}
}

func (s *cohStage) runSamples(k0, k1 int, x []complex64) {
	if s.tr.rowsOK {
		return
	}
	d := s.model.d
	s.fill(k0+d, k1+d, x)
	r := &s.runs[s.runAt]
	r.gen, r.g0, r.dir, r.t0, r.hz, r.k0, r.k1 = s.gen, s.g0, s.dir, s.t0, s.totalHz, k0, k1
}

const (
	cohRuns = 3
	cohSymN = 2*slotSymbols + cohShift + 4*runMargin
)

type cohRunCorr struct {
	corr   []complex64
	gen    int
	g0     int
	dir    int
	t0, hz float64
	k0, k1 int
}

func (r *cohRunCorr) holds(s *cohStage, k0, k1 int) bool {
	return r.gen == s.gen && r.g0 == s.g0 && r.dir == s.dir && r.t0 == s.t0 && r.hz == s.totalHz && r.k0 == k0 && r.k1 >= k1
}

func (s *cohStage) runPrior(k int, lp []float32) {
	clear(lp)
	j := s.g0 + s.dir*(k+2*s.model.d)
	if v, ok := s.prevAt(j); ok {
		lp[(v+3)/2] = cohKnownPrior
	}
	if j < 0 && j >= -cohNext && s.carryOK {
		for d := range lp {
			lp[d] += s.carry[j+cohNext][d]
		}
	}
	if j < 0 || j >= slotSymbols {
		return
	}
	if s.isK[j] && s.priorK {
		lp[(s.known[j]+3)/2] = cohKnownPrior
	}
	if s.hasPrior {
		for d := range lp {
			lp[d] += s.prior[j][d]
		}
	}
}

func (s *cohStage) runKeep(k int, app []float32) {
	d := s.model.d
	if k >= s.steps-d {
		return
	}
	dist := k + 2*d
	j := s.g0 + s.dir*dist
	if s.dir < 0 && j < 0 && j >= -cohNext && s.nextOK {
		copy(s.next[j+cohNext][:], app)
		return
	}
	if j < s.keepLo() || j >= slotSymbols {
		return
	}
	if s.cross {
		if j >= syncFirstSym+syncSymbols || s.dist[j] == math.MaxInt {
			return
		}
		ma, mb := app[0], s.app[j][0]
		for i := 1; i < 4; i++ {
			ma, mb = max(ma, app[i]), max(mb, s.app[j][i])
		}
		for i := range 4 {
			s.app[j][i] = 0.5 * ((app[i] - ma) + (s.app[j][i] - mb))
		}
		return
	} else if dist >= s.dist[j] {
		return
	}
	copy(s.app[j][:], app)
	s.dist[j] = dist
}

const (
	cohDT     = 1.0
	cohDT12   = -0.4
	cohDTSpan = 0.6
	cohDTWide = 2.4
	cohDTStep = 0.2
	cohDTGain = 0.2
)

func (s *cohStage) fineTime(k0, k1 int) float64 {
	return s.timeIn(k0, k1, cohDTSpan)
}

func (s *cohStage) syncTime(k0, k1 int) float64 {
	relock := s.relock
	s.relock = false
	if t0 := s.t0; relock {
		if r0 := s.rho(k0, k1); r0 < cohRelockRho {
			d := s.timeIn(k0, k1, cohDTWide)
			if r := s.rho(k0, k1); r >= cohPlaceRho && r > r0 {
				return d
			}
			s.t0 = t0
		}
	}
	return cohDTGain * s.fineTime(k0, k1)
}

func (s *cohStage) rho(k0, k1 int) float64 {
	a, n := s.levels(k0, k1)
	return a * a / (a*a + max(n, 0))
}

const cohRelockRho = 0.5

func (s *cohStage) timeIn(k0, k1 int, span float64) float64 {
	t0, best, bestD := s.t0, -1.0, 0.0
	for d := -span; d <= span+1e-9; d += cohDTStep {
		s.t0 = t0 + d*2/s.div
		if a, _ := s.levels(k0, k1); a > best {
			best, bestD = a, d
		}
	}
	s.t0 = t0 + bestD*2/s.div
	return bestD * 2 / s.div
}

const (
	cohFreqGain  = 0.03
	cohFreqSpan  = 200
	cohFreqFirst = 1000
	cohFreqVar   = 80 * 80
	cohFitVar    = 5000
	cohFreqWalk  = 10
)

func (s *cohStage) track() {
	hz, locked := s.src.cohTrack()
	if d := math.Abs(s.freq - hz); !s.freqOK || s.nFit == 0 && d > cohFreqSpan || locked && d > cohFreqFirst {
		s.freq, s.freqOK, s.nFit, s.fitVar = hz, true, 0, cohFreqVar
	}
	s.fitVar += cohFreqWalk
	s.totalHz = s.freq
}

func (s *cohStage) lost(lo int64) bool {
	return s.t0-s.sps < float64(lo) || s.src.cohWeight(s.t0-s.sps, s.t0+slotSymbols*s.sps) == 0
}

func (s *cohStage) lock() {
	s.lastT0, s.lastSync, s.mineOK, s.relock, s.hNew = 0, 0, false, true, true
	s.tfit = [5]float64{}
	s.radio, s.radios[0].ok, s.radios[1].ok = -1, false, false
	s.hVar += cohIndexWalk
}

const (
	cohSlotForget = 0.97
	cohSlotPrior  = 2.8
	cohSlotSpan   = 1.5
)

func (s *cohStage) slotMeasure(slot int64, pt float64, pOK bool) {
	if pOK && math.Abs(s.t0-pt) > cohSlotSpan {
		s.tfit = [5]float64{}
	}
	p := float64(slotSymbols) * s.sps
	f := &s.tfit
	if f[0] == 0 {
		s.tfitBase, s.tfitRef = slot, s.t0
	}
	d := float64(slot - s.tfitBase)
	f[1] -= d * f[0]
	f[2] -= 2*d*f[1] + d*d*f[0]
	f[4] -= d * f[3]
	s.tfitBase, s.tfitRef = slot, s.tfitRef+d*p
	y := s.t0 - s.tfitRef
	f[0] = cohSlotForget*f[0] + 1
	f[1] *= cohSlotForget
	f[2] *= cohSlotForget
	f[3] = cohSlotForget*f[3] + y
	f[4] *= cohSlotForget
	if t, ok := s.slotPredict(slot); ok {
		s.t0 = t
	}
}

func (s *cohStage) slotPredict(slot int64) (float64, bool) {
	f := &s.tfit
	if !s.slotFit || f[0] <= 1.5 {
		return 0, false
	}
	s2 := f[2] + cohSlotPrior
	det := f[0]*s2 - f[1]*f[1]
	if det <= 1e-9 {
		return 0, false
	}
	c := (f[0]*f[4] - f[1]*f[3]) / det
	a := (f[3]*s2 - f[1]*f[4]) / det
	if math.Abs(c) >= 1 {
		return 0, false
	}
	k := float64(slot - s.tfitBase)
	return s.tfitRef + k*float64(slotSymbols)*s.sps + a + c*k, true
}

func (s *cohStage) freqFit(k0, k1 int) float64 {
	return freqOf(s.freqAcc(k0, k1))
}

func freqOf(a1, aL complex128) float64 {
	if a1 == 0 {
		return 0
	}
	f := cmplx.Phase(a1) * symbolRate / (2 * math.Pi)
	if aL == 0 {
		return f
	}
	lag := float64(cohFreqLag)
	p := 2 * math.Pi * f * lag / symbolRate
	d := cmplx.Phase(aL * cmplx.Rect(1, -p))
	return (p + d) * symbolRate / (2 * math.Pi * lag)
}

const cohFreqLag = 8

func (s *cohStage) refFixed(fixed *layer2.RawBurst, k0, k1 int) {
	for k := k0; k < k1; k++ {
		p := 2 * (k - cachSymbols)
		b0 := fixed[p/8] >> (7 - p%8) & 1
		b1 := fixed[(p+1)/8] >> (7 - (p+1)%8) & 1
		s.refSym[k], s.refK[k] = symbolOf(unpacked.Bit(b0), unpacked.Bit(b1)), true
	}
}

func (s *cohStage) freqAcc(k0, k1 int) (complex128, complex128) {
	m := s.model
	span := m.pp.span
	lo, hi := max(k0-span, 0), min(k1+span, slotSymbols-1)
	n := hi - lo + 1
	syms := s.refIn[:n]
	for i := range syms {
		syms[i] = 0
		if s.refK[lo+i] {
			syms[i] = int(s.refSym[lo+i])
		}
	}
	s.g0, s.dir = lo, 1
	x := s.refX[:n*m.sps]
	s.fill(0, n, x)
	ref := s.refC[:n*m.sps]
	s.refWave(syms, ref)
	var prev, acc, w, wPrev, accL complex128
	nw := 0
	have := false
	for k := k0 - lo; k <= k1-lo; k++ {
		var c complex128
		if s.refNear(lo + k) {
			for j := k * m.sps; j < (k+1)*m.sps; j++ {
				c += complex128(x[j]) * complex128(ref[j])
			}
		}
		if have && c != 0 {
			acc += c * cmplx.Conj(prev)
		}
		prev, have = c, c != 0
		w += c
		if nw++; nw == cohFreqLag {
			if wPrev != 0 {
				accL += w * cmplx.Conj(wPrev)
			}
			wPrev, w, nw = w, 0, 0
		}
	}
	return acc, accL
}

func (s *cohStage) refNear(k int) bool {
	for i := max(k-syncEdge, 0); i <= min(k+syncEdge, slotSymbols-1); i++ {
		if !s.refK[i] {
			return false
		}
	}
	return true
}

func (s *cohStage) refWave(syms []int, out []complex64) {
	m := s.model
	taps := s.src.cohTaps()
	if taps == nil || 2*m.sps != samplesPerSy {
		ph := s.refPh[:len(out)]
		m.pp.refPhaseInto(syms, m.sps, m.h, ph)
		for i, v := range ph {
			sn, cs := math.Sincos(v)
			out[i] = complex(float32(cs), float32(-sn))
		}
		return
	}
	n := len(syms) * samplesPerSy
	ph := s.refPh[:n]
	s.pp48.refPhaseInto(syms, samplesPerSy, m.h, ph)
	r := s.refR[:n]
	for j := range n {
		sn, cs := math.Sincos(ph[j])
		r[j] = complex(float32(cs), float32(sn))
	}
	for j := range n - 1 {
		r[j] += r[j+1]
	}
	r[n-1] *= 2
	h := len(taps) / 2
	for i := range out {
		c := 2*i + 1 + h
		var y complex64
		for t, a := range taps {
			if j := c - t; j >= 0 && j < n {
				y += complex(a, 0) * r[j]
			}
		}
		out[i] = complex(real(y)/2, -imag(y)/2)
	}
}

func (s *cohStage) trackBurst(fixed *layer2.RawBurst) {
	for k := range burstSymbols {
		p := 2 * k
		b0 := fixed[p/8] >> (7 - p%8) & 1
		b1 := fixed[(p+1)/8] >> (7 - (p+1)%8) & 1
		s.known[cachSymbols+k] = symbolOf(unpacked.Bit(b0), unpacked.Bit(b1))
	}
	clear(s.refK[:])
	s.refFixed(fixed, cachSymbols, slotSymbols)
	acc, accL := s.freqAcc(cachSymbols, slotSymbols-1)
	var snr float64
	n := 0
	for k0 := cachSymbols; k0+syncSymbols <= slotSymbols; k0 += syncSymbols {
		if amp, noise := s.levels(k0, k0+syncSymbols-1); noise > 0 {
			snr += amp * amp / noise
			n++
		}
	}
	if acc == 0 || n == 0 {
		return
	}
	snr /= float64(n)
	fit := freqOf(acc, accL)
	if math.Abs(fit) >= cohFreqSpan {
		return
	}
	g := s.fitVar / (s.fitVar + cohFitVar/(snr*snr)/float64(n))
	s.fitVar *= 1 - g
	s.freq += max(cohFreqGain, g) * fit
	s.totalHz = s.freq
}

const (
	cohKnownPrior  = 4
	cohRefinePrior = 20
)

func (s *cohStage) detect(c *leanCore, b *Burst, llr *[layer2.BurstBits]int16) bool {
	if s.src.cohScale() <= 0 {
		return false
	}
	_, _, lo := s.ring()
	s.gen++
	s.bs, s.nextOK = c.bs, false
	s.carryOK = s.mineOK && s.mineSlot == b.Slot-1
	if s.carryOK {
		s.carry = s.mine
	}
	s.mineOK, s.mineSlot = false, b.Slot
	s.slotRadio(c, b.Slot)
	s.t0 = s.ringTime(c, c.base) + s.dt
	if s.lost(lo) {
		return false
	}
	s.track()
	segs, nseg := s.knownSegs(b)
	if nseg == 0 {
		return false
	}
	for i := range s.dist {
		s.dist[i] = math.MaxInt
	}
	if s.quiet(c, segs[0], nseg, b) {
		return false
	}
	if nseg != 1 && s.slotFit {
		if pt, ok := s.slotPredict(b.Slot); ok && math.Abs(pt-s.t0) <= cohSlotSpan {
			s.t0 = pt
		}
	}
	if nseg == 1 {
		pt, pOK := s.slotPredict(b.Slot)
		s.dt += s.syncTime(segs[0][0], segs[0][1])
		if s.slotFit {
			s.slotMeasure(b.Slot, pt, pOK)
		}
		a0, n0 := s.levels(segs[0][0], segs[0][1])
		fit := s.freqFit(segs[0][0], segs[0][1])
		f0 := s.freq
		s.totalHz = s.freq + fit
		amp, noise := s.levels(segs[0][0], segs[0][1])
		s.totalHz = s.freq
		ok := amp > 0 && noise > 0 && math.Abs(fit) < cohFreqFirst
		if ok && math.Abs(fit) >= cohFreqSpan {
			rho := amp * amp / (amp*amp + noise)
			ok = rho >= cohPlaceRho && rho > a0*a0/(a0*a0+max(n0, 0))
			if ok {
				s.nFit, s.fitVar = 0, cohFreqVar
			}
		}
		if ok {
			snr := amp * amp / noise
			g := s.fitVar / (s.fitVar + cohFitVar/(snr*snr))
			s.fitVar *= 1 - g
			s.nFit++
			s.freq += max(cohFreqGain, g) * fit
			s.totalHz = s.freq
		}
		s.fitDeviation(segs[0][0], segs[0][1], f0+fit-s.freq)
	}
	if p := s.ph; p != nil && p.lazy && !p.cal {
		return false
	}
	if !s.average(segs, nseg) {
		return false
	}
	s.scale = float32(2 * s.ampAvg / s.noiseAvg)
	if s.symNoise > 0 && s.symAvg > 0 {
		s.scale = float32(s.symNoise) / 16 * float32(2*s.ampAvg/s.symAvg)
	}
	s.easy = s.ampAvg*s.ampAvg/s.noiseAvg >= s.easySNR
	s.segs, s.nseg, s.hasPrior = segs, nseg, false
	s.prevOK = false
	if c.bs && s.lastSync != 0 {
		k := int(math.Round((s.lastT0-s.t0)/s.sps)) + syncFirstSym
		if k > syncFirstSym-slotSymbols-cohShift && k < syncFirstSym-slotSymbols+cohShift {
			s.prevOK, s.prevK0 = true, k
			syncSyms(s.lastSync, &s.prevSyms)
		}
	}
	s.lastT0, s.lastSync = s.t0, 0
	if nseg == 1 {
		s.lastSync = b.Sync
	}
	return !s.fitOnly && s.runAll(llr)
}

func (s *cohStage) knownSegs(b *Burst) (segs [2][2]int, nseg int) {
	clear(s.isK[:])
	if _, _, ok := layer2.SyncSourceOf(b.Sync); ok && b.Distance >= 0 {
		var syms [syncSymbols]int8
		syncSyms(b.Sync, &syms)
		clear(s.refK[:])
		for i, v := range syms {
			s.known[syncFirstSym+i], s.isK[syncFirstSym+i] = v, true
			s.refSym[syncFirstSym+i], s.refK[syncFirstSym+i] = v, true
		}
		if _, kind, _ := layer2.SyncSourceOf(b.Sync); kind == layer2.KindData && b.HasFixed && !b.Fixed.SlotType().FEC.Uncorrectable {
			s.refFixed(&b.Fixed, syncFirstSym-slotTypeSymbols, syncFirstSym)
			s.refFixed(&b.Fixed, syncFirstSym+syncSymbols, syncFirstSym+syncSymbols+slotTypeSymbols)
		}
		segs[0] = [2]int{syncFirstSym, syncFirstSym + syncSymbols - 1}
		nseg, s.priorK = 1, true
	} else if b.Voice != enums.VoiceBurstUnknown && b.HasFixed {
		for _, k := range [...]int{syncFirstSym, syncFirstSym + 1, syncFirstSym + 2, syncFirstSym + 3,
			syncFirstSym + syncSymbols - 4, syncFirstSym + syncSymbols - 3, syncFirstSym + syncSymbols - 2, syncFirstSym + syncSymbols - 1} {
			p := 2 * (k - cachSymbols)
			b0 := b.Fixed[p/8] >> (7 - p%8) & 1
			b1 := b.Fixed[(p+1)/8] >> (7 - (p+1)%8) & 1
			s.known[k], s.isK[k] = symbolOf(unpacked.Bit(b0), unpacked.Bit(b1)), true
		}
		segs[0] = [2]int{syncFirstSym, syncFirstSym + 3}
		segs[1] = [2]int{syncFirstSym + syncSymbols - 4, syncFirstSym + syncSymbols - 1}
		nseg, s.priorK = 2, false
	}
	return segs, nseg
}

func (s *cohStage) average(segs [2][2]int, nseg int) bool {
	var amp, noise, sym float64
	for _, sg := range segs[:nseg] {
		a, v := s.levels(sg[0], sg[1])
		amp += a / float64(nseg)
		noise += v / float64(nseg)
		sym += s.lastSym / float64(nseg)
	}
	if amp <= 0 || noise <= 0 {
		return false
	}
	n := float64(segs[nseg-1][1] - segs[0][0] + 1)
	if nseg == 2 {
		n = float64(segs[0][1] - segs[0][0] + 1 + segs[1][1] - segs[1][0] + 1)
	}
	if s.ampAvg == 0 {
		s.ampAvg, s.noiseAvg, s.symAvg = amp, noise, sym
	} else {
		g := n / (n + cohLevelSyms)
		s.ampAvg += g * (amp - s.ampAvg)
		s.noiseAvg += g * (noise - s.noiseAvg)
		s.symAvg += g * (sym - s.symAvg)
	}
	return true
}

func (s *cohStage) quiet(c *leanCore, seg [2]int, nseg int, b *Burst) bool {
	if !s.clear(c) {
		if s.fitOnly || s.spend(b.Slot) {
			return false
		}
		s.lastSync = 0
		return true
	}
	if nseg == 1 {
		if amp, noise := s.levels(seg[0], seg[1]); amp > 0 && noise > 0 {
			g := float64(syncSymbols) / (syncSymbols + cohLevelSyms)
			s.ampAvg += g * (amp - s.ampAvg)
			s.noiseAvg += g * (noise - s.noiseAvg)
			s.symAvg += g * (s.lastSym - s.symAvg)
		}
	}
	s.lastSync = 0
	return true
}

func (s *cohStage) spend(j int64) bool {
	if s.budget <= 0 {
		return true
	}
	if d := j - s.budgetSlot; d > 0 {
		s.credit = min(s.credit+s.budget*float64(d), cohBudgetMax*s.budget)
	}
	s.budgetSlot = j
	return s.credit > 0
}

const cohBudgetMax = 6

func (s *cohStage) clear(c *leanCore) bool {
	if s.skipSNR <= 0 {
		return false
	}
	return c.dev*c.dev >= cohClearCore*c.discVar || s.nFit >= cohQuietFits && s.ampAvg*s.ampAvg >= s.skipSNR*s.noiseAvg && c.dev*c.dev >= cohQuietCore*c.discVar
}

const (
	cohQuietFits = 4
	cohQuietCore = 4
	cohClearCore = 16
)

const cohLevelSyms = 48

const cohEasySNR = 6

const cohShift = 3

const slotTypeSymbols = 5

const cohMaxRun = slotSymbols + syncSymbols + cohShift + runMargin

const cohNext = slotSymbols - syncFirstSym - syncSymbols

func (s *cohStage) nextInto(llr *[layer2.BurstBits]int16) {
	for i := range cohNext {
		p := 2 * (syncFirstSym + syncSymbols + i - cachSymbols)
		a0, a1 := dibitLLR(s.next[i][:], s.llrK)
		llr[p] = llr[p]/2 + a0/2
		llr[p+1] = llr[p+1]/2 + a1/2
	}
}

func (s *cohStage) prevAt(j int) (int8, bool) {
	i := j - s.prevK0
	if !s.prevOK || i < 0 || i >= syncSymbols {
		return 0, false
	}
	return s.prevSyms[i], true
}

const cohPlaceRho = 0.3

func (s *cohStage) place(c *leanCore, b *Burst, j int32) bool {
	s.slotRadio(c, int64(j))
	if s.src.cohScale() <= 0 || !s.freqOK || s.clear(c) {
		return false
	}
	_, _, lo := s.ring()
	t0 := s.ringTime(c, c.base) + s.dt
	if t0-s.sps < float64(lo) {
		return false
	}
	s.totalHz = s.freq
	voice, data, _ := sourceAt(int(c.src)).SyncPatterns()
	best, bestP, bestD := 0.0, enums.SyncPattern(0), 0.0
	for _, p := range [...]enums.SyncPattern{voice, data} {
		var syms [syncSymbols]int8
		syncSyms(p, &syms)
		for i, v := range syms {
			s.known[syncFirstSym+i] = v
		}
		s.t0 = t0
		d := s.fineTime(syncFirstSym, syncFirstSym+syncSymbols-1)
		amp, noise := s.levels(syncFirstSym, syncFirstSym+syncSymbols-1)
		if rho := amp * amp / (amp*amp + max(noise, 0)); amp > 0 && rho > best {
			best, bestP, bestD = rho, p, d
		}
	}
	if best < cohPlaceRho {
		return false
	}
	s.dt += cohDTGain * bestD
	ch := &c.ch[j&1]
	b.Sync, b.Distance = bestP, syncDist(&b.Data, bestP)
	if bestP == voice {
		ch.sf, ch.data = j, false
	} else {
		ch.sf, ch.data = leanNoSlot, false
		if st := b.Data.SlotType(); !st.FEC.Uncorrectable {
			ch.data = !c.bs && !isHeader(st.DataType)
		}
	}
	return true
}

const (
	cohVerifyRho = 0.37
	cohVerifyPre = 0.2
)

const cohResidual = 4

func noiseShare(x []complex64) float64 {
	var p, p2 float64
	for _, v := range x {
		q := float64(real(v)*real(v) + imag(v)*imag(v))
		p += q
		p2 += q * q
	}
	if p <= 0 {
		return 1
	}
	p /= float64(len(x))
	p2 /= float64(len(x))
	return 1 - math.Sqrt(max(2*p*p-p2, 0))/p
}

func (s *cohStage) verify(c *leanCore, p enums.SyncPattern) bool {
	if s.src.cohScale() <= 0 {
		return false
	}
	_, _, lo := s.ring()
	base := c.ns - 1 - leanSyncEnd
	s.t0 = s.ringTime(c, base) + s.dt
	if s.t0-s.sps < float64(lo) {
		return false
	}
	if s.verifyScan > 0 && s.scanTpl != nil && s.scanShare(c, p) < s.verifyScan {
		return false
	}
	s.verifyHz()
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	for i, v := range syms {
		s.known[syncFirstSym+i] = v
	}
	if s.seedDeviation(c, syncFirstSym, syncFirstSym+syncSymbols-1) {
		s.verifyHz()
	}
	if amp, noise := s.levels(syncFirstSym, syncFirstSym+syncSymbols-1); amp <= 0 || amp*amp/(amp*amp+max(noise, 0)) < cohVerifyPre {
		return false
	}
	d := s.fineTime(syncFirstSym, syncFirstSym+syncSymbols-1)
	amp, noise := s.levels(syncFirstSym, syncFirstSym+syncSymbols-1)
	rho := amp * amp / (amp*amp + max(noise, 0))
	if amp <= 0 || rho < cohVerifyRho || s.src.cohResidual() && 1-rho > cohResidual*noiseShare(s.buf[:syncSymbols*s.model.sps]) {
		return false
	}
	s.dt += cohDTGain * d
	return true
}

func (s *cohStage) verifyHz() {
	s.totalHz = s.freq
	if !s.freqOK {
		s.totalHz = s.src.cohHz()
	}
}

func (s *cohStage) keepLo() int {
	if s.ex != nil && s.bs {
		return 0
	}
	return cachSymbols
}

func (s *cohStage) cachLLR(llr *[layer2.CACHBits]int16) bool {
	if s.ex == nil || !s.bs {
		return false
	}
	for k := range cachSymbols {
		if s.dist[k] == math.MaxInt {
			return false
		}
		llr[2*k], llr[2*k+1] = dibitLLR(s.app[k][:], s.llrK)
	}
	return true
}

func (s *cohStage) runAll(llr *[layer2.BurstBits]int16) bool {
	for i := range s.dist {
		s.dist[i] = math.MaxInt
	}
	lo, hi := cachSymbols, slotSymbols-1
	g0, g1 := s.segs[0][0], s.segs[s.nseg-1][1]
	s.runAt = 0
	s.run(g0, hi-g0+1+runMargin, 1, s.scale)
	s.runAt = 1
	if s.bs && s.nseg == 1 && !s.easy && !s.noNext {
		s.nextOK = true
		s.run(g1, g1+cohNext+1+runMargin, -1, s.scale)
	} else {
		s.run(g1, g1-s.keepLo()+1+runMargin, -1, s.scale)
	}
	if s.prevOK && !s.easy && !s.noCross && (!s.crossVoice || s.nseg == 2) {
		s.cross, s.runAt = true, 2
		s.run(s.prevK0, syncFirstSym+syncSymbols-s.prevK0+runMargin, 1, s.scale)
		s.cross = false
	}
	for k := lo; k <= hi; k++ {
		if s.dist[k] == math.MaxInt {
			return false
		}
		p := 2 * (k - cachSymbols)
		llr[p], llr[p+1] = dibitLLR(s.app[k][:], s.llrK)
	}
	return true
}

func (s *cohStage) refineData(ext *[layer2.BurstBits]float32, w float32, llr *[layer2.BurstBits]int16) bool {
	clear(s.prior[:])
	for k := range burstSymbols {
		lp := &s.prior[cachSymbols+k]
		extPrior(lp, ext[2*k], ext[2*k+1], w)
		for d := range lp {
			lp[d] /= s.llrK
		}
	}
	s.hasPrior = true
	ok := s.runAll(llr)
	s.hasPrior = false
	copy(s.mine[:], s.prior[slotSymbols-cohNext:])
	s.mineOK = true
	return ok
}

func (s *cohStage) refine(fixed *layer2.RawBurst, known *[layer2.BurstBits]bool, llr *[layer2.BurstBits]int16) bool {
	clear(s.prior[:])
	for k := range burstSymbols {
		fixedPrior(&s.prior[cachSymbols+k], fixed, known, k, cohRefinePrior)
	}
	s.hasPrior = true
	ok := s.runAll(llr)
	s.hasPrior = false
	copy(s.mine[:], s.prior[slotSymbols-cohNext:])
	s.mineOK = true
	return ok
}

func (s *cohStage) run(g, n, dir int, scale float32) {
	m := s.model
	d := m.d
	s.g0, s.dir = g, dir
	s.steps = n - 2*d
	var syms [cohKnown]int
	for i := range cohKnown {
		if v, ok := s.prevAt(g + dir*i); ok {
			syms[i] = int(v)
		} else {
			syms[i] = int(s.known[g+dir*i])
		}
	}
	s.fill(0, cohKnown, s.buf)
	rot := m.anchor(s.buf, syms[:])
	if s.ex != nil {
		s.work += int64(s.steps)
		s.runExact(rot, scale)
		return
	}
	for i := range 2 * d {
		s.runPrior(i-2*d, s.tr.slp[4*i:4*i+4])
	}
	s.work += int64(s.steps)
	r := &s.runs[s.runAt]
	s.tr.rows, s.tr.rowsOK = r.corr, r.holds(s, 0, s.steps)
	s.tr.run(s, s.steps, rot, scale, s.mu)
}

func (s *cohStage) levels(k0, k1 int) (amp, noise float64) {
	m := s.model
	s.g0, s.dir = k0, 1
	n := min(k1-k0+1, syncSymbols)
	var syms [syncSymbols]int
	for i := range n {
		syms[i] = int(s.known[k0+i])
	}
	s.fill(0, n, s.buf)
	var c complex128
	var e float64
	cs := &s.symSums
	*cs = [syncSymbols]complex128{}
	ph := 0.0
	cnt := 0
	for k := range n - 2*m.d {
		w := 0
		for i := range 2*m.d + 1 {
			w = w<<2 | (syms[k+i]+3)/2
		}
		rot := cmplx.Exp(complex(0, -ph))
		for j := range m.sps {
			x := complex128(s.buf[(k+m.d)*m.sps+j])
			v := x * complex128(m.tpl[w*m.sps+j]) * rot
			c += v
			cs[k] += v
			e += real(x)*real(x) + imag(x)*imag(x)
			cnt++
		}
		ph += m.h * math.Pi * float64(syms[k])
	}
	if cnt == 0 {
		return 0, 0
	}
	amp = cmplx.Abs(c) / float64(cnt)
	noise = e/float64(cnt) - amp*amp
	if s.symNoise > 0 && amp > 0 {
		u := c / complex(cmplx.Abs(c), 0) * complex(amp*float64(m.sps), 0)
		var q float64
		for _, v := range cs[:n-2*m.d] {
			d := v - u
			q += real(d)*real(d) + imag(d)*imag(d)
		}
		s.lastSym = q / float64(cnt)
	}
	return amp, noise
}

func (s *cohStage) runExact(rot complex64, scale float32) {
	m, e := s.model, s.ex
	d, sps := m.d, m.sps
	n := min(s.steps+2*d, len(e.buf)/sps)
	s.steps = n - 2*d
	s.fill(0, n, e.buf[:n*sps])
	for i := range n {
		tc := s.t0 + float64(s.g0+s.dir*i)*s.sps
		if w := s.src.cohWeight(tc-s.sps/2, tc+s.sps/2); w == 0 || w < 1 && s.clicks {
			for j := i * sps; j < (i+1)*sps; j++ {
				e.buf[j] *= complex(w, 0)
			}
		}
	}
	for i := range 2 * d {
		s.runPrior(i-2*d, e.xt.slp[4*i:4*i+4])
	}
	for k := range s.steps {
		s.runPrior(k, e.xt.lp[4*k:4*k+4])
	}
	app := e.exec(s.steps, rot, scale, anchorKappa)
	for k := range s.steps {
		s.runKeep(k, app[4*k:4*k+4])
	}
}
