//go:build !tinygo

package rf

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// genieCase is one input of the genie harness with its truth: every burst
// on the air with its time, talker, bits and CACH, the clock rate and the
// carrier of each talker, and the level of the input over time.
type genieCase struct {
	name   string
	format string
	truth  []mxTruth
	r      float64
	hz     []float64
	gainAt func(t float64) float64
	in     mxInput
	g0     float64
	iq     []complex64
	end    float64
	rx     func(tier Tier) ([]Burst, *genieReading)
	only   []Tier
}

// genieReading holds the estimates of a receiver at the end of its input
// that the harness sets against the truth.
type genieReading struct {
	offsetHz, deviationHz float64
	clockPPM              float64
	clockLearned          bool
	replayPPM             float64
	hasReplay             bool
}

// genieFront is the reference receiver of one talker: the discriminator
// output at the carrier of the talker, in Hz, through the root raised
// cosine filter matched to the transmit pulse (ETSI TS 102 361-1 clause
// 10.2), sampled at rate from t0.
type genieFront struct {
	rate, t0 float64
	y        []float64
}

func (f *genieFront) at(t float64) (float64, bool) {
	x := (t - f.t0) * f.rate
	i := int(math.Floor(x))
	if i < 1 || i+2 >= len(f.y) {
		return 0, false
	}
	mu := x - float64(i)
	p0, p1, p2, p3 := f.y[i-1], f.y[i], f.y[i+1], f.y[i+2]
	return p1 + 0.5*mu*(p2-p0+mu*(2*p0-5*p1+4*p2-p3+mu*(3*(p1-p2)+p3-p0))), true
}

func genieRRC(rate float64) []float64 {
	const alpha, span = 0.2, 4
	n := int(math.Ceil(span * rate / symbolRate))
	h := make([]float64, 2*n+1)
	var sum float64
	for i := range h {
		x := float64(i-n) / rate * symbolRate
		var v float64
		switch {
		case x == 0:
			v = 1 - alpha + 4*alpha/math.Pi
		case math.Abs(math.Abs(x)-1/(4*alpha)) < 1e-9:
			v = alpha / math.Sqrt2 * ((1+2/math.Pi)*math.Sin(math.Pi/(4*alpha)) + (1-2/math.Pi)*math.Cos(math.Pi/(4*alpha)))
		default:
			v = (math.Sin(math.Pi*x*(1-alpha)) + 4*alpha*x*math.Cos(math.Pi*x*(1+alpha))) / (math.Pi * x * (1 - 16*alpha*alpha*x*x))
		}
		h[i] = v
		sum += v
	}
	for i := range h {
		h[i] /= sum
	}
	return h
}

func genieFilter(x, h []float64) []float64 {
	y := make([]float64, len(x))
	m := len(h) / 2
	for i := range y {
		var s float64
		for k, c := range h {
			if j := i + k - m; j >= 0 && j < len(x) {
				s += c * x[j]
			}
		}
		y[i] = s
	}
	return y
}

func genieAudioFront(c *genieCase, who int) *genieFront {
	in := &c.in
	x := make([]float64, len(in.audio))
	for i, v := range in.audio {
		t := float64(i)/float64(in.hz) + in.delay
		x[i] = (float64(v)-mxAudioDC)/(c.g0*c.gainAt(t)) - c.hz[who]
	}
	return &genieFront{rate: float64(in.hz), t0: in.delay, y: genieFilter(x, genieRRC(float64(in.hz)))}
}

