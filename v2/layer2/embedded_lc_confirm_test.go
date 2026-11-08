package layer2_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func confirmLC(t *testing.T, src uint32) pdu.LCBytes {
	t.Helper()
	lc, err := pdu.NewVoiceLCBytes(true, 91, src, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return lc
}

func flipBits(f [4][4]byte, ps ...int) [4][4]byte {
	for _, p := range ps {
		f[p/32][p%32/8] ^= 0x80 >> (p % 8)
	}
	return f
}

func TestEmbeddedLCCandidate(t *testing.T) {
	t.Parallel()
	a := confirmLC(t, 3000101)
	three := flipBits(layer2.EncodeEmbeddedLC(a), 0, 41, 90)
	if _, res := layer2.DecodeEmbeddedLC(three); !res.Uncorrectable {
		t.Fatalf("three errors: DecodeEmbeddedLC accepts, %+v", res)
	}
	lc, res := layer2.DecodeEmbeddedLCCandidate(three)
	if res.Uncorrectable || res.ErrorsCorrected != 3 || lc != a {
		t.Fatalf("three errors: candidate %x %+v, want %x with 3 corrected", lc, res, a)
	}
	for _, w := range [][4]uint32{
		{0x66150966, 0x050c0607, 0x0e43300f, 0x06365a3a},
		{0x04854b06, 0x916c0641, 0x2803bc2b, 0x06363a3a},
	} {
		if lc, res := layer2.DecodeEmbeddedLCCandidate(elcFrags(w)); !res.Uncorrectable && res.ErrorsCorrected <= 1 {
			t.Errorf("%08x: candidate %x with %d corrected", w, lc, res.ErrorsCorrected)
		}
	}
}

func TestEmbeddedLCConfirmer(t *testing.T) {
	t.Parallel()
	a, b := confirmLC(t, 3000101), confirmLC(t, 3000202)
	threeA := flipBits(layer2.EncodeEmbeddedLC(a), 0, 41, 90)
	threeA2 := flipBits(layer2.EncodeEmbeddedLC(a), 7, 60, 120)
	threeB := flipBits(layer2.EncodeEmbeddedLC(b), 0, 41, 90)
	accept := func(c *layer2.EmbeddedLCConfirmer, f [4][4]byte, want pdu.LCBytes, name string) {
		t.Helper()
		if lc, res := c.Decode(f); res.Uncorrectable || lc != want {
			t.Errorf("%s: %x %+v, want %x", name, lc, res, want)
		}
	}
	refuse := func(c *layer2.EmbeddedLCConfirmer, f [4][4]byte, name string) {
		t.Helper()
		lc, res := c.Decode(f)
		want, wantRes := layer2.DecodeEmbeddedLC(f)
		wantRes.Uncorrectable = true
		if lc != want || res != wantRes {
			t.Errorf("%s: %x %+v, want the refusal %x %+v", name, lc, res, want, wantRes)
		}
	}

	var c layer2.EmbeddedLCConfirmer
	accept(&c, layer2.EncodeEmbeddedLC(a), a, "clean")
	c.Reset()
	refuse(&c, flipBits(layer2.EncodeEmbeddedLC(a), 77), "one error, alone")
	c.Reset()
	c.Header(a)
	accept(&c, flipBits(layer2.EncodeEmbeddedLC(a), 77), a, "one error, same LC as the header")
	c.Reset()
	refuse(&c, threeA, "three errors, alone")
	accept(&c, flipBits(layer2.EncodeEmbeddedLC(a), 77), a, "one error, one superframe before with the same LC")

	c.Reset()
	refuse(&c, threeA, "three errors, alone")
	refuse(&c, threeA2, "three errors, one superframe before with the same LC")
	accept(&c, threeA, a, "three errors, two superframes before with the same LC")

	c.Reset()
	accept(&c, layer2.EncodeEmbeddedLC(a), a, "clean")
	refuse(&c, threeA, "three errors after one clean superframe")
	accept(&c, threeA2, a, "three errors after a clean and a three-error superframe")

	c.Reset()
	c.Header(a)
	accept(&c, threeA, a, "three errors, same LC as the header")
	refuse(&c, threeB, "three errors, other LC than the header")

	for _, w := range [][4]uint32{
		{0x66150966, 0x050c0607, 0x0e43300f, 0x06365a3a},
		{0x66150966, 0x050c0607, 0x0e43300f, 0x06361a3a},
		{0x04854b06, 0x916c0641, 0x2803bc2b, 0x06363a3a},
	} {
		c.Reset()
		refuse(&c, elcFrags(w), "received at low C/N")
	}

	c.Reset()
	c.Header(a)
	accept(&c, threeA2, a, "talker A, three errors, same LC as the header")
	accept(&c, layer2.EncodeEmbeddedLC(a), a, "talker A, clean")
	refuse(&c, threeB, "new talker, three errors")
	accept(&c, layer2.EncodeEmbeddedLC(b), b, "new talker, clean")
	refuse(&c, threeA, "old talker after a talker change, same LC as the header and two superframes before")

	c.Reset()
	c.Header(a)
	c.Reset()
	refuse(&c, threeA, "header of an earlier call")
}

func TestEmbeddedLCAssemblerConfirms(t *testing.T) {
	t.Parallel()
	a := confirmLC(t, 3000101)
	order := [4]enums.LCSS{enums.LCSSFirstFragment, enums.LCSSContinuationFragment, enums.LCSSContinuationFragment, enums.LCSSLastFragment}
	run := func(as *layer2.EmbeddedLCAssembler, f [4][4]byte) bool {
		for i := range 3 {
			_, _, _, _ = as.Add(f[i], order[i])
		}
		lc, res, done, err := as.Add(f[3], order[3])
		return done && err == nil && !res.Uncorrectable && lc == a
	}
	var as layer2.EmbeddedLCAssembler
	if run(&as, flipBits(layer2.EncodeEmbeddedLC(a), 0, 41, 90)) {
		t.Error("three errors accepted alone")
	}
	if run(&as, flipBits(layer2.EncodeEmbeddedLC(a), 7, 60, 120)) {
		t.Error("three errors accepted after one superframe with the same LC")
	}
	if !run(&as, flipBits(layer2.EncodeEmbeddedLC(a), 2, 50, 100)) {
		t.Error("three errors refused after two superframes with the same LC")
	}
	as.Reset()
	as.Header(a)
	if !run(&as, flipBits(layer2.EncodeEmbeddedLC(a), 0, 41, 90)) {
		t.Error("three errors refused after the voice header")
	}
}

func flipEmbedded(raws []layer2.RawBurst, at int, bits ...int) {
	for i, p := range bits {
		r := &raws[at+i]
		q := layer2.EmbeddedDataStart + p
		r[q/8] ^= 0x80 >> (q % 8)
	}
}

func TestVoiceReceiverConfirmsEmbeddedLC(t *testing.T) {
	t.Parallel()
	lc := groupLC(enums.FIDStandardized)
	count := func(raws []layer2.RawBurst) (lcs []int, refused []int) {
		var r layer2.VoiceReceiver
		for i, raw := range raws {
			b, _ := layer2.NewBurstFromBytes(raw)
			got, ok, err := r.Receive(b, nil)
			if ok {
				if got.FEC.Uncorrectable {
					t.Fatalf("burst %d: LC with uncorrectable FEC", i)
				}
				lcs = append(lcs, i)
			}
			if errors.Is(err, layer2.ErrEmbeddedLCUncorrectable) {
				refused = append(refused, i)
			}
		}
		return lcs, refused
	}
	raws := transmission(t, lc, 18)
	flipEmbedded(raws, 2, 3, 9, 20)
	flipEmbedded(raws, 8, 5, 10, 22)
	flipEmbedded(raws, 14, 1, 12, 26)
	if lcs, refused := count(raws); len(lcs) != 3 || len(refused) != 0 {
		t.Errorf("with the header: LCs at %v, refused at %v; want all three superframes", lcs, refused)
	}
	if lcs, refused := count(raws[1:]); len(lcs) != 1 || len(refused) != 2 || lcs[0] <= refused[1] {
		t.Errorf("without the header: LCs at %v, refused at %v; want two refused, then the third taken", lcs, refused)
	}
}
