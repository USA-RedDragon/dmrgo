package digest

// Equal reports whether a and b hold the same octets, in a time that
// depends only on their lengths: it reads every octet and has no branch on
// their values, as crypto/subtle.ConstantTimeCompare. Lengths are not
// secret, so a length that differs returns false at once. The code that Go
// builds for amd64, arm64 and arm, and that TinyGo builds for Cortex-M4,
// branches only on the lengths and the loop counter.
func Equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	b = b[:len(a)]
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return (uint32(v)-1)>>31 == 1
}
