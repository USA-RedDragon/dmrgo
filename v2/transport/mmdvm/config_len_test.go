package mmdvm_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func TestDecodeConfigV1MMDVMCal(t *testing.T) {
	t.Parallel()

	raw, _ := hex.DecodeString("e01b02000000638080010080808080808080800080800000808600")
	f, _, st := mmdvm.Next(raw)
	if st != mmdvm.NextFrame || len(f.Payload) != 24 {
		t.Fatalf("frame %v, %d octets", st, len(f.Payload))
	}
	c, err := mmdvm.DecodeConfigV1(f.Payload)
	if err != nil {
		t.Fatalf("DecodeConfigV1 of the 24 octets of MMDVMCal cf37046: %v", err)
	}
	if c.State != 0x63 || c.RXLevel != 0x80 || c.DMRColorCode != 1 || c.OscOffset != 0x80 || c.M17TXLevel != 0x80 || c.M17TXHang != 0x86 {
		t.Errorf("fields %+v", c)
	}
	out, err := c.AppendBinary(nil)
	if err != nil || !bytes.Equal(out, f.Payload[:23]) {
		t.Errorf("AppendBinary %x %v, want the first 23 octets", out, err)
	}
	if _, err := mmdvm.DecodeConfigV1(f.Payload[:22]); !errors.Is(err, mmdvm.ErrShort) {
		t.Errorf("22 octets: %v, want ErrShort", err)
	}
}

func TestDecodeConfigV2Longer(t *testing.T) {
	t.Parallel()

	c := mmdvm.ConfigV2{DMR: true, DMRColorCode: 7, TXDelay: 10, RXLevel: 128, OscOffset: 128}
	p, err := c.AppendBinary(nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := mmdvm.DecodeConfigV2(append(p, 0, 0, 0))
	if err != nil || got != c {
		t.Errorf("40 octets: %+v %v", got, err)
	}
	if _, err := mmdvm.DecodeConfigV2(p[:36]); !errors.Is(err, mmdvm.ErrShort) {
		t.Errorf("36 octets: %v, want ErrShort", err)
	}
}
