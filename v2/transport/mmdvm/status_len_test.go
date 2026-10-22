package mmdvm_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func TestDecodeStatusV1Short(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, frame string
		want        mmdvm.StatusV1
	}{
		{"OpenGD77 b843c1f", "e00d0122020100" + "0a14000000" + "01", mmdvm.StatusV1{POCSAG: true, DMR: true, State: mmdvm.ModeDMR, TX: true, DMRSpace1: 10, DMRSpace2: 20, POCSAGSpace: 1, Size: 10}},
		{"MMDVM-SDR 8a68e74", "e00b010200000a0a0a0a0a", mmdvm.StatusV1{DMR: true, DStarSpace: 10, DMRSpace1: 10, DMRSpace2: 10, YSFSpace: 10, P25Space: 10, Size: 8}},
		{"first MMDVM", "e00a0102020001020304", mmdvm.StatusV1{DMR: true, State: mmdvm.ModeDMR, DStarSpace: 1, DMRSpace1: 2, DMRSpace2: 3, YSFSpace: 4, Size: 7}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw, _ := hex.DecodeString(tc.frame)
			f, _, st := mmdvm.Next(raw)
			if st != mmdvm.NextFrame {
				t.Fatalf("frame %x: %v", raw, st)
			}
			s, err := mmdvm.DecodeStatusV1(f.Payload)
			if err != nil {
				t.Fatalf("DecodeStatusV1 of %d octets: %v", len(f.Payload), err)
			}
			if s != tc.want {
				t.Errorf("got %+v\nwant %+v", s, tc.want)
			}
			out, err := s.AppendBinary(nil)
			if err != nil || !bytes.Equal(out, f.Payload) {
				t.Errorf("AppendBinary %x %v, want %x", out, err, f.Payload)
			}
		})
	}
}
