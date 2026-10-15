package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// MMDVMHostSeal is the counterpart of MMDVMHostAccepts: it rewrites the
// parity or CRC of the BPTC(196,96) payload p of a burst of dataType, after
// the caller edited p, so that MMDVMHostAccepts(dataType, *p) holds.
//
//   - Voice LC Header, Terminator with LC, PI Header, CSBK, MBC Header and
//     Data Header: the ETSI parity or CRC that pdu.Seal writes. MMDVMHost
//     does not check the PI Header CRC.
//   - MBC Continuation: octets 10 and 11 become the unmasked CRC-CCITT of
//     octets 0 to 9, the per-block check of MMDVMHost. ETSI TS 102 361-1
//     §7.4.0 puts one CRC over all continuation blocks in the last block
//     only (pdu.SetMBCLastBlockCRC); the two agree only for an MBC with a
//     single continuation block.
//   - Rate 1/2, Rate 3/4 and Rate 1 Data: MMDVMHost checks nothing, so p is
//     left as it is. The CRC-9 of a confirmed block covers its DBSN; use
//     the SetConfirmed methods of the pdu Rate types for it.
//   - Idle, Unified Single Block Data and reserved data types: MMDVMHost
//     discards these bursts, so MMDVMHostSeal returns an error and leaves p
//     as it is.
func MMDVMHostSeal(dataType enums.DataType, p *[12]byte) error {
	switch dataType {
	case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC, enums.DataTypePIHeader,
		enums.DataTypeCSBK, enums.DataTypeMBCHeader, enums.DataTypeDataHeader:
		return pdu.Seal(p, dataType)
	case enums.DataTypeMBCContinuation:
		c := crc.CCITT(p[:10])
		p[10] = byte(c >> 8)
		p[11] = byte(c) //nolint:gosec // low octet of the CRC
		return nil
	case enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1:
		return nil
	case enums.DataTypeIdle, enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
		return fmt.Errorf("MMDVMHost discards %s bursts", dataType.String())
	default:
		return fmt.Errorf("MMDVMHost discards %s bursts", dataType.String())
	}
}
