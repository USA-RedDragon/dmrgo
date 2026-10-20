package bs

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// CACH makes the CACH field that goes before each outbound burst: the TACT
// with the TC bit, the AT bit and the LCSS, and one fragment of a Short LC.
// A Short LC takes 4 CACH fields, 120 ms (ETSI TS 102 361-1 clauses 6.3,
// 7.1.4 and 9.1.4). The zero CACH is ready: its first Next starts a Short
// LC.
type CACH struct {
	frags [4]layer2.CACH
	n     uint8
}

// First reports whether the next call of Next starts a new Short LC, so
// that the caller builds the Short LC only then.
func (c *CACH) First() bool {
	return c.n == 0
}

// Next returns the CACH field before the outbound burst of tc, in transmit
// order, packed as layer2.DecodeCACHField takes it. busy is the AT bit.
// When First is true, Next encodes slc and sends its first fragment; else
// it ignores slc and sends the next fragment. It returns ErrSlot for a tc
// other than Slot1 or Slot2, and the error of pdu.ShortLC.Encode, after
// which First stays true.
func (c *CACH) Next(tc enums.Slot, busy bool, slc *pdu.ShortLC) ([3]byte, error) {
	if tc != enums.Slot1 && tc != enums.Slot2 {
		return [3]byte{}, ErrSlot
	}
	if c.n == 0 {
		frags, err := layer2.EncodeShortLCToCACH(slc, pdu.TACT{})
		if err != nil {
			return [3]byte{}, err
		}
		for i := range frags {
			c.frags[i] = layer2.DecodeCACHField(frags[i])
		}
	}
	f := c.frags[c.n]
	c.n = (c.n + 1) % 4
	f.TACT.AccessType, f.TACT.TDMAChannel = busy, tc == enums.Slot2
	logical, err := f.Encode()
	if err != nil {
		return [3]byte{}, err
	}
	return layer2.CACHInterleave(logical), nil
}

// Reset makes the next Next start a new Short LC.
func (c *CACH) Reset() {
	c.n = 0
}
