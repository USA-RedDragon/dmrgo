package trellis34

import (
	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
)

type Trellis34 struct {
}

func New() *Trellis34 {
	return &Trellis34{}
}

func (t *Trellis34) dibitsToBits(dibits [98]int8) [196]bit.Bit {
	var bits [196]bit.Bit

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

func (t *Trellis34) bitsToDibits(bits [196]bit.Bit) [98]int8 {
	var dibits [98]int8

	for i := 0; i < 196; i += 2 {
		o := i / 2
		switch {
		case bits[i] == 0 && bits[i+1] == 1:
			dibits[o] = 3
		case bits[i] == 0 && bits[i+1] == 0:
			dibits[o] = 1
		case bits[i] == 1 && bits[i+1] == 0:
			dibits[o] = -1
		case bits[i] == 1 && bits[i+1] == 1:
			dibits[o] = -3
		}
	}

	return dibits
}

func (t *Trellis34) Decode(bits [196]bit.Bit) ([144]bit.Bit, fec.FECResult) {
	result := fec.FECResult{BitsChecked: 196}
	dibits := t.bitsToDibits(bits)
	deinterleavedDibits := t.deinterleave(dibits)
	points := t.dibitsToPoints(deinterleavedDibits)
	tribits, errors := t.pointsToTribits(points)
	result.ErrorsCorrected = errors
	decoded := t.tribitsToBits(tribits)

	return decoded, result
}

func (t *Trellis34) deinterleave(dibits [98]int8) [98]int8 {
	var out [98]int8

	for i := 0; i < 98; i++ {
		out[interleave_matrix[i]] = dibits[i]
	}

	return out
}

func (t *Trellis34) dibitsToPoints(dibits [98]int8) [49]byte {
	var points [49]byte

	for i := 0; i < 98; i += 2 {
		o := i / 2
		points[o] = trellis34_constellation[[2]int8{dibits[i], dibits[i+1]}]
	}

	return points
}

func (t *Trellis34) pointsToTribits(points [49]byte) ([49]byte, int) {
	var tribits [49]byte
	last := byte(0)
	errors := 0

	for i := 0; i < 49; i++ {
		// Use the look-up table for O(1) matching
		// Maps [CurrentState][ReceivedSymbol] -> Tribit
		// If 0xFF, it's an invalid transition (bit limit error/noise)
		tribit := trellis34_transition_table[last][points[i]]

		if tribit != 0xFF {
			tribits[i] = tribit
			last = tribit
		} else {
			// fmt.Printf("Trellis data corrupted, index %d constellation point %v\n", i, points[i])
			errors++
			// Recovery strategy:
			// The original code did not update 'last' if no match was found,
			// effectively staying in the same state (or rather, assuming the state
			// for the next step is based on the LAST successful tribit).
			// We keep that behavior here.
		}
	}

	return tribits, errors
}

func (t *Trellis34) tribitsToBits(tribits [49]byte) [144]bit.Bit {
	var bits [144]bit.Bit

	for i := 0; i < 144; i += 3 {
		o := i / 3
		if (tribits[o] & 0x4) > 0 {
			bits[i] = 1
		} else {
			bits[i] = 0
		}

		if (tribits[o] & 0x2) > 0 {
			bits[i+1] = 1
		} else {
			bits[i+1] = 0
		}

		if (tribits[o] & 0x1) > 0 {
			bits[i+2] = 1
		} else {
			bits[i+2] = 0
		}
	}

	return bits
}

// Encode encodes 144 data bits using Trellis 3/4 rate coding, producing 196 interleaved bits.
func (t *Trellis34) Encode(data [144]bit.Bit) [196]bit.Bit {
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
