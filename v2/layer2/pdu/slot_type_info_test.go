package pdu_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/golay"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestSlotTypeInfo_IPSCExample(t *testing.T) {
	t.Parallel()

	st := pdu.SlotType{ColorCode: 1, DataType: enums.DataTypeCSBK}
	s := st.Info()
	if s != 0x13 {
		t.Fatalf("Info %#02x, want 0x13", byte(s))
	}
	if cw := s.Codeword(); cw != 0x132b2 {
		t.Fatalf("Codeword %#05x, want 0x132b2", cw)
	}
	bits := must(st.Encode())
	if got := hex.EncodeToString(bit.Pack(bits[:])); got != "132b20" {
		t.Fatalf("SlotType.Encode packs as %s, want 132b20", got)
	}
	d, res := pdu.DecodeSlotTypeCodeword(0x132b2)
	if d != 0x13 || res.ErrorsCorrected != 0 || res.Uncorrectable || d.SlotType() != st {
		t.Fatalf("DecodeSlotTypeCodeword %#02x %+v", byte(d), res)
	}
}

func TestSlotTypeInfo_AllValues(t *testing.T) {
	t.Parallel()

	for v := range 256 {
		s := pdu.SlotTypeInfo(v)
		var data [8]bit.Bit
		bit.PutUint(data[:], uint64(v))
		cw := golay.Encode2087(data)
		want := uint32(bit.Uint(cw[:]))
		if got := s.Codeword(); got != want {
			t.Fatalf("%#02x: Codeword %#05x, want %#05x", v, got, want)
		}
		st := s.SlotType()
		if st.ColorCode != enums.ColorCode(v>>4) || st.DataType != enums.DataType(v&0x0F) {
			t.Fatalf("%#02x: SlotType %s", v, st.String())
		}
		if back := st.Info(); back != s {
			t.Fatalf("%#02x: Info %#02x", v, byte(back))
		}
		if d, res := pdu.DecodeSlotTypeCodeword(want); d != s || res.ErrorsCorrected != 0 || res.Uncorrectable {
			t.Fatalf("%#05x: decodes as %#02x %+v", want, byte(d), res)
		}
		if d, _ := pdu.DecodeSlotTypeCodeword(want | 0xFFF00000); d != s {
			t.Fatalf("%#05x: high bits change the result", want)
		}
		for i := range 20 {
			d, res := pdu.DecodeSlotTypeCodeword(want ^ 1<<i)
			if d != s || res.ErrorsCorrected != 1 || res.Uncorrectable {
				t.Fatalf("%#05x with bit %d flipped: %#02x %+v", want, i, byte(d), res)
			}
		}
	}
}

func TestSlotTypeInfo_Setters(t *testing.T) {
	t.Parallel()

	for v := range 256 {
		orig := pdu.SlotTypeInfo(v)
		for x := range uint8(16) {
			s := orig
			if s.SetColorCode(enums.ColorCode(x)); s.ColorCode() != enums.ColorCode(x) || s.DataType() != orig.DataType() {
				t.Fatalf("%#02x: SetColorCode(%d) gives %#02x", v, x, byte(s))
			}
			s = orig
			if !s.SetDataType(enums.DataType(x)) || s.DataType() != enums.DataType(x) || s.ColorCode() != orig.ColorCode() {
				t.Fatalf("%#02x: SetDataType(%d) gives %#02x", v, x, byte(s))
			}
		}
		s := orig
		if s.SetDataType(16) || s != orig {
			t.Fatalf("%#02x: SetDataType accepts an out of range value", v)
		}
		if s.SetColorCode(0x1F); s.ColorCode() != 0xF || s.DataType() != orig.DataType() {
			t.Fatalf("%#02x: SetColorCode(0x1F) gives %#02x", v, byte(s))
		}
	}
	st := pdu.SlotType{ColorCode: 0x12, DataType: 0x13}
	if s := st.Info(); s != 0x23 {
		t.Fatalf("Info %#02x, want the low bits 0x23", byte(s))
	}
}
