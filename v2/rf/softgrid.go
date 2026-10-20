package rf

const (
	gridRows   = 13
	gridCols   = 15
	gridStates = 16
)

type softGrid struct {
	er, ec  [gridRows][gridCols]float32
	in, out [gridCols]float32
	alpha   [gridCols + 1][gridStates]float32
}

func (g *softGrid) component(col bool) {
	h := softGridRowH[:]
	if col {
		h = softGridColH[:]
	}
	in, out := g.in[:len(h)], g.out[:len(h)]
	a := &g.alpha
	for s := range gridStates {
		a[0][s] = negInf
	}
	a[0][0] = 0
	for k, x := range h {
		m := in[k] / 2
		ak, an := &a[k], &a[k+1]
		for s := range gridStates {
			an[s] = max(ak[s]+m, ak[(uint8(s)^x)&(gridStates-1)]-m)
		}
	}
	var beta, nb [gridStates]float32
	for s := range beta {
		beta[s] = negInf
	}
	beta[0] = 0
	for k := len(h) - 1; k >= 0; k-- {
		m := in[k] / 2
		x := h[k]
		ak := &a[k]
		b0, b1 := negInf, negInf
		for s := range gridStates {
			t := (uint8(s) ^ x) & (gridStates - 1)
			b0 = max(b0, ak[s]+m+beta[s])
			b1 = max(b1, ak[s]-m+beta[t])
			nb[s] = max(beta[s]+m, beta[t]-m)
		}
		out[k] = b0 - b1 - in[k]
		beta = nb
	}
}

func (g *softGrid) decode(llr *[softBurstBits]int16, iters int, weight float32) {
	g.er, g.ec = [gridRows][gridCols]float32{}, [gridRows][gridCols]float32{}
	for range iters {
		for r := range gridRows {
			for c := range gridCols {
				g.in[c] = float32(llr[softGridPos[r][c]])/8 + weight*g.ec[r][c]
			}
			g.component(false)
			copy(g.er[r][:], g.out[:])
		}
		for c := range gridCols {
			for r := range gridRows {
				g.in[r] = float32(llr[softGridPos[r][c]])/8 + weight*g.er[r][c]
			}
			g.component(true)
			for r := range gridRows {
				g.ec[r][c] = g.out[r]
			}
		}
	}
}

func (g *softGrid) posterior(llr, post *[softBurstBits]int16) {
	for r := range gridRows {
		for c := range gridCols {
			p := softGridPos[r][c]
			post[p] = satLLR(float32(llr[p])/8 + g.er[r][c] + g.ec[r][c])
		}
	}
}

func (g *softGrid) extrinsic(ext *[softBurstBits]float32) {
	*ext = [softBurstBits]float32{}
	for r := range gridRows {
		for c := range gridCols {
			ext[softGridPos[r][c]] = g.er[r][c] + g.ec[r][c]
		}
	}
}
