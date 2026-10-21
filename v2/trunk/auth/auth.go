// Package auth computes the answer of a Tier III MS to the authentication
// challenge of a TSCC: the RC4 keystream generator, keyed with the
// challenge and the 128-bit key of the MS, with an optional exclusive-or
// with the PSN of the MS. Package trunk has none of this code, so a
// program that does not authenticate does not link it. It does not
// allocate. (ETSI TS 102 361-4 clause 6.4.8)
package auth

// KeyLen is the length of the authentication key K in octets (ETSI TS 102
// 361-4 clause 6.4.8.1.1).
const KeyLen = 16

// MaxChallenge is the largest challenge that a TSCC sends in the Source
// Address of a C_AHOY. Larger values are the gateway and identifier
// addresses (ETSI TS 102 361-4 clause 6.4.8.2, Table 6.17).
const MaxChallenge = 0xFFFCDF

// Response returns the 24-bit response to the challenge for the key K: the
// octets 257 to 259 of the RC4 keystream keyed with the 3 octets of the
// challenge, most significant first, and the 16 octets of K, exclusive-or
// psn. A psn of 0 is an MS that uses no PSN. Bits above 24 of challenge
// and psn are ignored. It does not allocate. (ETSI TS 102 361-4 clauses
// 6.4.8.0 and 6.4.8.1.2, Figure 6.30, Tables 6.15 and 6.16)
func Response(challenge uint32, key [KeyLen]byte, psn uint32) uint32 {
	var k [3 + KeyLen]byte
	k[0], k[1], k[2] = byte(challenge>>16), byte(challenge>>8), byte(challenge)
	copy(k[3:], key[:])
	var s [256]byte
	for i := range s {
		s[i] = byte(i)
	}
	var j byte
	for i := range s {
		j += s[i] + k[i%len(k)]
		s[i], s[j] = s[j], s[i]
	}
	var i byte
	j = 0
	var r uint32
	for n := range 259 {
		i++
		j += s[i]
		s[i], s[j] = s[j], s[i]
		if n >= 256 {
			r = r<<8 | uint32(s[s[i]+s[j]])
		}
	}
	return (r ^ psn) & 0xFFFFFF
}