func genieIQFront(c *genieCase, who int) *genieFront {
	rate := float64(c.in.rate)
	z := make([]complex128, len(c.iq))
	var ph float64
	step := -2 * math.Pi * c.hz[who] / rate
	for i, v := range c.iq {
		s, co := math.Sincos(ph)
		z[i] = complex128(v) * complex(co, s)
		if ph += step; ph > math.Pi || ph < -math.Pi {
			ph = math.Remainder(ph, 2*math.Pi)
		}
	}
	switch {
	case rate > 2*basebandRate:
		d := int(rate / 64000)
		h := lowpassTaps(8*d+1, 7000/rate)
		m := len(h) / 2
		y := make([]complex128, len(z)/d)
		for o := range y {
			var s complex128
			for k, t := range h {
				if j := o*d + k - m; j >= 0 && j < len(z) {
					s += z[j] * complex(t, 0)
				}
			}
			y[o] = s
		}
		z, rate = y, rate/float64(d)
	case rate < basebandRate:
		u := int(math.Round(basebandRate / rate))
		h := lowpassTaps(16*u+1, 0.5/float64(u))
		m := len(h) / 2
		y := make([]complex128, len(z)*u)
		for o := range y {
			var s complex128
			for k, t := range h {
				if j := o + k - m; j >= 0 && j < len(y) && j%u == 0 {
					s += z[j/u] * complex(t*float64(u), 0)
				}
			}
			y[o] = s
		}
		z, rate = y, rate*float64(u)
	}
	ch := lowpassTaps(int(audioIFLen*rate/basebandRate)|1, audioIFHz/rate)
	m := len(ch) / 2
	d := make([]float64, len(z))
	var prev complex128
	for i := range z {
		var s complex128
		for k, t := range ch {
			if j := i + k - m; j >= 0 && j < len(z) {
				s += z[j] * complex(t, 0)
			}
		}
		p := s * complex(real(prev), -imag(prev))
		d[i] = math.Atan2(imag(p), real(p)) * rate / (2 * math.Pi)
		prev = s
	}
	return &genieFront{rate: rate, t0: -0.5 / rate, y: genieFilter(d, genieRRC(rate))}
}

func (c *genieCase) front(who int) *genieFront {
	if c.in.audio != nil {
		return genieAudioFront(c, who)
	}
	return genieIQFront(c, who)
}

type genieCal struct {
	tau, unit, dc float64
}

func genieSyms(tr *mxTruth) (sym [cachSymbols + burstSymbols]float64) {
	for k := range burstSymbols {
		b := 2 * k
		sym[cachSymbols+k] = float64(dibitSymbol(tr.data[b/8]>>(7-b%8)&1, tr.data[(b+1)/8]>>(7-(b+1)%8)&1))
	}
	for k := range cachSymbols {
		b := 2 * k
		sym[k] = float64(dibitSymbol(tr.cach[b/8]>>(7-b%8)&1, tr.cach[(b+1)/8]>>(7-(b+1)%8)&1))
	}
	return sym
}

func genieFit(c *genieCase, f *genieFront, tau float64) (g, dc, res float64, n int) {
	var sx, sy, sxx, sxy, syy float64
	for j := range c.truth {
		tr := &c.truth[j]
		if tr.who != 0 {
			continue
		}
		sym := genieSyms(tr)
		for k := cachSymbols; k < len(sym); k++ {
			v, ok := f.at(tr.at + tau + float64(k-cachSymbols)/(symbolRate*c.r))
			if !ok {
				continue
			}
			sx, sy, sxx, sxy, syy, n = sx+sym[k], sy+v, sxx+sym[k]*sym[k], sxy+sym[k]*v, syy+v*v, n+1
		}
	}
	m := float64(n)
	den := m*sxx - sx*sx
	if n == 0 || den <= 0 {
		return 0, 0, math.Inf(1), 0
	}
	g = (m*sxy - sx*sy) / den
	dc = (sy - g*sx) / m
	res = syy - 2*g*sxy - 2*dc*sy + g*g*sxx + 2*g*dc*sx + m*dc*dc
	return g, dc, res / m, n
}

// calibrate fits the delay of the reference front of talker 0 and the unit
// and DC of its output by least squares on the known symbols of every burst
// of that talker: the constant delay of the filters and the gain of the
// path, the same for every burst, so that each burst is sliced at its own
// true time.
func (c *genieCase) calibrate(f *genieFront) genieCal {
	best, cal := math.Inf(1), genieCal{}
	scan := func(lo, hi, step float64) {
		for tau := lo; tau <= hi; tau += step {
			if g, dc, res, n := genieFit(c, f, tau); n > 0 && g > 0 && res/(g*g) < best {
				best, cal = res/(g*g), genieCal{tau, g, dc}
			}
		}
	}
	scan(-400e-6, 400e-6, 4e-6)
	scan(cal.tau-4e-6, cal.tau+4e-6, 0.1e-6)
	return cal
}

