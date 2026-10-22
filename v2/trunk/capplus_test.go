package trunk_test

import (
	"bufio"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func readCapPlusCapture(t testing.TB, path string) []capture {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var caps []capture
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		cols := strings.Fields(line)
		if len(cols) != 3 {
			t.Fatalf("bad line %q", line)
		}
		ms, err1 := strconv.ParseFloat(cols[0], 64)
		cach, err2 := hex.DecodeString(cols[1])
		burst, err3 := hex.DecodeString(cols[2])
		if err1 != nil || err2 != nil || err3 != nil || len(cach) != 3 || len(burst) != 33 {
			t.Fatalf("bad line %q", line)
		}
		c := [3]byte(cach)
		caps = append(caps, capture{at: time.Duration(ms * float64(time.Millisecond)), raw: layer2.RawBurst(burst), cach: &c})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return caps
}

func TestCapacityPlusFollower_RealLCPBeacon(t *testing.T) {
	t.Parallel()
	caps := readCapPlusCapture(t, "testdata/capplus_lcp_beacon.txt")
	f, err := trunk.NewCapacityPlusFollower(nil)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	got := make([]trunk.OutputKind, 0, 4)
	for i := range caps {
		c := &caps[i]
		out = f.Burst(c.at, &c.raw, c.hint, c.cach, out[:0])
		got = append(got, kinds(out)...)
	}
	if len(got) != 1 || got[0] != trunk.OutputSite {
		t.Errorf("outputs %v, want one Site", got)
	}
	if lsn, ok := f.RestLSN(); !ok || lsn != 6 {
		t.Errorf("RestLSN %d %t, want 6", lsn, ok)
	}
	if site, ok := f.SiteNumber(); !ok || site != 3 {
		t.Errorf("SiteNumber %d %t, want 3", site, ok)
	}
	if f.State() != trunk.StateControl {
		t.Errorf("state %v", f.State())
	}
	last := caps[len(caps)-1].at
	if out = f.Tick(last+trunk.CapacityPlusNoSignalDefault, out[:0]); len(out) != 1 || out[0].Kind != trunk.OutputLost {
		t.Errorf("after the beacons: %v, want Lost", kinds(out))
	}
}

func TestCapacityPlusFollower_RealBusy(t *testing.T) {
	t.Parallel()
	caps := readCapPlusCapture(t, "testdata/capplus_busy.txt")
	f, err := trunk.NewCapacityPlusFollower(nil)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	var rests []uint8
	sites := 0
	for i := range caps {
		c := &caps[i]
		out = f.Burst(c.at, &c.raw, c.hint, c.cach, out[:0])
		for _, o := range out {
			if o.Kind != trunk.OutputSite {
				t.Fatalf("at %v: output %v", c.at, o.Kind)
			}
			sites++
		}
		if lsn, ok := f.RestLSN(); ok && (len(rests) == 0 || rests[len(rests)-1] != lsn) {
			rests = append(rests, lsn)
		}
	}
	if sites != 1 {
		t.Errorf("%d Site outputs, want 1", sites)
	}
	if len(rests) < 5 || rests[0] != 3 {
		t.Fatalf("rest LSNs %v, want several moves between 3 and 4 from 3", rests)
	}
	for _, lsn := range rests {
		if lsn != 3 && lsn != 4 {
			t.Fatalf("rest LSNs %v, want only 3 and 4", rests)
		}
	}
}

func capPlusStatusBurst(t testing.TB, slot2 bool, rest uint8, activity ...byte) layer2.RawBurst {
	t.Helper()
	p := pdu.CapacityPlusSiteStatusPDU{Segment: pdu.CapacityPlusSegmentSingle, Slot2: slot2, RestLSN: rest}
	copy(p.Activity[:], activity)
	var payload [8]byte
	payload[0] = byte(p.Segment)<<6 | rest&0x1F
	if slot2 {
		payload[0] |= 0x20
	}
	copy(payload[1:], p.Activity[:])
	c := &pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKCapacityPlusSiteStatus, FID: enums.FIDMotorola, RawPayload: payload}
	if got, ok := c.AsCapacityPlusSiteStatus(); !ok || got != p {
		t.Fatalf("site status %x reads as %+v %t, want %+v", payload, got, ok, p)
	}
	return bsBurst(t, c, testCC)
}

