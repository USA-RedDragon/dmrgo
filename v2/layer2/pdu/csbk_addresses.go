package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

func (s *CSBK) is0x38() bool {
	return s.Opcode == CSBKBSOutboundActivation && s.FID == enums.FIDStandardized
}

// Addresses returns the 24-bit target and source addresses of a CSBK whose
// payload carries both: the TargetAddress and SourceAddress fields of the
// payload, or the same bits of RawPayload for opcode 0x38. ok is false for a
// payload with one address or none (C_MOVE, C_ALOHA, C_BCAST, C_ACKD, C_ACKU,
// P_ACKD, P_ACKU and CT_CSBK) and for the RawPayload of another FID.
func (s *CSBK) Addresses() (target, source uint32, ok bool) {
	if s == nil {
		return 0, 0, false
	}
	if s.is0x38() {
		return uint24(s.RawPayload[2:5]), uint24(s.RawPayload[5:8]), true
	}
	if p, ok := s.AsUnitToUnitVoiceServiceRequest(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsUnitToUnitVoiceServiceAnswerResponse(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsNegativeAcknowledgement(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsPreamble(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsPrivateVoiceGrant(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsTalkgroupVoiceGrant(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsBroadcastTalkgroupVoiceGrant(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsPrivateDataGrant(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsTalkgroupDataGrant(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsDuplexPrivateVoiceGrant(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsDuplexPrivateDataGrant(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsPrivateDataGrantMultiItem(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsClear(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsProtect(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsAhoy(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsRandomAccess(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsAckvitation(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	if p, ok := s.AsMaintenance(); ok {
		return p.TargetAddress, p.SourceAddress, true
	}
	return 0, 0, false
}

// SetAddresses sets the target and source addresses of a CSBK for which
// Addresses returns ok. It returns ErrNoAddresses for other CSBKs and a
// *dmrgo.FieldRangeError when an address does not fit in 24 bits, and then
// changes nothing.
func (s *CSBK) SetAddresses(target, source uint32) error {
	if s == nil {
		return dmrgo.ErrNilReceiver
	}
	if err := checkAddresses(target, source, errCSBKTargetRange, errCSBKSourceRange); err != nil {
		return err
	}
	if s.is0x38() {
		putUint24(s.RawPayload[2:5], target)
		putUint24(s.RawPayload[5:8], source)
		return nil
	}
	if p, ok := s.AsUnitToUnitVoiceServiceRequest(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetUnitToUnitVoiceServiceRequest(p)
	}
	if p, ok := s.AsUnitToUnitVoiceServiceAnswerResponse(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetUnitToUnitVoiceServiceAnswerResponse(p)
	}
	if p, ok := s.AsNegativeAcknowledgement(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetNegativeAcknowledgement(p)
	}
	if p, ok := s.AsPreamble(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetPreamble(p)
	}
	if p, ok := s.AsPrivateVoiceGrant(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetPrivateVoiceGrant(p)
	}
	if p, ok := s.AsTalkgroupVoiceGrant(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetTalkgroupVoiceGrant(p)
	}
	if p, ok := s.AsBroadcastTalkgroupVoiceGrant(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetBroadcastTalkgroupVoiceGrant(p)
	}
	if p, ok := s.AsPrivateDataGrant(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetPrivateDataGrant(p)
	}
	if p, ok := s.AsTalkgroupDataGrant(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetTalkgroupDataGrant(p)
	}
	if p, ok := s.AsDuplexPrivateVoiceGrant(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetDuplexPrivateVoiceGrant(p)
	}
	if p, ok := s.AsDuplexPrivateDataGrant(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetDuplexPrivateDataGrant(p)
	}
	if p, ok := s.AsPrivateDataGrantMultiItem(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetPrivateDataGrantMultiItem(p)
	}
	if p, ok := s.AsClear(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetClear(p)
	}
	if p, ok := s.AsProtect(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetProtect(p)
	}
	if p, ok := s.AsAhoy(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetAhoy(p)
	}
	if p, ok := s.AsRandomAccess(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetRandomAccess(p)
	}
	if p, ok := s.AsAckvitation(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetAckvitation(p)
	}
	if p, ok := s.AsMaintenance(); ok {
		p.TargetAddress, p.SourceAddress = target, source
		return s.SetMaintenance(p)
	}
	return ErrNoAddresses
}
