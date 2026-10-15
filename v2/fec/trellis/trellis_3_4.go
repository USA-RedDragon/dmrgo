package trellis

import (
	"math"
	mathbits "math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

type Trellis34 struct {
}

func New() *Trellis34 {
	return &Trellis34{}
}

func (t *Trellis34) dibitsToBits(dibits [98]int8) [196]bit.Bit {
	var bits [196]bit.Bit

	for i := 0; i < 98; i++ {
		o := i * 2 //nolint:gosec // o ranges [0,194], o+1 ranges [1,195], both < 196
		switch dibits[i] {
		case 3:
			bits[o] = 0   //nolint:gosec // see above: o < 196
			bits[o+1] = 1 //nolint:gosec // see above: o+1 < 196
		case 1:
			bits[o] = 0   //nolint:gosec // see above: o < 196
			bits[o+1] = 0 //nolint:gosec // see above: o+1 < 196
		case -1:
			bits[o] = 1   //nolint:gosec // see above: o < 196
			bits[o+1] = 0 //nolint:gosec // see above: o+1 < 196
		case -3:
			bits[o] = 1   //nolint:gosec // see above: o < 196
			bits[o+1] = 1 //nolint:gosec // see above: o+1 < 196
		}
	}

	return bits
}

// Decode runs a hard-decision Viterbi decoder over the 49 received
// constellation points. The branch metric is the Hamming distance between the
// four received bits of a point and the four bits of the expected point. The
// path is forced to end in state 0 because the encoder appends a 000 tribit
// (ETSI TS 102 361-1 B.2.4).
//
// ErrorsCorrected is the number of received bits that differ from the
// re-encoded decoded path. Uncorrectable is set when a path ending in a
// nonzero state has a strictly lower metric than the best path ending in
// state 0. That is the only check: the trellis code has no other redundancy
// to detect a miscorrection, so a false Uncorrectable does not mean the data
// is right. Callers must check the block CRC (the CRC-9 of a confirmed block
// or the message CRC-32) to detect a miscorrected block.
func (t *Trellis34) Decode(bits [196]bit.Bit) ([144]bit.Bit, fec.FECResult) {
	bit.Mask(bits[:])
	result := fec.FECResult{BitsChecked: 196}

	var rxDibits [98]byte
	for i, j := range interleave_matrix {
		rxDibits[j] = byte(bits[2*i])<<1 | byte(bits[2*i+1])
	}

	var pointBits [16]byte
	for p, pair := range trellis34_reverse_constellation {
		pointBits[p] = dibitBits(pair[0])<<2 | dibitBits(pair[1])
	}

	const unreachable = math.MaxInt
	metric := [8]int{0, unreachable, unreachable, unreachable, unreachable, unreachable, unreachable, unreachable}
	var prev [49][8]byte

	for k := range prev {
		rx := rxDibits[2*k]<<2 | rxDibits[2*k+1]
		next := [8]int{unreachable, unreachable, unreachable, unreachable, unreachable, unreachable, unreachable, unreachable}
		for s := range metric {
			if metric[s] == unreachable {
				continue
			}
			for in := range next {
				point := encoder_state_transition_table[s*8+in]
				m := metric[s] + mathbits.OnesCount8(rx^pointBits[point])
				if m < next[in] {
					next[in] = m
					prev[k][in] = byte(s) //nolint:gosec // s < 8
				}
			}
		}
		metric = next
	}

	for s := 1; s < len(metric); s++ {
		if metric[s] < metric[0] {
			result.Uncorrectable = true
			break
		}
	}
	result.ErrorsCorrected = metric[0]

	var tribits [49]byte
	state := byte(0)
	for k := len(prev) - 1; k >= 0; k-- {
		tribits[k] = state
		state = prev[k][state]
	}

	return t.tribitsToBits(tribits), result
}

// dibitBits maps a ±1/±3 dibit symbol to its two transmitted bits.
func dibitBits(d int8) byte {
	switch d {
	case 3:
		return 0b01
	case -1:
		return 0b10
	case -3:
		return 0b11
	default:
		return 0b00
	}
}

func (t *Trellis34) tribitsToBits(tribits [49]byte) [144]bit.Bit {
	var bits [144]bit.Bit

	for o := range 48 {
		tb := tribits[o]
		bits[3*o] = bit.Bit((tb >> 2) & 1)
		bits[3*o+1] = bit.Bit((tb >> 1) & 1)
		bits[3*o+2] = bit.Bit(tb & 1)
	}

	return bits
}

// Encode encodes 144 data bits using Trellis 3/4 rate coding, producing 196 interleaved bits.
func (t *Trellis34) Encode(data [144]bit.Bit) [196]bit.Bit {
	bit.Mask(data[:])
	// Step 1: Data bits to tribits (48 data tribits + 1 tail)
	var tribits [49]byte
	for i := 0; i < 48; i++ {
		o := i * 3
		tribits[i] = (byte(data[o]) << 2) | (byte(data[o+1]) << 1) | byte(data[o+2])
	}
	tribits[48] = 0 // tail tribit to flush encoder back to state 0

	// Step 2: Tribits to constellation points via encoder state machine
	var points [49]byte
	state := byte(0)
	for i := 0; i < 49; i++ {
		points[i] = encoder_state_transition_table[state*8+tribits[i]]
		state = tribits[i]
	}

	// Step 3: Constellation points to dibits
	var dibits [98]int8
	for i := 0; i < 49; i++ {
		dibits[i*2] = trellis34_reverse_constellation[points[i]][0]
		dibits[i*2+1] = trellis34_reverse_constellation[points[i]][1]
	}

	// Step 4: Interleave (reverse of deinterleave)
	var interleavedDibits [98]int8
	for i := 0; i < 98; i++ {
		interleavedDibits[i] = dibits[interleave_matrix[i]]
	}

	// Step 5: Dibits to bits
	return t.dibitsToBits(interleavedDibits)
}
