package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	reedsolomon "github.com/USA-RedDragon/dmrgo/v2/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
)

// ETSI TS 102 361-1 §B.3.12 — Data Type CRC Mask, applied to the 96-bit
// BPTC(196,96) payload of a data burst as 12 octets, most significant bit
// first.

// CRCMask returns the Data Type CRC Mask of Table B.21 for dt and its width
// in bits: 24 for the RS(12,9) parity of a Voice LC Header or Terminator
// with LC, 16 for the CRC-CCITT of a PI Header, CSBK, MBC Header, Data
// Header or Unified Single Block Data, and 9 for the CRC-9 of a confirmed
// Rate 1/2, Rate 3/4 or Rate 1 data block. ok is false for MBC Continuation
// and Idle, which have no mask (Table B.21 note 1), and for reserved data
// types. The Reverse Channel mask 0x7A applies to a CRC-7 outside any data
// burst, so it has no data type here.
func CRCMask(dt elements.DataType) (mask uint32, width int, ok bool) {
	switch dt {
	case elements.DataTypeVoiceLCHeader:
		return 0x969696, 24, true
	case elements.DataTypeTerminatorWithLC:
		return 0x999999, 24, true
	case elements.DataTypePIHeader:
		return 0x6969, 16, true
	case elements.DataTypeCSBK:
		return 0xA5A5, 16, true
	case elements.DataTypeMBCHeader:
		return 0xAAAA, 16, true
	case elements.DataTypeDataHeader:
		return 0xCCCC, 16, true
	case elements.DataTypeUnifiedSingleBlock:
		return 0x3333, 16, true
	case elements.DataTypeRate12:
		return 0x0F0, 9, true
	case elements.DataTypeRate34:
		return 0x1FF, 9, true
	case elements.DataTypeRate1:
		return 0x10F, 9, true
	case elements.DataTypeMBCContinuation, elements.DataTypeIdle, elements.DataTypeReserved:
		return 0, 0, false
	default:
		return 0, 0, false
	}
}

// FullLCCodeword returns the 12-octet on-air codeword of the 72-bit LC lc
// in a burst of dt (§9.1.6, §B.3.6): lc followed by its RS(12,9) parity
// XORed with the Table B.21 mask of dt. It returns an error unless dt is
// DataTypeVoiceLCHeader or DataTypeTerminatorWithLC.
func FullLCCodeword(lc [9]byte, dt elements.DataType) ([12]byte, error) {
	var cw [12]byte
	mask, err := fullLCMask(dt)
	if err != nil {
		return cw, err
	}
	parity, err := reedsolomon.Encode(lc[:])
	if err != nil {
		return cw, err
	}
	copy(cw[:], parity)
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
func FullLCParityOK(cw [12]byte, dt elements.DataType) bool {
	var lc [9]byte
	copy(lc[:], cw[:9])
	want, err := FullLCCodeword(lc, dt)
	return err == nil && want == cw
}

// DecodeFullLCCodeword removes the mask of dt from the parity of cw and
// decodes the RS(12,9) codeword. It returns the 9 LC octets, corrected when
// the result is not Uncorrectable, and the FEC result, as
// DecodeFullLinkControl does. It returns an error unless dt is
// DataTypeVoiceLCHeader or DataTypeTerminatorWithLC.
func DecodeFullLCCodeword(cw [12]byte, dt elements.DataType) ([9]byte, fec.Result, error) {
	var lc [9]byte
	mask, err := fullLCMask(dt)
	if err != nil {
		return lc, fec.Result{}, err
	}
	for i := 9; i < 12; i++ {
		cw[i] ^= mask[i-9]
	}
	res := reedsolomon.Decode(cw[:])
	copy(lc[:], cw[:9])
	if !res.Uncorrectable {
		copy(lc[:], res.Data)
	}
	return lc, fec.Result{BitsChecked: 96, ErrorsCorrected: res.ErrorsFound, Uncorrectable: res.Uncorrectable}, nil
}

func fullLCMask(dt elements.DataType) ([3]byte, error) {
	mask, width, ok := CRCMask(dt)
	if !ok || width != 24 {
		return [3]byte{}, fmt.Errorf("data type %s carries no RS(12,9) Full LC", elements.DataTypeToName(dt))
	}
	return [3]byte{byte(mask >> 16), byte(mask >> 8), byte(mask)}, nil //nolint:gosec // 24-bit mask split into octets
}

// SealBlock writes into octets 10 and 11 of the single-block payload b the
// CRC-CCITT of octets 0 to 9 XORed with the Table B.21 mask of dt
// (§B.3.8). dt must be DataTypePIHeader, DataTypeCSBK, DataTypeMBCHeader,
// DataTypeDataHeader or DataTypeUnifiedSingleBlock; for any other data type
// SealBlock returns an error and leaves b as it is.
//
// The CRCs that a single block and its data type do not determine are out
// of scope: the MBC Continuation last block CRC over all continuation
// blocks (MBCLastBlockCRC, SetMBCLastBlockCRC), the CRC-9 of confirmed
// Rate 1/2, Rate 3/4 and Rate 1 blocks, which covers the DBSN
// (Rate12Data.SetConfirmed and its Rate 3/4 and Rate 1 forms), and the
// CRC-32 of a data continuation last block.
func SealBlock(b *[12]byte, dt elements.DataType) error {
	mask, err := blockMask(dt)
	if err != nil {
		return err
	}
	c := crc.CalculateCRCCCITT(b[:10]) ^ mask
	b[10] = byte(c >> 8)
	b[11] = byte(c) //nolint:gosec // low octet of the CRC
	return nil
}

// BlockCRCOK reports whether octets 10 and 11 of the single-block payload b
// hold the CRC that SealBlock writes for dt. It returns false for the data
// types that SealBlock rejects.
func BlockCRCOK(b [12]byte, dt elements.DataType) bool {
	want := b
	if SealBlock(&want, dt) != nil {
		return false
	}
	return want == b
}

func blockMask(dt elements.DataType) (uint16, error) {
	mask, width, ok := CRCMask(dt)
	if !ok || width != 16 {
		return 0, fmt.Errorf("data type %s has no single-block CRC-CCITT", elements.DataTypeToName(dt))
	}
	return uint16(mask), nil //nolint:gosec // width 16 masks fit in uint16
}
