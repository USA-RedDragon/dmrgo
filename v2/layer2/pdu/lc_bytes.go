package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// LCBytes is the 72 LC bits of a Full LC without RS(12,9) parity, as
// FullLinkControl.Bytes returns them (ETSI TS 102 361-1 clause 9.1.6). Its
// methods read or write one field in place and leave every other bit as it
// is. After an edit, Codeword gives the new codeword and
// layer2.EncodeEmbeddedLC the new embedded LC.
type LCBytes [9]byte

// NewVoiceLCBytes returns the LC octets of the voice call LC that NewVoiceLC
// returns: octet 0 holds the FLCO with PF and Reserved clear, octet 1 the
// standard FID, octet 2 so.Byte() and octets 3 to 8 the addresses. It
// returns a *bit.FieldRangeError when an address does not fit in 24 bits.
func NewVoiceLCBytes(group bool, target, source uint32, so layer3.ServiceOptions) (LCBytes, error) {
	if err := checkAddresses("LCBytes", target, source); err != nil {
		return LCBytes{}, err
	}
	l := LCBytes{byte(enums.FLCOUnitToUnitVoiceChannelUser), byte(enums.FIDStandardized), so.Byte()}
	if group {
		l[0] = byte(enums.FLCOGroupVoiceChannelUser)
	}
	putUint24(l[3:], target)
	putUint24(l[6:], source)
	return l, nil
}

// FLCO returns the 6-bit FLCO, bits 2 to 7 of octet 0.
func (l LCBytes) FLCO() enums.FLCO {
	return enums.FLCO(l[0] & 0x3F)
}

// SetFLCO writes the low 6 bits of f into the FLCO and keeps the PF and
// Reserved bits. DMRGateway clears the Reserved bit when it rewrites the
// FLCO; to do the same, call SetReserved(false) as well.
func (l *LCBytes) SetFLCO(f enums.FLCO) {
	l[0] = l[0]&0xC0 | byte(f)&0x3F
}

// ProtectFlag returns the PF bit, bit 0 of octet 0.
func (l LCBytes) ProtectFlag() bool {
	return l[0]&0x80 != 0
}

// SetProtectFlag writes the PF bit, bit 0 of octet 0.
func (l *LCBytes) SetProtectFlag(pf bool) {
	if pf {
		l[0] |= 0x80
	} else {
		l[0] &^= 0x80
	}
}

// Reserved returns the Reserved bit, bit 1 of octet 0. The standard does not
// define it (ETSI TS 102 361-1 clause 9.1.6, Table 9.6).
func (l LCBytes) Reserved() bool {
	return l[0]&0x40 != 0
}

// SetReserved writes the Reserved bit, bit 1 of octet 0.
func (l *LCBytes) SetReserved(r bool) {
	if r {
		l[0] |= 0x40
	} else {
		l[0] &^= 0x40
	}
}

// FID returns the Feature set ID, octet 1.
func (l LCBytes) FID() enums.FeatureSetID {
	return enums.FeatureSetID(l[1])
}

// SetFID writes fid into octet 1.
func (l *LCBytes) SetFID(fid enums.FeatureSetID) {
	l[1] = byte(fid)
}

// ServiceOptions returns octet 2, the Service Options of a Grp_V_Ch_Usr
// or UU_V_Ch_Usr LC. Other FLCOs use the octet for other fields.
func (l LCBytes) ServiceOptions() byte {
	return l[2]
}

// SetServiceOptions writes so into octet 2.
func (l *LCBytes) SetServiceOptions(so byte) {
	l[2] = so
}

func (l LCBytes) isVoiceChannelUser() bool {
	f := l.FLCO()
	return f == enums.FLCOGroupVoiceChannelUser || f == enums.FLCOUnitToUnitVoiceChannelUser
}

// Addresses returns the 24-bit target (octets 3 to 5) and source (octets 6
// to 8) addresses of a Grp_V_Ch_Usr (FLCO 0) or UU_V_Ch_Usr (FLCO 3) LC.
// For a group call the target is the group address. It ignores the FID, as
// FullLinkControl.Addresses does, so for a manufacturer FID the addresses
// are a best effort. ok is false for other FLCOs.
func (l LCBytes) Addresses() (target, source uint32, ok bool) {
	if !l.isVoiceChannelUser() {
		return 0, 0, false
	}
	target = uint32(l[3])<<16 | uint32(l[4])<<8 | uint32(l[5])
	source = uint32(l[6])<<16 | uint32(l[7])<<8 | uint32(l[8])
	return target, source, true
}

// SetAddresses writes target into octets 3 to 5 and source into octets 6
// to 8 of a Grp_V_Ch_Usr or UU_V_Ch_Usr LC, whatever the FID. It returns
// false, and changes nothing, for other FLCOs or when an address does not
// fit in 24 bits.
func (l *LCBytes) SetAddresses(target, source uint32) bool {
	if !l.isVoiceChannelUser() || target>>24 != 0 || source>>24 != 0 {
		return false
	}
	l[3], l[4], l[5] = byte(target>>16), byte(target>>8), byte(target) //nolint:gosec // 24-bit address split into octets
	l[6], l[7], l[8] = byte(source>>16), byte(source>>8), byte(source) //nolint:gosec // 24-bit address split into octets
	return true
}

// Group reports whether the FLCO is Grp_V_Ch_Usr (0). ok is false unless
// the FLCO is Grp_V_Ch_Usr or UU_V_Ch_Usr (3).
func (l LCBytes) Group() (group, ok bool) {
	if !l.isVoiceChannelUser() {
		return false, false
	}
	return l.FLCO() == enums.FLCOGroupVoiceChannelUser, true
}

// SetGroup sets the FLCO to Grp_V_Ch_Usr (0) when group is true and to
// UU_V_Ch_Usr (3) when it is false, and keeps every other bit, so the
// Service Options and addresses carry over. It returns false, and changes
// nothing, unless the FLCO is already one of the two.
func (l *LCBytes) SetGroup(group bool) bool {
	if !l.isVoiceChannelUser() {
		return false
	}
	if group {
		l.SetFLCO(enums.FLCOGroupVoiceChannelUser)
	} else {
		l.SetFLCO(enums.FLCOUnitToUnitVoiceChannelUser)
	}
	return true
}