func genieSlice(f *genieFront, cal genieCal, at, r float64) (data layer2.RawBurst, cach [3]byte, ok bool) {
	for k := -cachSymbols; k < burstSymbols; k++ {
		v, in := f.at(at + cal.tau + float64(k)/(symbolRate*r))
		if !in {
			if k >= 0 {
				return data, cach, false
			}
			continue
		}
		y := (v - cal.dc) / cal.unit
		var d byte
		if y < 0 {
			d = 2
			y = -y
		}
		if y > 2 {
			d |= 1
		}
		if k < 0 {
			b := 2 * (k + cachSymbols)
			cach[b/8] |= d << (6 - b%8)
		} else {
			b := 2 * k
			data[b/8] |= d << (6 - b%8)
		}
	}
	return data, cach, true
}

func genieKind(want *layer2.RawBurst) string {
	p, d := want.Sync()
	if d > damagedSyncDistance {
		return "voice"
	}
	if p == enums.SyncBSSourcedData || p == enums.SyncMSSourcedData || p == enums.SyncTDMA1Data || p == enums.SyncTDMA2Data {
		return "data"
	}
	if p == enums.SyncMSSourcedRC {
		return "rc"
	}
	return "voice"
}

// genieSame reports whether got decodes to the content of want: the C0 and
// C1 words of the AMBE frames of a voice burst, which their Golay codes
// protect, the slot type and the payload of a data burst
// (ETSI TS 102 361-1 annex B), and the EMB and the payload of an RC burst.
func genieSame(got, want *layer2.RawBurst) bool {
	switch genieKind(want) {
	case "data":
		gs, ws := got.SlotType(), want.SlotType()
		if gs.FEC.Uncorrectable || gs.ColorCode != ws.ColorCode || gs.DataType != ws.DataType {
			return false
		}
		if ws.DataType == enums.DataTypeRate34 {
			a, ra := got.Rate34Payload()
			b, _ := want.Rate34Payload()
			return !ra.Uncorrectable && a == b
		}
		if ws.DataType == enums.DataTypeRate1 {
			a, _ := got.Rate1Payload()
			b, _ := want.Rate1Payload()
			return a == b
		}
		a, ra := got.BPTCPayload()
		b, _ := want.BPTCPayload()
		return !ra.Uncorrectable && a == b
	case "rc":
		ge, we := got.StandaloneRCEMB(), want.StandaloneRCEMB()
		return !ge.FEC.Uncorrectable && ge.ColorCode == we.ColorCode && ge.PreemptionAndPowerControlIndicator == we.PreemptionAndPowerControlIndicator && got.StandaloneRCData() == want.StandaloneRCData()
	}
	a, _ := got.AMBEFrames()
	b, _ := want.AMBEFrames()
	for k := range a {
		if !slices.Equal(a[k][:3], b[k][:3]) {
			return false
		}
	}
	return true
}

// genieBurst is the verdict on one burst on the air for one receiver.
type genieBurst struct {
	j                   int
	present, ok, cachOK bool
	bits, cachBits      int
	dt                  float64
	gBits, gtBits       int
	gOK, gtOK, gCach    bool
	verdict             string
	stream              int
	slot                int64
}

// genieReport is the outcome of one receiver on one case.
type genieReport struct {
	tier    Tier
	bursts  []genieBurst
	extra   []float64
	split   int
	reading *genieReading
	counts  map[string]int
}

func genieBits(a, b *layer2.RawBurst) int { return hamming(a[:], b[:]) }

func (c *genieCase) receive(tier Tier) ([]Burst, *genieReading) {
	if c.rx != nil {
		return c.rx(tier)
	}
	return genieReceive(c.in, tier)
}

func genieReceive(in mxInput, tier Tier) ([]Burst, *genieReading) {
	var got []Burst
	if in.audio != nil {
		r := mustAudio(AudioRxConfig{Rate: in.hz, Tier: tier})
		for k := 0; k < len(in.audio); k += 1024 {
			got = r.AppendBursts(got, in.audio[k:min(k+1024, len(in.audio))])
		}
		got = r.AppendFlush(got)
		return got, genieRead(&r.leanCore, float64(r.Offset()), float64(r.Level()))
	}
	r := mustNew(RxConfig{Rate: in.rate, Tier: tier, Format: in.iqFmt})
	for k := 0; k < len(in.iq); k += 8192 {
		got = r.AppendBursts(got, in.iq[k:min(k+8192, len(in.iq))])
	}
	got = r.AppendFlush(got)
	return got, genieRead(r.core(), r.OffsetHz(), r.DeviationHz())
}

