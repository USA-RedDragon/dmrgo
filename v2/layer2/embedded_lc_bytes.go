package layer2

// fragmentWord returns the 32 bits of an embedded signalling field as a
// word, the first bit most significant.
func fragmentWord(f [4]byte) uint32 {
	return uint32(f[0])<<24 | uint32(f[1])<<16 | uint32(f[2])<<8 | uint32(f[3])
}

// fragmentWords returns the words of four embedded signalling fields.
func fragmentWords(f [4][4]byte) [4]uint32 {
	return [4]uint32{fragmentWord(f[0]), fragmentWord(f[1]), fragmentWord(f[2]), fragmentWord(f[3])}
}

// fragmentOctets returns the four fragment words w as embedded signalling
// fields.
func fragmentOctets(w [4]uint32) [4][4]byte {
	var out [4][4]byte
	for i, x := range w {
		out[i] = [4]byte{byte(x >> 24), byte(x >> 16 & 0xFF), byte(x >> 8 & 0xFF), byte(x & 0xFF)}
	}
	return out
}
