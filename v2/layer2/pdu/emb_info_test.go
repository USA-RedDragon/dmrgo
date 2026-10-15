package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/quadraticresidue"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestEMBInfo_IPSCExample(t *testing.T) {
	t.Parallel()

	s := pdu.EmbeddedSignalling{ColorCode: 1, LCSS: enums.LCSSFirstFragment}
	e := s.Info()
	if e != 0x12 {
		t.Fatalf("Info %#02x, want 0x12", byte(e))
	}
	if cw := e.Codeword(); cw != 0x1391 {
		t.Fatalf("Codeword %#04x, want 0x1391", cw)
	}
	bits := must(s.Encode())
	if got := bit.Pack(bits[:]); got[0] != 0x13 || got[1] != 0x91 {
		t.Fatalf("EmbeddedSignalling.Encode %x", got)
	}
	d, res := pdu.DecodeEMBCodeword(0x1391)
	if d != 0x12 || res.ErrorsCorrected != 0 || res.Uncorrectable || d.EmbeddedSignalling() != s {
		t.Fatalf("DecodeEMBCodeword %#02x %+v", byte(d), res)
	}
}

func TestEMBInfo_AllValues(t *testing.T) {
	t.Parallel()

	for v := range 128 {
		e := pdu.EMBInfo(v << 1)
		var data [7]bit.Bit
		bit.PutUint(data[:], uint64(v))
		cw := quadraticresidue.Encode(data)
		want := uint16(bit.Uint(cw[:]))
		if got := e.Codeword(); got != want {
			t.Fatalf("%#02x: Codeword %#04x, want %#04x", byte(e), got, want)
		}
		if (e | 1).Codeword() != want {
			t.Fatalf("%#02x: bit 0 changes the codeword", byte(e))
		}
		s := e.EmbeddedSignalling()
		if s.ColorCode != enums.ColorCode(v>>3) || s.PreemptionAndPowerControlIndicator != (v&4 != 0) || s.LCSS != enums.LCSS(v&3) {
			t.Fatalf("%#02x: EmbeddedSignalling %s", byte(e), s.String())
		}
		if back := s.Info(); back != e {
			t.Fatalf("%#02x: Info %#02x", byte(e), byte(back))
		}
		if enc := must(s.Encode()); uint16(bit.Uint(enc[:])) != want {
			t.Fatalf("%#02x: EmbeddedSignalling.Encode differs", byte(e))
		}
		if d, res := pdu.DecodeEMBCodeword(want); d != e || res.ErrorsCorrected != 0 || res.Uncorrectable {
			t.Fatalf("%#04x: decodes as %#02x %+v", want, byte(d), res)
		}
		for i := range 16 {
			d, res := pdu.DecodeEMBCodeword(want ^ 1<<i)
			if d != e || res.ErrorsCorrected != 1 || res.Uncorrectable {
				t.Fatalf("%#04x with bit %d flipped: %#02x %+v", want, i, byte(d), res)
			}
		}
	}
}

func TestEMBInfo_Setters(t *testing.T) {
	t.Parallel()

	for v := range 256 {
		orig := pdu.EMBInfo(v)
		for cc := range enums.ColorCode(16) {
			e := orig
			if e.SetColorCode(cc); e.ColorCode() != cc || (e^orig)&^0xF0 != 0 {
				t.Fatalf("%#02x: SetColorCode(%d) gives %#02x", v, cc, byte(e))
			}
		}
		e := orig
		if e.SetColorCode(0x1F); e.ColorCode() != 0xF || (e^orig)&^0xF0 != 0 {
			t.Fatalf("%#02x: SetColorCode(0x1F) gives %#02x", v, byte(e))
		}
		for l := range enums.LCSS(4) {
			e := orig
			if !e.SetLCSS(l) || e.LCSS() != l || (e^orig)&^0x06 != 0 {
				t.Fatalf("%#02x: SetLCSS(%d) gives %#02x", v, l, byte(e))
			}
		}
		for _, pi := range []bool{false, true} {
			e := orig
			if e.SetPI(pi); e.PI() != pi || (e^orig)&^0x08 != 0 {
				t.Fatalf("%#02x: SetPI(%t) gives %#02x", v, pi, byte(e))
			}
		}
		e = orig
		if e.SetLCSS(4) || e != orig {
			t.Fatalf("%#02x: SetLCSS accepts an out of range value", v)
		}
	}
	s := pdu.EmbeddedSignalling{ColorCode: 0x13, LCSS: 5}
	if e := s.Info(); e.ColorCode() != 3 || e.LCSS() != 1 {
		t.Fatalf("Info %#02x, want the low bits of CC 0x13 and LCSS 5", byte(e))
	}
}
