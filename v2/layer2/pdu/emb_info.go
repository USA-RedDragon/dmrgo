package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// EMBInfo is the 7 information bits of an EMB PDU without the QR(16,7,6)
// parity, in bits 7 to 1 of one octet as IPSC carries them. Bit 0 is not an
// EMB bit: the getters and Codeword ignore it and the setters keep it. A
// setter returns a *dmrgo.FieldRangeError, and changes nothing, when its
// value does not fit its bits. For example, CC 1 with LCSS 1 is 0x12.
// (ETSI TS 102 361-1 clause 9.1.2, Table 9.3)
type EMBInfo byte

// ColorCode returns the 4-bit color code, bits 7 to 4.
func (e EMBInfo) ColorCode() enums.ColorCode {
	return enums.ColorCode(e >> 4)
}

// SetColorCode writes cc into bits 7 to 4.
func (e *EMBInfo) SetColorCode(cc enums.ColorCode) error {
	if cc>>4 != 0 {
		return errEMBInfoColorCodeRange
	}
	*e = *e&0x0F | EMBInfo(cc<<4)
	return nil
}

// PI returns the Pre-emption and power control Indicator, bit 3.
func (e EMBInfo) PI() bool {
	return e&0x08 != 0
}

// SetPI writes the Pre-emption and power control Indicator into bit 3.
func (e *EMBInfo) SetPI(pi bool) {
	if pi {
		*e |= 0x08
	} else {
		*e &^= 0x08
	}
}

// LCSS returns the Link Control Start/Stop, bits 2 and 1.
func (e EMBInfo) LCSS() enums.LCSS {
	return enums.LCSS(e >> 1 & 0x03)
}

// SetLCSS writes l into bits 2 and 1.
func (e *EMBInfo) SetLCSS(l enums.LCSS) error {
	if l>>2 != 0 {
		return errEMBInfoLCSSRange
	}
	*e = *e&^0x06 | EMBInfo(l<<1)
	return nil
}

// EmbeddedSignalling returns the fields of e as an EmbeddedSignalling with
// a zero FEC result.
func (e EMBInfo) EmbeddedSignalling() EmbeddedSignalling {
	return EmbeddedSignalling{
		ColorCode:                          e.ColorCode(),
		PreemptionAndPowerControlIndicator: e.PI(),
		LCSS:                               e.LCSS(),
	}
}

// Info returns the information bits of s as an EMBInfo with bit 0 clear:
// the color code, the PI and the LCSS. It returns the errors of Encode.
func (s *EmbeddedSignalling) Info() (EMBInfo, error) {
	cw, err := s.Encode()
	return EMBInfo(cw[0] & 0xFE), err
}

// Codeword returns the 16-bit QR(16,7,6) codeword of e, the 7 information
// bits followed by the 9 parity bits, first bit most significant (ETSI TS
// 102 361-1 clause B.3.2).
func (e EMBInfo) Codeword() uint16 {
	s := e.EmbeddedSignalling()
	cw, _ := s.Encode()
	return uint16(cw[0])<<8 | uint16(cw[1])
}

// DecodeEMBCodeword decodes the 16-bit QR(16,7,6) codeword cw, first bit
// most significant, as DecodeEmbeddedSignalling does. It returns the
// information bits with bit 0 clear, corrected when the result is not
// Uncorrectable, and the FEC result.
func DecodeEMBCodeword(cw uint16) (EMBInfo, fec.Result) {
	s := DecodeEmbeddedSignalling([2]byte{byte(cw >> 8), byte(cw & 0xFF)})
	info := EMBInfo(s.ColorCode<<4) | EMBInfo(s.LCSS<<1)
	if s.PreemptionAndPowerControlIndicator {
		info |= 0x08
	}
	return info, s.FEC
}

// The Field of the FieldRangeErrors of the setters of EMBInfo, SlotTypeInfo
// and the confirmed data blocks.
const (
	fieldColorCode = "ColorCode"
	fieldLCSS      = "LCSS"
	fieldDBSN      = "DBSN"
)

// The errors of the EMBInfo setters.
var (
	errEMBInfoColorCodeRange = &dmrgo.FieldRangeError{Type: "EMBInfo", Field: fieldColorCode, Bits: 4}
	errEMBInfoLCSSRange      = &dmrgo.FieldRangeError{Type: "EMBInfo", Field: fieldLCSS, Bits: 2}
)
