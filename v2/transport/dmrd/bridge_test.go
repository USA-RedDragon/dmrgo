package dmrd_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

func TestBridgeV1(t *testing.T) {
	t.Parallel()

	p, _ := hex.DecodeString(dmrdVectors()[3].hex)
	p[15] &^= 0x80
	sig := bytes.Repeat([]byte{0x5A}, 20)
	pkt := append(append([]byte{}, p...), sig...)
	b, err := dmrd.DecodeBridge(pkt)
	if err != nil || b.Version != 1 || b.Data.Destination != 3140598 || b.Data.Slot != enums.Slot1 || !bytes.Equal(b.Signature[:], sig) {
		t.Fatalf("%+v %v", b, err)
	}
	out, err := b.AppendBinary(nil)
	if err != nil || !bytes.Equal(out, pkt) {
		t.Fatalf("encode %x %v", out, err)
	}
	for _, n := range []int{72, 74, 53} {
		q := append(append([]byte{}, p...), make([]byte, 20)...)[:n]
		if n == 74 {
			q = append(append([]byte{}, pkt...), 0)
		}
		if _, err := dmrd.DecodeBridge(q); err == nil {
			t.Errorf("length %d: no error", n)
		}
	}
}

func TestBridgeDMRE(t *testing.T) {
	t.Parallel()

	d := dmrd.Data{Seq: 3, Source: 3191868, Destination: 91, Repeater: 2345678, Slot: enums.Slot1, Frame: dmrd.FrameVoice, Value: 2, Stream: 0xCAFE, BER: 4, RSSI: 90}
	d.Burst[0] = 0x11
	for _, v := range []uint8{4, 5} {
		b := dmrd.Bridge{Version: v, Data: d, Time: 1_760_000_000_000_000_000, SourceServer: 23450, SourceRepeater: 3191868, Hops: 2}
		b.Signature[0], b.Signature[15] = 0xAB, 0xCD
		pkt, err := b.AppendBinary(nil)
		if err != nil {
			t.Fatal(err)
		}
		want := 85
		if v == 5 {
			want = 89
		}
		if len(pkt) != want || string(pkt[:4]) != "DMRE" || pkt[53] != 4 || pkt[54] != 90 || pkt[55] != v {
			t.Fatalf("v%d: %x", v, pkt)
		}
		got, err := dmrd.DecodeBridge(pkt)
		if err != nil {
			t.Fatal(err)
		}
		if v == 4 {
			b.SourceRepeater = 0
		}
		b.Data.Size = 55
		if got != b {
			t.Errorf("v%d: got %+v, want %+v", v, got, b)
		}
		if got.Signature[16] != 0 {
			t.Errorf("v%d: signature tail %x", v, got.Signature)
		}
	}
	if _, err := (&dmrd.Bridge{Version: 2, Data: d}).AppendBinary(nil); err == nil {
		t.Error("version 2: no error")
	}
	if _, err := (&dmrd.Bridge{Version: 1, Data: dmrd.Data{Slot: enums.Slot1, Size: 55}}).AppendBinary(nil); err == nil {
		t.Error("version 1 with Size 55: no error")
	}
	if _, err := dmrd.DecodeBridge(append([]byte("DMRE"), make([]byte, 82)...)); !errors.Is(err, dmrd.ErrLength) {
		t.Errorf("86 octets: %v", err)
	}
	if _, err := dmrd.DecodeBridge([]byte("RPTL1234")); !errors.Is(err, dmrd.ErrCommand) {
		t.Errorf("RPTL: %v", err)
	}
}

func TestBridgeControl(t *testing.T) {
	t.Parallel()

	for _, c := range []dmrd.BridgeControl{
		{Command: dmrd.CommandBCKA},
		{Command: dmrd.CommandBCST},
		{Command: dmrd.CommandBCSQ, TalkGroup: 91, Stream: 0x01020304},
		{Command: dmrd.CommandBCVE, Version: 5},
	} {
		c.Signature[19] = 0x77
		p, err := c.AppendBinary(nil)
		if err != nil || string(p[:4]) != c.Command.String() {
			t.Fatalf("%v: %x %v", c.Command, p, err)
		}
		got, err := dmrd.DecodeBridgeControl(p)
		if err != nil || got != c {
			t.Errorf("%v: %+v %v", c.Command, got, err)
		}
	}
	p, _ := (&dmrd.BridgeControl{Command: dmrd.CommandBCSQ, TalkGroup: 0x123456, Stream: 0x0A0B0C0D}).AppendBinary(nil)
	if !bytes.Equal(p[:11], []byte{'B', 'C', 'S', 'Q', 0x12, 0x34, 0x56, 0x0A, 0x0B, 0x0C, 0x0D}) || len(p) != 31 {
		t.Errorf("BCSQ %x", p)
	}
	if _, err := (&dmrd.BridgeControl{Command: dmrd.CommandRPTL}).AppendBinary(nil); err == nil {
		t.Error("RPTL: no error")
	}
	if _, err := dmrd.DecodeBridgeControl([]byte("BCKA")); !errors.Is(err, dmrd.ErrShort) {
		t.Errorf("short BCKA: %v", err)
	}
	if _, err := dmrd.DecodeBridgeControl([]byte("DMRD")); !errors.Is(err, dmrd.ErrCommand) {
		t.Errorf("DMRD: %v", err)
	}
}
