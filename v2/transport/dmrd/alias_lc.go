package dmrd

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// LC decodes the talker alias block as the Full LC it came from: FLCO
// Talker Alias Header plus Type, standard FID, and Data. A 15-octet packet
// gives the 3 octets that came, and 0 in the others. It returns a
// *dmrgo.FieldRangeError when Type is above 3, and dmrgo.ErrNilReceiver for a nil t. It does not
// allocate (ETSI TS 102 361-2 clauses 7.1.1.4 and 7.1.1.5).
func (t *TalkerAlias) LC() (pdu.FullLinkControl, error) {
	if t == nil {
		return pdu.FullLinkControl{}, dmrgo.ErrNilReceiver
	}
	return aliasLC(t.Type, &t.Data, errTalkerAliasTypeRange)
}

// LC decodes the talker alias block as TalkerAlias.LC does.
func (t *LocalTalkerAlias) LC() (pdu.FullLinkControl, error) {
	if t == nil {
		return pdu.FullLinkControl{}, dmrgo.ErrNilReceiver
	}
	return aliasLC(t.Type, &t.Data, errLocalTalkerAliasTypeRange)
}

// LC decodes the position as the GPS Info Full LC it came from: FLCO GPS
// Info, standard FID, and Data. A 14-octet packet gives the 3 octets that
// came, and 0 in the others. It returns dmrgo.ErrNilReceiver for a nil p.
// It does not allocate (ETSI TS 102 361-2 clause 7.1.1.3).
func (p *Position) LC() (pdu.FullLinkControl, error) {
	if p == nil {
		return pdu.FullLinkControl{}, dmrgo.ErrNilReceiver
	}
	return lcOf(enums.FLCOGPSInfo, &p.Data)
}

// LC decodes the position as Position.LC does.
func (p *LocalPosition) LC() (pdu.FullLinkControl, error) {
	if p == nil {
		return pdu.FullLinkControl{}, dmrgo.ErrNilReceiver
	}
	return lcOf(enums.FLCOGPSInfo, &p.Data)
}

func aliasLC(typ uint8, data *[7]byte, errType error) (pdu.FullLinkControl, error) {
	if typ > 3 {
		return pdu.FullLinkControl{}, errType
	}
	return lcOf(enums.FLCOTalkerAliasHeader+enums.FLCO(typ), data)
}

func lcOf(f enums.FLCO, data *[7]byte) (pdu.FullLinkControl, error) {
	lc := [9]byte{byte(f), byte(enums.FIDStandardized)}
	copy(lc[2:], data[:])
	return pdu.DecodeFullLinkControlBytes(lc), nil
}
