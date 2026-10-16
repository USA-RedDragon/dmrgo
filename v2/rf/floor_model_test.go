package rf

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// floorModel is the genie trellis metric of one symbol path over the span
// of the free bits of a slot: the log-likelihood that the trellis gives the
// path, up to a constant, with the frequency pulse of the free symbols cut
// to the window as the trellis cuts it. The searches of the DMR floor rank
// candidates by it and then check each with the exact likelihood.
type floorModel struct {
	n0, d, steps, nw, p0 int
	// corr holds the correlation of window w at step k at k*nw+w.
	corr []complex64
	// tail holds the metric of the samples after the span for each carrier
	// phase state that a path ends at.
	tail [floorPhases]float32
	g    *genieTrellis
}

// setModel sets m to the model of the free bits of slot i.
func (e *floorEval) setModel(i int, m *floorModel) {
	g, f := e.g, e.f
	n0, n1, mask := e.span(i)
	g.f, g.n0, g.mask = f, n0, mask
	g.correct(n1)
	d := g.d
	m.n0, m.d, m.steps, m.nw, m.g = n0, d, n1-n0, 1<<(4*d+2), g
	m.corr = append(m.corr[:0], make([]complex64, m.steps*m.nw)...)
	for k := range m.steps {
		ws := g.windows(n0 + k)
		g.correlate(n0+k, ws)
		for _, w := range ws {
			m.corr[k*m.nw+w] = g.corr[w]
		}
	}
	m.p0 = f.phaseBefore(n0 - d)
	pEnd := f.phaseBefore(n1 - d)
	var zt complex128
	if e.tail {
		zt = (f.zp[len(f.zv)] - f.zp[f.center(n1)-samplesPerSy/2]) * complex(f.scale, 0)
	}
	for p := range floorPhases {
		dp := float64(p-pEnd) * 2 * math.Pi / floorPhases
		m.tail[p] = float32(real(zt * (cmplx.Exp(complex(0, dp)) - 1)))
	}
}

// metric returns the model metric of the stream symbols sym.
func (m *floorModel) metric(sym []int8) float64 {
	d, g := m.d, m.g
	w := 0
	for n := m.n0 - d; n < m.n0+d; n++ {
		w = w<<2 | digitOf(sym[n])
	}
	p := m.p0
	var acc float32
	mask := m.nw - 1
	for k := range m.steps {
		n := m.n0 + k
		w = (w<<2 | digitOf(sym[n+d])) & mask
		c := m.corr[k*m.nw+w]
		acc += real(c)*g.cos[p] + imag(c)*g.sin[p]
		p = wrapPhase(p + 27*int(sym[n-d]))
	}
	return float64(acc + m.tail[p])
}

// dibitSymbol returns the symbol value of the dibit b0, b1.
func dibitSymbol(b0, b1 uint8) int8 {
	v := int8(1)
	if b1 != 0 {
		v = 3
	}
	if b0 != 0 {
		v = -v
	}
	return v
}

// TestFloorModel checks the model metric of random rate 3/4 bursts
// against the exact likelihood change: within 5% and 3 nats.
func TestFloorModel(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 2)
	e := newFloorEval(1)
	e.f = newFloorSignal(slots, 2, 3)
	e.slots, e.truth = slots, truth
	rng := rand.New(rand.NewPCG(3, 4))
	var m floorModel
	sym := make([]int8, len(e.f.sym))
	checked := 0
	for i, ts := range truth {
		if ts.kind != "data" || ts.dt != enums.DataTypeRate34 {
			continue
		}
		e.setFree((&softTrellisTables).pos[:])
		e.setModel(i, &m)
		base := m.metric(e.f.sym)
		for range 5 {
			cand := ts.burst
			for range 1 + rng.IntN(4) {
				p := (&softTrellisTables).pos[rng.IntN(len((&softTrellisTables).pos))]
				cand[p/8] ^= 0x80 >> (p % 8)
			}
			copy(sym, e.f.sym)
			for k, v := range rftx.SymbolsFromBurst(cand) {
				sym[i*slotSymbols+cachSymbols+k] = int8(v)
			}
			e.ch = e.f.burstChanges(e.ch[:0], i, &cand)
			got, want := m.metric(sym)-base, e.f.delta(e.ch, e.tail)
			if math.Abs(got-want) > 3+0.05*math.Abs(want) {
				t.Errorf("slot %d: model %.2f, exact %.2f", i, got, want)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no rate 3/4 burst")
	}
}
