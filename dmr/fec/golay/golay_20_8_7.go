package golay

// ETSI TS 102 361-1 V2.5.1 (2017-10) - B.3.1 Golay (20,8,7)
func Golay_20_8_Parity(bits [8]byte) []byte {
	var p = make([]byte, 12)
	p[0] = bits[1] ^ bits[4] ^ bits[5] ^ bits[6] ^ bits[7]
	p[1] = bits[1] ^ bits[2] ^ bits[4]
	p[2] = bits[0] ^ bits[2] ^ bits[3] ^ bits[5]
	p[3] = bits[0] ^ bits[1] ^ bits[3] ^ bits[4] ^ bits[6]
	p[4] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[7]
	p[5] = bits[0] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[7]
	p[6] = bits[3] ^ bits[6] ^ bits[7]
	p[7] = bits[0] ^ bits[1] ^ bits[5] ^ bits[6]
	p[8] = bits[0] ^ bits[1] ^ bits[2] ^ bits[6] ^ bits[7]
	p[9] = bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[6]
	p[10] = bits[0] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[6] ^ bits[7]
	p[11] = bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7]
	return p
}

func Golay_20_8_Check(bits [20]byte) bool {
	var dataBits [8]byte
	copy(dataBits[:], bits[:8])

	parity := Golay_20_8_Parity(dataBits)
	for i := 0; i < 12; i++ {
		if parity[i] != bits[8+i] {
			return false
		}
	}
	return true
}