func genieRead(c *leanCore, off, dev float64) *genieReading {
	g := &genieReading{offsetHz: off, deviationHz: dev}
	if c.tm.step > 0 {
		g.clockPPM = -float64(c.tm.rate) / float64(2*c.tm.step) * 1e6
	}
	g.clockLearned = c.tm.learned
	if x, ok := c.ext.(*softExt); ok && c.ext != nil {
		g.replayPPM, g.hasReplay = -float64(x.stretch)*1e6, true
	}
	return g
}

// genieRun runs the receiver of tier on c and gives the verdict on every
// burst on the air: ok when the receiver decodes it; phys when the
// reference receiver, at the true timing, clock rate, carrier and level,
// does not decode it either, a physical limit of the input; and a receiver
// loss otherwise: rx-miss when the receiver gives no burst, rx-time when
// the reference decodes it at the true timing but not at the timing that
// the receiver stamped, rx-bits when it decodes it at both, and rx-cach for
// a wrong CACH that the reference reads right. A burst of the receiver with
// no burst on the air within 1 ms is extra, a receiver loss too.
func genieRun(c *genieCase, tier Tier) *genieReport {
	fronts := map[int]*genieFront{0: c.front(0)}
	cal := c.calibrate(fronts[0])
	frontOf := func(who int) *genieFront {
		if f, ok := fronts[who]; ok {
			return f
		}
		f := c.front(who)
		fronts[who] = f
		return f
	}
	got, reading := c.receive(tier)
	rep := &genieReport{tier: tier, reading: reading, counts: map[string]int{}}
	byTruth := map[int]*Burst{}
	streams := map[int]map[int]bool{}
	for i := range got {
		b := &got[i]
		j := genieNearest(c.truth, b.Seconds+c.in.delay)
		if j < 0 || byTruth[j] != nil {
			rep.extra = append(rep.extra, b.Seconds)
			continue
		}
		byTruth[j] = b
		w := c.truth[j].who
		if streams[w] == nil {
			streams[w] = map[int]bool{}
		}
		streams[w][b.Stream] = true
	}
	for _, s := range streams {
		rep.split += len(s) - 1
	}
	for j := range c.truth {
		tr := &c.truth[j]
		if !payloadInInput(tr.at - c.in.delay) {
			continue
		}
		f := frontOf(tr.who)
		gb := genieBurst{j: j, gtBits: -1}
		gd, gc, in := genieSlice(f, cal, tr.at, c.r)
		gb.gBits, gb.gOK = genieBits(&gd, &tr.data), in && genieSame(&gd, &tr.data)
		gb.gCach = !tr.hasCACH || gc == tr.cach
		if b := byTruth[j]; b != nil {
			gb.present, gb.stream, gb.slot = true, b.Stream, b.Slot
			data, cach := b.Decoded()
			gb.bits, gb.ok = genieBits(&data, &tr.data), genieSame(&data, &tr.data)
			gb.cachOK = !tr.hasCACH && !b.HasCACH || tr.hasCACH && b.HasCACH && cach == tr.cach
			gb.cachBits = hamming(cach[:], tr.cach[:])
			gb.dt = b.Seconds + c.in.delay - tr.at
			td, _, tin := genieSlice(f, cal, b.Seconds+c.in.delay, c.r)
			gb.gtBits, gb.gtOK = genieBits(&td, &tr.data), tin && genieSame(&td, &tr.data)
		}
		switch {
		case gb.ok && (gb.cachOK || !gb.gCach):
			gb.verdict = "ok"
		case gb.ok:
			gb.verdict = "rx-cach"
		case !gb.gOK:
			gb.verdict = "phys"
		case !gb.present:
			gb.verdict = "rx-miss"
		case !gb.gtOK:
			gb.verdict = "rx-time"
		default:
			gb.verdict = "rx-bits"
		}
		rep.counts[gb.verdict]++
		rep.bursts = append(rep.bursts, gb)
	}
	rep.counts["extra"] = len(rep.extra)
	rep.counts["split"] = rep.split
	return rep
}

func genieNearest(truth []mxTruth, at float64) int {
	j := -1
	for k := range truth {
		if d := math.Abs(truth[k].at - at); d < 0.001 && (j < 0 || d < math.Abs(truth[j].at-at)) {
			j = k
		}
	}
	return j
}

