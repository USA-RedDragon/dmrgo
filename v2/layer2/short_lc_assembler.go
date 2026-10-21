package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2"
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
	if a == nil {
		return
	}
	a.fragments = [4][3]byte{}
	a.count = 0
}

// Count returns the number of fragments accumulated so far.
func (a *ShortLCAssembler) Count() int {
	if a == nil {
		return 0
	}
	return a.count
}

// Add adds a decoded CACH and returns the Short LC when the CACH completes
// it, with done set, as AddFragment and Complete do. A CACH whose TACT is
// uncorrectable has no LCSS, and a Single fragment LCSS is not used in the
// CACH, so either discards the partial Short LC; Add then returns
// ErrShortLCSequence when fragments were discarded. It allocates the payload
// struct of a completed Short LC; AddInto does not.
func (a *ShortLCAssembler) Add(c CACH) (slc pdu.ShortLC, done bool, err error) {
	if a == nil {
		return pdu.ShortLC{}, false, dmrgo.ErrNilReceiver
	}
	if c.TACT.FEC.Uncorrectable || c.TACT.LCSS == enums.LCSSSingleFragment {
		n := a.count
		a.Reset()
		if n > 0 {
			return slc, false, ErrShortLCSequence
		}
		return slc, false, nil
	}
	if done, err = a.AddFragment(c.TACT.LCSS, c.Payload); err != nil || !done {
		return slc, false, err
	}
	slc, err = a.Complete()
	return slc, err == nil, err
}

// AddFragment adds a 17-bit CACH signalling payload, packed as CACH.Payload
// holds it, with the LCSS of the TACT of the same CACH. It ignores the low 7
// bits of payload[2]. It returns true when a Last fragment follows a
// First and two Continuation fragments, and the assembler is ready for
// Complete. Any other LCSS or order discards the partial Short LC and
// returns ErrShortLCSequence.
func (a *ShortLCAssembler) AddFragment(lcss enums.LCSS, payload [3]byte) (bool, error) {
	if a == nil {
		return false, dmrgo.ErrNilReceiver
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
	case enums.LCSSSingleFragment:
		a.Reset()
		return false, ErrShortLCSequence
	default:
		a.Reset()
		return false, ErrShortLCSequence
	}
	a.fragments[a.count] = payload
	a.count++
	return a.count == 4, nil
}

// Complete decodes the assembled Short LC PDU from the 4 accumulated
// fragments and resets the assembler. The FEC field of the ShortLC is the
// combined result of the BPTC FEC and the CRC-8 check. Complete returns
// ErrShortLCIncomplete if the assembler does not hold a full sequence. It
// allocates the payload struct of the Short LC; CompleteInto does not.
func (a *ShortLCAssembler) Complete() (pdu.ShortLC, error) {
	if a == nil {
		return pdu.ShortLC{}, dmrgo.ErrNilReceiver
	}
	if a.count != 4 {
		return pdu.ShortLC{}, ErrShortLCIncomplete
	}
	bits, bptcResult := a.decode()
	slc := pdu.DecodeShortLC(bits)
	slc.FEC = bptcResult.Combine(slc.FEC)
	return slc, nil
}

// CompleteInto decodes the Short LC into dst as Complete does, with the
// payload struct of dst in p as pdu.DecodeShortLCInto places it. It returns
// ErrShortLCIncomplete, and leaves dst as it is, if the assembler does not
// hold a full sequence. It does not allocate.
func (a *ShortLCAssembler) CompleteInto(dst *pdu.ShortLC, p *pdu.ShortLCPayloads) error {
	if a == nil {
		return dmrgo.ErrNilReceiver
	}
	if a.count != 4 {
		return ErrShortLCIncomplete
	}
	bits, bptcResult := a.decode()
	pdu.DecodeShortLCInto(dst, p, bits)
	dst.FEC = bptcResult.Combine(dst.FEC)
	return nil
}

// AddInto adds a decoded CACH as Add does, and decodes a completed Short LC
// into dst, with its payload struct in p, as CompleteInto does. It does not
// allocate.
func (a *ShortLCAssembler) AddInto(dst *pdu.ShortLC, p *pdu.ShortLCPayloads, c CACH) (done bool, err error) {
	if a == nil {
		return false, dmrgo.ErrNilReceiver
	}
	if c.TACT.FEC.Uncorrectable || c.TACT.LCSS == enums.LCSSSingleFragment {
		n := a.count
		a.Reset()
		if n > 0 {
			return false, ErrShortLCSequence
		}
		return false, nil
	}
	if done, err = a.AddFragment(c.TACT.LCSS, c.Payload); err != nil || !done {
		return false, err
	}
	err = a.CompleteInto(dst, p)
	return err == nil, err
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
