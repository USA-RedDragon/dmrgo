package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// MMDVMHostPayload returns the BPTC(196,96) payload of r, as BPTCPayload
// returns it, and whether MMDVMHost accepts it from the network for a burst
// of dt, as MMDVMHostAccepts reports.
func (r RawBurst) MMDVMHostPayload(dt enums.DataType) ([12]byte, bool) {
	p, _ := r.BPTCPayload()
	return p, MMDVMHostAccepts(dt, p)
}

// MMDVMHostAccepts reports whether MMDVMHost (g4klx/MMDVMHost 0acb758) uses
// a data burst of dataType from the network, given the 96-bit BPTC(196,96)
// payload that RawBurst.BPTCPayload returns. It does not check call state.
// The package doc lists the check for each data type.
func MMDVMHostAccepts(dataType enums.DataType, payload [12]byte) bool {
	switch dataType {
	case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC:
		return pdu.FullLCParityOK(payload, dataType)
	case enums.DataTypeCSBK, enums.DataTypeMBCHeader, enums.DataTypeDataHeader:
		return pdu.BlockCRCOK(payload, dataType)
	case enums.DataTypeMBCContinuation:
		sealed := payload
		return MMDVMHostSeal(dataType, &sealed) == nil && sealed == payload
	case enums.DataTypePIHeader,
		enums.DataTypeRate12,
		enums.DataTypeRate34,
		enums.DataTypeRate1:
		return true
	case enums.DataTypeIdle, enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
		return false
	default:
		return false
	}
}
