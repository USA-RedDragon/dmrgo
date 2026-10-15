// Package enums defines the enumerated information elements of ETSI TS 102
// 361: data types, SYNC patterns, FLCO, SLCO, FID, LCSS, the address and
// port identifiers of the compressed headers and others. Each type converts
// from its on-air value with a plain cast and prints with String.
package enums

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

// IsReserved reports whether dt is one of the reserved Data Type values
// 1100 to 1111 (Table 9.22). The 4-bit Data Type field is kept raw, so
// values 13 to 15 are reserved as well as DataTypeReserved.
func (dt DataType) IsReserved() bool {
	return dt >= DataTypeReserved && dt <= 0xF
}

// IsBPTC reports whether a burst of data type dt carries 96 information
// bits coded with BPTC(196,96) (ETSI TS 102 361-1 B.1.1): every data type
// except Rate 3/4, Rate 1 and the reserved values.
func (dt DataType) IsBPTC() bool {
	switch dt {
	case DataTypePIHeader, DataTypeVoiceLCHeader, DataTypeTerminatorWithLC, DataTypeCSBK,
		DataTypeMBCHeader, DataTypeMBCContinuation, DataTypeDataHeader, DataTypeRate12,
		DataTypeIdle, DataTypeUnifiedSingleBlock:
		return true
	case DataTypeRate34, DataTypeRate1, DataTypeReserved:
	}
	return false
}

// CarriesFullLC reports whether a burst of data type dt carries a Full LC
// with RS(12,9) parity: a Voice LC Header or a Terminator with LC.
func (dt DataType) CarriesFullLC() bool {
	return dt == DataTypeVoiceLCHeader || dt == DataTypeTerminatorWithLC
}

func (dt DataType) String() string {
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
		if dt.IsReserved() {
			return "Reserved"
		}
		return "Unknown"
	}
}
