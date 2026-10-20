//go:build !tinygo

package fsktx

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"os"
	"testing"
)

func loadCorrIQ(t *testing.T, path string) (c int, rows [][]float64) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		C   int
		Sym bool
		Hz  []float64
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	n := 1 << (2 * d.C)
	for i := range len(d.Hz) / 10 {
		rows = append(rows, d.Hz[10*i:10*i+10])
	}
	if d.Sym {
		for i := n/2 - 1; i >= 0; i-- {
			r := make([]float64, 10)
			for p, v := range rows[i] {
				r[p] = -v
			}
			rows = append(rows, r)
		}
	}
	if len(rows) != n {
		t.Fatalf("%s: %d rows for %d symbols", path, len(rows), d.C)
	}
	return d.C, rows
}

func corrIQBands(c int, rows [][]float64, split float64) (lo, hi float64) {
	const n = 4096
	rng := rand.New(rand.NewPCG(5, 6))
	codes := make([]int, n/10+c)
	for i := range codes {
		codes[i] = rng.IntN(4)
	}
	x := make([]float64, 0, n)
	for j := 0; len(x) < n; j++ {
		idx := 0
		for _, v := range codes[j : j+c] {
			idx = idx<<2 | v
		}
		x = append(x, rows[idx]...)
	}
	x = x[:n]
	for k := 1; k < n/2; k++ {
		var re, im float64
		for i, v := range x {
			w := v * (0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/n))
			s, co := math.Sincos(-2 * math.Pi * float64(k*i) / n)
			re += w * co
			im += w * s
		}
		if float64(k)*48000/n < split {
			lo += re*re + im*im
		} else {
			hi += re*re + im*im
		}
	}
	return lo, hi
}

func TestCorrIQBandLimited(t *testing.T) {
	t.Parallel()
	c, e := loadCorrIQ(t, "testdata/mcorriq-o51e.json")
	_, b := loadCorrIQ(t, "testdata/mcorriq-o51b.json")
	mask := len(e) - 1
	for i := range b {
		for p := range b[i] {
			if math.Abs(b[i][p]+b[i^mask][p]) > 0.05 {
				t.Fatalf("row %d is not the negation of row %d", i, i^mask)
			}
		}
	}
	pat := [4]int{3, 3, 0, 0}
	for s := range 4 {
		idx := 0
		for i := range c {
			idx = idx<<2 | pat[(s-c+1+i+4*c)%4]
		}
		for _, r := range []int{idx, idx ^ mask} {
			for p := range b[r] {
				if b[r][p] != e[r][p] {
					t.Fatalf("row %d of the ETSI pattern differs", r)
				}
			}
		}
	}
	loE, hiE := corrIQBands(c, e, 14000)
	loB, hiB := corrIQBands(c, b, 14000)
	if hiB > 0.1*hiE {
		t.Errorf("power above 14 kHz %.3g, o51e %.3g", hiB, hiE)
	}
	if r := loB / loE; r < 0.7 || r > 1.4 {
		t.Errorf("power under 14 kHz %.3g of o51e", r)
	}
}
