package rf

import (
	"math"
	"math/bits"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestSoftGridComponent(t *testing.T) {
	t.Parallel()
	d := newSoftDecoder()
	g := &d.grid
	rng := rand.New(rand.NewPCG(3, 4))
	for col, h := range [][]uint8{softGridRowH[:], softGridColH[:]} {
		n := len(h)
		for range 10 {
			for k := range n {
				g.in[k] = float32(rng.NormFloat64() * 3)
			}
			g.component(col == 1)
			for k := range n {
				b0, b1 := math.Inf(-1), math.Inf(-1)
				for w := range 1 << n {
					var s uint8
					var m float64
					for j := range n {
						if w>>j&1 != 0 {
							s ^= h[j]
							m -= float64(g.in[j]) / 2
						} else {
							m += float64(g.in[j]) / 2
						}
					}
					if s != 0 {
						continue
					}
					if w>>k&1 == 0 {
						b0 = math.Max(b0, m)
					} else {
						b1 = math.Max(b1, m)
					}
				}
				want := b0 - b1 - float64(g.in[k])
				if math.Abs(want-float64(g.out[k])) > 1e-3 {
					t.Fatalf("length %d, bit %d: extrinsic %v, enumeration %v", n, k, g.out[k], want)
				}
			}
		}
	}
	n := 0
	for w := range 1 << gridCols {
		var s uint8
		for j := range gridCols {
			if w>>j&1 != 0 {
				s ^= softGridRowH[j]
			}
		}
		if s == 0 {
			n++
		}
	}
	if n != 1<<11 || bits.OnesCount8(softGridColH[0]|softGridColH[1]|softGridColH[2]|softGridColH[3]) == 0 {
		t.Fatalf("row code has %d codewords", n)
	}
}

func TestDecodeBPTCGrid(t *testing.T) {
	t.Parallel()
	d := newSoftDecoder()
	check := softBPTCCheck(enums.DataTypeCSBK)
	rng := rand.New(rand.NewPCG(5, 6))
	osdFails := 0
	for range 50 {
		p := softBPTCPayload(t, rng)
		var r layer2.RawBurst
		r.SetBPTCPayload(p)
		var bits [264]unpacked.Bit
		unpacked.UnpackAt(bits[:], r[:], 0)
		var llr [softBurstBits]int16
		for i, b := range bits {
			llr[i] = int16(80 + rng.IntN(80))
			if b != 0 {
				llr[i] = -llr[i]
			}
		}
		for k := range 4 {
			q := softGridPos[2+3*k][1+4*k]
			llr[q] = -llr[q] * 2
		}
		if got, res := d.decodeBPTCOSD(&llr, check, nil); res.Uncorrectable || got.Payload != p {
			osdFails++
		}
		got, res := d.decodeBPTCGrid(&llr, check, nil)
		if res.Uncorrectable || got.Payload != p {
			t.Fatalf("grid decode of %x: %x, %+v", p, got.Payload, res)
		}
	}
	if osdFails == 0 {
		t.Fatal("ordered statistics corrected every block")
	}
}
