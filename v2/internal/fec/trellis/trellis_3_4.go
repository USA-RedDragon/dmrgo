// Package trellis encodes and decodes the rate 3/4 trellis code that
// protects rate 3/4 data blocks (ETSI TS 102 361-1 clause B.2.4).
package trellis

import (
	"math"
	mathbits "math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
)

func dibitsToBits(dibits [98]int8) [196]bit.Bit {
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

// Decode decodes 196 received bits into 144 data bits with a hard-decision
// Viterbi decoder whose path ends in state 0. ErrorsCorrected counts the
// received bits that differ from the decoded path. Uncorrectable is set only
// when a path that ends in another state has a lower metric, so callers check
// the block CRC-9 or the message CRC-32 to detect a miscorrected block.
func Decode(bits [196]bit.Bit) ([144]bit.Bit, fec.Result) {
	lowbit.Mask(bits[:])
	result := fec.Result{BitsChecked: 196}

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

	return tribitsToBits(tribits), result
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

func tribitsToBits(tribits [49]byte) [144]bit.Bit {
	var bits [144]bit.Bit

	for o := range 48 {
		tb := tribits[o]
		bits[3*o] = bit.Bit((tb >> 2) & 1)
		bits[3*o+1] = bit.Bit((tb >> 1) & 1)
		bits[3*o+2] = bit.Bit(tb & 1)
	}

	return bits
}

// Encode encodes 144 data bits with the rate 3/4 trellis code and a tail
// tribit of 0 into 196 interleaved bits.
func Encode(data [144]bit.Bit) [196]bit.Bit {
	lowbit.Mask(data[:])
	var tribits [49]byte
	for i := 0; i < 48; i++ {
		o := i * 3
		tribits[i] = (byte(data[o]) << 2) | (byte(data[o+1]) << 1) | byte(data[o+2])
	}
	tribits[48] = 0

	var points [49]byte
	state := byte(0)
	for i := 0; i < 49; i++ {
		points[i] = encoder_state_transition_table[state*8+tribits[i]]
		state = tribits[i]
	}

	var dibits [98]int8
	for i := 0; i < 49; i++ {
		dibits[i*2] = trellis34_reverse_constellation[points[i]][0]
		dibits[i*2+1] = trellis34_reverse_constellation[points[i]][1]
	}

	var interleavedDibits [98]int8
	for i := 0; i < 98; i++ {
		interleavedDibits[i] = dibits[interleave_matrix[i]]
	}

	return dibitsToBits(interleavedDibits)
}
