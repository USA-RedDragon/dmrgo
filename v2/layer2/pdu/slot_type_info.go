package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// SlotTypeInfo is the 8 information bits of a SLOT PDU without the
// Golay(20,8) parity, as IPSC carries them: the color code in the high 4
// bits and the Data Type in the low 4 bits (ETSI TS 102 361-1 clause 9.1.3,
// Table 9.4). For example, CC 1 with a CSBK is 0x13, whose codeword is
// 0x132b2.
type SlotTypeInfo byte

// ColorCode returns the 4-bit color code, the high 4 bits.
func (s SlotTypeInfo) ColorCode() enums.ColorCode {
	return enums.ColorCode(s >> 4)
}

// SetColorCode writes the low 4 bits of cc into the high 4 bits.
func (s *SlotTypeInfo) SetColorCode(cc enums.ColorCode) {
	*s = *s&0x0F | SlotTypeInfo(cc<<4)
}

// DataType returns the Data Type, the low 4 bits.
func (s SlotTypeInfo) DataType() enums.DataType {
	return enums.DataType(s & 0x0F)
}

// SetDataType writes dt into the low 4 bits. It returns false, and changes
// nothing, when dt does not fit in 4 bits.
func (s *SlotTypeInfo) SetDataType(dt enums.DataType) bool {
	if dt>>4 != 0 {
		return false
	}
	*s = *s&0xF0 | SlotTypeInfo(dt)
	return true
}

// SlotType returns the fields of s as a SlotType with a zero FEC result.
func (s SlotTypeInfo) SlotType() SlotType {
	return SlotType{ColorCode: s.ColorCode(), DataType: s.DataType()}
}

// Info returns the information bits of s as a SlotTypeInfo: the low 4 bits
// of the color code and of the data type.
func (s *SlotType) Info() SlotTypeInfo {
	var i SlotTypeInfo
	i.SetColorCode(s.ColorCode)
	i.SetDataType(s.DataType & 0x0F)
	return i
}

// Codeword returns the 20-bit Golay(20,8) codeword of s in the low 20 bits:
// the 8 information bits followed by the 12 parity bits, first bit most
// significant (ETSI TS 102 361-1 clause B.3.1).
func (s SlotTypeInfo) Codeword() uint32 {
	st := s.SlotType()
	cw, _ := st.encodeBytes()
	return uint32(cw[0])<<12 | uint32(cw[1])<<4 | uint32(cw[2]>>4)
}

// DecodeSlotTypeCodeword decodes the 20-bit Golay(20,8) codeword in the low
// 20 bits of cw, first bit most significant, as DecodeSlotType does. It
// ignores the high 12 bits of cw. It returns the information bits,
// corrected when the result is not Uncorrectable, and the FEC result.
func DecodeSlotTypeCodeword(cw uint32) (SlotTypeInfo, fec.Result) {
	st := decodeSlotTypeBytes([3]byte{byte(cw >> 12 & 0xFF), byte(cw >> 4 & 0xFF), byte(cw << 4 & 0xF0)})
	return st.Info(), st.FEC
}
