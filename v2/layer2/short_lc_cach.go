package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
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
// First, Continuation, Continuation and Last (ETSI TS 102 361-1 clause
// 7.1.4).
func EncodeShortLCToCACH(slc *pdu.ShortLC, tact pdu.TACT) ([4][CACHBits]bit.Bit, error) {
	var out [4][CACHBits]bit.Bit
	info, err := slc.Encode()
	if err != nil {
		return out, fmt.Errorf("cannot encode short LC: %w", err)
	}
	fragments := bptc.EncodeCACH(bit.Uint(info[:]))
	for i, fragment := range fragments {
		c := CACH{TACT: tact}
		bit.PutUint(c.Payload[:], uint64(fragment))
		c.TACT.LCSS = shortLCFragmentLCSS[i]
		logical, err := c.Encode()
		if err != nil {
			return out, err
		}
		out[i] = CACHInterleave(logical)
	}
	return out, nil
}
