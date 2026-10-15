package packet

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Bursts returns the on-air data bursts of p: the Data Header burst, then
// one Rate 1/2, Rate 3/4 or Rate 1 burst per block, each with a slot type of
// color code colorCode and the data SYNC pattern of src. It returns an error
// when the header or a block does not encode, a block has another data type,
// or src is unknown.
func Bursts(p Packet, colorCode enums.ColorCode, src layer2.SyncSource) ([]layer2.RawBurst, error) {
	raw, err := layer2.BuildBurst(&p.Header, colorCode, src)
	if err != nil {
		return nil, fmt.Errorf("header: %w", err)
	}
	out := make([]layer2.RawBurst, 0, 1+len(p.Blocks))
	out = append(out, raw)
	for i, d := range p.Blocks {
		raw, err = blockBurst(d, colorCode, src)
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", i, err)
		}
		out = append(out, raw)
	}
	return out, nil
}

func blockBurst(d layer2.Data, colorCode enums.ColorCode, src layer2.SyncSource) (layer2.RawBurst, error) {
	if dt := d.DataType(); dt != enums.DataTypeRate12 && dt != enums.DataTypeRate34 && dt != enums.DataTypeRate1 {
		return layer2.RawBurst{}, fmt.Errorf("%w: %T with data type %s", ErrRate, d, dt.String())
	}
	return layer2.BuildBurst(d, colorCode, src)
}
