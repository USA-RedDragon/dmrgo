package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// MMDVMHostSeal is the counterpart of MMDVMHostAccepts: it rewrites the
// parity or CRC of the BPTC(196,96) payload p of a burst of dataType, after
// the caller edited p, so that MMDVMHostAccepts(dataType, *p) holds.
//
//   - Voice LC Header and Terminator with LC: octets 9 to 11 become the
//     masked RS(12,9) parity of octets 0 to 8 (pdu.FullLCCodeword).
//   - PI Header, CSBK, MBC Header and Data Header: octets 10 and 11 become
//     the masked CRC-CCITT of octets 0 to 9 (pdu.SealBlock). MMDVMHost does
//     not check the PI Header CRC, but the seal is the ETSI one.
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
func MMDVMHostSeal(dataType elements.DataType, p *[12]byte) error {
	switch dataType {
	case elements.DataTypeVoiceLCHeader, elements.DataTypeTerminatorWithLC:
		cw, err := pdu.FullLCCodeword([9]byte(p[:9]), dataType)
		if err != nil {
			return err
		}
		*p = cw
		return nil
	case elements.DataTypePIHeader, elements.DataTypeCSBK, elements.DataTypeMBCHeader, elements.DataTypeDataHeader:
		return pdu.SealBlock(p, dataType)
	case elements.DataTypeMBCContinuation:
		c := crc.CalculateCRCCCITT(p[:10])
		p[10] = byte(c >> 8)
		p[11] = byte(c) //nolint:gosec // low octet of the CRC
		return nil
	case elements.DataTypeRate12, elements.DataTypeRate34, elements.DataTypeRate1:
		return nil
	case elements.DataTypeIdle, elements.DataTypeUnifiedSingleBlock, elements.DataTypeReserved:
		return fmt.Errorf("MMDVMHost discards %s bursts", elements.DataTypeToName(dataType))
	default:
		return fmt.Errorf("MMDVMHost discards %s bursts", elements.DataTypeToName(dataType))
	}
}
