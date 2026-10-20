package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func buildRCInfoBits(payload byte) [2]byte {
	var info [2]byte
	bits.SetField(info[:], 0, 4, uint64(payload&0x0F))
	crcVal := crc.CRC7(info[:], 4) ^ pdu.CRC7MaskRC
	bits.SetField(info[:], 4, 7, uint64(crcVal))
	return info
}

func TestReverseChannel_DecodeEncode_RoundTrip(t *testing.T) {
	t.Parallel()
	for payload := byte(0); payload < 16; payload++ {
		info := buildRCInfoBits(payload)
		rc := pdu.DecodeReverseChannel(info)
		fecResult := rc.FEC
		if fecResult.Uncorrectable {
			t.Errorf("payload=%d: unexpected uncorrectable error", payload)
			continue
		}
		if byte(rc.RCCommand) != payload {
			t.Errorf("payload=%d: got %d", payload, rc.RCCommand)
			continue
		}

		encoded, err := rc.Encode()
		if err != nil {
			t.Fatalf("payload=%d: unexpected error: %v", payload, err)
		}
		if encoded != info {
			t.Errorf("payload=%d: round-trip failed: encoded %v != original %v", payload, encoded, info)
		}
	}
}

func TestReverseChannel_CRCFailure(t *testing.T) {
	t.Parallel()
	info := buildRCInfoBits(5)

	info[0] ^= 0x01

	fecResult := pdu.DecodeReverseChannel(info).FEC
	if !fecResult.Uncorrectable {
		t.Error("expected uncorrectable after CRC corruption")
	}
}

func TestReverseChannel_BitsChecked(t *testing.T) {
	t.Parallel()
	info := buildRCInfoBits(0)
	fecResult := pdu.DecodeReverseChannel(info).FEC
	if fecResult != (fec.Result{}) {
		t.Errorf("CRC-7 only PDU: got %+v, want a zero result", fecResult)
	}
}

func TestReverseChannel_OutOfRangeRejected(t *testing.T) {
	t.Parallel()
	for _, cmd := range []enums.RCCommand{0x10, 0x14, 0xFF} {
		rc := pdu.ReverseChannel{RCCommand: cmd}
		if _, err := rc.Encode(); err == nil {
			t.Errorf("RCCommand 0x%X: expected error, got nil", int(cmd))
		}
	}
}

func TestReverseChannel_String(t *testing.T) {
	t.Parallel()
	info := buildRCInfoBits(7)
	rc := pdu.DecodeReverseChannel(info)
	s := rc.String()
	if s == "" {
		t.Error("String should not be empty")
	}
}

func TestReverseChannel_CRC7Mask(t *testing.T) {
	t.Parallel()
	if pdu.CRC7MaskRC != 0x7A {
		t.Errorf("CRC7MaskRC should be 0x7A, got 0x%02X", pdu.CRC7MaskRC)
	}
}
