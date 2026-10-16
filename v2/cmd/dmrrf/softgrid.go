package main

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// Size of the BPTC(196,96) grid: 13 rows of the Hamming(15,11,3) row code,
// whose columns are codewords of the Hamming(13,9,3) column code.
const (
	gridRows = 13
	gridCols = 15
	// gridStates is the number of syndrome states of a component code.
	gridStates = 16
)

// gridChecks returns the columns that each parity check of the row code
// covers, and the rows that each check of the column code covers (ETSI TS
// 102 361-1 clause B.1.1).
func gridChecks() (row, col [4]uint16) {
	row = [4]uint16{
		1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<5 | 1<<7 | 1<<8 | 1<<11,
		1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<6 | 1<<8 | 1<<9 | 1<<12,
		1<<2 | 1<<3 | 1<<4 | 1<<5 | 1<<7 | 1<<9 | 1<<10 | 1<<13,
		1<<0 | 1<<1 | 1<<2 | 1<<4 | 1<<6 | 1<<7 | 1<<10 | 1<<14,
	}
	col = [4]uint16{
		1<<0 | 1<<1 | 1<<3 | 1<<5 | 1<<6 | 1<<9,
		1<<0 | 1<<1 | 1<<2 | 1<<4 | 1<<6 | 1<<7 | 1<<10,
		1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<5 | 1<<7 | 1<<8 | 1<<11,
		1<<0 | 1<<2 | 1<<4 | 1<<5 | 1<<8 | 1<<12,
	}
	return row, col
}

// softGrid decodes the BPTC(196,96) grid soft in, soft out: each row and
// column by the max-log BCJR over the 16 syndrome states of its code, rows
// and columns in turn, so that each passes its extrinsic information to
// the other.
type softGrid struct {
	// pos holds the burst bit of grid cell row r, column c.
	pos [gridRows][gridCols]int16
	// rowH and colH hold the syndrome of each position of the row and the
	// column code.
	rowH [gridCols]uint8
	colH [gridRows]uint8
	// er and ec hold the row and column extrinsic log-likelihood ratios of
	// the cells, in nats, and in and out the scratch of one component
	// decode.
	er, ec  [gridRows][gridCols]float32
	in, out [gridCols]float32
	alpha   [gridCols + 1][gridStates]float32
}

// newSoftGrid lays out the grid on the burst bits info of the 196
// information bits: grid cell i, from 1, is information bit 181i mod 196.
// It checks the layout against the codewords of code.
func newSoftGrid(info *[softTrellisBits]int16, code *softOSDCode) (*softGrid, error) {
	g := &softGrid{}
	for r := range gridRows {
		for c := range gridCols {
			g.pos[r][c] = info[(1+gridCols*r+c)*181%softTrellisBits]
		}
	}
	rowChecks, colChecks := gridChecks()
	for k, m := range rowChecks {
		for c := range gridCols {
			if m>>c&1 != 0 {
				g.rowH[c] |= 1 << k
			}
		}
	}
	for k, m := range colChecks {
		for r := range gridRows {
			if m>>r&1 != 0 {
				g.colH[r] |= 1 << k
			}
		}
	}
	var img [softBurstBits]unpacked.Bit
	for i := range code.k {
		clear(img[:])
		for j := range code.n {
			if softBit(&code.rows[i], j) {
				img[code.pos[j]] = 1
			}
		}
		if !g.isCodeword(&img) {
			return nil, fmt.Errorf("%w: BPTC(196,96) grid layout", errSoftDerive)
		}
	}
	return g, nil
}

// isCodeword reports whether every row and column of the grid in img has a
// zero syndrome.
func (g *softGrid) isCodeword(img *[softBurstBits]unpacked.Bit) bool {
	var col [gridCols]uint8
	for r := range gridRows {
		var s uint8
		for c := range gridCols {
			if img[g.pos[r][c]] != 0 {
				s ^= g.rowH[c]
				col[c] ^= g.colH[r]
			}
		}
		if s != 0 {
			return false
		}
	}
	for _, s := range col {
		if s != 0 {
			return false
		}
	}
	return true
}

// component sets g.out[:n] to the extrinsic log-likelihood ratios of a
// codeword of a component code with syndromes h from the a priori ratios
// g.in[:n], by the max-log BCJR over the syndrome states.
func (g *softGrid) component(h []uint8) {
	n := len(h)
	a := &g.alpha
	for s := range gridStates {
		a[0][s] = negInf
	}
	a[0][0] = 0
	for k := range n {
		m := g.in[k] / 2
		x := h[k]
		for s := range gridStates {
			a[k+1][s] = max(a[k][s]+m, a[k][uint8(s)^x]-m)
		}
	}
	var beta, nb [gridStates]float32
	for s := range beta {
		beta[s] = negInf
	}
	beta[0] = 0
	for k := n - 1; k >= 0; k-- {
		m := g.in[k] / 2
		x := h[k]
		b0, b1 := negInf, negInf
		for s := range gridStates {
			t := uint8(s) ^ x
			b0 = max(b0, a[k][s]+m+beta[s])
			b1 = max(b1, a[k][s]-m+beta[t])
			nb[s] = max(beta[s]+m, beta[t]-m)
		}
		g.out[k] = b0 - b1 - g.in[k]
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
				g.in[c] = float32(llr[g.pos[r][c]])/8 + weight*g.ec[r][c]
			}
			g.component(g.rowH[:])
			copy(g.er[r][:], g.out[:])
		}
		for c := range gridCols {
			for r := range gridRows {
				g.in[r] = float32(llr[g.pos[r][c]])/8 + weight*g.er[r][c]
			}
			g.component(g.colH[:])
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
			p := g.pos[r][c]
			post[p] = satLLR(float32(llr[p])/8 + g.er[r][c] + g.ec[r][c])
		}
	}
}
