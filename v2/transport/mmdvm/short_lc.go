package mmdvm

import (
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const cachFragmentBits = 17

// NewDMRShortLC returns the CommandDMRShortLC payload that carries slc: its
// BPTC coding, the 4 CACH fragments of 17 bits one after the other, as
// MMDVMHost CDMRShortLC encodes it. It returns the errors of slc.Encode,
// and an error for a nil slc (ETSI TS 102 361-1 clause 7.1.4, Annex B.2.3).
func NewDMRShortLC(slc *pdu.ShortLC) (DMRShortLC, error) {
	if slc == nil {
		return DMRShortLC{}, errNilShortLC
	}
	info, err := slc.Encode()
	if err != nil {
		return DMRShortLC{}, err
	}
	v := uint64(info[0])<<32 | uint64(info[1])<<24 | uint64(info[2])<<16 | uint64(info[3])<<8 | uint64(info[4])
	var s DMRShortLC
	for i, f := range bptc.EncodeCACH(v >> 4) {
		bits.SetField(s.LC[:], i*cachFragmentBits, cachFragmentBits, uint64(f))
	}
	return s, nil
}

// ShortLC decodes the Short LC that s carries. Its FEC field combines the
// BPTC result and the CRC-8 check. It does not allocate. A nil s gives the
// zero ShortLC.
func (s *DMRShortLC) ShortLC() pdu.ShortLC {
	if s == nil {
		return pdu.ShortLC{}
	}
	info, res := s.info()
	slc := pdu.DecodeShortLC(info)
	slc.FEC = res.Combine(slc.FEC)
	return slc
}

func (s *DMRShortLC) info() ([5]byte, fec.Result) {
	var fragments [4]uint32
	for i := range fragments {
		fragments[i] = uint32(bits.Field(s.LC[:], i*cachFragmentBits, cachFragmentBits) & (1<<cachFragmentBits - 1))
	}
	info, res := bptc.DecodeCACH(fragments)
	v := info << 4
	return [5]byte{byte(v >> 32 & 0xFF), byte(v >> 24 & 0xFF), byte(v >> 16 & 0xFF), byte(v >> 8 & 0xFF), byte(v & 0xF0)}, res
}