var genieVerdicts = []string{"ok", "phys", "rx-miss", "rx-time", "rx-bits", "rx-cach", "extra", "split"}

func (rep *genieReport) summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "t%d:", rep.tier-Tier0)
	for _, v := range genieVerdicts {
		fmt.Fprintf(&b, " %s %d", v, rep.counts[v])
	}
	if g := rep.reading; g != nil {
		fmt.Fprintf(&b, " | offset %.1f Hz, deviation %.1f Hz, clock %+.1f ppm learned %v", g.offsetHz, g.deviationHz, g.clockPPM, g.clockLearned)
		if g.hasReplay {
			fmt.Fprintf(&b, ", replay stretch %+.1f ppm", g.replayPPM)
		}
	}
	return b.String()
}

func (rep *genieReport) log(t testing.TB, c *genieCase, all bool) {
	t.Logf("GENIE\t%s\t%s", c.name, rep.summary())
	for _, gb := range rep.bursts {
		if gb.verdict == "ok" && !all {
			continue
		}
		tr := &c.truth[gb.j]
		rx := "none"
		if gb.present {
			rx = fmt.Sprintf("%d bits, cach %d, %+.1f us, stream %d slot %d", gb.bits, gb.cachBits, gb.dt*1e6, gb.stream, gb.slot)
		}
		t.Logf("GENIE\t%s\tt%d\t#%d %.4f s talker %d %s\t%s\trx %s\tgenie %d bits ok %v\tgenie at rx time %d bits ok %v", c.name, rep.tier-Tier0, gb.j, tr.at, tr.who, genieKind(&tr.data), gb.verdict, rx, gb.gBits, gb.gOK, gb.gtBits, gb.gtOK)
	}
	for _, s := range rep.extra {
		t.Logf("GENIE\t%s\tt%d\textra burst at %.4f s", c.name, rep.tier-Tier0, s)
	}
}

func mxRowName(name string) mxRow {
	var row mxRow
	d := 0
	for _, p := range strings.Split(name, ".") {
		i := slices.Index(mxValues[min(d, mxDims-1)], p)
		for d >= mxBaseDims && i < 0 && d+1 < mxDims {
			d++
			i = slices.Index(mxValues[d], p)
		}
		if d < mxDims && i >= 0 {
			row[d] = uint8(i)
		}
		d++
	}
	return row
}

func genieIQOf(in mxInput) []complex64 {
	n := len(in.iq) / in.iqFmt.bytes()
	z := make([]complex64, n)
	for i := range z {
		var re, im float64
		if in.iqFmt == IQFormatCU8 {
			re, im = (float64(in.iq[2*i])-128)/127.5, (float64(in.iq[2*i+1])-128)/127.5
		} else {
			re = float64(int16(binary.LittleEndian.Uint16(in.iq[4*i:]))) / 32767
			im = float64(int16(binary.LittleEndian.Uint16(in.iq[4*i+2:]))) / 32767
		}
		z[i] = complex(float32(re), float32(im))
	}
	return z
}

// genieRowCase builds the case of matrix row row, after mod changes its
// signal: a level step, a C/N, a cut.
func genieRowCase(t testing.TB, row mxRow, mod func(s *mxSignal)) (*genieCase, error) {
	for d := range row {
		if int(row[d]) >= len(mxValues[d]) {
			return nil, fmt.Errorf("row %v: no value in dimension %s", row, mxDimName(d))
		}
	}
	s, err := mxBuild(t, row)
	if err != nil {
		return nil, err
	}
	if mod != nil {
		mod(s)
	}
	format := row.val(mxFormat)
	off := s.parts[0].hz
	in := mxInputFor(s, format, row.val(mxLevel), off, row.val(mxDC) != "none")
	c := &genieCase{name: row.name(), format: format, truth: s.truth, r: s.r, gainAt: s.gainAt, in: in, end: s.end}
	for _, p := range s.parts {
		c.hz = append(c.hz, p.hz)
	}
	if in.audio != nil {
		c.g0 = in.gain / s.gain[1]
	} else {
		c.iq = genieIQOf(in)
		if row.val(mxDC) != "none" {
			lo := mxIQLevel(format, "low")
			for i := range c.iq {
				c.iq[i] -= complex(float32(mxDCRe*lo), float32(mxDCIm*lo))
			}
		}
	}
	return c, nil
}

