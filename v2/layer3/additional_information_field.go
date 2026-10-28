package layer3

// AdditionalInformationField is the 1-bit Additional Information Field: it
// shows whether the additional information of a PDU is valid (ETSI TS 102
// 361-2 clause 7.2.6, Table 7.15).
type AdditionalInformationField uint8

// The AdditionalInformationField values.
const (
	AdditionalInfoIgnore AdditionalInformationField = 0 // ignore the additional information
	AdditionalInfoValid  AdditionalInformationField = 1 // the additional information is valid
)

func (ai AdditionalInformationField) String() string {
	switch ai {
	case AdditionalInfoIgnore:
		return "Ignore"
	case AdditionalInfoValid:
		return "Valid"
	default:
		return unknownName
	}
}
