// Package auth computes the answer of a Tier III MS to the authentication
// challenge of a TSCC: the RC4 keystream generator, keyed with the
// challenge and the 128-bit key of the MS, with an optional exclusive-or
// with the PSN of the MS. Key and Keys are the trunk.Authenticator of an
// MS and of a TSCC, for the stun, revive and kill procedures. Package trunk
// has none of this code, so a program that does not authenticate does not
// link it. It does not allocate. (ETSI TS 102 361-4 clause 6.4.8)
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
	k[0], k[1], k[2] = byte(challenge>>16&0xFF), byte(challenge>>8&0xFF), byte(challenge&0xFF)
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

// Key is the authentication key K and the PSN of one MS. A *Key is a
// trunk.Authenticator for the MSConfig.Auth of that MS: Response answers
// each challenge with K and PSN, whatever the address. (ETSI TS 102 361-4
// clauses 6.4.8, 6.4.9.2 and 6.4.10)
type Key struct {
	// K is the authentication key of the MS.
	K [KeyLen]byte
	// PSN is the PSN of the MS, or 0 for an MS that uses no PSN.
	PSN uint32
}

// Response returns the response of the MS to challenge, as the package
// function Response computes it, and true, for any address. A nil k has
// no key. It does not allocate.
func (k *Key) Response(_, challenge uint32) (response uint32, ok bool) {
	if k == nil {
		return 0, false
	}
	return Response(challenge, k.K, k.PSN), true
}

// Entry is the Key of the MS Addr.
type Entry struct {
	// Addr is the individual address of the MS.
	Addr uint32
	// Key is the key of the MS.
	Key Key
}

// Keys holds the keys of the MSs that a TSCC authenticates. A *Keys is a
// trunk.Authenticator for TSCCConfig.Auth. (ETSI TS 102 361-4 clauses
// 6.4.8, 6.4.9.2 and 6.4.10)
type Keys struct {
	// Entries holds the key of each MS. The first entry for an address
	// wins.
	Entries []Entry
}

// Response returns the response of the MS addr to challenge, and false
// when Entries has no key for addr or k is nil. It does not allocate.
func (k *Keys) Response(addr, challenge uint32) (response uint32, ok bool) {
	if k == nil {
		return 0, false
	}
	for i := range k.Entries {
		if e := &k.Entries[i]; e.Addr == addr {
			return e.Key.Response(addr, challenge)
		}
	}
	return 0, false
}