// genieBenchCase builds the case of a BS stream of the bench family: the
// slots at a 0.3 s pad, x their baseband at 48 kHz with the carrier at hz
// and a clock ppm off. mode "iq" receives x as I/Q at 48 kHz, "iqlow" as
// I/Q filtered to 9600 Hz, and "a<rate>" as the audio of an FM radio.
func genieBenchCase(name string, slots []rftx.BSSlot, x []complex64, hz, ppm float64, mode string) *genieCase {
	r := 1 + ppm*1e-6
	ramp := (slotSeconds - burstSeconds) / 2
	truth := make([]mxTruth, 0, len(slots))
	for i, s := range slots {
		at := benchPad + (ramp+float64(i)*slotSeconds+float64(cachSymbols)/symbolRate)/r
		truth = append(truth, mxTruth{at: at, data: s.Burst, cach: s.CACH, hasCACH: true, bs: true, g: i})
	}
	return genieBBCase(name, truth, x, hz, r, mode)
}

// genieBBCase builds the case of the baseband x at 48 kHz that carries
// truth, a talker at hz with clock rate r, received as genieBenchCase does
// by mode.
func genieBBCase(name string, truth []mxTruth, x []complex64, hz, r float64, mode string) *genieCase {
	c := &genieCase{name: name, r: r, hz: []float64{hz}, gainAt: func(float64) float64 { return 1 }, end: float64(len(x)) / basebandRate, truth: truth}
	switch {
	case strings.HasPrefix(mode, "a"):
		rate, _ := strconv.Atoi(strings.TrimPrefix(mode, "a"))
		c.format = mode
		c.in = mxInput{hz: rate, delay: secondsAudioLag / basebandRate, gain: audioGain, audio: discAudio(x, rate)}
		c.g0 = audioGain
	case mode == "iqlow":
		y := lowIQ(x, iq0LowHz)
		c.format, c.iq = "cs16-9600", y
		c.in = mxInput{rate: iq0LowHz, iqFmt: IQFormatCF32}
		c.rx = func(Tier) ([]Burst, *genieReading) {
			o := discOptions()
			o.MidTiming, o.Front96, o.BSSuspectRun, o.RawSearch = true, true, 3, 2
			var r Receiver
			var out []Burst
			r.setLow(RxConfig{Rate: iq0LowHz, Format: IQFormatCF32, OnBursts: func(bs []Burst) { out = append(out, bs...) }}, &o)
			if _, err := r.Write(cf32(y)); err != nil {
				return nil, nil
			}
			if err := r.Flush(); err != nil {
				return nil, nil
			}
			return out, genieRead(r.core(), r.OffsetHz(), r.DeviationHz())
		}
	default:
		c.format, c.iq = "cs16-48k", x
		c.in = mxInput{rate: basebandRate, iqFmt: IQFormatCF32}
		c.rx = func(tier Tier) ([]Burst, *genieReading) {
			r := mustNew(RxConfig{Rate: basebandRate, Tier: tier, Format: IQFormatCU8})
			var out []Burst
			for k := 0; k < len(x); k += 4096 {
				out = append(out, must(r.writeBB(x[k:min(k+4096, len(x))]))...)
			}
			out = append(out, r.flushAll()...)
			return out, genieRead(r.core(), r.OffsetHz(), r.DeviationHz())
		}
	}
	return c
}

// genieIQBytesCase builds the case of I/Q bytes iq of format f at rate that
// carry truth, a talker at hz with clock rate r.
func genieIQBytesCase(name string, iq []byte, f IQFormat, rate int, truth []mxTruth, hz, r float64) *genieCase {
	in := mxInput{iq: iq, rate: rate, iqFmt: f}
	return &genieCase{name: name, format: fmt.Sprintf("iq%d", rate), truth: truth, r: r, hz: []float64{hz}, gainAt: func(float64) float64 { return 1 }, in: in, iq: genieIQOf(in), end: float64(len(iq)/f.bytes()) / float64(rate)}
}

// genieScene is a named case of the harness with the question it answers
// and the tiers it runs.
type genieScene struct {
	name, q string
	tiers   []Tier
	build   func(t testing.TB) ([]*genieCase, error)
}

