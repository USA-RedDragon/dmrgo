package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	reedsolomon "github.com/USA-RedDragon/dmrgo/v2/internal/fec/reedsolomon"
)

// CRCMask returns the Data Type CRC Mask for dt and its width in bits: 24
// for an RS(12,9) parity, 16 for a CRC-CCITT and 9 for a CRC-9 (ETSI TS 102
// 361-1 clause B.3.12, Table B.21). ok is false for MBC Continuation, Idle
// and reserved data types, which have no mask.
func CRCMask(dt enums.DataType) (mask uint32, width int, ok bool) {
	switch dt {
	case enums.DataTypeVoiceLCHeader:
		return 0x969696, 24, true
	case enums.DataTypeTerminatorWithLC:
		return 0x999999, 24, true
	case enums.DataTypePIHeader:
		return 0x6969, 16, true
	case enums.DataTypeCSBK:
		return 0xA5A5, 16, true
	case enums.DataTypeMBCHeader:
		return 0xAAAA, 16, true
	case enums.DataTypeDataHeader:
		return 0xCCCC, 16, true
	case enums.DataTypeUnifiedSingleBlock:
		return 0x3333, 16, true
	case enums.DataTypeRate12:
		return 0x0F0, 9, true
	case enums.DataTypeRate34:
		return 0x1FF, 9, true
	case enums.DataTypeRate1:
		return 0x10F, 9, true
	case enums.DataTypeMBCContinuation, enums.DataTypeIdle, enums.DataTypeReserved:
		return 0, 0, false
	default:
		return 0, 0, false
	}
}

// Codeword returns the 12-octet on-air codeword of l in a burst of dt: l
// followed by its RS(12,9) parity XORed with the mask of dt (ETSI TS 102
// 361-1 clause 9.1.6, clause B.3.6). It returns an error unless
// dt.CarriesFullLC reports true.
func (l LCBytes) Codeword(dt enums.DataType) ([12]byte, error) {
	mask, err := fullLCMask(dt)
	if err != nil {
		return [12]byte{}, err
	}
	cw := reedsolomon.Encode(l)
	for i := 9; i < 12; i++ {
		cw[i] ^= mask[i-9]
	}
	return cw, nil
}

// FullLCParityOK reports whether the RS(12,9) parity of the codeword cw, with
// the mask of dt removed, equals the parity of its 9 LC octets. It corrects
// no error, so it accepts exactly the codewords that MMDVMHost accepts
// (CRS129::check). It returns false unless dt is DataTypeVoiceLCHeader or
// DataTypeTerminatorWithLC.
func FullLCParityOK(cw [12]byte, dt enums.DataType) bool {
	want, err := LCBytes(cw[:9]).Codeword(dt)
	return err == nil && want == cw
}

// DecodeFullLCCodeword removes the mask of dt from the parity of cw and
// decodes the RS(12,9) codeword. It returns the 9 LC octets, corrected when
// the result is not Uncorrectable, and the FEC result, as
// DecodeFullLinkControl does. It returns an error unless dt is
// DataTypeVoiceLCHeader or DataTypeTerminatorWithLC.
func DecodeFullLCCodeword(cw [12]byte, dt enums.DataType) ([9]byte, fec.Result, error) {
	mask, err := fullLCMask(dt)
	if err != nil {
		return [9]byte{}, fec.Result{}, err
	}
	for i := 9; i < 12; i++ {
		cw[i] ^= mask[i-9]
	}
	lc, res := reedsolomon.Decode(cw)
	return lc, res, nil
}

func fullLCMask(dt enums.DataType) ([3]byte, error) {
	mask, width, ok := CRCMask(dt)
	if !ok || width != 24 {
		return [3]byte{}, fmt.Errorf("data type %s carries no RS(12,9) Full LC", dt.String())
	}
	return [3]byte{byte(mask >> 16 & 0xFF), byte(mask >> 8 & 0xFF), byte(mask & 0xFF)}, nil
}

// Seal rewrites the check bits of the BPTC(196,96) payload p of a burst of
// dt: the masked RS(12,9) parity in octets 9 to 11 for a Voice LC Header or
// Terminator with LC, and the masked CRC-CCITT of octets 0 to 9 in octets 10
// and 11 for a PI Header, CSBK, MBC Header, Data Header or USBD. For other
// data types it returns an error and leaves p unchanged.
func Seal(p *[12]byte, dt enums.DataType) error {
	if dt.CarriesFullLC() {
		cw, err := LCBytes(p[:9]).Codeword(dt)
		if err != nil {
			return err
		}
		*p = cw
		return nil
	}
	mask, err := blockMask(dt)
	if err != nil {
		return err
	}
	c := crc.CCITT(p[:10]) ^ mask
	p[10] = byte(c >> 8)
	p[11] = byte(c & 0xFF)
	return nil
}

// BlockCRCOK reports whether octets 10 and 11 of the single-block payload b
// hold the CRC that Seal writes for dt. It returns false unless dt is a PI
// Header, CSBK, MBC Header, Data Header or Unified Single Block Data.
func BlockCRCOK(b [12]byte, dt enums.DataType) bool {
	if dt.CarriesFullLC() {
		return false
	}
	want := b
	if Seal(&want, dt) != nil {
		return false
	}
	return want == b
}

func blockMask(dt enums.DataType) (uint16, error) {
	mask, width, ok := CRCMask(dt)
	if !ok || width != 16 {
		return 0, fmt.Errorf("data type %s has no single-block CRC-CCITT", dt.String())
	}
	return uint16(mask & 0xFFFF), nil
}
