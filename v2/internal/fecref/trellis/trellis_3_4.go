// Package trellis encodes the rate 3/4 trellis code that
// protects rate 3/4 data blocks (ETSI TS 102 361-1 clause B.2.4).
package trellis

import "github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"

func dibitsToBits(dibits [98]int8) [196]unpacked.Bit {
	var bits [196]unpacked.Bit

	for i := 0; i < 98; i++ {
		o := i * 2
		switch dibits[i] {
		case 3:
			bits[o] = 0
			bits[o+1] = 1
		case 1:
			bits[o] = 0
			bits[o+1] = 0
		case -1:
			bits[o] = 1
			bits[o+1] = 0
		case -3:
			bits[o] = 1
			bits[o+1] = 1
		}
	}

	return bits
}

// Encode encodes 144 data bits with the rate 3/4 trellis code and a tail
// tribit of 0 into 196 interleaved bits.
func Encode(data [144]unpacked.Bit) [196]unpacked.Bit {
	unpacked.Mask(data[:])
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
