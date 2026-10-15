package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// EMBInfo is the 7 information bits of an EMB PDU without the QR(16,7,6)
// parity, in bits 7 to 1 of one octet as IPSC carries them (ETSI TS 102
// 361-1 clause 9.1.2, Table 9.3). Bit 0 is not an EMB bit: the getters and
// Codeword ignore it and the setters keep it. For example, CC 1 with LCSS 1
// is 0x12.
type EMBInfo byte

// ColorCode returns the 4-bit Colour Code, bits 7 to 4.
func (e EMBInfo) ColorCode() enums.ColorCode {
	return enums.ColorCode(e >> 4)
}

// SetColorCode writes the low 4 bits of cc into bits 7 to 4.
func (e *EMBInfo) SetColorCode(cc enums.ColorCode) {
	*e = *e&0x0F | EMBInfo(cc<<4)
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

// SetLCSS writes l into bits 2 and 1. It returns false, and changes
// nothing, when l does not fit in 2 bits.
func (e *EMBInfo) SetLCSS(l enums.LCSS) bool {
	if l>>2 != 0 {
		return false
	}
	*e = *e&^0x06 | EMBInfo(l<<1)
	return true
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
// the low 4 bits of the color code, the PI and the low 2 bits of the LCSS.
func (s *EmbeddedSignalling) Info() EMBInfo {
	var e EMBInfo
	e.SetColorCode(s.ColorCode)
	e.SetPI(s.PreemptionAndPowerControlIndicator)
	e.SetLCSS(s.LCSS & 0x03)
	return e
}

// Codeword returns the 16-bit QR(16,7,6) codeword of e, the 7 information
// bits followed by the 9 parity bits, first bit most significant (ETSI TS
// 102 361-1 clause B.3.2).
func (e EMBInfo) Codeword() uint16 {
	s := e.EmbeddedSignalling()
	bits, _ := s.Encode()
	return uint16(bit.Uint(bits[:])) //nolint:gosec // 16 bits
}

// DecodeEMBCodeword decodes the 16-bit QR(16,7,6) codeword cw, first bit
// most significant, as DecodeEmbeddedSignalling does. It returns the
// information bits with bit 0 clear, corrected when the result is not
// Uncorrectable, and the FEC result.
func DecodeEMBCodeword(cw uint16) (EMBInfo, fec.Result) {
	var bits [16]bit.Bit
	bit.PutUint(bits[:], uint64(cw))
	s := DecodeEmbeddedSignalling(bits)
	return s.Info(), s.FEC
}
