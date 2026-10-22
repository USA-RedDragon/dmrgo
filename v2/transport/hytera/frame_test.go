package hytera_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

func csbkFrame(t *testing.T) hytera.Frame {
	t.Helper()
	raw, err := layer2.BuildDataBurst([12]byte{0xBD, 0, 1, 2, 3, 4, 5, 6, 7, 8}, enums.DataTypeCSBK, 1, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	return hytera.Frame{
		Port: 50002, Mark: 0x0050, Seq: 0x2A, PacketType: 1, Slot: enums.Slot2, SlotType: hytera.SlotTypeCSBK,
		ColorCode: 1, Burst: raw, Pad: 0x5A, Destination: 262, Source: 2628347,
		Reserved: [17]byte{3, 0, 0, 0, 5, 1, 2, 0, 0, 0, 0x40, 0xB8},
	}
}

func TestFrameOctets(t *testing.T) {
	t.Parallel()

	f := csbkFrame(t)
	p, err := f.AppendBinary(nil)
	if err != nil || len(p) != hytera.FrameLen {
		t.Fatalf("%x %v", p, err)
	}
	want := map[int]byte{0: 0xC3, 1: 0x52, 2: 0x00, 3: 0x50, 4: 0x2A, 5: 3, 8: 1, 10: 5, 11: 1, 12: 2, 16: 0x22, 17: 0x22,
		18: 0x33, 19: 0x33, 20: 0x11, 21: 0x11, 24: 0x40, 25: 0xB8, 26: f.Burst[1], 27: f.Burst[0], 58: 0x5A, 59: f.Burst[32],
		64: 0x06, 65: 0x01, 66: 0x00, 68: 0xFB, 69: 0x1A, 70: 0x28}
	for i, b := range want {
		if p[i] != b {
			t.Errorf("octet %d is %#x, want %#x", i, p[i], b)
		}
	}
	got, err := hytera.DecodeFrame(p)
	if err != nil || got != f {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestFrameErrors(t *testing.T) {
	t.Parallel()

	f := csbkFrame(t)
	p, _ := f.AppendBinary(nil)
	cases := []struct {
		name string
		p    []byte
		want error
	}{
		{"short", p[:71], hytera.ErrShort},
		{"long", append(append([]byte(nil), p...), 0), hytera.ErrLength},
		{"slot", with(p, 16, 0x33), hytera.ErrCode},
		{"color code", with(p, 21, 0x12), hytera.ErrCode},
	}
	for _, c := range cases {
		if _, err := hytera.DecodeFrame(c.p); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	bad := f
	bad.Slot = enums.SlotUnknown
	if out, err := bad.AppendBinary(p[:0]); err == nil || len(out) != 0 {
		t.Errorf("slot: %x %v", out, err)
	}
	bad = f
	bad.ColorCode = 16
	if _, err := bad.AppendBinary(nil); err == nil {
		t.Error("color code 16 encodes")
	}
	bad = f
	bad.Source = 1 << 24
	if _, err := bad.AppendBinary(nil); err == nil {
		t.Error("25-bit source encodes")
	}
}

func with(p []byte, i int, b byte) []byte {
	q := append([]byte(nil), p...)
	q[i] = b
	return q
}

func TestFrameHint(t *testing.T) {
	t.Parallel()

	voice := []hytera.SlotType{hytera.SlotTypeVoiceA, hytera.SlotTypeVoiceB, hytera.SlotTypeVoiceC, hytera.SlotTypeVoiceD, hytera.SlotTypeVoiceE, hytera.SlotTypeVoiceF}
	for i, s := range voice {
		f := hytera.Frame{Slot: enums.Slot1, SlotType: s, Seq: 9}
		h := f.Hint()
		pos, _ := enums.VoiceBurstAt(i)
		if h.Kind != layer2.KindVoice || h.VoiceBurst != pos || h.Slot != enums.Slot1 || !h.HasSeq || h.Seq != 9 || h.SeqBits != 8 {
			t.Errorf("%v: %+v", s, h)
		}
		var back hytera.Frame
		if err := back.SetHint(layer2.Hint{Slot: enums.Slot2, Kind: layer2.KindVoice, VoiceBurst: pos}); err != nil || back.SlotType != s || back.Slot != enums.Slot2 {
			t.Errorf("%v: SetHint %v %v", s, back.SlotType, err)
		}
	}
	data := map[hytera.SlotType]enums.DataType{
		hytera.SlotTypePIHeader: enums.DataTypePIHeader, hytera.SlotTypeVoiceLCHeader: enums.DataTypeVoiceLCHeader,
		hytera.SlotTypeTerminator: enums.DataTypeTerminatorWithLC, hytera.SlotTypeCSBK: enums.DataTypeCSBK,
		hytera.SlotTypeDataHeader: enums.DataTypeDataHeader, hytera.SlotTypeRate12: enums.DataTypeRate12,
		hytera.SlotTypeRate34: enums.DataTypeRate34,
	}
	for s, dt := range data {
		f := hytera.Frame{SlotType: s}
		if h := f.Hint(); h.Kind != layer2.KindData || !h.HasDataType || h.DataType != dt || !f.HasBurst() {
			t.Errorf("%v: %+v", s, h)
		}
		var back hytera.Frame
		if err := back.SetHint(layer2.Hint{Kind: layer2.KindData, DataType: dt, HasDataType: true}); err != nil || back.SlotType != s {
			t.Errorf("%v: SetHint %v %v", dt, back.SlotType, err)
		}
	}
	for _, s := range []hytera.SlotType{hytera.SlotTypeSync, hytera.SlotTypeWakeup, 0x1234, 0xFFFF} {
		f := hytera.Frame{SlotType: s}
		if h := f.Hint(); h.Kind != layer2.KindUnknown {
			t.Errorf("%v: %+v", s, h)
		}
	}
	if (&hytera.Frame{SlotType: hytera.SlotTypeSync}).HasBurst() || (&hytera.Frame{SlotType: hytera.SlotTypeWakeup}).HasBurst() {
		t.Error("sync or wakeup has a burst")
	}
	f := hytera.Frame{SlotType: hytera.SlotTypeCSBK}
	for _, c := range []struct {
		k    layer2.Kind
		pos  enums.VoiceBurstType
		dt   enums.DataType
		want error
	}{
		{layer2.KindVoice, enums.VoiceBurstUnknown, 0, layer2.ErrVoicePosition},
		{layer2.KindData, 0, enums.DataTypeIdle, layer2.ErrDataType},
		{layer2.KindData, 0, enums.DataTypeRate1, layer2.ErrDataType},
		{layer2.KindUnknown, 0, 0, nil},
		{layer2.KindReverseChannel, 0, 0, nil},
	} {
		err := f.SetHint(layer2.Hint{Kind: c.k, VoiceBurst: c.pos, DataType: c.dt, HasDataType: true})
		if err == nil || f.SlotType != hytera.SlotTypeCSBK {
			t.Errorf("%+v: %v %v", c, f.SlotType, err)
		}
		if c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("%+v: %v, want %v", c, err, c.want)
		}
	}
	if err := f.SetHint(layer2.Hint{Kind: layer2.KindData, DataType: enums.DataTypePIHeader}); err == nil || f.SlotType != hytera.SlotTypeCSBK {
		t.Errorf("data without a data type: %v %v", f.SlotType, err)
	}
	var nf *hytera.Frame
	if err := nf.SetHint(layer2.Hint{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstA}); err == nil {
		t.Error("nil Frame: no error")
	}
}

func TestSlotTypeString(t *testing.T) {
	t.Parallel()

	for s, want := range map[hytera.SlotType]string{hytera.SlotTypeVoiceA: "Voice A", hytera.SlotTypeVoiceF: "Voice F", hytera.SlotTypeSync: "Sync", hytera.SlotTypePIHeader: "PI Header", 0x1234: "SlotType(4660)"} {
		if got := s.String(); got != want {
			t.Errorf("%#x: %q, want %q", uint16(s), got, want)
		}
	}
	if got := hytera.ServiceDMR.String(); got != "DMR" {
		t.Error(got)
	}
	if got := hytera.Service(1).String(); got != "Service(1)" {
		t.Error(got)
	}
}

func TestHeartbeat(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"ZZZZ\x0a\x00\x00\x00\x14\x00\x00\x00\x5a\x5a\x59\x5a\x00\x00\x00\x00", "\x00", "\x41"} {
		h, err := hytera.DecodeHeartbeat([]byte(s))
		if err != nil {
			t.Fatalf("%x: %v", s, err)
		}
		out, err := h.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, []byte(s)) {
			t.Fatalf("%x: %x %v", s, out, err)
		}
		if len(s) == 20 && (h.Keepalive || h.Code != 0x0A || h.Seq != 0x5A || h.Status != [3]byte{0x5A, 0x59, 0x5A} || h.Reserved[3] != 0x14) {
			t.Errorf("%+v", h)
		}
	}
	for _, c := range []struct {
		p    string
		want error
	}{{"", hytera.ErrShort}, {"ZZZZ", hytera.ErrShort}, {"YZZZ\x0a\x00\x00\x00\x14\x00\x00\x00\x5a\x5a\x59\x5a\x00\x00\x00\x00", hytera.ErrSignature}} {
		if _, err := hytera.DecodeHeartbeat([]byte(c.p)); !errors.Is(err, c.want) {
			t.Errorf("%x: %v, want %v", c.p, err, c.want)
		}
	}
}

func TestP2P(t *testing.T) {
	t.Parallel()

	for _, s := range []string{
		"P2P8\x01\x00\x00\x00\x14\x00\x00\x00\x1d\xff\x11\x02\x00\x00\x00\x00\x10\x00\x00\x00",
		"P2PP\x02\x00\x00\x00\x14\x00\x00\x00\x1d\x01\x01\x5a\x00\x00\x00\x00\x10\x00\x00\x00\x01",
		"P2P8\x0c\x00\x00\x00\x14\x00\x00\x00\x42\x01\x59\x5a\x00\x00\x00\x00",
		"P2PP\x0b\x00\x00\x00\x14\x00\x00\x00\x42\xff\x01\x00\x00\x00\x00\x00\x11\x00\x00\x00\xff\x01\x51\xc3",
	} {
		c, err := hytera.DecodeP2P([]byte(s))
		if err != nil {
			t.Fatalf("%x: %v", s, err)
		}
		out, err := c.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, []byte(s)) {
			t.Fatalf("%x: %x %v", s, out, err)
		}
		port, ok := c.RedirectPort()
		svc, sok := c.Service()
		if c.Code == 0x0B && (!ok || port != 50001 || svc != hytera.ServiceDMR) || c.Code != 0x0B && ok || sok != (c.Code != 0x0C) {
			t.Errorf("%x: %+v port %d %v service %v %v", s, c, port, ok, svc, sok)
		}
	}
	if _, err := hytera.DecodeP2P([]byte("P2P8\x01\x00\x00\x00\x14\x00\x00\x00\x1d\xff\x11\x02\x00\x00\x00")); !errors.Is(err, hytera.ErrShort) {
		t.Error(err)
	}
	if _, err := hytera.DecodeP2P([]byte("Q2P8\x01\x00\x00\x00\x14\x00\x00\x00\x1d\xff\x11\x02\x00\x00\x00\x00\x10\x00\x00\x00")); !errors.Is(err, hytera.ErrSignature) {
		t.Error(err)
	}
}
