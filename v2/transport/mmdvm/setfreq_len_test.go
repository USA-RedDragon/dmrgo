package mmdvm_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func TestSetFreqDefaultIs14(t *testing.T) {
	t.Parallel()

	s := mmdvm.SetFreq{RX: 434000000, TX: 439000000, RFLevel: 255, POCSAG: 439987500}
	p, err := s.AppendBinary(nil)
	if err != nil || len(p) != 14 {
		t.Fatalf("zero Size gave %d octets %v; MMDVM-IQ dd4a0fc NAKs a SET_FREQ shorter than 14 and MMDVMHost sends 14", len(p), err)
	}
	got, err := mmdvm.DecodeSetFreq(append(p, 0))
	if err != nil || got.RX != s.RX || got.POCSAG != s.POCSAG || got.Size != 14 {
		t.Errorf("15 octets: %+v %v", got, err)
	}
	s.Size = 9
	if p, err := s.AppendBinary(nil); err != nil || len(p) != 9 {
		t.Errorf("Size 9 gave %d octets %v", len(p), err)
	}
}
