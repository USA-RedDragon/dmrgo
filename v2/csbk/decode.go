package csbk

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

type rxKind uint8

const (
	rxNone rxKind = iota
	rxCSBK
	rxDataHeader
	rxMBCHeader
	rxOther
)

type decoder struct {
	b        *layer2.Burst
	opts     layer2.DecodeOptions
	dataType enums.DataType
}

func newDecoder(scratch *layer2.Burst) decoder {
	if scratch == nil {
		scratch = layer2.NewBurst()
	}
	return decoder{b: scratch}
}

func (d *decoder) decode(raw *layer2.RawBurst, h *layer2.Hint) *layer2.Burst {
	if raw == nil || d.b == nil {
		return nil
	}
	d.opts.Kind, d.opts.VoiceBurst, d.opts.DataType = h.Kind, enums.VoiceBurstUnknown, nil
	if h.Kind == layer2.KindVoice {
		d.opts.VoiceBurst = h.VoiceBurst
	}
	if h.HasDataType {
		d.dataType = h.DataType
		d.opts.DataType = &d.dataType
	}
	if d.b.DecodeFromBytesWith(*raw, &d.opts) != nil {
		return nil
	}
	return d.b
}

func classify(b *layer2.Burst) (rxKind, *pdu.CSBK, pdu.CSBKBytes) {
	if b == nil {
		return rxNone, nil, pdu.CSBKBytes{}
	}
	if b.Kind != layer2.KindData {
		return rxOther, nil, pdu.CSBKBytes{}
	}
	if b.SlotType.FEC.Uncorrectable {
		return rxNone, nil, pdu.CSBKBytes{}
	}
	switch b.SlotType.DataType {
	case enums.DataTypeCSBK:
		c, ok := b.Data.(*pdu.CSBK)
		if !ok || c.FEC.Uncorrectable {
			return rxNone, nil, pdu.CSBKBytes{}
		}
		octets, err := c.Encode()
		if err != nil {
			return rxNone, nil, pdu.CSBKBytes{}
		}
		return rxCSBK, c, pdu.CSBKBytes(octets)
	case enums.DataTypeDataHeader:
		return rxDataHeader, nil, pdu.CSBKBytes{}
	case enums.DataTypeMBCHeader:
		return rxMBCHeader, nil, pdu.CSBKBytes{}
	case enums.DataTypePIHeader, enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC,
		enums.DataTypeMBCContinuation, enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeIdle,
		enums.DataTypeRate1, enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
	}
	return rxOther, nil, pdu.CSBKBytes{}
}

func addresses(c *pdu.CSBK) (target, source uint32, ok bool) {
	if c.FID == enums.FIDStandardized {
		return c.Addresses()
	}
	if p, ok := c.AsMotorolaRadioCheck(); ok {
		target, source = p.Addresses()
		return target, source, true
	}
	if p, ok := c.AsMotorolaCallAlert(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := c.AsMotorolaCallAlertAck(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	r := &c.RawPayload
	return uint32(r[2])<<16 | uint32(r[3])<<8 | uint32(r[4]), uint32(r[5])<<16 | uint32(r[6])<<8 | uint32(r[7]), true
}

func checkAddress(a uint32, err error) error {
	if a>>24 != 0 {
		return err
	}
	return nil
}
