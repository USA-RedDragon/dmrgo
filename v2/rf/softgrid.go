package rf

// Size of the BPTC(196,96) grid: 13 rows of the Hamming(15,11,3) row code,
// whose columns are codewords of the Hamming(13,9,3) column code.
const (
	gridRows = 13
	gridCols = 15
	// gridStates is the number of syndrome states of a component code.
	gridStates = 16
)

// softGrid decodes the BPTC(196,96) grid soft in, soft out: each row and
// column by the max-log BCJR over the 16 syndrome states of its code, rows
// and columns in turn, so that each passes its extrinsic information to
// the other. softGridPos holds the burst bit of grid cell row r, column
// c, and softGridRowH and softGridColH the syndrome of each position of the
// row and the column code.
type softGrid struct {
	// er and ec hold the row and column extrinsic log-likelihood ratios of
	// the cells, in nats, and in and out the scratch of one component
	// decode.
	er, ec  [gridRows][gridCols]float32
	in, out [gridCols]float32
	alpha   [gridCols + 1][gridStates]float32
}

// component sets g.out[:n] to the extrinsic log-likelihood ratios of a
// codeword of the row code, or of the column code when col, from the a
// priori ratios g.in[:n], by the max-log BCJR over the syndrome states.
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
	// The syndromes are below gridStates, so the masks change no state:
	// they let the compiler drop the bounds checks.
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

// decode runs iters row and column passes over the a priori ratios llr of
// the burst bits, in units of 1/8 nat, positive for 0, and keeps the row
// and column extrinsic ratios of the grid cells. weight scales the
// extrinsic information that one component passes to the other.
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

// posterior sets post at the grid cells to the a priori ratios llr plus
// the extrinsic ratios of the last decode, in units of 1/8 nat.
func (g *softGrid) posterior(llr, post *[softBurstBits]int16) {
	for r := range gridRows {
		for c := range gridCols {
			p := softGridPos[r][c]
			post[p] = satLLR(float32(llr[p])/8 + g.er[r][c] + g.ec[r][c])
		}
	}
}

// extrinsic sets ext to the extrinsic ratios of the last decode, in nats,
// at the grid cells and 0 elsewhere.
func (g *softGrid) extrinsic(ext *[softBurstBits]float32) {
	*ext = [softBurstBits]float32{}
	for r := range gridRows {
		for c := range gridCols {
			ext[softGridPos[r][c]] = g.er[r][c] + g.ec[r][c]
		}
	}
}
