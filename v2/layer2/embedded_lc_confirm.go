package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// DecodeEmbeddedLCCandidate decodes the 9 LC octets from the embedded
// signalling fields f of bursts B to E to the nearest codeword of the BPTC,
// up to 8 bit errors, whose 5-bit checksum matches its LC. At low signal
// levels a candidate with more than one corrected bit is often a wrong LC:
// DecodeEmbeddedLC refuses it, and EmbeddedLCConfirmer takes it only when
// another superframe of the call gives the same LC. The FEC result sets
// Uncorrectable when no nearest codeword, or more than one, has a matching
// checksum.
func DecodeEmbeddedLCCandidate(f [4][4]byte) (lc pdu.LCBytes, res fec.Result) {
	return decodeEmbeddedLCCandidate(fragmentWords(f))
}

func decodeEmbeddedLCCandidate(fragments [4]uint32) ([9]byte, fec.Result) {
	return embeddedLCChecksum(bptc.DecodeEmbeddedWithCheck(fragments, embeddedLCChecksumValid))
}

func embeddedLCChecksumValid(info [10]byte) bool {
	_, result := embeddedLCChecksum(info, fec.Result{})
	return !result.Uncorrectable
}

const embeddedLCSeen = 8

// EmbeddedLCConfirmer decodes the embedded LCs of the superframes of one
// call. It takes an LC that decodes with no bit error at once. It takes an
// LC with corrected bits only when independent decodes confirm it: the LC
// of the voice header of the call, or the same LC from one of the last 8
// other superframes for an LC with one corrected bit, or from two of them
// for a candidate of DecodeEmbeddedLCCandidate with more. A voice LC
// of another talker or target starts a new talker: the header and the
// superframes before it then confirm nothing, so that the LC of the earlier
// talker does not come back. The zero EmbeddedLCConfirmer is ready to use.
// It is not safe for concurrent use.
type EmbeddedLCConfirmer struct {
	seen      [embeddedLCSeen][9]byte
	header    [9]byte
	talker    [8]byte
	n, next   uint8
	hasHeader bool
	hasTalker bool
}

// Reset starts a new call: no header and no earlier superframes.
func (c *EmbeddedLCConfirmer) Reset() {
	if c == nil {
		return
	}
	*c = EmbeddedLCConfirmer{}
}

// Header starts a new call whose voice header carries lc.
func (c *EmbeddedLCConfirmer) Header(lc pdu.LCBytes) {
	if c == nil {
		return
	}
	*c = EmbeddedLCConfirmer{header: lc, hasHeader: true}
	c.talker, c.hasTalker = embeddedLCTalker(lc)
}

// Decode decodes the embedded LC of the next superframe of the call from
// the embedded signalling fields f of bursts B to E. The FEC result sets
// Uncorrectable when the LC has corrected bits and nothing confirms it, or
// when it does not decode.
func (c *EmbeddedLCConfirmer) Decode(f [4][4]byte) (lc pdu.LCBytes, res fec.Result) {
	if c == nil {
		return decodeEmbeddedLC(fragmentWords(f))
	}
	return c.decode(fragmentWords(f))
}

func (c *EmbeddedLCConfirmer) decode(fragments [4]uint32) ([9]byte, fec.Result) {
	lc, res := decodeEmbeddedLC(fragments)
	if !res.Uncorrectable && res.ErrorsCorrected == 0 {
		c.take(lc)
		return lc, res
	}
	need := 1
	if res.Uncorrectable {
		lc, res = decodeEmbeddedLCCandidate(fragments)
		if res.Uncorrectable {
			return lc, res
		}
		need = 2
	}
	if c.confirms(lc, need) {
		c.take(lc)
		return lc, res
	}
	c.add(lc)
	res.Uncorrectable = true
	return lc, res
}

func (c *EmbeddedLCConfirmer) confirms(lc [9]byte, need int) bool {
	if c.hasHeader && lc == c.header {
		return true
	}
	n := 0
	for i := range c.n {
		if c.seen[i] == lc {
			n++
		}
	}
	return n >= need
}

func (c *EmbeddedLCConfirmer) take(lc [9]byte) {
	if t, ok := embeddedLCTalker(lc); ok {
		if c.hasTalker && t != c.talker {
			c.n, c.next, c.hasHeader = 0, 0, false
		}
		c.talker, c.hasTalker = t, true
	}
	c.add(lc)
}

func (c *EmbeddedLCConfirmer) add(lc [9]byte) {
	c.seen[c.next] = lc
	c.next = (c.next + 1) % embeddedLCSeen
	c.n = min(c.n+1, embeddedLCSeen)
}

func embeddedLCTalker(lc [9]byte) ([8]byte, bool) {
	flco := lc[0] & 0x3F
	if flco != 0x00 && flco != 0x03 {
		return [8]byte{}, false
	}
	return [8]byte{flco, lc[1], lc[3], lc[4], lc[5], lc[6], lc[7], lc[8]}, true
}
