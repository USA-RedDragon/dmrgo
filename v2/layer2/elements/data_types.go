package elements

// ETSI TS 102 361-1 - 9.3.6 Data Type
type DataType uint8

const (
	DataTypePIHeader DataType = iota
	DataTypeVoiceLCHeader
	DataTypeTerminatorWithLC
	DataTypeCSBK
	DataTypeMBCHeader
	DataTypeMBCContinuation
	DataTypeDataHeader
	DataTypeRate12
	DataTypeRate34
	DataTypeIdle
	DataTypeRate1
	DataTypeUnifiedSingleBlock
	DataTypeReserved
)

func DataTypeToName(dt DataType) string {
	switch dt {
	case DataTypePIHeader:
		return "PI Header"
	case DataTypeVoiceLCHeader:
		return "Voice LC Header"
	case DataTypeTerminatorWithLC:
		return "Terminator With LC"
	case DataTypeCSBK:
		return "CSBK"
	case DataTypeMBCHeader:
		return "MBC Header"
	case DataTypeMBCContinuation:
		return "MBC Continuation"
	case DataTypeDataHeader:
		return "Data Header"
	case DataTypeRate12:
		return "Rate 1/2"
	case DataTypeRate34:
		return "Rate 3/4"
	case DataTypeIdle:
		return "Idle"
	case DataTypeRate1:
		return "Rate 1"
	case DataTypeUnifiedSingleBlock:
		return "Unified Single Block"
	case DataTypeReserved:
		return "Reserved"
	default:
		return "Unknown"
	}
}
