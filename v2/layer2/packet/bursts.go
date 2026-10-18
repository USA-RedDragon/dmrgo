package packet

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Bursts returns the on-air data bursts of p: the Data Header burst, then
// one Rate 1/2, Rate 3/4 or Rate 1 burst per block, each with a slot type of
// color code colorCode and the data SYNC pattern of src. It returns an error
// when the header or a block does not encode, a block has another data type,
// or src is unknown, and a *dmrgo.FieldRangeError when colorCode does not fit
// in 4 bits. It allocates the slice; AppendBursts does not.
func Bursts(p Packet, colorCode enums.ColorCode, src layer2.SyncSource) ([]layer2.RawBurst, error) {
	out, err := AppendBursts(make([]layer2.RawBurst, 0, 1+len(p.Blocks)), &p, colorCode, src)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AppendBursts appends the bursts that Bursts returns to dst and returns the
// extended slice. On error it returns dst unchanged. It allocates only when
// dst has no room.
func AppendBursts(dst []layer2.RawBurst, p *Packet, colorCode enums.ColorCode, src layer2.SyncSource) ([]layer2.RawBurst, error) {
	n := len(dst)
	coded, err := p.Header.Encode()
	if err != nil {
		return dst, err
	}
	raw, err := layer2.BuildDataBurst(coded, enums.DataTypeDataHeader, colorCode, src)
	if err != nil {
		return dst, err
	}
	dst = append(dst, raw)
	for _, d := range p.Blocks {
		raw, err = blockBurst(d, colorCode, src)
		if err != nil {
			return dst[:n], err
		}
		dst = append(dst, raw)
	}
	return dst, nil
}

// blockBurst returns the burst of the data block d. It encodes the block
// types of the pdu package without a call through an interface, so that
// escape analysis keeps the Packet of AppendBursts off the heap.
func blockBurst(d layer2.Data, colorCode enums.ColorCode, src layer2.SyncSource) (layer2.RawBurst, error) {
	switch b := d.(type) {
	case *pdu.Rate12Data:
		if b != nil {
			coded, err := b.Encode()
			if err != nil {
				return layer2.RawBurst{}, err
			}
			return layer2.BuildDataBurst(coded, enums.DataTypeRate12, colorCode, src)
		}
	case *pdu.Rate34Data:
		if b != nil {
			coded, err := b.Encode()
			if err != nil {
				return layer2.RawBurst{}, err
			}
			return layer2.BuildRate34DataBurst(coded, colorCode, src)
		}
	case *pdu.Rate1Data:
		if b != nil {
			coded, err := b.Encode()
			if err != nil {
				return layer2.RawBurst{}, err
			}
			return layer2.BuildRate1DataBurst(coded, colorCode, src)
		}
	}
	if d == nil {
		return layer2.BuildBurst(d, colorCode, src)
	}
	if dt := d.DataType(); dt != enums.DataTypeRate12 && dt != enums.DataTypeRate34 && dt != enums.DataTypeRate1 {
		return layer2.RawBurst{}, ErrRate
	}
	return layer2.BuildBurst(d, colorCode, src)
}
