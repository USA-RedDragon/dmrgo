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

func TestBurstType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		b    ipsc.BurstType
		typ  ipsc.BurstType
		dt   enums.DataType
		ok   bool
		name string
	}{
		{0x00, 0x00, enums.DataTypePIHeader, true, enums.DataTypePIHeader.String()},
		{0x08, 0x08, enums.DataTypeRate34, true, enums.DataTypeRate34.String()},
		{ipsc.BurstVoice, ipsc.BurstVoice, 0, false, "Voice"},
		{ipsc.BurstVoice | 0x80, ipsc.BurstVoice, 0, false, "Voice slot 2"},
		{ipsc.BurstSyncLost | 0x80, ipsc.BurstSyncLost, 0, false, "SyncLost slot 2"},
		{ipsc.BurstRaw, ipsc.BurstRaw, 0, false, "Raw"},
		{0x09, 0x09, 0, false, "BurstType(9)"},
	}
	for _, tt := range tests {
		dt, ok := tt.b.DataType()
		if tt.b.Type() != tt.typ || dt != tt.dt || ok != tt.ok || tt.b.String() != tt.name {
			t.Errorf("%#x: Type %#x, DataType %v %v, String %q", byte(tt.b), byte(tt.b.Type()), dt, ok, tt.b.String())
		}
	}
	u := ipsc.User{Burst: ipsc.BurstVoice | 0x80}
	if !u.IsVoice() {
		t.Error("slot 2 voice is not voice")
	}
}

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
	if _, err := (&ipsc.Control{Opcode: ipsc.OpGroupVoice}).AppendBinary(nil); err == nil {
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
		if _, err := u.SetRawBurst(raw, layer2.Hint{Kind: layer2.KindData, DataType: enums.DataTypeCSBK}); err == nil {
			t.Error("data without a data type: no error")
		}
		var nu *ipsc.User
		if _, err := nu.SetRawBurst(raw, layer2.Hint{Kind: layer2.KindData, DataType: enums.DataTypeCSBK, HasDataType: true}); err == nil {
			t.Error("nil User: no error")
		}
		_, err := u.SetRawBurst(raw, layer2.Hint{Kind: tt.k, VoiceBurst: tt.pos, DataType: tt.dt, HasDataType: true})
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
	if _, err := u.SetRawBurst(raw, layer2.Hint{Kind: layer2.KindData, DataType: enums.DataTypeCSBK, HasDataType: true}); err != nil {
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
	if h := got.Hint(0); h.Kind != layer2.KindData || h.DataType != enums.DataTypeCSBK || h.Slot != enums.Slot2 {
		t.Fatalf("hint %+v", h)
	}
}

func TestUserCall(t *testing.T) {
	t.Parallel()

	u := ipsc.User{Opcode: ipsc.OpPrivateVoice, Destination: 3120001, Source: 3191868}
	if g, tg, src := u.Call(); g || tg != 3120001 || src != 3191868 {
		t.Fatalf("Call: %v %d %d", g, tg, src)
	}
	for _, c := range []struct {
		op    ipsc.Opcode
		burst ipsc.BurstType
		group bool
		want  ipsc.Opcode
	}{
		{ipsc.OpPrivateVoice, 0, true, ipsc.OpGroupVoice}, {ipsc.OpGroupVoice, 0, false, ipsc.OpPrivateVoice},
		{ipsc.OpPrivateData, ipsc.BurstVoice, true, ipsc.OpGroupData}, {ipsc.OpGroupData, 0, false, ipsc.OpPrivateData},
		{0, ipsc.BurstVoice, true, ipsc.OpGroupVoice}, {0, 3, false, ipsc.OpPrivateData},
	} {
		u := ipsc.User{Opcode: c.op, Burst: c.burst}
		if err := u.SetCall(c.group, 91, 7); err != nil || u.Opcode != c.want || u.Destination != 91 || u.Source != 7 {
			t.Errorf("%+v: %v %v", c, err, u.Opcode)
		}
		if g, _, _ := u.Call(); g != c.group {
			t.Errorf("%+v: group %v", c, g)
		}
	}
	want := u
	if err := u.SetCall(true, 1<<24, 1); err == nil || u != want {
		t.Fatalf("target over 24 bits: %v", err)
	}
	if err := u.SetCall(true, 1, 1<<24); err == nil || u != want {
		t.Fatalf("source over 24 bits: %v", err)
	}
	var nu *ipsc.User
	if g, tg, src := nu.Call(); g || tg != 0 || src != 0 {
		t.Fatal("nil User Call")
	}
	if err := nu.SetCall(true, 1, 1); err == nil {
		t.Fatal("nil User SetCall: no error")
	}
}
