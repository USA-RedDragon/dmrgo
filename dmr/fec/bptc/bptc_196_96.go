package bptc

import "fmt"

type BPTC19696 struct {
}

var (
	// deinterleave matrix
	dm = [256]uint8{}
)

func init() {
	for i := 0; i < 0x100; i++ {
		dm[i] = uint8((i * 181) % 196)
	}
}

func (b *BPTC19696) DeinterleaveDataBits(bits [196]byte) [96]byte {
	var deinterleavedBits [196]byte
	var temp [96]byte

	var i, j, k uint

	for i = 1; i < 197; i++ {
		deinterleavedBits[i-1] = bits[dm[i]]
	}

	if err := hamming_check(deinterleavedBits); err != nil {
		//return err
		fmt.Printf("hamming check failed: %v\n", err)
	}

	// Extract data bits
	for i, k = 3, 0; i < 11; i, k = i+1, k+1 {
		temp[k] = deinterleavedBits[0*15+i]
	}
	for j = 1; j < 9; j++ {
		for i = 0; i < 11; i, k = i+1, k+1 {
			temp[k] = deinterleavedBits[j*15+i]
		}
	}

	return temp
}

func hamming_check(bits [196]byte) error {
	var row [15]byte
	var col [13]byte

	// Run through each of the 9 rows containing data
	for r := 0; r < 9; r++ {
		k := r*15 + 1
		for a := 0; a < 15; a++ {
			row[a] = bits[k]
		}
		if !hamming_15_11_3_parity(row) {
			return fmt.Errorf("hamming(15, 11, 3) check failed on row #%d", r)
		}
	}

	// Run through each of the 15 columns
	for c := 0; c < 15; c++ {
		k := c + 1
		for a := 0; a < 13; a, k = a+1, k+15 {
			col[a] = bits[k]
		}
		if !hamming_13_9_3_parity(col) {
			return fmt.Errorf("hamming(13, 9, 3) check failed on col #%d", c)
		}
	}

	return nil
}

func hamming_15_11_3_parity(bits [15]byte) bool {
	var errs [4]byte
	errs[0] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8]
	errs[1] = bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[6] ^ bits[8] ^ bits[9]
	errs[2] = bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[7] ^ bits[9] ^ bits[10]
	errs[3] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7] ^ bits[10]
	return (errs[0] == bits[11]) && (errs[1] == bits[12]) && (errs[2] == bits[13]) && (errs[3] == bits[14])
}

func hamming_13_9_3_parity(bits [13]byte) bool {
	var errs [4]byte
	errs[0] = bits[0] ^ bits[1] ^ bits[3] ^ bits[5] ^ bits[6]
	errs[1] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7]
	errs[2] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8]
	errs[3] = bits[0] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[8]
	return (errs[0] == bits[9]) && (errs[1] == bits[10]) && (errs[2] == bits[11]) && (errs[3] == bits[12])
}
