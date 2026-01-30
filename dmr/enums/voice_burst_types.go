package enums

// VoiceBurstType is the type of a DMR voice burst.
type VoiceBurstType int

const (
	VoiceBurstUnknown     VoiceBurstType = 1
	voiceBurstUnknownName                = "Unknown"

	VoiceBurstA VoiceBurstType = 100
	VoiceBurstB VoiceBurstType = 101
	VoiceBurstC VoiceBurstType = 102
	VoiceBurstD VoiceBurstType = 103
	VoiceBurstE VoiceBurstType = 104
	VoiceBurstF VoiceBurstType = 105
)

// VoiceBurstTypeToName returns the name of a voice burst type.
func VoiceBurstTypeToName(burstType VoiceBurstType) string {
	switch burstType {
	case VoiceBurstUnknown:
		return voiceBurstUnknownName
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
	return voiceBurstUnknownName
}
