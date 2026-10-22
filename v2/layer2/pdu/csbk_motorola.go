package pdu

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
