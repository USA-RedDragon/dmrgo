//go:build !tinygo

package fsktx

import (
	"bytes"
	"fmt"
	"math"
)

func preEmphPulse() (g []float64, n, taps int) {
	n, taps = len(taps24k), len(taps24k[0])
	const nfft = 1 << 16
	x := make([]complex128, nfft)
	for p := range n {
		for i := range taps {
			x[i*n+p] = complex(float64(taps24k[p][i]), 0)
		}
	}
	fft(x)
	for i := range x {
		k := i
		if k > nfft/2 {
			k -= nfft
		}
		f := float64(k) * 24000 / nfft
		x[i] *= (1 + complex(0, f/3000)) / (1 + complex(0, f/300))
	}
	ifft(x)
	g = make([]float64, nfft)
	for i := range g {
		g[i] = real(x[i])
	}
	return g, n, taps
}

func renderPreEmph(b *bytes.Buffer) {
	g, n, taps := preEmphPulse()
	l := n * taps
	rho := math.Pow(g[l+5*n]/g[l+n], 1/float64(4*n))
	head := g[l]
	fmt.Fprintf(b, "var taps24kPre = [%d][%d]int16{\n", n, taps)
	var peak int32
	for p := range n {
		b.WriteString("{")
		var s int32
		for i := range taps {
			v := int16(math.Round(g[i*n+p]))
			s += int32(math.Abs(float64(v)))
			fmt.Fprintf(b, "%d,", v)
		}
		peak = max(peak, s)
		b.WriteString("},\n")
	}
	b.WriteString("}\n\n")
	peak += int32(math.Ceil(3 * math.Abs(head) / (1 - math.Pow(rho, float64(n)))))
	fmt.Fprintf(b, "const (\ntaps24kPrePeak = %d\ntail24kG = %d\ntail24kRN = %d\n)\n\n", peak, int32(math.Round(head*256)), int32(math.Round(math.Pow(rho, float64(n))*(1<<30))))
	fmt.Fprintf(b, "var tail24kRP = [%d]int32{", n)
	for p := range n {
		fmt.Fprintf(b, "%d,", int32(math.Round(math.Pow(rho, float64(p))*(1<<30))))
	}
	b.WriteString("}\n\n")
}
