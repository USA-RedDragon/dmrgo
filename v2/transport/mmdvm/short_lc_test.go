package mmdvm_test

import (
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func TestDMRShortLCFromMMDVMHost(t *testing.T) {
	t.Parallel()

	p, err := hex.DecodeString("0339a0005330a55a50")
	if err != nil {
		t.Fatal(err)
	}
	s, err := mmdvm.DecodeDMRShortLC(p)
	if err != nil {
		t.Fatal(err)
	}
	slc := s.ShortLC()
	if slc.FEC.Uncorrectable || slc.FEC.ErrorsCorrected != 0 {
		t.Fatalf("MMDVMHost Short LC: %+v", slc)
	}
	back, err := mmdvm.NewDMRShortLC(&slc)
	if err != nil || back != s {
		t.Fatalf("NewDMRShortLC: %x %v, want %x", back.LC, err, s.LC)
	}
	var into pdu.ShortLC
	var pl pdu.ShortLCPayloads
	s.ShortLCInto(&into, &pl)
	if into.SLCO != slc.SLCO || into.FEC != slc.FEC {
		t.Fatalf("ShortLCInto: %+v", into)
	}
	s.ShortLCInto(nil, &pl)
	if _, err := mmdvm.NewDMRShortLC(nil); err == nil {
		t.Fatal("nil Short LC: no error")
	}
	s.LC[0] ^= 0xE0
	s.LC[5] ^= 0x0F
	if got := s.ShortLC(); !got.FEC.Uncorrectable && got.FEC.ErrorsCorrected == 0 {
		t.Fatalf("damaged: %+v", got.FEC)
	}
}
