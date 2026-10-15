package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

//nolint:gochecknoglobals
var shortLCFragmentLCSS = [4]enums.LCSS{
	enums.FirstFragmentLC,
	enums.ContinuationFragmentLCorCSBK,
	enums.ContinuationFragmentLCorCSBK,
	enums.LastFragmentLCorCSBK,
}

// EncodeShortLCToCACH encodes a Short LC PDU into the 4 CACH fields that
// carry it (ETSI TS 102 361-1 §7.1.4), in transmit order as CACHInterleave
// returns them. Each field carries tact with its LCSS set to First,
// Continuation, Continuation and Last. It is the inverse of
// CACHDeinterleave, DecodeCACH and ShortLCAssembler.
func EncodeShortLCToCACH(slc *pdu.ShortLC, tact pdu.TACT) ([4][CACHBits]bit.Bit, error) {
	var out [4][CACHBits]bit.Bit
	info, err := pdu.EncodeShortLC(slc)
	if err != nil {
		return out, fmt.Errorf("cannot encode short LC: %w", err)
	}
	fragments := bptc.EncodeCACHBPTC(info)
	for i, fragment := range fragments {
		c := CACH{TACT: tact, Payload: fragment}
		c.TACT.LCSS = shortLCFragmentLCSS[i]
		logical, err := EncodeCACH(&c)
		if err != nil {
			return out, err
		}
		out[i] = CACHInterleave(logical)
	}
	return out, nil
}
