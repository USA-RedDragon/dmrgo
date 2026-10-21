package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ShortLCAssembler collects the signalling payloads of 4 CACHs and decodes
// the Short LC PDU they carry. The zero ShortLCAssembler is ready to use.
// (ETSI TS 102 361-1 clause 7.1.4)
type ShortLCAssembler struct {
	fragments [4][3]byte
	count     int
}

// Reset clears the assembler state for reuse.
func (a *ShortLCAssembler) Reset() {
	a.fragments = [4][3]byte{}
	a.count = 0
}

// Count returns the number of fragments accumulated so far.
func (a *ShortLCAssembler) Count() int {
	return a.count
}

// Add adds a decoded CACH and returns the Short LC when the CACH completes
// it, with done set. A First fragment starts a new Short LC, and a Last
// fragment after a First and two Continuation fragments completes it. The
// FEC field of the ShortLC is the combined result of the BPTC FEC and the
// CRC-8 check. A CACH whose TACT is uncorrectable has no LCSS, and a Single
// fragment LCSS is not used in the CACH, so either discards the partial
// Short LC; Add then returns ErrShortLCSequence when fragments were
// discarded. A fragment out of order also discards the partial Short LC and
// returns ErrShortLCSequence. Add allocates the payload struct of a
// completed Short LC; AddInto does not.
func (a *ShortLCAssembler) Add(c CACH) (slc pdu.ShortLC, done bool, err error) {
	if done, err = a.add(c); err != nil || !done {
		return slc, false, err
	}
	bits, bptcResult := a.decode()
	slc = pdu.DecodeShortLC(bits)
	slc.FEC = bptcResult.Combine(slc.FEC)
	return slc, true, nil
}

// AddInto adds a decoded CACH as Add does, and decodes a completed Short LC
// into dst, with its payload struct in p, as pdu.DecodeShortLCInto places
// it. It leaves dst as it is until the Short LC is complete. It does not
// allocate.
func (a *ShortLCAssembler) AddInto(dst *pdu.ShortLC, p *pdu.ShortLCPayloads, c CACH) (done bool, err error) {
	if done, err = a.add(c); err != nil || !done {
		return false, err
	}
	bits, bptcResult := a.decode()
	pdu.DecodeShortLCInto(dst, p, bits)
	dst.FEC = bptcResult.Combine(dst.FEC)
	return true, nil
}

func (a *ShortLCAssembler) add(c CACH) (bool, error) {
	lcss := c.TACT.LCSS
	if c.TACT.FEC.Uncorrectable {
		lcss = enums.LCSSSingleFragment
	}
	switch lcss {
	case enums.LCSSFirstFragment:
		a.Reset()
	case enums.LCSSContinuationFragment:
		if a.count != 1 && a.count != 2 {
			a.Reset()
			return false, ErrShortLCSequence
		}
	case enums.LCSSLastFragment:
		if a.count != 3 {
			a.Reset()
			return false, ErrShortLCSequence
		}
	default:
		n := a.count
		a.Reset()
		if n > 0 {
			return false, ErrShortLCSequence
		}
		return false, nil
	}
	a.fragments[a.count] = c.Payload
	a.count++
	return a.count == 4, nil
}

func (a *ShortLCAssembler) decode() ([5]byte, fec.Result) {
	defer a.Reset()
	var fragments [4]uint32
	for i := range fragments {
		f := a.fragments[i]
		fragments[i] = uint32(f[0])<<9 | uint32(f[1])<<1 | uint32(f[2])>>7
	}
	info, res := bptc.DecodeCACH(fragments)
	v := info << 4
	return [5]byte{byte(v >> 32 & 0xFF), byte(v >> 24 & 0xFF), byte(v >> 16 & 0xFF), byte(v >> 8 & 0xFF), byte(v & 0xF0)}, res
}
