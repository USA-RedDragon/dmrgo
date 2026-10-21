package ipsc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
)

func TestStreamMatchesXPR8400(t *testing.T) {
	t.Parallel()

	streams := map[uint64]*ipsc.Stream{}
	checked := 0
	for _, c := range readXPR(t) {
		if c.tag != "voicecall" && c.tag != "datacall" {
			continue
		}
		u, err := ipsc.DecodeUser(c.p)
		if err != nil {
			continue
		}
		key := uint64(u.CallSeq)<<32 | uint64(u.CallControl)
		s, ok := streams[key]
		if !ok || u.RTP.Marker {
			s = &ipsc.Stream{CallSeq: u.CallSeq, CallControl: u.CallControl, Seq: u.RTP.Seq, Timestamp: u.RTP.Timestamp}
			if !u.RTP.Marker {
				s.Sent = 1
			}
			streams[key] = s
		}
		if gap := u.RTP.Seq - s.Seq; gap != 0 {
			s.Skip(gap)
		}
		got := ipsc.User{Info: u.Info &^ 0x40, RTP: ipsc.RTP{SSRC: u.RTP.SSRC}}
		s.Fill(&got, u.RTP.PayloadType == 0x5E)
		if got.RTP != u.RTP || got.CallSeq != u.CallSeq || got.CallControl != u.CallControl || got.Info.End() != (u.RTP.PayloadType == 0x5E || u.Info.End()) {
			t.Errorf("%x: filled %+v %d %#x, want %+v %d %#x", c.p[:31], got.RTP, got.CallSeq, uint8(got.Info), u.RTP, u.CallSeq, uint8(u.Info))
		}
		checked++
	}
	if checked < 50 {
		t.Fatalf("%d packets checked", checked)
	}
}

func TestCallInfoSetters(t *testing.T) {
	t.Parallel()

	var c ipsc.CallInfo
	c.SetPrivacy(true)
	c.SetEnd(true)
	if err := c.SetSlot(enums.Slot2); err != nil || c != 0xE0 || !c.Privacy() || !c.End() || c.Slot() != enums.Slot2 {
		t.Fatalf("set: %#x %v", uint8(c), err)
	}
	if err := c.SetSlot(enums.SlotUnknown); err == nil || c != 0xE0 {
		t.Fatalf("unknown slot: %#x %v", uint8(c), err)
	}
	c.SetPrivacy(false)
	c.SetEnd(false)
	if err := c.SetSlot(enums.Slot1); err != nil || c != 0 {
		t.Fatalf("clear: %#x %v", uint8(c), err)
	}
	var nc *ipsc.CallInfo
	nc.SetEnd(true)
	nc.SetPrivacy(true)
	if err := nc.SetSlot(enums.Slot1); err == nil {
		t.Fatal("nil CallInfo: no error")
	}
	var ns *ipsc.Stream
	ns.Fill(&ipsc.User{}, true)
	ns.Skip(3)
	(&ipsc.Stream{}).Fill(nil, false)
	for _, o := range []struct {
		group, data bool
		want        ipsc.Opcode
	}{{true, true, ipsc.OpGroupData}, {false, true, ipsc.OpPrivateData}, {true, false, ipsc.OpGroupVoice}, {false, false, ipsc.OpPrivateVoice}} {
		if got := ipsc.UserOpcode(o.group, o.data); got != o.want {
			t.Errorf("%+v: %v", o, got)
		}
	}
}
