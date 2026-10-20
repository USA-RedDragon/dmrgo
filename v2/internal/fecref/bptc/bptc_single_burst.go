package bptc

import (
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// ETSI TS 102 361-1 clause B.2.2

// DecodeSingleBurst decodes the 11 information bits from the 32-bit embedded
// data field of one burst. Set oddParity for a Reverse Channel burst. It
// corrects up to 3 bit errors, and it always detects 4 and sets Uncorrectable.
func DecodeSingleBurst(txBits [32]unpacked.Bit, oddParity bool) ([11]unpacked.Bit, fec.Result) {
	unpacked.Mask(txBits[:])
	result := fec.Result{BitsChecked: 32}

	var deinterleaved [32]unpacked.Bit
	for i := 0; i < 32; i++ {
		deinterleaved[single_burst_bptc_interleave[i]] = txBits[i]
	}

	var matrix [2][16]unpacked.Bit
	for i := 0; i < 32; i++ {
		row := i % 2
		col := i / 2
		matrix[row][col] = deinterleaved[i]
	}

	// clause B.2.2.1; clause B.2.2.2
	row1 := matrix[1]
	if oddParity {
		for col := range row1 {
			row1[col] ^= 1
		}
	}

	best, bestDist := matrix[0], 33
	for _, row := range [2][16]unpacked.Bit{matrix[0], row1} {
		candidate, ok := correctHamming16_11(row)
		if !ok {
			continue
		}
		if d := hammingDistance16(candidate, matrix[0]) + hammingDistance16(candidate, row1); d < bestDist {
			best, bestDist = candidate, d
		}
	}

	if bestDist > 3 {
		result.Uncorrectable = true
	} else {
		matrix[0] = best
		result.ErrorsCorrected = bestDist
	}

	var info [11]unpacked.Bit
	copy(info[:], matrix[0][:11])

	return info, result
}

// EncodeSingleBurst encodes 11 information bits into the 32-bit embedded
// data field of one burst. Set oddParity for a Reverse Channel burst.
func EncodeSingleBurst(info [11]unpacked.Bit, oddParity bool) [32]unpacked.Bit {
	unpacked.Mask(info[:])
	var matrix [2][16]unpacked.Bit

	copy(matrix[0][:11], info[:])

	parity := parityHamming16_11(info)
	matrix[0][11] = parity[0]
	matrix[0][12] = parity[1]
	matrix[0][13] = parity[2]
	matrix[0][14] = parity[3]
	matrix[0][15] = parity[4]

	for col := 0; col < 16; col++ {
		matrix[1][col] = matrix[0][col]
		if oddParity {
			matrix[1][col] ^= 1
		}
	}

	var stream [32]unpacked.Bit
	for col := 0; col < 16; col++ {
		stream[col*2] = matrix[0][col]
		stream[col*2+1] = matrix[1][col]
	}

	var txBits [32]unpacked.Bit
	for i := 0; i < 32; i++ {
		txBits[(i*17)%32] = stream[i]
	}

	return txBits
}

func correctHamming16_11(row [16]unpacked.Bit) ([16]unpacked.Bit, bool) {
	s := calculateSyndrome16_11(row)
	if s == 0 {
		return row, true
	}
	errPos := hamming16_11_syndrome_table[s]
	if errPos < 0 {
		return row, false
	}
	row[errPos] ^= 1
	return row, true
}

func hammingDistance16(a, b [16]unpacked.Bit) int {
	d := 0
	for i := range a {
		d += int(a[i] ^ b[i])
	}
	return d
}
