package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// MMDVMHostSeal rewrites the parity or CRC of the BPTC(196,96) payload p of
// a burst of dataType, after the caller edited p, so that
// MMDVMHostAccepts(dataType, *p) holds. It leaves the Rate 1/2, Rate 3/4 and
// Rate 1 data types as they are. For a data type that MMDVMHost discards it
// returns an error that wraps ErrDataType, and for a nil p
// dmrgo.ErrNilArgument; both leave p as it is.
func MMDVMHostSeal(dataType enums.DataType, p *[12]byte) error {
	if p == nil {
		return dmrgo.ErrNilArgument
	}
	switch dataType {
	case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC, enums.DataTypePIHeader,
		enums.DataTypeCSBK, enums.DataTypeMBCHeader, enums.DataTypeDataHeader:
		return pdu.Seal(p, dataType)
	case enums.DataTypeMBCContinuation:
		c := crc.CCITT(p[:10])
		p[10] = byte(c >> 8)
		p[11] = byte(c & 0xFF)
		return nil
	case enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1:
		return nil
	case enums.DataTypeIdle, enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
		return errMMDVMHostData
	default:
		return errMMDVMHostData
	}
}
