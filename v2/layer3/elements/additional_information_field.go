package elements

// ETSI TS 102 361-2 - 7.2.6 Additional Information Field
type AdditionalInformationField uint8

const (
	AdditionalInfoIgnore AdditionalInformationField = 0 // Additional Information shall be ignored
	AdditionalInfoValid  AdditionalInformationField = 1 // Additional Information field is valid
)

func AdditionalInformationFieldToName(ai AdditionalInformationField) string {
	switch ai {
	case AdditionalInfoIgnore:
		return "Ignore"
	case AdditionalInfoValid:
		return "Valid"
	default:
		return "Unknown"
	}
}

func (ai AdditionalInformationField) ToString() string {
	return "AdditionalInformationField{ " + AdditionalInformationFieldToName(ai) + " }"
}
