package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

var shortLCFragmentLCSS = [4]enums.LCSS{
	enums.LCSSFirstFragment,
	enums.LCSSContinuationFragment,
	enums.LCSSContinuationFragment,
	enums.LCSSLastFragment,
}

// EncodeShortLCToCACH encodes a Short LC PDU into the 4 CACH fields that
// carry it, in transmit order. Each field carries tact with its LCSS set to
// First, Continuation, Continuation and Last. It returns an error for a nil
// slc or one that does not encode (ETSI TS 102 361-1 clause 7.1.4).
func EncodeShortLCToCACH(slc *pdu.ShortLC, tact pdu.TACT) ([4][3]byte, error) {
	var out [4][3]byte
	if slc == nil {
		return out, dmrgo.ErrNilArgument
	}
	info, err := slc.Encode()
	if err != nil {
		return out, err
	}
	v := uint64(info[0])<<32 | uint64(info[1])<<24 | uint64(info[2])<<16 | uint64(info[3])<<8 | uint64(info[4])
	fragments := bptc.EncodeCACH(v >> 4)
	for i, fragment := range fragments {
		c := CACH{TACT: tact}
		p := fragment << 7
		c.Payload = [3]byte{byte(p >> 16 & 0xFF), byte(p >> 8 & 0xFF), byte(p & 0x80)}
		c.TACT.LCSS = shortLCFragmentLCSS[i]
		logical, err := c.Encode()
		if err != nil {
			return out, err
		}
		out[i] = CACHInterleave(logical)
	}
	return out, nil
}
