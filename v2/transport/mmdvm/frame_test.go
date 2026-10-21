package mmdvm_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func TestNext(t *testing.T) {
	t.Parallel()

	long := make([]byte, 300)
	for i := range long {
		long[i] = byte(i)
	}
	lf, err := mmdvm.AppendFrame(nil, mmdvm.CommandFMData, long)
	if err != nil || len(lf) != 304 || lf[1] != 0 || lf[2] != 304-255 || lf[3] != byte(mmdvm.CommandFMData) {
		t.Fatalf("%x %v", lf[:4], err)
	}
	cases := []struct {
		name string
		p    []byte
		n    int
		st   mmdvm.Status
		cmd  mmdvm.Command
		pl   []byte
	}{
		{"empty", nil, 0, mmdvm.StatusMore, 0, nil},
		{"start only", []byte{0xE0}, 0, mmdvm.StatusMore, 0, nil},
		{"partial", []byte{0xE0, 4, 0x70}, 0, mmdvm.StatusMore, 0, nil},
		{"ack", []byte{0xE0, 4, 0x70, 2, 0xE0}, 4, mmdvm.StatusFrame, mmdvm.CommandAck, []byte{2}},
		{"no payload", []byte{0xE0, 3, 1}, 3, mmdvm.StatusFrame, mmdvm.CommandGetStatus, []byte{}},
		{"junk", []byte{1, 2, 0xE0, 3}, 2, mmdvm.StatusSkip, 0, nil},
		{"all junk", []byte{1, 2, 3}, 3, mmdvm.StatusSkip, 0, nil},
		{"length 2", []byte{0xE0, 2, 0, 0}, 1, mmdvm.StatusSkip, 0, nil},
		{"long partial", []byte{0xE0, 0}, 0, mmdvm.StatusMore, 0, nil},
		{"long 255", []byte{0xE0, 0, 0, 0}, 1, mmdvm.StatusSkip, 0, nil},
		{"long", lf, 304, mmdvm.StatusFrame, mmdvm.CommandFMData, long},
		{"long short", lf[:303], 0, mmdvm.StatusMore, 0, nil},
	}
	for _, c := range cases {
		f, n, st := mmdvm.Next(c.p)
		if n != c.n || st != c.st || st == mmdvm.StatusFrame && (f.Command != c.cmd || !bytes.Equal(f.Payload, c.pl)) {
			t.Errorf("%s: %+v %d %v", c.name, f, n, st)
		}
	}
	if _, err := mmdvm.AppendFrame(nil, mmdvm.CommandFMData, make([]byte, 507)); err == nil {
		t.Error("511-octet frame encodes")
	}
	if p, err := mmdvm.AppendFrame(nil, mmdvm.CommandFMData, make([]byte, 506)); err != nil || len(p) != mmdvm.MaxFrameLen {
		t.Errorf("%d %v", len(p), err)
	}
	if p, _ := mmdvm.AppendFrame(nil, mmdvm.CommandFMData, make([]byte, 252)); len(p) != 255 || p[1] != 255 {
		t.Errorf("%x", p[:3])
	}
	if p, _ := mmdvm.AppendFrame(nil, mmdvm.CommandFMData, make([]byte, 253)); len(p) != 257 || p[1] != 0 || p[2] != 2 {
		t.Errorf("%x", p[:3])
	}
}

func TestDMRData(t *testing.T) {
	t.Parallel()

	raw, _ := layer2.BuildDataBurst([12]byte{1}, enums.DataTypeCSBK, 1, layer2.SyncSourceBS)
	d := mmdvm.DMRData{Slot: enums.Slot1, Control: mmdvm.ControlDataSync | mmdvm.Control(enums.DataTypeCSBK), Burst: raw, RSSI: 0x1234, HasRSSI: true}
	pl, _ := d.AppendBinary(nil)
	if len(pl) != mmdvm.DMRDataLen+2 || pl[34] != 0x12 || pl[35] != 0x34 || d.Command() != mmdvm.CommandDMRData1 {
		t.Fatalf("%x %v", pl, d.Command())
	}
	got, err := mmdvm.DecodeDMRData(mmdvm.Frame{Command: mmdvm.CommandDMRData1, Payload: pl})
	if err != nil || got != d {
		t.Fatalf("%+v %v", got, err)
	}
	if h := got.Hint(); h.Kind != layer2.KindData || h.DataType != enums.DataTypeCSBK || h.Slot != enums.Slot1 {
		t.Errorf("%+v", h)
	}
	for _, c := range []struct {
		ctl mmdvm.Control
		k   layer2.Kind
		pos enums.VoiceBurstType
	}{{mmdvm.ControlVoiceSync, layer2.KindVoice, enums.VoiceBurstA}, {1, layer2.KindVoice, enums.VoiceBurstB}, {5, layer2.KindVoice, enums.VoiceBurstF}, {0, layer2.KindUnknown, enums.VoiceBurstUnknown}, {6, layer2.KindUnknown, enums.VoiceBurstUnknown}} {
		h := mmdvm.DMRData{Slot: enums.Slot2, Control: c.ctl}.Hint()
		if h.Kind != c.k || h.VoiceBurst != c.pos {
			t.Errorf("%#x: %+v", uint8(c.ctl), h)
		}
	}
	idle := mmdvm.ControlIdle | mmdvm.ControlDataSync | mmdvm.Control(enums.DataTypeCSBK)
	if !idle.Idle() || !idle.DataSync() || idle.VoiceSync() || idle.DataType() != enums.DataTypeCSBK || idle.N() != 3 {
		t.Errorf("%#x", uint8(idle))
	}
	lost, err := mmdvm.DecodeDMRData(mmdvm.Frame{Command: mmdvm.CommandDMRLost2})
	if err != nil || !lost.Lost || lost.Slot != enums.Slot2 || lost.Command() != mmdvm.CommandDMRLost2 || !lost.Hint().SyncLost {
		t.Fatalf("%+v %v", lost, err)
	}
	if p, _ := lost.AppendBinary(nil); len(p) != 0 {
		t.Errorf("%x", p)
	}
	if c := (mmdvm.DMRData{Slot: enums.Slot1, Lost: true}).Command(); c != mmdvm.CommandDMRLost1 {
		t.Error(c)
	}
	if c := (mmdvm.DMRData{Slot: enums.Slot2}).Command(); c != mmdvm.CommandDMRData2 {
		t.Error(c)
	}
	for _, c := range []struct {
		f    mmdvm.Frame
		want error
	}{
		{mmdvm.Frame{Command: mmdvm.CommandAck, Payload: pl}, mmdvm.ErrCommand},
		{mmdvm.Frame{Command: mmdvm.CommandDMRData2, Payload: pl[:33]}, mmdvm.ErrShort},
		{mmdvm.Frame{Command: mmdvm.CommandDMRData2, Payload: pl[:35]}, mmdvm.ErrLength},
		{mmdvm.Frame{Command: mmdvm.CommandDMRData2, Payload: append(append([]byte(nil), pl...), 0)}, mmdvm.ErrLength},
		{mmdvm.Frame{Command: mmdvm.CommandDMRLost1, Payload: pl[:1]}, mmdvm.ErrLength},
	} {
		if _, err := mmdvm.DecodeDMRData(c.f); !errors.Is(err, c.want) {
			t.Errorf("%v %d: %v, want %v", c.f.Command, len(c.f.Payload), err, c.want)
		}
	}
}

