package enums

// VoiceBurstType is the position of a voice burst in a voice superframe
// (ETSI TS 102 361-1 §5.1.2.1). The zero value is VoiceBurstUnknown.
type VoiceBurstType uint8

const (
	// VoiceBurstUnknown is a burst whose superframe position is not known.
	VoiceBurstUnknown VoiceBurstType = iota

	VoiceBurstA
	VoiceBurstB
	VoiceBurstC
	VoiceBurstD
	VoiceBurstE
	VoiceBurstF
)

// String returns the name of a voice burst type.
func (burstType VoiceBurstType) String() string {
	switch burstType {
	case VoiceBurstUnknown:
		return unknownName
	case VoiceBurstA:
		return "A"
	case VoiceBurstB:
		return "B"
	case VoiceBurstC:
		return "C"
	case VoiceBurstD:
		return "D"
	case VoiceBurstE:
		return "E"
	case VoiceBurstF:
		return "F"
	}
	return unknownName
}
