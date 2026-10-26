package privacy

import (
	"crypto/cipher"
	"encoding/binary"

	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

const (
	framesPerSuperframe = 18
	octetsPerFrame      = 7
	arc4Drop            = 256
	maxSkip             = 1 << 12
)

type arc4 struct {
	s    [256]byte
	i, j uint8
}

func (c *arc4) init(key []byte) {
	for i := range c.s {
		c.s[i] = byte(i)
	}
	var j uint8
	n := len(key)
	for i := range c.s {
		j += c.s[i] + key[i%n]
		c.s[i], c.s[j] = c.s[j], c.s[i]
	}
	c.i, c.j = 0, 0
}

func (c *arc4) next() byte {
	c.i++
	x := c.s[c.i]
	c.j += x
	y := c.s[c.j]
	c.s[c.i], c.s[c.j] = y, x
	return c.s[x+y]
}

func arc4Step(mi uint32) uint32 {
	for range 32 {
		bit := (mi>>31 ^ mi>>3 ^ mi>>1) & 1
		mi = mi<<1 | bit
	}
	return mi
}

func aesIV(mi uint32) [16]byte {
	var iv [16]byte
	binary.BigEndian.PutUint32(iv[:4], mi)
	l := uint64(mi)
	for x := 32; x < 128; x++ {
		bit := byte((l>>31 ^ l>>21 ^ l>>1 ^ l) & 1)
		l = l<<1 | uint64(bit)
		iv[x/8] = iv[x/8]<<1 | bit
	}
	return iv
}

func ofb(b cipher.Block, blk *[16]byte, dst []byte) {
	for i := 0; i < len(dst); i += len(blk) {
		b.Encrypt(blk[:], blk[:])
		copy(dst[i:], blk[:])
	}
}

type stream struct {
	key  *Key
	mi0  uint32
	mi   uint32
	sf   int
	has  bool
	ks   [128]byte
	blk  [16]byte
	rc   arc4
	rkey [9]byte
}

func (s *stream) start(k *Key, mi uint32) {
	s.key, s.mi0, s.mi, s.sf, s.has = k, mi, mi, 0, false
}

func (s *stream) stop() {
	s.key, s.has = nil, false
	clear(s.ks[:])
	clear(s.blk[:])
	clear(s.rkey[:])
	s.rc = arc4{}
}

func (s *stream) active() bool {
	return s.key != nil
}

func (s *stream) generate() {
	switch s.key.alg {
	case AlgorithmARC4:
		copy(s.rkey[:5], s.key.arc4[:])
		binary.BigEndian.PutUint32(s.rkey[5:], s.mi)
		s.rc.init(s.rkey[:])
		for range arc4Drop {
			s.rc.next()
		}
		for i := range framesPerSuperframe * octetsPerFrame {
			s.ks[i] = s.rc.next()
		}
	case AlgorithmAES128, AlgorithmAES256:
		s.blk = aesIV(s.mi)
		ofb(s.key.block, &s.blk, s.ks[:len(s.blk)])
		ofb(s.key.block, &s.blk, s.ks[:])
	default:
	}
	s.has = true
}

func (s *stream) advance() {
	switch s.key.alg {
	case AlgorithmARC4:
		s.mi = arc4Step(s.mi)
	case AlgorithmAES128, AlgorithmAES256:
		iv := aesIV(s.mi)
		s.mi = binary.BigEndian.Uint32(iv[4:8])
	default:
	}
	s.sf++
	s.has = false
}

func (s *stream) seek(sf int) {
	if sf < s.sf {
		s.mi, s.sf, s.has = s.mi0, 0, false
	}
	for s.sf < sf {
		s.advance()
	}
	if !s.has {
		s.generate()
	}
}

func (s *stream) apply(n int, f *vocoder.AMBEFrame, rx bool) {
	if s.key == nil || n < 0 {
		return
	}
	if s.key.alg == AlgorithmBasic {
		for i, b := range s.key.basic {
			f[i] ^= b
		}
		return
	}
	if rx && *f == vocoder.AMBESilence() {
		return
	}
	sf := n / framesPerSuperframe
	if sf-s.sf > maxSkip {
		return
	}
	s.seek(sf)
	off := (n % framesPerSuperframe) * octetsPerFrame
	for i := range octetsPerFrame - 1 {
		f[i] ^= s.ks[off+i]
	}
	f[octetsPerFrame-1] ^= s.ks[off+octetsPerFrame-1] & 0x80
}
