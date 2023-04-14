package quadratic_residue1676

import "bytes"

// ETSI TS 102 361-1 V2.5.1 (2017-10) - B.3.2  Quadratic residue (16,7,6)

var (
	validDataParities = [128][9]byte{}
)

type QuadraticResidue16_7_6 struct {
	Data   [7]byte
	Parity [9]byte
}

func NewQuadraticResidue16_7_6(bits [16]byte) *QuadraticResidue16_7_6 {
	qr := QuadraticResidue16_7_6{}
	copy(qr.Data[:], bits[:7])
	copy(qr.Parity[:], bits[7:16])

	return &qr
}

func ParityBits(bits [7]byte) [9]byte {
	parity := [9]byte{}

	// Multiplying the generator matrix with the given data bits.
	// See DMR AI spec. page 134.
	parity[0] = bits[1] ^ bits[2] ^ bits[3] ^ bits[4]
	parity[1] = bits[2] ^ bits[3] ^ bits[4] ^ bits[5]
	parity[2] = bits[0] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[6]
	parity[3] = bits[2] ^ bits[3] ^ bits[5] ^ bits[6]
	parity[4] = bits[1] ^ bits[2] ^ bits[6]
	parity[5] = bits[0] ^ bits[1] ^ bits[4]
	parity[6] = bits[0] ^ bits[1] ^ bits[2] ^ bits[5]
	parity[7] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[6]
	parity[8] = bits[0] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[6]

	return parity
}

func Check(bits [16]byte) bool {
	codeword := NewQuadraticResidue16_7_6(bits)
	if codeword == nil {
		return false
	}

	var dataval uint8
	for col := uint8(0); col < 7; col++ {
		if codeword.Data[col] == 1 {
			dataval |= (1 << (7 - col))
		}
	}

	return bytes.Equal(codeword.Parity[:], validDataParities[dataval][:])
}

func toBits(b byte) [7]byte {
	var o [7]byte
	for bit, mask := 0, byte(128); bit < 7; bit, mask = bit+1, mask>>1 {
		if b&mask != 0 {
			o[bit] = 1
		}
	}
	return o
}

func init() {
	for i := byte(0); i < 128; i++ {
		bits := toBits(i)
		validDataParities[i] = ParityBits(bits)
	}
}

// type QuadraticResidue16_7_6 struct {
// }

// // Returns 9 bits of CRC
// func (qr *QuadraticResidue16_7_6) Generate(bits [7]byte) int {
// 	parity := make([]byte, 9)

// 	// Multiplying the generator matrix with the given data bits.
// 	// See DMR AI spec. page 134.
// 	parity[0] = bits[1] ^ bits[2] ^ bits[3] ^ bits[4]
// 	parity[1] = bits[2] ^ bits[3] ^ bits[4] ^ bits[5]
// 	parity[2] = bits[0] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[6]
// 	parity[3] = bits[2] ^ bits[3] ^ bits[5] ^ bits[6]
// 	parity[4] = bits[1] ^ bits[2] ^ bits[6]
// 	parity[5] = bits[0] ^ bits[1] ^ bits[4]
// 	parity[6] = bits[0] ^ bits[1] ^ bits[2] ^ bits[5]
// 	parity[7] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[6]
// 	parity[8] = bits[0] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[6]

// 	// Now convert the parity bits to an integer.
// 	crc := 0
// 	for i := 0; i < 9; i++ {
// 		if parity[i] == 1 {
// 			crc |= 1 << uint(i)
// 		}
// 	}
// 	return crc
// }

// func (qr *QuadraticResidue16_7_6) Check(parity int) bool {
// 	bits := make([]byte, 7)
// 	for i := 0; i < 7; i++ {
// 		bits[i] = byte(parity & 1)
// 		parity >>= 1
// 	}

// 	syndrome := getSyndromeForWord(bits, PARITY_CHECK_MATRIX, 2)
// 	if len(syndrome) != 9 {
// 		panic("Syndrome should be 9 bits long")
// 	}

// 	fmt.Printf("Syndrome: %v\n", syndrome)

// 	// Return true if the syndrome equals the correct syndrome.
// 	for i := 0; i < 9; i++ {
// 		if syndrome[i] != CORRECT_SYNDROME[i] {
// 			return false
// 		}
// 	}
// 	return true
// }
