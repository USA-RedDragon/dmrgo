package ipsc_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
)

func TestDecodeErrors(t *testing.T) {
	t.Parallel()

	voice, _ := hex.DecodeString("800004ce2e1d30b43c0000090200000d9e20805d7b60482e7f1e000000000a1440541cbd8ee6d793cd54372cbe82e8c4004a2a78")
	tests := []struct {
		p    []byte
		want error
	}{
		{voice[:30], ipsc.ErrShort},
		{voice[:40], ipsc.ErrShort},
		{append(append([]byte{}, voice...), 0), ipsc.ErrLength},
		{append([]byte{0x96}, voice[1:]...), ipsc.ErrCommand},
	}
	for _, tt := range tests {
		if _, err := ipsc.DecodeUser(tt.p); !errors.Is(err, tt.want) {
			t.Errorf("%d octets: %v, want %v", len(tt.p), err, tt.want)
		}
	}
	for _, p := range [][]byte{nil, {0x90, 0, 0, 0}, {0x90, 0, 0, 0, 1}, {0x80, 0, 0, 0, 1}, {0x93, 0, 0, 0, 1, 0, 11}} {
		if _, err := ipsc.DecodeControl(p); err == nil {
			t.Errorf("%x: no error", p)
		}
	}
	if _, err := ipsc.DecodeRaw([]byte{1, 2}); !errors.Is(err, ipsc.ErrShort) {
		t.Errorf("raw: %v", err)
	}
}

func TestEncodeMisuse(t *testing.T) {
	t.Parallel()

	if _, err := (&ipsc.User{Opcode: ipsc.OpAliveRequest}).AppendBinary(nil); err == nil {
		t.Error("user with a control opcode: no error")
	}
	if _, err := (&ipsc.User{Opcode: ipsc.OpGroupVoice, Burst: ipsc.BurstVoice}).AppendBinary(nil); err == nil {
		t.Error("voice without a layout: no error")
	}
	if out, err := (&ipsc.User{Opcode: ipsc.OpGroupVoice, Source: 1 << 24}).AppendBinary([]byte{9}); err == nil || !bytes.Equal(out, []byte{9}) {
		t.Errorf("source over 24 bits: %x %v", out, err)
	}
	if _, err := (ipsc.Control{Opcode: ipsc.OpGroupVoice}).AppendBinary(nil); err == nil {
		t.Error("control with a user opcode: no error")
	}
	var u ipsc.User
	var raw layer2.RawBurst
	for _, tt := range []struct {
		k    layer2.Kind
		pos  enums.VoiceBurstType
		dt   enums.DataType
		want error
	}{
		{layer2.KindUnknown, 0, 0, nil},
		{layer2.KindVoice, enums.VoiceBurstUnknown, 0, layer2.ErrVoicePosition},
		{layer2.KindData, 0, enums.DataTypeRate1, layer2.ErrDataType},
		{layer2.KindData, 0, enums.DataTypeIdle, layer2.ErrDataType},
	} {
		err := u.SetRawBurst(raw, tt.k, tt.pos, tt.dt)
		if err == nil {
			t.Errorf("%+v: no error", tt)
		}
		if tt.want != nil && !errors.Is(err, tt.want) {
			t.Errorf("%+v: %v, want %v", tt, err, tt.want)
		}
	}
}

func TestBuildAndRebuild(t *testing.T) {
	t.Parallel()

	var p [12]byte
	copy(p[:], []byte{0x00, 0x00, 0x20, 0x00, 0x00, 0x5B, 0x30, 0xB4, 0x3C})
	sealed := p
	raw, err := layer2.BuildDataBurst(sealed, enums.DataTypeCSBK, 7, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	u := ipsc.User{Opcode: ipsc.OpGroupData, Info: 0x20, RTP: ipsc.RTP{Flags: 0x80, PayloadType: 0x5D}}
	if err := u.SetRawBurst(raw, layer2.KindData, 0, enums.DataTypeCSBK); err != nil {
		t.Fatal(err)
	}
	pkt, err := u.AppendBinary(nil)
	if err != nil || len(pkt) != 54 || pkt[30] != 3 || pkt[31] != 0xC0 || pkt[33] != 10 || pkt[51] != 0x73 {
		t.Fatalf("%x %v", pkt, err)
	}
	got, err := ipsc.DecodeUser(pkt)
	if err != nil {
		t.Fatal(err)
	}
	back, err := got.RawBurst(1, layer2.SyncSourceBS)
	if err != nil || back != raw {
		t.Fatalf("rebuilt %x, want %x (%v)", back, raw, err)
	}
	if h := got.Hint(); h.Kind != layer2.KindData || h.DataType != enums.DataTypeCSBK || h.Slot != enums.Slot2 {
		t.Fatalf("hint %+v", h)
	}
}
