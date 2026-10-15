package pdu

import "github.com/USA-RedDragon/dmrgo/v2/enums"

// addressFields returns pointers to the target and source address fields of
// the payload that is set, or nil when the CSBK has no such payload.
func (s *CSBK) addressFields() (target, source *uint32) {
	switch {
	case s.UnitToUnitVoiceServiceRequestPDU != nil:
		p := s.UnitToUnitVoiceServiceRequestPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.UnitToUnitVoiceServiceAnswerResponsePDU != nil:
		p := s.UnitToUnitVoiceServiceAnswerResponsePDU
		return &p.TargetAddress, &p.SourceAddress
	case s.NegativeAcknowledgementPDU != nil:
		p := s.NegativeAcknowledgementPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.PreamblePDU != nil:
		p := s.PreamblePDU
		return &p.TargetAddress, &p.SourceAddress
	case s.PrivateVoiceGrantPDU != nil:
		p := s.PrivateVoiceGrantPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.TalkgroupVoiceGrantPDU != nil:
		p := s.TalkgroupVoiceGrantPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.BroadcastTalkgroupVoiceGrantPDU != nil:
		p := s.BroadcastTalkgroupVoiceGrantPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.PrivateDataGrantPDU != nil:
		p := s.PrivateDataGrantPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.TalkgroupDataGrantPDU != nil:
		p := s.TalkgroupDataGrantPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.DuplexPrivateVoiceGrantPDU != nil:
		p := s.DuplexPrivateVoiceGrantPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.DuplexPrivateDataGrantPDU != nil:
		p := s.DuplexPrivateDataGrantPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.PrivateDataGrantMultiItemPDU != nil:
		p := s.PrivateDataGrantMultiItemPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.ClearPDU != nil:
		p := s.ClearPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.ProtectPDU != nil:
		p := s.ProtectPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.AhoyPDU != nil:
		p := s.AhoyPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.RandomAccessPDU != nil:
		p := s.RandomAccessPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.AckvitationPDU != nil:
		p := s.AckvitationPDU
		return &p.TargetAddress, &p.SourceAddress
	case s.MaintenancePDU != nil:
		p := s.MaintenancePDU
		return &p.TargetAddress, &p.SourceAddress
	}
	return nil, nil
}

// is0x38 reports whether s is a standard FID CSBK with opcode 0x38, whose
// payload is in RawPayload.
func (s *CSBK) is0x38() bool {
	return s.Opcode == CSBKBSOutboundActivation && s.FID == enums.FIDStandardized
}

// Addresses returns the 24-bit target and source addresses of a CSBK whose
// payload carries both: the TargetAddress and SourceAddress fields of the
// payload, or the same bits of RawPayload for opcode 0x38. ok is false for a
// payload with one address or none (C_MOVE, C_ALOHA, C_BCAST, C_ACKD, C_ACKU,
// P_ACKD, P_ACKU and CT_CSBK) and for the RawPayload of another FID.
func (s *CSBK) Addresses() (target, source uint32, ok bool) {
	if s.is0x38() {
		return uint24(s.RawPayload[2:5]), uint24(s.RawPayload[5:8]), true
	}
	t, src := s.addressFields()
	if t == nil {
		return 0, 0, false
	}
	return *t, *src, true
}

// SetAddresses sets the target and source addresses of a CSBK for which
// Addresses returns ok. It returns false, and changes nothing, for other
// CSBKs or when an address does not fit in 24 bits.
func (s *CSBK) SetAddresses(target, source uint32) bool {
	if target>>24 != 0 || source>>24 != 0 {
		return false
	}
	if s.is0x38() {
		putUint24(s.RawPayload[2:5], target)
		putUint24(s.RawPayload[5:8], source)
		return true
	}
	t, src := s.addressFields()
	if t == nil {
		return false
	}
	*t, *src = target, source
	return true
}
