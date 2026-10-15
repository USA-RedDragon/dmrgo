package packet

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Bursts returns the on-air data bursts of p: the Data Header burst, then
// one Rate 1/2, Rate 3/4 or Rate 1 burst per block, each with a slot type of
// color code colorCode and the data SYNC pattern of src. A block is any
// layer2.Data whose DataType is one of these rates, coded from its
// EncodeBits. It returns an error when the header or a block does not
// encode, a block has another data type, colorCode does not fit in 4 bits
// or src is unknown.
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
		raw, err = blockBurst(d, colorCode, src)
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", i, err)
		}
		out = append(out, raw)
	}
	return out, nil
}

func blockBurst(d layer2.Data, colorCode uint8, src layer2.SyncSource) ([33]byte, error) {
	dt := d.DataType()
	want := map[enums.DataType]int{enums.DataTypeRate12: 96, enums.DataTypeRate34: 144, enums.DataTypeRate1: 192}[dt]
	if want == 0 {
		return [33]byte{}, fmt.Errorf("%w: %T with data type %s", ErrRate, d, dt.String())
	}
	bits, err := d.EncodeBits()
	if err != nil {
		return [33]byte{}, err
	}
	if len(bits) != want {
		return [33]byte{}, fmt.Errorf("%T encodes %d bits, data type %s carries %d", d, len(bits), dt.String(), want)
	}
	packed := bit.Pack(bits)
	if dt == enums.DataTypeRate34 {
		return layer2.BuildRate34DataBurst([18]byte(packed), colorCode, src)
	}
	if dt == enums.DataTypeRate1 {
		return layer2.BuildRate1DataBurst([24]byte(packed), colorCode, src)
	}
	return layer2.BuildDataBurst([12]byte(packed), dt, colorCode, src)
}
