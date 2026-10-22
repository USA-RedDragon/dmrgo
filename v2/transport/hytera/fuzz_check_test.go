package hytera_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

func fuzzSeeds() [][]byte {
	raw, _ := layer2.BuildDataBurst([12]byte{1, 2, 3}, enums.DataTypeVoiceLCHeader, 7, layer2.SyncSourceBS)
	f := hytera.Frame{Port: 50002, Seq: 1, PacketType: 1, Slot: enums.Slot1, SlotType: hytera.SlotTypeVoiceLCHeader, ColorCode: 7, Burst: raw, Destination: 9, Source: 3191868}
	frame, _ := f.AppendBinary(nil)
	return [][]byte{
		frame,
		[]byte("ZZZZ\x0a\x00\x00\x00\x14\x00\x00\x00\x5a\x5a\x59\x5a\x00\x00\x00\x00"),
		[]byte("P2PP\x0b\x00\x00\x00\x14\x00\x00\x00\x42\xff\x01\x00\x00\x00\x00\x00\x11\x00\x00\x00\xff\x01\x51\xc3"),
		{0x00},
	}
}

func checkPacket(t *testing.T, p []byte) {
	t.Helper()
	if f, err := hytera.DecodeFrame(p); err == nil {
		out, err := f.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, p) {
			t.Fatalf("frame round trip %x: %x %v", p, out, err)
		}
		h := f.Hint(0)
		back := f
		if err := back.SetHint(h); err == nil && back.SlotType != f.SlotType && back.Hint(0) != h {
			t.Fatalf("%x: SetHint gives %v, want %v or a slot type with the same Hint", p, back.SlotType, f.SlotType)
		}
	}
	if h, err := hytera.DecodeHeartbeat(p); err == nil {
		out, err := h.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, p) {
			t.Fatalf("heartbeat round trip %x: %x %v", p, out, err)
		}
	}
	if c, err := hytera.DecodeP2P(p); err == nil {
		out, err := c.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, p) {
			t.Fatalf("P2P round trip %x: %x %v", p, out, err)
		}
		_, _ = c.RedirectPort()
		_, _ = c.Service()
	}
}
