package packet

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// Bursts returns the on-air data bursts of p: the Data Header burst, then
// one Rate 1/2, Rate 3/4 or Rate 1 burst per block, each with a slot type of
// color code colorCode and the data SYNC pattern of src. It returns an error
// when the header or a block does not encode, a block has another data type,
// or src is unknown, and a *dmrgo.FieldRangeError when colorCode does not fit
// in 4 bits. A nil p returns dmrgo.ErrNilArgument. It allocates the slice;
// AppendBursts does not.
func Bursts(p *Packet, colorCode enums.ColorCode, src layer2.SyncSource) ([]layer2.RawBurst, error) {
	if p == nil {
		return nil, dmrgo.ErrNilArgument
	}
	out, err := AppendBursts(make([]layer2.RawBurst, 0, 1+len(p.Blocks)), p, colorCode, src)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AppendBursts appends the bursts that Bursts returns to dst and returns the
// extended slice. On error, and for a nil p, it returns dst unchanged with
// an error. It allocates only when dst has no room.
func AppendBursts(dst []layer2.RawBurst, p *Packet, colorCode enums.ColorCode, src layer2.SyncSource) ([]layer2.RawBurst, error) {
	if p == nil {
		return dst, dmrgo.ErrNilArgument
	}
	n := len(dst)
	raw, err := layer2.BuildBurst(&p.Header, colorCode, src)
	if err != nil {
		return dst, err
	}
	dst = append(dst, raw)
	for _, d := range p.Blocks {
		switch d.(type) {
		case nil, *pdu.Rate12Data, *pdu.Rate34Data, *pdu.Rate1Data:
		default:
			return dst[:n], ErrRate
		}
		if raw, err = layer2.BuildBurst(d, colorCode, src); err != nil {
			return dst[:n], err
		}
		dst = append(dst, raw)
	}
	return dst, nil
}
