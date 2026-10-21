package ipsc_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func damagedRate34(t *testing.T) layer2.RawBurst {
	t.Helper()
	var p [18]byte
	for i := range p {
		p[i] = byte(i * 37)
	}
	raw, err := layer2.BuildRate34DataBurst(p, 7, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	info := raw.Info()
	for i := range 12 {
		info[i] ^= 0xA5
	}
	if err := raw.SetInfo(info); err != nil {
		t.Fatal(err)
	}
	if _, res := raw.Rate34Payload(); !res.Uncorrectable {
		t.Fatal("damage is correctable")
	}
	return raw
}

func TestBurstRawRoundTrip(t *testing.T) {
	t.Parallel()

	raw := damagedRate34(t)
	u := ipsc.User{Opcode: ipsc.OpGroupData, Info: 0x20, RTP: ipsc.RTP{Flags: 0x80, PayloadType: 0x5D}}
	res, err := u.SetRawBurst(raw, layer2.Hint{Kind: layer2.KindData, DataType: enums.DataTypeRate34, HasDataType: true})
	if err != nil || !res.Uncorrectable || u.Burst != ipsc.BurstRaw || u.Data.Bits != 196 {
		t.Fatalf("SetRawBurst: %v %+v burst %#x bits %d", err, res, u.Burst, u.Data.Bits)
	}
	pkt, err := u.AppendBinary(nil)
	if err != nil || len(pkt) != 68 || pkt[33] != 17 {
		t.Fatalf("%x %v", pkt, err)
	}
	got, err := ipsc.DecodeUser(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if h := got.Hint(); h.Kind != layer2.KindData || !h.HasDataType || h.DataType != enums.DataTypeRate34 || h.Slot != enums.Slot2 {
		t.Fatalf("hint %+v", h)
	}
	back, err := got.RawBurst(1, layer2.SyncSourceBS)
	if err != nil || back != raw {
		t.Fatalf("rebuilt %x, want %x (%v)", back, raw, err)
	}
	if _, err := got.RawBurst(1, layer2.SyncSource(9)); err == nil {
		t.Fatal("unknown sync source: no error")
	}
	noST := got
	noST.Data.Flags[1] &^= 0x08
	if _, err := noST.RawBurst(1, layer2.SyncSourceBS); err == nil {
		t.Fatal("BurstRaw without a slot type: no error")
	}
}

func TestXPR8400BurstRaw(t *testing.T) {
	t.Parallel()

	n, again := 0, 0
	for _, c := range readXPR(t) {
		u, err := ipsc.DecodeUser(c.p)
		if err != nil || u.Burst != ipsc.BurstRaw {
			continue
		}
		n++
		st, ok := u.Data.SlotType()
		raw, err := u.RawBurst(7, layer2.SyncSourceBS)
		if !ok || err != nil {
			t.Fatalf("%x: %v %v", c.p, ok, err)
		}
		if info := raw.Info(); !bytes.Equal(info[:], u.Data.Octets[:25]) || raw.SlotType().ColorCode != st.ColorCode() {
			t.Fatalf("%x: info %x", c.p, info)
		}
		back := ipsc.User{Info: u.Info}
		h := u.Hint()
		h.DataType = enums.DataTypeRate34
		res, err := back.SetRawBurst(raw, h)
		if err != nil {
			t.Fatalf("%x: %v", c.p, err)
		}
		if res.Uncorrectable != (back.Burst == ipsc.BurstRaw) || back.Burst == ipsc.BurstRaw && back.Data.Octets != u.Data.Octets {
			t.Fatalf("%x: octets %x, want %x", c.p, back.Data.Octets, u.Data.Octets)
		}
		if back.Burst == ipsc.BurstRaw {
			again++
		}
		if rssi, ok := u.Data.RSSI(); ok && u.Data.Flags[1]&0x40 == 0 && rssi>>8 != 0x46 {
			t.Errorf("%x: RSSI %x", c.p, rssi)
		}
	}
	if n != 6 || again != 6 {
		t.Fatalf("%d BurstRaw packets, %d sent again as BurstRaw, want 6", n, again)
	}
}

func TestSetRawBurstVoiceBadFrames(t *testing.T) {
	t.Parallel()

	frames := [3]vocoder.AMBEFrame{vocoder.AMBESilence(), vocoder.AMBESilence(), vocoder.AMBESilence()}
	raw, err := layer2.BuildVoiceBurst(frames, enums.VoiceBurstA, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	slots := raw.VoiceSlots()
	clean := slots[0]
	for k := range 256 {
		slots[0] = clean
		slots[0][0] ^= byte(k)
		slots[0][1] ^= 0x0F
		if _, r := vocoder.DecodeAMBE72(slots[0]); r.Uncorrectable {
			break
		}
	}
	raw.SetVoiceSlots(slots)
	var u ipsc.User
	res, err := u.SetRawBurst(raw, layer2.Hint{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstA})
	if err != nil || !res.Uncorrectable || u.Voice.Control != 0x41 {
		t.Fatalf("%v %+v control %#x", err, res, u.Voice.Control)
	}
}
