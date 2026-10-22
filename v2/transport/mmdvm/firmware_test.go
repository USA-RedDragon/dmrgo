package mmdvm_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func rangeErr(err error) bool {
	var re *dmrgo.FieldRangeError
	return errors.As(err, &re)
}

func TestFirmwareRejects(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		v2   mmdvm.ConfigV2
		v1   mmdvm.ConfigV1
	}{
		{"color code 16", mmdvm.ConfigV2{DMRColorCode: 16}, mmdvm.ConfigV1{DMRColorCode: 16}},
		{"tx delay 51", mmdvm.ConfigV2{TXDelay: 51}, mmdvm.ConfigV1{TXDelay: 51}},
	} {
		out, err := tt.v2.AppendBinary([]byte{7})
		if !rangeErr(err) || len(out) != 1 {
			t.Errorf("ConfigV2 %s: %x %v", tt.name, out, err)
		}
		out, err = tt.v1.AppendBinary([]byte{7})
		if !rangeErr(err) || len(out) != 1 {
			t.Errorf("ConfigV1 %s: %x %v", tt.name, out, err)
		}
	}
	ok2 := mmdvm.ConfigV2{DMRColorCode: 15, TXDelay: 50}
	p2, err := ok2.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	ok1 := mmdvm.ConfigV1{DMRColorCode: 15, TXDelay: 50}
	p1, err := ok1.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name string
		p    []byte
		i    int
		v    byte
		dec  func([]byte) error
	}{
		{"ConfigV2 TXDelay", p2, 3, 51, func(p []byte) error { _, err := mmdvm.DecodeConfigV2(p); return err }},
		{"ConfigV2 DMRColorCode", p2, 26, 16, func(p []byte) error { _, err := mmdvm.DecodeConfigV2(p); return err }},
		{"ConfigV1 TXDelay", p1, 2, 51, func(p []byte) error { _, err := mmdvm.DecodeConfigV1(p); return err }},
		{"ConfigV1 DMRColorCode", p1, 6, 16, func(p []byte) error { _, err := mmdvm.DecodeConfigV1(p); return err }},
	} {
		if err := c.dec(c.p); err != nil {
			t.Errorf("%s: limit value: %v", c.name, err)
		}
		bad := append([]byte(nil), c.p...)
		bad[c.i] = c.v
		if err := c.dec(bad); !rangeErr(err) {
			t.Errorf("%s %d: %v", c.name, c.v, err)
		}
	}
	for _, s := range []enums.Slot{0, 3} {
		a := mmdvm.DMRAbort{Slot: s}
		if out, err := a.AppendBinary([]byte{7}); !rangeErr(err) || len(out) != 1 {
			t.Errorf("DMRAbort slot %d: %x %v", s, out, err)
		}
		if _, err := mmdvm.DecodeDMRAbort([]byte{byte(s)}); !rangeErr(err) {
			t.Errorf("DecodeDMRAbort %d: %v", s, err)
		}
	}
	for _, s := range []enums.Slot{enums.Slot1, enums.Slot2} {
		if a, err := mmdvm.DecodeDMRAbort([]byte{byte(s)}); err != nil || a.Slot != s {
			t.Errorf("DecodeDMRAbort %d: %v %v", s, a, err)
		}
	}
	for _, b := range []byte{0x02, 0x03, 0x80, 0xFF} {
		if _, err := mmdvm.DecodeDMRStart([]byte{b}); !rangeErr(err) {
			t.Errorf("DecodeDMRStart %#x: %v", b, err)
		}
	}
	for b, want := range []bool{false, true} {
		if s, err := mmdvm.DecodeDMRStart([]byte{byte(b)}); err != nil || s.TX != want {
			t.Errorf("DecodeDMRStart %d: %v %v", b, s, err)
		}
	}
}