func capPlusGrantBurst(t testing.TB, tg uint8, rest uint8) layer2.RawBurst {
	t.Helper()
	g := pdu.CapacityPlusGroupVoiceGrant{GroupAddress: tg, RestLSN: rest, SourceAddress: 1400}
	payload := [7]byte{0, 0, 0, tg, rest & 0x1F, byte(g.SourceAddress >> 8), byte(g.SourceAddress)}
	lc := &pdu.FullLinkControl{BurstDataType: enums.DataTypeVoiceLCHeader, FLCO: pdu.FLCOCapacityPlusGroupVoiceGrant, FID: enums.FIDMotorola, RawPayload: payload}
	if got, ok := lc.AsCapacityPlusGroupVoiceGrant(); !ok || got != g {
		t.Fatalf("group grant %x reads as %+v %t, want %+v", payload, got, ok, g)
	}
	return bsBurst(t, lc, testCC)
}

func capPlusPlan(t testing.TB) *trunk.ChannelPlan {
	t.Helper()
	var p trunk.ChannelPlan
	for r := uint16(1); r <= 3; r++ {
		if err := p.Add(trunk.Channel{Number: r, RXHz: 450_000_000 + uint64(r)*25_000}); err != nil {
			t.Fatal(err)
		}
	}
	return &p
}

func TestCapacityPlusFollower_HuntAndMove(t *testing.T) {
	t.Parallel()
	f, err := trunk.NewCapacityPlusFollower(&trunk.CapacityPlusConfig{Plan: capPlusPlan(t)})
	if err != nil {
		t.Fatal(err)
	}
	out := f.Tick(0, nil)
	wantKinds(t, "start", out, trunk.OutputTune)
	if out[0].Tune.Channel != 1 || out[0].Tune.RXHz != 450_025_000 {
		t.Fatalf("first tune %+v", out[0].Tune)
	}
	out = f.Tick(trunk.CapacityPlusNoSignalDefault, nil)
	wantKinds(t, "dwell", out, trunk.OutputTune)
	if out[0].Tune.Channel != 2 {
		t.Fatalf("second tune %+v", out[0].Tune)
	}
	now := trunk.CapacityPlusNoSignalDefault + time.Second
	raw := capPlusStatusBurst(t, false, 5)
	out = f.Burst(now, &raw, layer2.Hint{Slot: enums.Slot1}, nil, nil)
	wantKinds(t, "status on repeater 2 names LSN 5", out, trunk.OutputSite, trunk.OutputTune)
	if tu := out[1].Tune; tu.Channel != 3 || tu.Slot != enums.Slot1 || tu.RXHz != 450_075_000 {
		t.Fatalf("tune to the rest %+v", tu)
	}
	raw = capPlusStatusBurst(t, false, 5)
	if out = f.Burst(now+slot, &raw, layer2.Hint{Slot: enums.Slot1}, nil, nil); len(out) != 0 {
		t.Fatalf("status on the rest: %v", kinds(out))
	}
	if at, ok := f.Deadline(); !ok || at != now+slot+trunk.CapacityPlusNoSignalDefault {
		t.Errorf("Deadline %v %t", at, ok)
	}
}

