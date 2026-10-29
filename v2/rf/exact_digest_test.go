package rf

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"
)

type xtCase struct {
	np        int
	logmap    bool
	cn        float64
	kappa     float32
	rot       float64
	jump      float32
	fade      bool
	below     float32
	drifts    int
	priors    bool
	slip, cut bool
}

func xtInput(m *cpmModel, steps int, c xtCase, seed uint64) (x []complex64, scale float32, lp, slp []float32) {
	rng := rand.New(rand.NewPCG(seed, 7))
	sps := m.sps
	dig := make([]int, steps+2*m.d)
	for i := range dig {
		dig[i] = rng.IntN(4)
	}
	sigma := math.Sqrt(math.Pow(10, -c.cn/10) * float64(sps) / 2)
	x = make([]complex64, steps*sps)
	theta := 0.0
	for k := range steps {
		w := 0
		for i := range 2*m.d + 1 {
			w = w<<2 | dig[k+i]
		}
		if c.slip && k == steps/3 {
			theta += 1.3
		}
		g := 1.0
		if c.cut && k > steps/2 && k < steps/2+20 {
			g = 0.05
		}
		rot := cmplx.Exp(complex(0, theta+c.rot))
		for j := range sps {
			v := complex128(m.tpl[w*sps+j])
			s := complex(g, 0)*cmplx.Conj(v)*rot + complex(sigma*rng.NormFloat64(), sigma*rng.NormFloat64())
			x[k*sps+j] = complex64(s)
		}
		theta += m.h * math.Pi * float64(2*dig[k]-3)
	}
	scale = float32(2 / (2 * sigma * sigma))
	lp = make([]float32, 4*steps)
	slp = make([]float32, 8*m.d)
	if c.priors {
		for i := range lp {
			lp[i] = float32(rng.Float64()*2 - 1)
		}
		for i := range slp {
			slp[i] = float32(rng.Float64()*4 - 2)
		}
	}
	return x, scale, lp, slp
}

func xtCases() []xtCase {
	out := make([]xtCase, 0, 32)
	for _, np := range []int{52, phaseStates} {
		for _, logmap := range []bool{true, false} {
			for _, cn := range []float64{30, 12, 6, 2} {
				out = append(out, xtCase{np: np, logmap: logmap, cn: cn, drifts: 1})
			}
			out = append(out,
				xtCase{np: np, logmap: logmap, cn: 10, kappa: anchorKappa, rot: 0.4, priors: true, drifts: 1},
				xtCase{np: np, logmap: logmap, cn: 8, rot: 2.1, slip: true, jump: 4, drifts: 2},
				xtCase{np: np, logmap: logmap, cn: 8, cut: true, jump: 3, fade: true, drifts: 1},
				xtCase{np: np, logmap: logmap, cn: 8, cut: true, jump: 3, fade: true, below: 0.5, priors: true, drifts: 1},
			)
		}
	}
	return out
}

func xtDigest() string {
	m := newCPMModel(1, 10)
	const steps = exactGapSlots * slotSymbols
	h := sha256.New()
	var b [4]byte
	for i, c := range xtCases() {
		xt := newXTrellis(m, c.np, steps)
		xt.logmap, xt.jump, xt.fade, xt.below, xt.drifts = c.logmap, c.jump, c.fade, c.below, c.drifts
		x, scale, lp, slp := xtInput(m, steps, c, uint64(i)+1)
		copy(xt.lp, lp)
		copy(xt.slp, slp)
		app := make([]float32, 4*steps)
		for _, n := range []int{steps, steps / 2, 2*slotSymbols + 7} {
			xt.run(x, n, 1, scale, c.kappa, app)
			for _, v := range app[:4*n] {
				binary.LittleEndian.PutUint32(b[:], math.Float32bits(v))
				h.Write(b[:])
			}
			for _, v := range xt.beta {
				binary.LittleEndian.PutUint32(b[:], math.Float32bits(v))
				h.Write(b[:])
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

const xtWantDigest = "6a24ccdfa7b80f0db8a4db763546e9bfca8deba9007a39757bfae3933fa30e2b"

func TestExactTrellisDigest(t *testing.T) {
	t.Parallel()
	if got := xtDigest(); got != xtWantDigest {
		t.Errorf("xtrellis digest %s, want %s", got, xtWantDigest)
	}
}

func tier3Digest(t testing.TB) string {
	streams := rxStreams(t)
	c := rxConds()[0]
	h := sha256.New()
	for _, sn := range []string{"bs", "ms"} {
		for _, cn := range []float64{40, 14, 10, 7} {
			in := rxBuild(streams[sn], c, cn, false, rxSeed(sn+"digest"))
			for _, b := range receiveBaseband(in.x, Tier3) {
				fmt.Fprintf(h, "%s %g %x %x %d %v %v %x %v %v %d %d %v\n", sn, cn, b.Data, b.Fixed, b.Distance, b.Sync, b.Voice,
					b.CACH, b.HasCACH, b.BS, b.Stream, b.Slot, b.HasFixed)
				fmt.Fprintf(h, "%x %x\n", math.Float64bits(b.Seconds), b.FixedCACH)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

const tier3WantDigest = "9fcc50db1183ef4da57eeca8718158283eaaef036df321724ac64f424a2acc26"

func TestTier3Digest(t *testing.T) {
	t.Parallel()
	if got := tier3Digest(t); got != tier3WantDigest {
		t.Errorf("Tier3 digest %s, want %s", got, tier3WantDigest)
	}
}

func BenchmarkExactTrellis(b *testing.B) {
	m := newCPMModel(1, 10)
	const steps = exactGapSlots * slotSymbols
	for _, c := range []xtCase{{np: 52, logmap: true, cn: 10, drifts: 1}, {np: 52, logmap: true, cn: 10, kappa: anchorKappa, priors: true, drifts: 1}} {
		b.Run(fmt.Sprintf("kappa=%g", c.kappa), func(b *testing.B) {
			xt := newXTrellis(m, c.np, steps)
			xt.logmap, xt.drifts = c.logmap, c.drifts
			x, scale, lp, slp := xtInput(m, steps, c, 99)
			copy(xt.lp, lp)
			copy(xt.slp, slp)
			app := make([]float32, 4*steps)
			b.ReportAllocs()
			for b.Loop() {
				xt.run(x, steps, 1, scale, c.kappa, app)
			}
		})
	}
}