var allTiers = []Tier{Tier0, Tier1, Tier2, Tier3}

func genieRunScene(t *testing.T, sc genieScene, all bool) map[Tier][]*genieReport {
	cases, err := sc.build(t)
	if err != nil {
		t.Fatal(err)
	}
	out := map[Tier][]*genieReport{}
	for _, c := range cases {
		tiers := sc.tiers
		if c.only != nil {
			tiers = c.only
		}
		for _, tier := range tiers {
			if (c.format == "cs16-9600" || c.format == "cu8-9600") && tier != Tier0 {
				continue
			}
			rep := genieRun(c, tier)
			rep.log(t, c, all)
			out[tier] = append(out[tier], rep)
		}
	}
	return out
}

// TestGenie runs the genie harness: for each scene whose name matches the
// regular expression in DMRGO_GENIE, it runs the receivers of the scene's
// tiers and the reference receiver, which slices every burst on the air at
// its true timing, clock rate, carrier, level, talker and slot, and logs
// for each tier the count of each verdict of genieRun, the receiver's
// readings of offset, deviation and clock rate, and a line for each burst
// that the receiver lost and each extra burst. DMRGO_GENIE_ALL=1 logs the
// bursts the receiver decodes too. DMRGO_GENIE=list lists the scenes and
// their questions. The scenes take the payloads and the noise of the run
// seed, so DMRGO_TESTSEED repeats a run. For example:
//
//	DMRGO_GENIE='^a03' DMRGO_TESTSEED=14539222942668465707 go test -run '^TestGenie$' -v ./rf
func TestGenie(t *testing.T) {
	t.Parallel()
	sel := os.Getenv("DMRGO_GENIE")
	if sel == "" {
		t.Skip("DMRGO_GENIE selects the genie scenes; DMRGO_GENIE=list lists them")
	}
	if sel == "list" {
		for _, sc := range genieScenes() {
			t.Logf("%s\t%s", sc.name, sc.q)
		}
		return
	}
	re, err := regexp.Compile(sel)
	if err != nil {
		t.Fatal(err)
	}
	all := os.Getenv("DMRGO_GENIE_ALL") == "1"
	for _, sc := range genieScenes() {
		if !re.MatchString(sc.name) {
			continue
		}
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("GENIE\t%s\tquestion: %s", sc.name, sc.q)
			genieRunScene(t, sc, all)
		})
	}
}

// TestGenieKnownAnswer checks the reference receiver of the genie harness on
// clean inputs of every kind of front it has: it must slice every burst on
// the air with no bit wrong, so that a burst it loses on a noisy input is
// lost to the noise.
func TestGenieKnownAnswer(t *testing.T) {
	t.Parallel()
	for _, row := range []string{
		"s0.none.+1500.bs.voice.+500ppm.nom.a9600.inf",
		"s0.none.-2500.ms2.data12.-500ppm.nom.a48000.inf",
		"s0.none.+700.bs.data34.0ppm.nom.cs16-48k.inf",
		"gap.none.-1500.ms2.voice.+500ppm.nom.cs16-9600.inf",
	} {
		t.Run(row, func(t *testing.T) {
			t.Parallel()
			c, err := genieRowCase(t, mxRowName(row), nil)
			if err != nil {
				t.Fatal(err)
			}
			f := map[int]*genieFront{}
			cal := c.calibrate(c.front(0))
			n := 0
			for j := range c.truth {
				tr := &c.truth[j]
				if f[tr.who] == nil {
					f[tr.who] = c.front(tr.who)
				}
				if !payloadInInput(tr.at - c.in.delay) {
					continue
				}
				d, cach, ok := genieSlice(f[tr.who], cal, tr.at, c.r)
				if !ok || d != tr.data || tr.hasCACH && cach != tr.cach {
					t.Errorf("burst %d talker %d: %d bits, CACH %d bits", j, tr.who, genieBits(&d, &tr.data), hamming(cach[:], tr.cach[:]))
				}
				n++
			}
			if n < 4 {
				t.Errorf("%d bursts on the air", n)
			}
			t.Logf("%d bursts, delay %.1f us, unit %.1f Hz, DC %.1f Hz", n, cal.tau*1e6, cal.unit, cal.dc)
		})
	}
}
