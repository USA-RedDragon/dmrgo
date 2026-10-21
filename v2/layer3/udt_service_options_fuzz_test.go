package layer3_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

func udtOptionsSeeds(add func(b byte)) {
	for _, b := range []byte{0x00, 0x20, 0x0A, 0x32, 0xFF} {
		add(b)
	}
}

func fuzzUDTOptions(t *testing.T, b byte) {
	in := [1]byte{b}
	s := layer3.DecodeShortDataServiceOptions(in)
	_ = s.String()
	if e, err := s.Encode(); err != nil || e[0] != b&0x60 {
		t.Fatalf("ShortDataServiceOptions of %02x encodes as %02x %v", b, e, err)
	}
	p := layer3.DecodePollingServiceOptions(in)
	_ = p.String()
	if e, err := p.Encode(); err != nil || e[0] != b&0x5E {
		t.Fatalf("PollingServiceOptions of %02x encodes as %02x %v", b, e, err)
	}
	d := layer3.DecodeDiversionServiceOptions(in)
	_ = d.String()
	if e, err := d.Encode(); err != nil || e[0] != b&0x7E {
		t.Fatalf("DiversionServiceOptions of %02x encodes as %02x %v", b, e, err)
	}
}

func FuzzUDTServiceOptions(f *testing.F) {
	udtOptionsSeeds(func(b byte) { f.Add(b) })
	f.Fuzz(fuzzUDTOptions)
}