func TestCapacityPlusFollower_GroupCall(t *testing.T) {
	t.Parallel()
	cfg := trunk.CapacityPlusConfig{TalkgroupsLen: 1}
	cfg.Talkgroups[0] = 14
	f, err := trunk.NewCapacityPlusFollower(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	h := layer2.Hint{Slot: enums.Slot1}
	raw := capPlusStatusBurst(t, false, 1)
	wantKinds(t, "status", f.Burst(0, &raw, h, nil, nil), trunk.OutputSite)

	raw = capPlusGrantBurst(t, 9, 2)
	if out := f.Burst(slot, &raw, h, nil, nil); len(out) != 0 || f.State() != trunk.StateControl {
		t.Fatalf("unwanted grant: %v %v", kinds(out), f.State())
	}
	if lsn, _ := f.RestLSN(); lsn != 2 {
		t.Fatalf("rest after the unwanted grant %d, want 2", lsn)
	}

	h.Slot = enums.Slot2
	raw = capPlusGrantBurst(t, 14, 1)
	out := f.Burst(2*slot, &raw, h, nil, nil)
	wantKinds(t, "wanted grant", out, trunk.OutputGrant)
	g := out[0].Grant
	if !g.Group || g.Target != 14 || g.Source != 1400 || g.Channel != 1 || g.Slot != enums.Slot2 || g.LateEntry {
		t.Fatalf("grant %+v", g)
	}
	if c, ok := f.Call(); !ok || c != g {
		t.Fatalf("Call %+v %t", c, ok)
	}
	raw = terminator(t, 14)
	if out = f.Burst(3*slot, &raw, h, nil, nil); len(out) != 0 {
		t.Fatalf("terminator: %v", kinds(out))
	}
	out = f.Tick(3*slot+4*time.Second, nil)
	wantKinds(t, "hangtime", out, trunk.OutputCallEnd, trunk.OutputTune)
	if out[0].Reason != trunk.ReasonHangtime || out[1].Tune.Channel != 1 || out[1].Tune.Slot != enums.Slot1 {
		t.Fatalf("leave %+v", out)
	}
}

func TestCapacityPlusFollower_LateEntry(t *testing.T) {
	t.Parallel()
	f, err := trunk.NewCapacityPlusFollower(&trunk.CapacityPlusConfig{Plan: capPlusPlan(t), Inactive: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	wantKinds(t, "start", f.Tick(0, nil), trunk.OutputTune)
	h := layer2.Hint{Slot: enums.Slot2}
	raw := capPlusStatusBurst(t, true, 2, 0x00, 0x20, 30)
	out := f.Burst(time.Second, &raw, h, nil, nil)
	wantKinds(t, "status with LSN 11 busy", out, trunk.OutputSite, trunk.OutputGrant, trunk.OutputTune)
	g := out[1].Grant
	if g.Target != 30 || !g.LateEntry || g.Opcode != pdu.CSBKCapacityPlusSiteStatus || g.Channel != 6 || g.Slot != enums.Slot1 || g.RXHz != 0 {
		t.Fatalf("late entry grant %+v", g)
	}
	if tu := out[2].Tune; tu.Channel != 6 || tu.Slot != enums.Slot1 {
		t.Fatalf("tune %+v", tu)
	}
	out = f.Tick(2*time.Second, nil)
	wantKinds(t, "inactive", out, trunk.OutputCallEnd, trunk.OutputTune)
	if out[0].Reason != trunk.ReasonInactive || out[1].Tune.Channel != 1 || out[1].Tune.Slot != enums.Slot2 {
		t.Fatalf("leave %+v", out)
	}
}

func TestCapacityPlusFollower_LateEntryTune(t *testing.T) {
	t.Parallel()
	f, err := trunk.NewCapacityPlusFollower(&trunk.CapacityPlusConfig{Plan: capPlusPlan(t)})
	if err != nil {
		t.Fatal(err)
	}
	wantKinds(t, "start", f.Tick(0, nil), trunk.OutputTune)
	raw := capPlusStatusBurst(t, false, 1, 0x08, 7)
	out := f.Burst(time.Second, &raw, layer2.Hint{Slot: enums.Slot1}, nil, nil)
	wantKinds(t, "status with LSN 5 busy", out, trunk.OutputSite, trunk.OutputGrant, trunk.OutputTune)
	if tu := out[2].Tune; tu.Channel != 3 || tu.Slot != enums.Slot1 || tu.RXHz != 450_075_000 {
		t.Fatalf("tune %+v", tu)
	}
	f.Reset()
	if _, ok := f.RestLSN(); ok || f.State() != trunk.StateHunting {
		t.Error("Reset kept the site")
	}
}

func TestCapacityPlusFollower_Config(t *testing.T) {
	t.Parallel()
	if _, err := trunk.NewCapacityPlusFollower(&trunk.CapacityPlusConfig{TalkgroupsLen: trunk.MaxTalkgroups + 1}); err == nil {
		t.Error("TalkgroupsLen over MaxTalkgroups accepted")
	}
	f, err := trunk.NewCapacityPlusFollower(&trunk.CapacityPlusConfig{NoFollow: true, NoSignal: -1})
	if err != nil {
		t.Fatal(err)
	}
	raw := capPlusStatusBurst(t, false, 1, 0x08, 7)
	out := f.Burst(0, &raw, layer2.Hint{Slot: enums.Slot1}, nil, nil)
	wantKinds(t, "no follow", out, trunk.OutputSite, trunk.OutputGrant)
	if _, ok := f.Deadline(); ok {
		t.Error("Deadline with NoSignal off")
	}
	if out = f.Lost(time.Second, out[:0]); len(out) != 0 {
		t.Errorf("Lost on the rest: %v", kinds(out))
	}
}
