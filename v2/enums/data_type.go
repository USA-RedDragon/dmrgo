// Package enums defines the enumerated information elements of ETSI TS 102
// 361: data types, SYNC patterns, FLCO, SLCO, FID, LCSS, the address and
// port identifiers of the compressed headers and others. Each type converts
// from its on-air value with a plain cast and prints with String.
package enums

// DataType is the 4-bit Data Type of a slot type: the content of a general
// data burst (ETSI TS 102 361-1 clause 9.3.6, Table 9.22).
type DataType uint8

// The DataType values.
const (
	DataTypePIHeader           DataType = iota // PI Header
	DataTypeVoiceLCHeader                      // Voice LC Header
	DataTypeTerminatorWithLC                   // Terminator with LC
	DataTypeCSBK                               // CSBK
	DataTypeMBCHeader                          // MBC Header
	DataTypeMBCContinuation                    // MBC Continuation
	DataTypeDataHeader                         // Data Header
	DataTypeRate12                             // Rate 1/2 Data
	DataTypeRate34                             // Rate 3/4 Data
	DataTypeIdle                               // Idle
	DataTypeRate1                              // Rate 1 Data
	DataTypeUnifiedSingleBlock                 // Unified Single Block Data
	DataTypeReserved                           // first reserved value, 1100
)

// IsReserved reports whether dt is one of the reserved values 12 to 15
// (DataTypeReserved and above) (ETSI TS 102 361-1 Table 9.22).
func (dt DataType) IsReserved() bool {
	return dt >= DataTypeReserved && dt <= 0xF
}

// IsBPTC reports whether a burst of data type dt carries 96 information
// bits coded with BPTC(196,96): every data type except Rate 3/4, Rate 1 and
// the reserved values (ETSI TS 102 361-1 clause B.1.1).
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
