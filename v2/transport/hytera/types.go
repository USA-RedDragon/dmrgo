package hytera

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// SlotType is octets 18 and 19 of a DMR frame: what the burst is, as one
// nibble four times.
type SlotType uint16

// The SlotType values. The voice positions follow the capture of a real
// repeater, where every burst A carries the voice SYNC; Hytera_Homebrew_Bridge
// 05e1bf9 maps them two positions off.
const (
	SlotTypePIHeader      SlotType = 0x0000
	SlotTypeVoiceLCHeader SlotType = 0x1111
	SlotTypeTerminator    SlotType = 0x2222
	SlotTypeCSBK          SlotType = 0x3333
	SlotTypeDataHeader    SlotType = 0x4444
	SlotTypeRate12        SlotType = 0x5555
	SlotTypeRate34        SlotType = 0x6666
	SlotTypeVoiceA        SlotType = 0x7777
	SlotTypeVoiceB        SlotType = 0x8888
	SlotTypeVoiceC        SlotType = 0x9999
	SlotTypeVoiceD        SlotType = 0xAAAA
	SlotTypeVoiceE        SlotType = 0xBBBB
	SlotTypeVoiceF        SlotType = 0xCCCC
	SlotTypeWakeup        SlotType = 0xDDDD
	SlotTypeSync          SlotType = 0xEEEE
)

// String returns the name of the slot type. It allocates only for a value
// without a name.
func (s SlotType) String() string {
	switch s {
	case SlotTypePIHeader:
		return "PI Header"
	case SlotTypeVoiceLCHeader:
		return "Voice LC Header"
	case SlotTypeTerminator:
		return "Terminator with LC"
	case SlotTypeCSBK:
		return "CSBK"
	case SlotTypeDataHeader:
		return "Data Header"
	case SlotTypeRate12:
		return "Rate 1/2 Data"
	case SlotTypeRate34:
		return "Rate 3/4 Data"
	case SlotTypeVoiceA:
		return "Voice A"
	case SlotTypeVoiceB:
		return "Voice B"
	case SlotTypeVoiceC:
		return "Voice C"
	case SlotTypeVoiceD:
		return "Voice D"
	case SlotTypeVoiceE:
		return "Voice E"
	case SlotTypeVoiceF:
		return "Voice F"
	case SlotTypeWakeup:
		return "Wakeup"
	case SlotTypeSync:
		return "Sync"
	}
	return "SlotType(" + text.Int(int64(s)) + ")"
}

// Service is octet 20 of a P2P packet: the service that the repeater
// requests.
type Service uint8

// The Service values.
const (
	ServiceRegistration Service = 0x10
	ServiceDMR          Service = 0x11
	ServiceRDAC         Service = 0x12
)

// String returns the name of the service. It allocates only for a value
// without a name.
func (s Service) String() string {
	switch s {
	case ServiceRegistration:
		return "Registration"
	case ServiceDMR:
		return "DMR"
	case ServiceRDAC:
		return "RDAC"
	}
	return "Service(" + text.Int(int64(s)) + ")"
}
