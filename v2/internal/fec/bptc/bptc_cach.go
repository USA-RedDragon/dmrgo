package bptc

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
)

// The CACH BPTC spreads a 36-bit Short LC PDU over the 17-bit payloads of 4
// CACH bursts (ETSI TS 102 361-1 clause B.2.3). Rows 0 to 2 of the 4×17
// encode matrix hold 12 information bits and 5 Hamming(17,12,3) parity bits,
// and row 3 holds the even parity of each column. The transmit stream reads
// the encode matrix column by column, top to bottom.

// calculateSyndrome17_12 returns the 5-bit Hamming(17,12,3) syndrome of a
// 17-bit row, from the parity check matrix H = [Pᵀ | I₅].
func calculateSyndrome17_12(bits [17]bit.Bit) int {
	var s [5]bit.Bit
	s[0] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[6] ^ bits[7] ^ bits[9] ^ bits[12]
	s[1] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[7] ^ bits[8] ^ bits[10] ^ bits[13]
	s[2] = bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[8] ^ bits[9] ^ bits[11] ^ bits[14]
	s[3] = bits[0] ^ bits[1] ^ bits[4] ^ bits[5] ^ bits[7] ^ bits[10] ^ bits[15]
	s[4] = bits[0] ^ bits[1] ^ bits[2] ^ bits[5] ^ bits[6] ^ bits[8] ^ bits[11] ^ bits[16]
	return int(s[0]) | int(s[1])<<1 | int(s[2])<<2 | int(s[3])<<3 | int(s[4])<<4
}

// parityHamming17_12 returns the 5 Hamming(17,12,3) parity bits of 12 data
// bits (ETSI TS 102 361-1 Table B.13).
func parityHamming17_12(data [12]bit.Bit) [5]bit.Bit {
	return [5]bit.Bit{
		data[0] ^ data[1] ^ data[2] ^ data[3] ^ data[6] ^ data[7] ^ data[9],
		data[0] ^ data[1] ^ data[2] ^ data[3] ^ data[4] ^ data[7] ^ data[8] ^ data[10],
		data[1] ^ data[2] ^ data[3] ^ data[4] ^ data[5] ^ data[8] ^ data[9] ^ data[11],
		data[0] ^ data[1] ^ data[4] ^ data[5] ^ data[7] ^ data[10],
		data[0] ^ data[1] ^ data[2] ^ data[5] ^ data[6] ^ data[8] ^ data[11],
	}
}

// DecodeCACH decodes a 36-bit Short LC PDU (28 data bits and a CRC-8) from
// the 17-bit signalling payloads of CACH bursts 0 to 3, in that order, and
// returns it with a fec.Result.
func DecodeCACH(fragments [4][17]bit.Bit) ([36]bit.Bit, fec.Result) {
	for i := range fragments {
		lowbit.Mask(fragments[i][:])
	}
	result := fec.Result{BitsChecked: 68}

	var stream [68]bit.Bit
	for i := 0; i < 4; i++ {
		copy(stream[i*17:(i+1)*17], fragments[i][:])
	}

	var matrix [4][17]bit.Bit
	for j := 0; j < 68; j++ {
		encRow := j % 4
		encCol := j / 4
		matrix[encRow][encCol] = stream[j]
	}

	received := matrix
	correctCACHMatrix(&matrix)

	for row := range matrix {
		for col := range matrix[row] {
			result.ErrorsCorrected += int(matrix[row][col] ^ received[row][col])
		}
	}
	result.Uncorrectable = !cachMatrixValid(&matrix)

	var info [36]bit.Bit
	for row := 0; row < 3; row++ {
		copy(info[row*12:(row+1)*12], matrix[row][:12])
	}

	return info, result
}

// correctCACHMatrix corrects the 4×17 product code in place. When only one
// row has a nonzero syndrome, the failing column parities give the error
// positions in that row; otherwise each row is corrected for one error.
func correctCACHMatrix(matrix *[4][17]bit.Bit) {
	for pass := 0; pass < 3; pass++ {
		var syndromes [4]int
		badRow, badRows := 0, 0
		for row := range matrix {
			syndromes[row] = calculateSyndrome17_12(matrix[row])
			if syndromes[row] != 0 {
				badRow = row
				badRows++
			}
		}

		fixing := false
		if badRows == 1 {
			for col := 0; col < 17; col++ {
				if cachColumnParity(matrix, col) != 0 {
					matrix[badRow][col] ^= 1
					fixing = true
				}
			}
		}
		if !fixing {
			for row := range matrix {
				if syndromes[row] == 0 {
					continue
				}
				if errPos := hamming17_12_syndrome_table[syndromes[row]]; errPos >= 0 {
					matrix[row][errPos] ^= 1
					fixing = true
				}
			}
		}

		if !fixing {
			return
		}
	}
}

func cachColumnParity(matrix *[4][17]bit.Bit, col int) bit.Bit {
	return matrix[0][col] ^ matrix[1][col] ^ matrix[2][col] ^ matrix[3][col]
}

func cachMatrixValid(matrix *[4][17]bit.Bit) bool {
	for row := range matrix {
		if calculateSyndrome17_12(matrix[row]) != 0 {
			return false
		}
	}
	for col := 0; col < 17; col++ {
		if cachColumnParity(matrix, col) != 0 {
			return false
		}
	}
	return true
}

// EncodeCACH encodes a 36-bit Short LC PDU into the 17-bit signalling
// payloads of 4 CACH bursts.
func EncodeCACH(info [36]bit.Bit) [4][17]bit.Bit {
	lowbit.Mask(info[:])
	var matrix [4][17]bit.Bit

	for row := 0; row < 3; row++ {
		copy(matrix[row][:12], info[row*12:(row+1)*12])
	}

	for row := 0; row < 3; row++ {
		var data [12]bit.Bit
		copy(data[:], matrix[row][:12])
		parity := parityHamming17_12(data)
		matrix[row][12] = parity[0]
		matrix[row][13] = parity[1]
		matrix[row][14] = parity[2]
		matrix[row][15] = parity[3]
		matrix[row][16] = parity[4]
	}

	for col := 0; col < 17; col++ {
		matrix[3][col] = matrix[0][col] ^ matrix[1][col] ^ matrix[2][col]
	}

	var stream [68]bit.Bit
	for j := 0; j < 68; j++ {
		encRow := j % 4
		encCol := j / 4
		stream[j] = matrix[encRow][encCol]
	}

	var fragments [4][17]bit.Bit
	for i := 0; i < 4; i++ {
		copy(fragments[i][:], stream[i*17:(i+1)*17])
	}

	return fragments
}
