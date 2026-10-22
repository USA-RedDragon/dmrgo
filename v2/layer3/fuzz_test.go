package layer3_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

var cdefMask = [8]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xC0}

func FuzzLayer3(f *testing.F) {
	layer3Seeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzLayer3)
}

func layer3Seeds(add func(data []byte)) {
	add([]byte{0x20, 0x01, 0x6D, 0x80, 0x00, 0x5B, 0x60, 0x00, 0x00})
	add([]byte{0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	add([]byte{0xC3, 0x00, 0x1A, 0x2B, 0x3C, 0x4D, 0x5E, 0x6F, 0x80})
}

func fuzzLayer3(t *testing.T, data []byte) {
	var in [9]byte
	copy(in[:], data)

	so := layer3.DecodeServiceOptions([1]byte{in[0]})
	_ = so.String()
	b, err := so.Encode()
	if err != nil || b[0] != in[0] {
		t.Fatalf("ServiceOptions of %02x encodes as %02x %v", in[0], b, err)
	}
	if v, err := so.Field(); err != nil || v != in[0] {
		t.Fatalf("ServiceOptions.Field of %02x = %02x %v", in[0], v, err)
	}

	raw := [8]byte(in[1:])
	cd := layer3.DecodeCdefParms(raw)
	_ = cd.String()
	out, err := cd.Encode()
	if err != nil {
		t.Fatalf("CdefParms of %x: %v", raw, err)
	}
	for i := range raw {
		if out[i] != raw[i]&cdefMask[i] {
			t.Fatalf("CdefParms of %x encodes as %x", raw, out)
		}
	}
	if back := layer3.DecodeCdefParms(out); back != cd {
		t.Fatalf("CdefParms of %x decodes back as %+v, want %+v", raw, back, cd)
	}
	if tx, rx := cd.TXFrequencyHz(), cd.RXFrequencyHz(); tx != uint64(cd.TXMHz)*1_000_000+uint64(cd.TXKHz)*125 || rx != uint64(cd.RXMHz)*1_000_000+uint64(cd.RXKHz)*125 {
		t.Fatalf("CdefParms %+v frequencies %d %d", cd, tx, rx)
	}
	if d := cd.TXFrequencyMHz()*1e6 - float64(cd.TXFrequencyHz()); d > 1e-3 || d < -1e-3 {
		t.Fatalf("CdefParms %+v TXFrequencyMHz %v differs from TXFrequencyHz %d", cd, cd.TXFrequencyMHz(), cd.TXFrequencyHz())
	}
	if d := cd.RXFrequencyMHz()*1e6 - float64(cd.RXFrequencyHz()); d > 1e-3 || d < -1e-3 {
		t.Fatalf("CdefParms %+v RXFrequencyMHz %v differs from RXFrequencyHz %d", cd, cd.RXFrequencyMHz(), cd.RXFrequencyHz())
	}

	for _, s := range []string{
		layer3.SourceType(in[0]).String(),
		layer3.PositionAccuracy(in[0]).String(),
		layer3.AdditionalInformationField(in[0]).String(),
		layer3.TalkerAliasDataFormat(in[0]).String(),
	} {
		if s == "" {
			t.Fatalf("empty String for %d", in[0])
		}
	}
}
