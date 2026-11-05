package layer2_test

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func elcFrags(w [4]uint32) (f [4][4]byte) {
	for i := range w {
		binary.BigEndian.PutUint32(f[i][:], w[i])
	}
	return f
}

func elcAssemble(f [4][4]byte) ([9]byte, bool) {
	var a layer2.EmbeddedLCAssembler
	order := [4]enums.LCSS{enums.LCSSFirstFragment, enums.LCSSContinuationFragment, enums.LCSSContinuationFragment, enums.LCSSLastFragment}
	for i := range 3 {
		_, _, _, _ = a.Add(f[i], order[i])
	}
	lc, res, done, err := a.Add(f[3], order[3])
	return lc, done && err == nil && !res.Uncorrectable
}

func elcAssembleErr(f [4][4]byte) bool {
	var a layer2.EmbeddedLCAssembler
	order := [4]enums.LCSS{enums.LCSSFirstFragment, enums.LCSSContinuationFragment, enums.LCSSContinuationFragment, enums.LCSSLastFragment}
	for i := range 3 {
		_, _, _, _ = a.Add(f[i], order[i])
	}
	_, res, _, err := a.Add(f[3], order[3])
	return res.Uncorrectable == errors.Is(err, layer2.ErrEmbeddedLCUncorrectable)
}

func elcDecodeAll(t *testing.T, f [4][4]byte) ([9]byte, bool) {
	t.Helper()
	lc, res := layer2.DecodeEmbeddedLC(f)
	alc, aok := elcAssemble(f)
	flc := layer2.DecodeEmbeddedFullLinkControl(f)
	if aok && res.Uncorrectable || !elcAssembleErr(f) || flc.FEC.Uncorrectable != res.Uncorrectable || aok && alc != lc {
		t.Fatalf("%x: DecodeEmbeddedLC %x %+v, assembler %x %t, Full LC FEC %+v", f, lc, res, alc, aok, flc.FEC)
	}
	return lc, !res.Uncorrectable
}

func TestEmbeddedLC_NeverWrong(t *testing.T) {
	t.Parallel()

	truth := [4]uint32{0x06050906, 0x050c0603, 0x0c03300f, 0x06363a3a}
	want, ok := elcDecodeAll(t, elcFrags(truth))
	if !ok {
		t.Fatal("the clean LC does not decode")
	}
	for _, w := range [][4]uint32{
		{0x66150966, 0x050c0607, 0x0e43300f, 0x06365a3a},
		{0x66150966, 0x050c0607, 0x0e43300f, 0x06361a3a},
		{0x04854b06, 0x916c0641, 0x2803bc2b, 0x06363a3a},
	} {
		if lc, ok := elcDecodeAll(t, elcFrags(w)); ok {
			t.Errorf("%08x: received at low C/N, decodes to %x, sent %x", w, lc, want)
		}
	}
	for p := range 128 {
		w := truth
		w[p/32] ^= 1 << (31 - p%32)
		if lc, ok := elcDecodeAll(t, elcFrags(w)); !ok || lc != want {
			t.Fatalf("one error at bit %d: %x %t", p, lc, ok)
		}
		if _, res := layer2.DecodeEmbeddedLC(elcFrags(w)); res.ErrorsCorrected != 1 {
			t.Fatalf("one error at bit %d: %+v", p, res)
		}
		for q := p + 1; q < 128; q++ {
			v := w
			v[q/32] ^= 1 << (31 - q%32)
			if lc, ok := elcDecodeAll(t, elcFrags(v)); ok {
				t.Fatalf("errors at bits %d and %d: %x", p, q, lc)
			}
			r := v
			r[(p+q)%128/32] ^= 1 << (31 - (p+q)%128%32)
			if (p+q)%128 != p && (p+q)%128 != q {
				if lc, ok := elcDecodeAll(t, elcFrags(r)); ok {
					t.Fatalf("errors at bits %d, %d and %d: %x", p, q, (p+q)%128, lc)
				}
			}
		}
	}
}
