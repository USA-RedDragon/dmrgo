package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// The CSBK opcodes of the Motorola feature set (FID 0x10). No public spec
// defines them. The layouts of their payloads come from captures of real
// radios. Each FID has its own opcodes, so these share values with
// standard opcodes: CSBKMotorolaCallAlert with CSBKRandomAccess and
// CSBKMotorolaCallAlertAck with CSBKAckOutbound.
const (
	CSBKMotorolaCallAlert    CSBKOpcode = 0b00011111 // call alert
	CSBKMotorolaCallAlertAck CSBKOpcode = 0b00100000 // call alert acknowledgement
	CSBKMotorolaRadioCheck   CSBKOpcode = 0b00100100 // radio check request and answer
)

// MotorolaRadioCheckPDU is the payload of a Motorola radio check CSBK
// (FID 0x10, opcode 0x24). The request and the answer have the same
// layout: octets 4 to 6 hold the radio that asked for the check, and
// octets 7 to 9 the radio that is checked, so the source and target swap
// places from the request to the answer.
//
//dmr:subpdu
type MotorolaRadioCheckPDU struct {
	Reserved         uint8  `dmr:"bits:0-7"`   // octet 2, 0
	Answer           bool   `dmr:"bit:8"`      // false in the request, true in the answer (octet 3 is 0x80)
	Reserved2        uint8  `dmr:"bits:9-15"`  // reserved, 0
	RequesterAddress uint32 `dmr:"bits:16-39"` // the radio that asks: source of the request, target of the answer
	CheckedAddress   uint32 `dmr:"bits:40-63"` // the radio checked: target of the request, source of the answer
}

// Addresses returns the target and source of the CSBK that carries p: the
// checked radio and the requester for a request, and the reverse for an
// answer. ok is false for a nil p.
func (p *MotorolaRadioCheckPDU) Addresses() (target, source uint32, ok bool) {
	switch {
	case p == nil:
		return 0, 0, false
	case p.Answer:
		return p.RequesterAddress, p.CheckedAddress, true
	}
	return p.CheckedAddress, p.RequesterAddress, true
}

// MotorolaCallAlertPDU is the payload of a Motorola call alert CSBK (FID
// 0x10, opcode 0x1F), which makes the target radio alert its user.
//
//dmr:subpdu
type MotorolaCallAlertPDU struct {
	Reserved      uint16 `dmr:"bits:0-15"`  // octets 2 and 3, 0
	TargetAddress uint32 `dmr:"bits:16-39"` // 24-bit address of the radio to alert
	SourceAddress uint32 `dmr:"bits:40-63"` // 24-bit address of the alerting radio
}

// MotorolaCallAlertAckPDU is the payload of the Motorola call alert
// acknowledgement CSBK (FID 0x10, opcode 0x20) that the alerted radio
// sends back.
//
//dmr:subpdu
type MotorolaCallAlertAckPDU struct {
	Request       uint8  `dmr:"bits:0-7"`   // octet 0 of the call alert it answers, 0x9F
	Reserved      uint8  `dmr:"bits:8-15"`  // octet 3, 0
	TargetAddress uint32 `dmr:"bits:16-39"` // 24-bit address of the radio that sent the call alert
	SourceAddress uint32 `dmr:"bits:40-63"` // 24-bit address of the alerted radio
}

// AsMotorolaRadioCheck decodes the RawPayload of a Motorola radio check
// CSBK. ok is false for another opcode or FID.
func (s *CSBK) AsMotorolaRadioCheck() (p MotorolaRadioCheckPDU, ok bool) {
	if s == nil {
		return MotorolaRadioCheckPDU{}, false
	}
	if s.Opcode != CSBKMotorolaRadioCheck || s.FID != enums.FIDMotorola {
		return p, false
	}
	return decodeMotorolaRadioCheckPDU(s.RawPayload), true
}

// AsMotorolaCallAlert decodes the RawPayload of a Motorola call alert CSBK.
// ok is false for another opcode or FID.
func (s *CSBK) AsMotorolaCallAlert() (p MotorolaCallAlertPDU, ok bool) {
	if s == nil {
		return MotorolaCallAlertPDU{}, false
	}
	if s.Opcode != CSBKMotorolaCallAlert || s.FID != enums.FIDMotorola {
		return p, false
	}
	return decodeMotorolaCallAlertPDU(s.RawPayload), true
}

// AsMotorolaCallAlertAck decodes the RawPayload of a Motorola call alert
// acknowledgement CSBK. ok is false for another opcode or FID.
func (s *CSBK) AsMotorolaCallAlertAck() (p MotorolaCallAlertAckPDU, ok bool) {
	if s == nil {
		return MotorolaCallAlertAckPDU{}, false
	}
	if s.Opcode != CSBKMotorolaCallAlertAck || s.FID != enums.FIDMotorola {
		return p, false
	}
	return decodeMotorolaCallAlertAckPDU(s.RawPayload), true
}

// SetMotorolaRadioCheck makes s a Motorola radio check CSBK that carries
// p: it sets Opcode, FID and RawPayload, clears the payload fields and
// keeps LastBlock and ProtectFlag. It returns a *dmrgo.FieldRangeError when
// a field does not fit in its bits, and leaves s as it is on an error.
func (s *CSBK) SetMotorolaRadioCheck(p MotorolaRadioCheckPDU) error {
	if s == nil {
		return dmrgo.ErrNilReceiver
	}
	payload, err := p.Encode()
	if err != nil {
		return err
	}
	s.setMotorola(CSBKMotorolaRadioCheck, payload)
	return nil
}

// SetMotorolaCallAlert makes s a Motorola call alert CSBK that carries p,
// as SetMotorolaRadioCheck does.
func (s *CSBK) SetMotorolaCallAlert(p MotorolaCallAlertPDU) error {
	if s == nil {
		return dmrgo.ErrNilReceiver
	}
	payload, err := p.Encode()
	if err != nil {
		return err
	}
	s.setMotorola(CSBKMotorolaCallAlert, payload)
	return nil
}

// SetMotorolaCallAlertAck makes s a Motorola call alert acknowledgement
// CSBK that carries p, as SetMotorolaRadioCheck does.
func (s *CSBK) SetMotorolaCallAlertAck(p MotorolaCallAlertAckPDU) error {
	if s == nil {
		return dmrgo.ErrNilReceiver
	}
	payload, err := p.Encode()
	if err != nil {
		return err
	}
	s.setMotorola(CSBKMotorolaCallAlertAck, payload)
	return nil
}

func (s *CSBK) setMotorola(op CSBKOpcode, payload [8]byte) {
	*s = CSBK{
		LastBlock:   s.LastBlock,
		ProtectFlag: s.ProtectFlag,
		Opcode:      op,
		FID:         enums.FIDMotorola,
		RawPayload:  payload,
	}
}
