package fuzztest

import "bytes"

const (
	maxScript   = 128
	maxStream   = 4096
	maxBitFlips = 64
)

// Stream builds a bit stream of at most 4096 octets from a fuzz
// script of at most 128 octets. The top two bits of each script octet
// select the operation: 0 copies the next octets of the script, 1
// repeats one octet, 2 repeats the end of the stream, and 3 appends a
// burst of pool. Each octet pair of flips, at most 32 pairs, then
// inverts one bit of the stream.
func Stream(pool [][33]byte, script, flips []byte) []byte {
	script = script[:min(len(script), maxScript)]
	var s []byte
	for i := 0; i < len(script) && len(s) < maxStream; {
		op := script[i]
		i++
		n := int(op&0x3f) + 1
		switch op >> 6 {
		case 0:
			end := min(i+n, len(script))
			s = append(s, script[i:end]...)
			i = end
		case 1:
			if i+2 > len(script) {
				i = len(script)
				break
			}
			count := (int(op&0x3f)<<8 | int(script[i])) + 1
			s = append(s, bytes.Repeat([]byte{script[i+1]}, min(count, maxStream-len(s)))...)
			i += 2
		case 2:
			if i >= len(script) {
				break
			}
			r := int(script[i]) + 1
			i++
			d := min(n, len(s))
			for k := 0; k < r && d > 0 && len(s) < maxStream; k++ {
				s = append(s, s[len(s)-d:]...)
			}
		case 3:
			if len(pool) > 0 {
				b := pool[int(op&0x3f)%len(pool)]
				s = append(s, b[:]...)
			}
		}
	}
	s = s[:min(len(s), maxStream)]
	flips = flips[:min(len(flips), maxBitFlips)]
	for k := 0; k+1 < len(flips) && len(s) > 0; k += 2 {
		i := (int(flips[k])<<8 | int(flips[k+1])) % (8 * len(s))
		s[i/8] ^= 0x80 >> (i % 8)
	}
	return s
}
