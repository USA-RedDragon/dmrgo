package packet

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Bursts returns the on-air data bursts of p: the Data Header burst, then
// one Rate 1/2, Rate 3/4 or Rate 1 burst per block, each with a slot type of
// color code colorCode and the data SYNC pattern of src. It returns an error
// when the header does not encode, a block is not a *pdu.Rate12Data,
// *pdu.Rate34Data or *pdu.Rate1Data, colorCode does not fit in 4 bits or src
// is unknown.
func Bursts(p Packet, colorCode uint8, src layer2.SyncSource) ([][33]byte, error) {
	hdr, err := p.Header.Encode()
	if err != nil {
		return nil, fmt.Errorf("header: %w", err)
	}
	raw, err := layer2.BuildDataBurst(bit.Pack96(hdr), enums.DataTypeDataHeader, colorCode, src)
	if err != nil {
		return nil, fmt.Errorf("header: %w", err)
	}
	out := make([][33]byte, 0, 1+len(p.Blocks))
	out = append(out, raw)
	for i, d := range p.Blocks {
		switch b := d.(type) {
		case *pdu.Rate12Data:
			raw, err = layer2.BuildDataBurst(b.Data, enums.DataTypeRate12, colorCode, src)
		case *pdu.Rate34Data:
			raw, err = layer2.BuildRate34DataBurst(b.Data, colorCode, src)
		case *pdu.Rate1Data:
			raw, err = layer2.BuildRate1DataBurst(b.Data, colorCode, src)
		default:
			err = fmt.Errorf("%w: %T", ErrRate, d)
		}
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", i, err)
		}
		out = append(out, raw)
	}
	return out, nil
}
