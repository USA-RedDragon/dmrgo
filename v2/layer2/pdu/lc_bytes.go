package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// LCBytes is the 72 LC bits of a Full LC without RS(12,9) parity, as
// FullLinkControl.Bytes returns them. Its methods read or write one field in
// place and leave every other bit as it is. After an edit, Codeword gives the
// new codeword and layer2.EncodeEmbeddedLC the new embedded LC.
// (ETSI TS 102 361-1 clause 9.1.6)
type LCBytes [9]byte

// NewVoiceLCBytes returns the LC octets of the voice call LC that NewVoiceLC
// returns: octet 0 holds the FLCO with PF and Reserved clear, octet 1 the
// standard FID, octet 2 the octet of so.Encode and octets 3 to 8 the
// addresses. It returns a *dmrgo.FieldRangeError when an address does not fit
// in 24 bits or a field of so does not fit its bits.
func NewVoiceLCBytes(group bool, target, source uint32, so layer3.ServiceOptions) (LCBytes, error) {
	if err := checkAddresses(target, source, errLCBytesTargetRange, errLCBytesSourceRange); err != nil {
		return LCBytes{}, err
	}
	octet, err := so.Encode()
	if err != nil {
		return LCBytes{}, err
	}
	l := LCBytes{byte(enums.FLCOUnitToUnitVoiceChannelUser), byte(enums.FIDStandardized), octet[0]}
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

// SetFLCO writes f into the FLCO and keeps the PF and Reserved bits. It
// returns a *dmrgo.FieldRangeError, and changes nothing, when f does not fit
// in 6 bits. DMRGateway clears the Reserved bit when it rewrites the FLCO;
// to do the same, call SetReserved(false) as well.
func (l *LCBytes) SetFLCO(f enums.FLCO) error {
	if l == nil {
		return dmrgo.ErrNilReceiver
	}
	if f>>6 != 0 {
		return errLCBytesFLCORange
	}
	l[0] = l[0]&0xC0 | byte(f)
	return nil
}

var errLCBytesFLCORange = &dmrgo.FieldRangeError{Type: "LCBytes", Field: "FLCO", Bits: 6}

// ProtectFlag returns the PF bit, bit 0 of octet 0.
func (l LCBytes) ProtectFlag() bool {
	return l[0]&0x80 != 0
}

// SetProtectFlag writes the PF bit, bit 0 of octet 0.
func (l *LCBytes) SetProtectFlag(pf bool) {
	if l == nil {
		return
	}
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
	if l == nil {
		return
	}
	if r {
		l[0] |= 0x40
	} else {
		l[0] &^= 0x40
	}
}

// FID returns the Feature set ID, octet 1.
func (l LCBytes) FID() enums.FID {
	return enums.FID(l[1])
}

// SetFID writes fid into octet 1.
func (l *LCBytes) SetFID(fid enums.FID) {
	if l == nil {
		return
	}
	l[1] = byte(fid)
}

// ServiceOptions returns octet 2, the Service Options of a Grp_V_Ch_Usr
// (FLCO 0) or UU_V_Ch_Usr (FLCO 3) LC. ok is false for other FLCOs, which
// use the octet for other fields.
func (l LCBytes) ServiceOptions() (so layer3.ServiceOptions, ok bool) {
	if !l.isVoiceChannelUser() {
		return so, false
	}
	return layer3.DecodeServiceOptions([1]byte{l[2]}), true
}

// SetServiceOptions writes so into octet 2 of a Grp_V_Ch_Usr or
// UU_V_Ch_Usr LC. It returns ErrNoServiceOptions for other FLCOs and a nil
// l, and a *dmrgo.FieldRangeError when a field of so does not fit its bits,
// and then changes nothing.
func (l *LCBytes) SetServiceOptions(so layer3.ServiceOptions) error {
	octet, err := so.Field()
	if err != nil {
		return err
	}
	if l == nil || !l.isVoiceChannelUser() {
		return ErrNoServiceOptions
	}
	l[2] = octet
	return nil
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
// ErrNoAddresses for other FLCOs and a *dmrgo.FieldRangeError when an
// address does not fit in 24 bits, and then changes nothing.
func (l *LCBytes) SetAddresses(target, source uint32) error {
	if l == nil {
		return dmrgo.ErrNilReceiver
	}
	if err := checkAddresses(target, source, errLCBytesTargetRange, errLCBytesSourceRange); err != nil {
		return err
	}
	if !l.isVoiceChannelUser() {
		return ErrNoAddresses
	}
	putUint24(l[3:6], target)
	putUint24(l[6:9], source)
	return nil
}

// Call returns the call of a Grp_V_Ch_Usr (FLCO 0) or UU_V_Ch_Usr (FLCO 3)
// LC: group for FLCO 0, the 24-bit target and the source. ok is false for
// other FLCOs. It ignores the FID, as Addresses does.
func (l LCBytes) Call() (group bool, target, source uint32, ok bool) {
	target, source, ok = l.Addresses()
	return ok && l.FLCO() == enums.FLCOGroupVoiceChannelUser, target, source, ok
}

// SetCall sets the FLCO to Grp_V_Ch_Usr (0) when group is true and to
// UU_V_Ch_Usr (3) when it is false, and writes the target and source
// addresses. It keeps the PF, Reserved, FID and Service Options bits. It
// returns a *dmrgo.FieldRangeError, and changes nothing, when an address
// does not fit in 24 bits.
func (l *LCBytes) SetCall(group bool, target, source uint32) error {
	if l == nil {
		return dmrgo.ErrNilReceiver
	}
	if err := checkAddresses(target, source, errLCBytesTargetRange, errLCBytesSourceRange); err != nil {
		return err
	}
	f := enums.FLCOUnitToUnitVoiceChannelUser
	if group {
		f = enums.FLCOGroupVoiceChannelUser
	}
	l[0] = l[0]&0xC0 | byte(f)
	putUint24(l[3:6], target)
	putUint24(l[6:9], source)
	return nil
}

// SetGroup sets the FLCO to Grp_V_Ch_Usr (0) when group is true and to
// UU_V_Ch_Usr (3) when it is false, and keeps every other bit, so the
// Service Options and addresses carry over. It returns ErrNoAddresses, and
// changes nothing, unless the FLCO is already one of the two.
func (l *LCBytes) SetGroup(group bool) error {
	if l == nil {
		return dmrgo.ErrNilReceiver
	}
	if !l.isVoiceChannelUser() {
		return ErrNoAddresses
	}
	f := enums.FLCOUnitToUnitVoiceChannelUser
	if group {
		f = enums.FLCOGroupVoiceChannelUser
	}
	l[0] = l[0]&0xC0 | byte(f)
	return nil
}
