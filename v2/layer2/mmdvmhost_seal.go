package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// MMDVMHostSeal rewrites the parity or CRC of the BPTC(196,96) payload p of
// a burst of dataType, after the caller edited p, so that
// MMDVMHostAccepts(dataType, *p) holds. It leaves the Rate 1/2, Rate 3/4 and
// Rate 1 data types as they are, and returns an error and leaves p as it is
// for a data type that MMDVMHost discards.
func MMDVMHostSeal(dataType enums.DataType, p *[12]byte) error {
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
