//go:build !tinygo

package fsktx

import (
	"math"
	"math/rand"
	"os"
	"testing"
)

func altWelch(x []complex128, rate float64) float64 {
	n := 2048
	w := make([]float64, n)
	var wsum float64
	for i := range w {
		t := 2 * math.Pi * float64(i) / float64(n)
		w[i] = 0.35875 - 0.48829*math.Cos(t) + 0.14128*math.Cos(2*t) - 0.01168*math.Cos(3*t)
		wsum += w[i] * w[i]
	}
	avg := make([]float64, n)
	buf := make([]complex128, n)
	var pw float64
	for _, v := range x {
		pw += real(v)*real(v) + imag(v)*imag(v)
	}
	pw /= float64(len(x))
	frames := 0
	for s := 0; s+n <= len(x); s += n / 4 {
		for i := range buf {
			buf[i] = x[s+i] * complex(w[i], 0)
		}
		fft(buf)
		for i, v := range buf {
			avg[i] += (real(v)*real(v) + imag(v)*imag(v)) / (float64(n) * wsum) / pw
		}
		frames++
	}
	band := func(lo, hi float64) float64 {
		var p float64
		for i, v := range avg {
			j := i
			if j >= n/2 {
				j -= n
			}
			if f := float64(j) * rate / float64(n); f >= lo && f <= hi {
				p += v
			}
		}
		return p / float64(frames)
	}
	return 10 * math.Log10(max(band(20750, 29250), band(-29250, -20750)))
}

func TestAltSplit(t *testing.T) {
	if os.Getenv("ALTSPLIT") == "" {
		t.Skip()
	}
	const rate = 96000
	var m MCUIQ
	if err := m.Init(rate, 0, 96); err != nil {
		t.Fatal(err)
	}
	m.Key(true)
	r := rand.New(rand.NewSource(1))
	syms := []int8{-3, -1, 1, 3}
	const nsym, skip = 120000, 200
	ns := m.Samples()
	dst := make([]int16, 2*ns)
	var outI []complex128
	var ws []float64
	var vs []int32
	for k := range nsym {
		m.Symbol(dst, syms[r.Intn(4)])
		if k < skip {
			continue
		}
		for i := 0; i < len(dst); i += 2 {
			outI = append(outI, complex(float64(dst[i]), float64(dst[i+1])))
		}
		prev := m.prev
		_ = prev
		for _, x := range m.buf {
			ws = append(ws, float64(x))
		}
	}
	t.Logf("A full int16 MCUIQ: %.2f dBc", altWelch(outI, rate))
	vs = vs[:0]
	prev := int32(ws[0])
	for _, wf := range ws[1:] {
		w := int32(wf)
		dw := (w - prev) >> 1
		v := prev + dw
		if tier0Exact {
			v -= dw >> 1
		}
		vs = append(vs, v, v+dw)
		prev = w
	}
	fm := func(inc []float64, q func(float64) float64) []complex128 {
		out := make([]complex128, len(inc))
		ph := 0.0
		for i, v := range inc {
			ph += v
			a := 2 * math.Pi * ph / (1 << 32)
			out[i] = complex(q(32600*math.Cos(a)), q(32600*math.Sin(a)))
		}
		return out
	}
	id := func(x float64) float64 { return x }
	rnd := math.Round
	inc := make([]float64, len(vs))
	for i, v := range vs {
		inc[i] = float64(v)
	}
	for _, bits := range []int{10, 11, 12} {
		N := 1 << bits
		q := N / 4
		T := make([]int64, N+q+1)
		for k := range T {
			T[k] = int64(math.Round(32600*65536*math.Cos(2*math.Pi*float64(k-q)/float64(N)))) + 32768
		}
		C := int64(math.Round(2 * math.Pi / float64(N) * (1 << 32)))
		out := make([]complex128, len(vs))
		var ph uint32
		for i, v := range vs {
			ph += uint32(v)
			j := ph >> (32 - bits)
			fr := int64(uint64(ph<<bits) * uint64(C) >> 32)
			c, sn := T[j+uint32(q)], T[j]
			co := c - (sn*fr)>>32
			si := sn + (c*fr)>>32
			out[i] = complex(float64(int16(co>>16)), float64(int16(si>>16)))
		}
		t.Logf("Q16 table %d bits: %.2f dBc", bits, altWelch(out, rate))
	}
	t.Logf("B linear interp, exact cos, float: %.2f dBc", altWelch(fm(inc, id), rate))
	t.Logf("B2 linear interp, exact cos, int16 round: %.2f dBc", altWelch(fm(inc, rnd), rate))
	n := 1
	for n < len(ws) {
		n <<= 1
	}
	X := make([]complex128, n)
	for i, v := range ws {
		X[i] = complex(v, 0)
	}
	fft(X)
	up := make([]complex128, 4*n)
	for i := range n / 2 {
		up[i] = X[i]
		up[4*n-n/2+i] = X[n/2+i]
	}
	up[n/2] = X[n/2] / 2
	up[4*n-n/2] = X[n/2] / 2
	for i := range up {
		up[i] = complex(real(up[i]), -imag(up[i]))
	}
	fft(up)
	ideal := make([]float64, 0, 2*len(ws))
	for i := 1; i < len(ws)-1; i++ {
		ideal = append(ideal, real(up[4*(i-1)+1])/float64(n), real(up[4*(i-1)+3])/float64(n))
	}
	t.Logf("C ideal interp, exact cos, float: %.2f dBc", altWelch(fm(ideal, id), rate))
	t.Logf("C2 ideal interp, int16 round: %.2f dBc", altWelch(fm(ideal, rnd), rate))
	diff := make([]float64, len(ideal))
	for i := range ideal {
		diff[i] = inc[i] - ideal[i]
	}
	var de, dt float64
	for i := range diff {
		de += diff[i] * diff[i]
		dt += ideal[i] * ideal[i]
	}
	t.Logf("interp error rms %.3g of rms %.3g (inc units)", math.Sqrt(de/float64(len(diff))), math.Sqrt(dt/float64(len(diff))))
}
