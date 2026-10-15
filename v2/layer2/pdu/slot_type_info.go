package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// SlotTypeInfo is the 8 information bits of a SLOT PDU (ETSI TS 102 361-1
// §9.1.3, Table 9.4) without the Golay(20,8) parity of §B.3.1, as IPSC
// carries them: the Colour Code in the high 4 bits and the Data Type in
// the low 4 bits. For example, CC 1 with a CSBK is 0x13, whose codeword is
// 0x132b2, or 13 2b 20 when packed into 3 octets first bit first.
type SlotTypeInfo byte

// ColorCode returns the 4-bit Colour Code, the high 4 bits.
func (s SlotTypeInfo) ColorCode() uint8 {
	return uint8(s >> 4)
}

// SetColorCode writes cc into the high 4 bits. It returns false, and
// changes nothing, when cc does not fit in 4 bits.
func (s *SlotTypeInfo) SetColorCode(cc uint8) bool {
	if cc>>4 != 0 {
		return false
	}
	*s = *s&0x0F | SlotTypeInfo(cc<<4)
	return true
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

// Info returns the information bits of s as a SlotTypeInfo. It returns the
// errors that Encode returns for the fields.
func (s *SlotType) Info() (SlotTypeInfo, error) {
	bits, err := s.Encode()
	if err != nil {
		return 0, err
	}
	return SlotTypeInfo(bit.Uint(bits[:8])), nil //nolint:gosec // 8 bits
}

// Codeword returns the 20-bit Golay(20,8) codeword of s (§B.3.1) in the low
// 20 bits: the 8 information bits followed by the 12 parity bits, first bit
// most significant.
func (s SlotTypeInfo) Codeword() uint32 {
	st := s.SlotType()
	bits, _ := st.Encode()
	return uint32(bit.Uint(bits[:])) //nolint:gosec // 20 bits
}

// DecodeSlotTypeCodeword decodes the 20-bit Golay(20,8) codeword in the low
// 20 bits of cw, first bit most significant, as DecodeSlotType does. It
// ignores the high 12 bits of cw. It returns the information bits,
// corrected when the result is not Uncorrectable, and the FEC result.
func DecodeSlotTypeCodeword(cw uint32) (SlotTypeInfo, fec.Result) {
	var bits [20]bit.Bit
	bit.PutUint(bits[:], uint64(cw))
	st, res := DecodeSlotType(bits)
	s, _ := st.Info()
	return s, res
}