func TestVersionV2(t *testing.T) {
	t.Parallel()

	v := mmdvm.Version{Protocol: 2, Capabilities: mmdvm.CapDMR | mmdvm.CapDStar | mmdvm.CapPOCSAG, CPU: mmdvm.CPUST, UDID: [16]byte{1, 2, 3}, Description: []byte("MMDVM 20260101")}
	p, _ := v.AppendBinary(nil)
	if p[1] != 0x03 || p[2] != 0x01 || p[3] != 2 || len(p) != 20+14 {
		t.Fatalf("%x", p)
	}
	got, err := mmdvm.DecodeVersion(p)
	if err != nil || got.Capabilities != v.Capabilities || got.CPU != v.CPU || got.UDID != v.UDID || !bytes.Equal(got.Description, v.Description) {
		t.Fatalf("%+v %v", got, err)
	}
	if !got.Capabilities.Has(mmdvm.CapDMR|mmdvm.CapPOCSAG) || got.Capabilities.Has(mmdvm.CapFM) {
		t.Error(got.Capabilities)
	}
	if _, err := mmdvm.DecodeVersion(p[:19]); !errors.Is(err, mmdvm.ErrShort) {
		t.Error(err)
	}
	if _, err := mmdvm.DecodeVersion(nil); !errors.Is(err, mmdvm.ErrShort) {
		t.Error(err)
	}
	if v3, err := mmdvm.DecodeVersion([]byte{3, 'x'}); err != nil || v3.Capabilities != 0 || string(v3.Description) != "x" {
		t.Errorf("%+v %v", v3, err)
	}
}

func TestDebug(t *testing.T) {
	t.Parallel()

	d := mmdvm.Debug{Command: mmdvm.CommandDebug3, Text: []byte("DMRSlotRX: pos"), N: [4]int16{-2, 300}}
	p, err := d.AppendBinary(nil)
	if err != nil || d.Count() != 2 || !bytes.Equal(p[len(p)-4:], []byte{0xFF, 0xFE, 0x01, 0x2C}) {
		t.Fatalf("%x %v", p, err)
	}
	got, err := mmdvm.DecodeDebug(mmdvm.Frame{Command: mmdvm.CommandDebug3, Payload: p})
	if err != nil || got.N != d.N || !bytes.Equal(got.Text, d.Text) {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := mmdvm.DecodeDebug(mmdvm.Frame{Command: mmdvm.CommandDebug5, Payload: p[:7]}); !errors.Is(err, mmdvm.ErrShort) {
		t.Error(err)
	}
	if _, err := mmdvm.DecodeDebug(mmdvm.Frame{Command: mmdvm.CommandAck}); !errors.Is(err, mmdvm.ErrCommand) {
		t.Error(err)
	}
	dump, err := mmdvm.DecodeDebug(mmdvm.Frame{Command: mmdvm.CommandDebugDump, Payload: []byte{1, 2}})
	if err != nil || dump.Count() != 0 || !bytes.Equal(dump.Text, []byte{1, 2}) {
		t.Errorf("%+v %v", dump, err)
	}
	if _, err := (mmdvm.Debug{Command: mmdvm.CommandAck}).AppendBinary(nil); err == nil {
		t.Error("Ack encodes as debug")
	}
}

func TestStrings(t *testing.T) {
	t.Parallel()

	for c, want := range map[mmdvm.Command]string{mmdvm.CommandDMRData2: "DMRData2", mmdvm.CommandDebug4: "Debug", mmdvm.CommandM17EOT: "M17EOT", 0x0F: "Command(15)"} {
		if got := c.String(); got != want {
			t.Errorf("%#x: %q", uint8(c), got)
		}
	}
	for m, want := range map[mmdvm.Mode]string{mmdvm.ModeDMR: "DMR", mmdvm.ModeFM: "FM", 98: "Mode(98)"} {
		if got := m.String(); got != want {
			t.Errorf("%d: %q", m, got)
		}
	}
}
