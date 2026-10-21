package dmrd

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// LC decodes the talker alias block as the Full LC it came from: FLCO
// Talker Alias Header plus Type, standard FID, and Data. A 15-octet packet
// gives the 3 octets that came, and 0 in the others. It returns an error
// when Type is above 3 or the LC does not decode, and for a nil t. It
// allocates the payload struct; LCInto does not (ETSI TS 102 361-2 clauses
// 7.1.1.4 and 7.1.1.5).
func (t *TalkerAlias) LC() (pdu.FullLinkControl, error) {
	if t == nil {
		return pdu.FullLinkControl{}, errNilMessage
	}
	return aliasLC(t.Type, &t.Data)
}

// LC decodes the talker alias block as TalkerAlias.LC does. It allocates
// the payload struct; LCInto does not.
func (t *LocalTalkerAlias) LC() (pdu.FullLinkControl, error) {
	if t == nil {
		return pdu.FullLinkControl{}, errNilMessage
	}
	return aliasLC(t.Type, &t.Data)
}

// LC decodes the position as the GPS Info Full LC it came from: FLCO GPS
// Info, standard FID, and Data. A 14-octet packet gives the 3 octets that
// came, and 0 in the others. It returns an error when the LC does not
// decode, and for a nil p. It allocates the payload struct; LCInto does
// not (ETSI TS 102 361-2 clause 7.1.1.3).
func (p *Position) LC() (pdu.FullLinkControl, error) {
	if p == nil {
		return pdu.FullLinkControl{}, errNilMessage
	}
	return lcOf(enums.FLCOGPSInfo, &p.Data)
}

// LC decodes the position as Position.LC does. It allocates the payload
// struct; LCInto does not.
func (p *LocalPosition) LC() (pdu.FullLinkControl, error) {
	if p == nil {
		return pdu.FullLinkControl{}, errNilMessage
	}
	return lcOf(enums.FLCOGPSInfo, &p.Data)
}

func aliasLC(typ uint8, data *[7]byte) (pdu.FullLinkControl, error) {
	if typ > 3 {
		return pdu.FullLinkControl{}, errAliasType
	}
	return lcOf(enums.FLCOTalkerAliasHeader+enums.FLCO(typ), data)
}

func lcOf(f enums.FLCO, data *[7]byte) (pdu.FullLinkControl, error) {
	lc := [9]byte{byte(f), byte(enums.FIDStandardized)}
	copy(lc[2:], data[:])
	return pdu.DecodeFullLinkControlBytes(lc)
}

// LCInto decodes the block into dst as LC does, with the payload struct in
// p as pdu.DecodeFullLinkControlBytesInto places it. It does not allocate.
func (t *TalkerAlias) LCInto(dst *pdu.FullLinkControl, p *pdu.FullLinkControlPayloads) error {
	if t == nil {
		return errNilMessage
	}
	return aliasLCInto(dst, p, t.Type, &t.Data)
}

// LCInto decodes the block into dst as TalkerAlias.LCInto does.
func (t *LocalTalkerAlias) LCInto(dst *pdu.FullLinkControl, p *pdu.FullLinkControlPayloads) error {
	if t == nil {
		return errNilMessage
	}
	return aliasLCInto(dst, p, t.Type, &t.Data)
}

// LCInto decodes the position into dst as LC does, with the payload struct
// in pl as pdu.DecodeFullLinkControlBytesInto places it. It does not
// allocate.
func (p *Position) LCInto(dst *pdu.FullLinkControl, pl *pdu.FullLinkControlPayloads) error {
	if p == nil {
		return errNilMessage
	}
	return lcInto(dst, pl, enums.FLCOGPSInfo, &p.Data)
}

// LCInto decodes the position into dst as Position.LCInto does.
func (p *LocalPosition) LCInto(dst *pdu.FullLinkControl, pl *pdu.FullLinkControlPayloads) error {
	if p == nil {
		return errNilMessage
	}
	return lcInto(dst, pl, enums.FLCOGPSInfo, &p.Data)
}

func aliasLCInto(dst *pdu.FullLinkControl, p *pdu.FullLinkControlPayloads, typ uint8, data *[7]byte) error {
	if typ > 3 {
		return errAliasType
	}
	return lcInto(dst, p, enums.FLCOTalkerAliasHeader+enums.FLCO(typ), data)
}

func lcInto(dst *pdu.FullLinkControl, p *pdu.FullLinkControlPayloads, f enums.FLCO, data *[7]byte) error {
	if dst == nil || p == nil {
		return errNilMessage
	}
	lc := [9]byte{byte(f), byte(enums.FIDStandardized)}
	copy(lc[2:], data[:])
	return pdu.DecodeFullLinkControlBytesInto(dst, p, lc)
}
