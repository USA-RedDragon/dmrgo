package voice_test

import (
	"bufio"
	"encoding/hex"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// TestReceiver_EmbeddedData sends a talker alias header, a talker alias
// block and GPS in the embedded LC of superframes 1 to 3.
func TestReceiver_EmbeddedData(t *testing.T) {
	t.Parallel()

	alias, err := pdu.EncodeTalkerAlias("KI5VMF JACOB", layer3.TalkerAliasISOEightBit)
	if err != nil || len(alias) != 2 {
		t.Fatalf("alias LCs %d: %v", len(alias), err)
	}
	gps := pdu.FullLinkControl{FLCO: enums.FLCOGPSInfo, GPSInfo: &pdu.FLCGPSInfo{Longitude: -87.5, Latitude: 41.75}}
	hook := func(sf int) *pdu.FullLinkControl {
		switch sf {
		case 1, 2:
			return &alias[sf-1]
		case 3:
			return &gps
		}
		return nil
	}
	bursts := transmission(t, groupLC(t, 91, 3191868), 4, func(o *layer2.VoiceTransmitterOptions) { o.EmbeddedLC = hook })
	got := feed(t, newReceiver(t, nil), timed(bursts, 0, layer2.Hint{}), -1)
	want := "S" + repeat("V", 6) + repeat("V", 5) + "M" + "V" + repeat("V", 5) + "MT" + "V" + repeat("V", 5) + "M" + "V" + "E"
	if k := kinds(got); k != want {
		t.Fatalf("events %s, want %s", k, want)
	}
	m := find(got, voice.EventEmbedded)
	if m[0].LC.FLCO != enums.FLCOTalkerAliasHeader || m[1].LC.FLCO != enums.FLCOTalkerAliasBlock1 ||
		m[2].LC.GPSInfo == nil || m[2].LC.GPSInfo.Latitude < 41.7 || m[2].Call.Target != 91 {
		t.Errorf("embedded %v %v %+v", m[0].LC.FLCO, m[1].LC.FLCO, m[2].LC.GPSInfo)
	}
	if ta := find(got, voice.EventTalkerAlias); string(ta[0].Text) != "KI5VMF JACOB" {
		t.Errorf("alias %q", ta[0].Text)
	}
}

// TestReceiver_XPR8400 runs the TS2 bursts of a real XPR 8400 outbound
// stream, with the slot from the CACH and the time from the sample number.
// Bursts without a SYNC that the RF receiver placed by timing are placed by
// their EMB here.
func TestReceiver_XPR8400(t *testing.T) {
	t.Parallel()

	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var in []input
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) != 4 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		sample, err1 := strconv.Atoi(fields[0])
		cach, err2 := hex.DecodeString(fields[1])
		b, err3 := hex.DecodeString(fields[3])
		if err1 != nil || err2 != nil || err3 != nil || len(cach) != 3 || len(b) != 33 {
			t.Fatalf("line %q", sc.Text())
		}
		if !layer2.DecodeCACHField([3]byte(cach)).TACT.TDMAChannel {
			continue
		}
		at := time.Duration(sample) * time.Second / 48000
		in = append(in, input{at: at, raw: layer2.RawBurst(b), hint: layer2.Hint{Slot: enums.Slot2}})
	}
	cc := enums.ColorCode(7)
	got := feed(t, newReceiver(t, &voice.ReceiverConfig{ColorCode: &cc}), in, -1)
	starts, ends := find(got, voice.EventCallStart), find(got, voice.EventCallEnd)
	if len(starts) != 1 || len(ends) != 1 {
		t.Fatalf("events %s", kinds(got))
	}
	if c := starts[0].Call; c.Target != 9 || c.Source != 3191868 || !c.Group || c.Origin != voice.OriginHeader {
		t.Errorf("call %+v", c)
	}
	if p := positions(got); p != repeat("ABCDEF", 5) {
		t.Errorf("positions %s", p)
	}
	if e := ends[0]; e.Reason != voice.EndTerminator || e.Stats.Lost != 0 || e.Stats.Bursts != 30 {
		t.Errorf("end %v %+v", e.Reason, e.Stats)
	}
}

// TestReceiver_Timers ends a call after Lost without a burst, and a late
// entry that never learns its LC.
func TestReceiver_Timers(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 2, nil)
	rx := newReceiver(t, &voice.ReceiverConfig{Lost: 500 * time.Millisecond, Hang: time.Second})
	got := feed(t, rx, timed(bursts[:5], 0, layer2.Hint{}), 4*slot+500*time.Millisecond)
	if k := kinds(got); k != "SVVVVE" || got[5].Reason != voice.EndLost || got[5].Stats.Duration != 4*slot+500*time.Millisecond {
		t.Fatalf("events %s %+v", k, got[5])
	}
	if rx.State() != voice.StateHang {
		t.Fatalf("state %v", rx.State())
	}

	rx.Reset()
	if rx.State() != voice.StateIdle {
		t.Fatal("Reset")
	}
	got = feed(t, rx, timed(bursts[2:5], 0, layer2.Hint{}), 10*time.Second)
	if k := kinds(got); k != "VVV" || rx.State() != voice.StateIdle {
		t.Fatalf("late entry: %s, state %v", k, rx.State())
	}

	off := newReceiver(t, &voice.ReceiverConfig{Lost: -1, Hang: -1})
	got = feed(t, off, timed(bursts[:5], 0, layer2.Hint{}), time.Hour)
	if k := kinds(got); k != "SVVVV" || off.State() != voice.StateInCall {
		t.Fatalf("timers off: %s, state %v", k, off.State())
	}
	if _, ok := off.Deadline(); ok {
		t.Error("deadline with the timers off")
	}
	got = feed(t, off, timed(bursts[len(bursts)-1:], time.Hour, layer2.Hint{}), -1)
	if k := kinds(got); k != "E" || off.State() != voice.StateIdle {
		t.Fatalf("no hang: %s, state %v", k, off.State())
	}
}

// TestReceiver_Ends checks the ends of a call other than the terminator.
func TestReceiver_Ends(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	idle, err := layer2.BuildDataBurst(layer2.IdleMessageInfo(), enums.DataTypeIdle, 1, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	csbk, err := pdu.NewBSOutboundActivation(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	data, err := layer2.BuildBurst(&csbk, 1, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	call := timed(bursts[:4], 0, layer2.Hint{})
	next := 4 * slot
	tests := []struct {
		name  string
		last  input
		end   voice.EndReason
		state voice.State
	}{
		{"idle burst", input{at: next, raw: idle}, voice.EndIdle, voice.StateIdle},
		{"data burst", input{at: next, raw: data}, voice.EndDataSync, voice.StateIdle},
		{"strong end", input{at: next, raw: bursts[4], hint: layer2.Hint{End: layer2.EndStrong}}, voice.EndTransport, voice.StateHang},
		{"sync lost", input{at: next, lost: true}, voice.EndSyncLost, voice.StateHang},
		{"sync lost hint", input{at: next, raw: bursts[4], hint: layer2.Hint{SyncLost: true}}, voice.EndSyncLost, voice.StateLateEntry},
	}
	for _, tt := range tests {
		rx := newReceiver(t, nil)
		got := feed(t, rx, append(append([]input(nil), call...), tt.last), -1)
		ends := find(got, voice.EventCallEnd)
		if len(ends) != 1 || ends[0].Reason != tt.end || rx.State() != tt.state {
			t.Errorf("%s: events %s, state %v", tt.name, kinds(got), rx.State())
		}
	}
}

// TestReceiver_Stream ends a call when the transport stream changes, and
// starts the next one from its header.
func TestReceiver_Stream(t *testing.T) {
	t.Parallel()

	a := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	b := transmission(t, groupLC(t, 91, 3120001), 1, nil)
	in := append(timed(a[:4], 0, layer2.Hint{HasStream: true, Stream: 1}), timed(b, 4*slot, layer2.Hint{HasStream: true, Stream: 2})...)
	got := feed(t, newReceiver(t, nil), in, -1)
	if k := kinds(got); k != "SVVVE"+"S"+repeat("V", 6)+"E" {
		t.Fatalf("events %s", k)
	}
	if got[4].Reason != voice.EndSuperseded || got[0].Call.Stream != 1 || got[5].Call.Stream != 2 || got[5].Call.Source != 3120001 {
		t.Errorf("end %v, streams %d %d", got[4].Reason, got[0].Call.Stream, got[5].Call.Stream)
	}
}

// TestReceiver_Superseded starts a new talker without a terminator, once
// with a header and once with the embedded LC only (V3).
func TestReceiver_Superseded(t *testing.T) {
	t.Parallel()

	a := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	b := transmission(t, groupLC(t, 92, 3120001), 2, nil)
	in := append(timed(a[:4], 0, layer2.Hint{}), timed(b, 4*slot, layer2.Hint{})...)
	got := feed(t, newReceiver(t, nil), in, -1)
	if k := kinds(got); k != "SVVVEeS"+repeat("V", 12)+"E" {
		t.Fatalf("header: events %s", k)
	}

	in = append(timed(a[:4], 0, layer2.Hint{}), timed(b[1:], 4*slot, layer2.Hint{})...)
	got = feed(t, newReceiver(t, nil), in, -1)
	if k := kinds(got); k != "SVVV"+"VVVV"+"EeSV"+repeat("V", 7)+"E" {
		t.Fatalf("embedded: events %s", k)
	}
	if e := find(got, voice.EventCallEnd)[0]; e.Reason != voice.EndSuperseded || e.Call.Target != 91 {
		t.Errorf("end %+v", e)
	}
	if s := find(got, voice.EventCallStart)[1]; s.Call.Origin != voice.OriginEmbedded || s.Call.Target != 92 {
		t.Errorf("start %+v", s.Call)
	}
}

// TestReceiver_Hang checks the call hang time rules.
func TestReceiver_Hang(t *testing.T) {
	t.Parallel()

	a := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	same := transmission(t, groupLC(t, 91, 3120001), 1, nil)
	other := transmission(t, groupLC(t, 92, 3120001), 1, nil)
	afterA := time.Duration(len(a)) * slot

	got := feed(t, newReceiver(t, nil), append(timed(a, 0, layer2.Hint{}), timed(same, afterA+time.Second, layer2.Hint{})...), -1)
	if k := kinds(got); k != "S"+repeat("V", 6)+"E"+"S"+repeat("V", 6)+"E" {
		t.Errorf("same target: events %s", k)
	}
	got = feed(t, newReceiver(t, nil), append(timed(a, 0, layer2.Hint{}), timed(other, afterA+time.Second, layer2.Hint{})...), -1)
	if k := kinds(got); k != "S"+repeat("V", 6)+"E"+"eS"+repeat("V", 6)+"E" {
		t.Errorf("other target: events %s", k)
	}

	rx := newReceiver(t, nil)
	got = feed(t, rx, timed(a[len(a)-1:], 0, layer2.Hint{}), -1)
	if k := kinds(got); k != "s" || got[0].Call.Origin != voice.OriginTerminator || got[0].Call.Target != 91 || rx.State() != voice.StateHang {
		t.Fatalf("terminator in Idle: %s, state %v", k, rx.State())
	}
	got = feed(t, rx, timed(a[len(a)-1:], slot, layer2.Hint{}), -1)
	if len(got) != 0 {
		t.Errorf("repeated terminator: %s", kinds(got))
	}
	got = feed(t, rx, timed(other[len(other)-1:], 2*slot, layer2.Hint{}), -1)
	if k := kinds(got); k != "es" || got[1].Call.Target != 92 {
		t.Errorf("other terminator: %s", k)
	}

	lc := groupLC(t, 91, 3191868)
	lc.GroupVoice.ServiceOptions.IsBroadcast = true
	all := groupLC(t, constants.AllMSID, 3191868)
	for _, l := range []pdu.FullLinkControl{lc, all} {
		rx := newReceiver(t, nil)
		feed(t, rx, timed(transmission(t, l, 1, nil), 0, layer2.Hint{}), -1)
		if rx.State() != voice.StateIdle {
			t.Errorf("broadcast or all call: state %v, want Idle", rx.State())
		}
	}
}

// TestReceiver_Expect starts a call from Expect, then learns its LC from a
// header with the same addresses, without an event.
func TestReceiver_Expect(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	rx := newReceiver(t, nil)
	rx.Expect(0, voice.Call{Group: true, Target: 91, Source: 3191868}, time.Second)
	in := timed(bursts[1:3], 0, layer2.Hint{})
	in = append(in, input{at: 2 * slot, raw: bursts[0]})
	in = append(in, timed(bursts[3:], 3*slot, layer2.Hint{})...)
	got := feed(t, rx, in, -1)
	if k := kinds(got); k != "S"+repeat("V", 6)+"E" {
		t.Fatalf("events %s", k)
	}
	if c := got[0].Call; c.Origin != voice.OriginExpected || c.LC != ([9]byte{}) {
		t.Errorf("start %+v", c)
	}
	if c := got[len(got)-1].Call; c.Origin != voice.OriginExpected || c.LC == ([9]byte{}) {
		t.Errorf("end %+v, want the LC of the header", c)
	}

	rx = newReceiver(t, nil)
	rx.Expect(0, voice.Call{Group: true, Target: 91, Source: 3191868}, slot)
	got = feed(t, rx, timed(bursts[1:3], 2*slot, layer2.Hint{}), -1)
	if k := kinds(got); k != "VV" || got[0].Call.Origin != voice.OriginUnknown {
		t.Errorf("expired Expect: %s", k)
	}
}

// TestReceiver_Duplicates repeats a header and a voice burst.
func TestReceiver_Duplicates(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 1, nil)
	in := timed(bursts[:1], 0, layer2.Hint{})
	in = append(in, input{at: slot, raw: bursts[0]}, input{at: 2 * slot, raw: bursts[0]})
	in = append(in, timed(bursts[1:3], 3*slot, layer2.Hint{})...)
	in = append(in, input{at: 4*slot + 10*time.Millisecond, raw: bursts[2]})
	in = append(in, timed(bursts[3:], 5*slot, layer2.Hint{})...)
	got := feed(t, newReceiver(t, nil), in, -1)
	end := got[len(got)-1]
	if k := kinds(got); k != "S"+repeat("V", 6)+"E" || end.Stats.Duplicates != 3 || end.Stats.Lost != 0 {
		t.Fatalf("events %s, stats %+v", k, end.Stats)
	}
	if p := positions(got); p != "ABCDEF" {
		t.Errorf("positions %s", p)
	}
}

// network returns bursts 60 ms apart with HomeBrew-like hints: a
// sequence number that counts every burst from seq, and a stream ID.
func network(bursts []layer2.RawBurst, seq uint16) []input {
	in := timed(bursts, 0, layer2.Hint{})
	for i := range in {
		in[i].hint = layer2.Hint{Seq: seq + uint16(i), HasSeq: true, Stream: 7, HasStream: true}
	}
	return in
}

// TestReceiver_Sequence places network bursts by their sequence number,
// with lost and late bursts and jitter in the arrival time.
func TestReceiver_Sequence(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 3, nil)
	in := network(bursts, 65530)
	for i := range in {
		in[i].at = time.Duration(i) * 5 * time.Millisecond
	}
	drop := map[int]bool{3: true, 9: true, 10: true, 11: true, 13: true}
	var kept []input
	for i, x := range in {
		if !drop[i] {
			kept = append(kept, x)
		}
		if i == 6 {
			kept = append(kept, in[0], in[4])
		}
	}
	got := feed(t, newReceiver(t, nil), kept, -1)
	if p := positions(got); p != "ABDEF"+"AB"+"F"+"BCDEF" {
		t.Errorf("positions %s", p)
	}
	if l := lostCounts(got); !mapsEqual(l, map[int]int{2: 1, 7: 3, 8: 1}) {
		t.Errorf("lost %v", l)
	}
	if end := got[len(got)-1]; end.Stats.Lost != 5 || end.Stats.Late != 1 || end.Stats.Duplicates != 1 {
		t.Errorf("stats %+v, want 5 lost, the repeated burst D late and the repeated header a duplicate", end.Stats)
	}
}

// TestReceiver_TransportPosition places bursts by Hint.VoiceBurst and
// checks that a burst without its SYNC where the sequence number predicts A
// is placed at B.
func TestReceiver_TransportPosition(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 2, nil)
	in := network(bursts, 0)
	for i := 1; i < len(bursts)-1; i++ {
		v, _ := enums.VoiceBurstAt((i - 1) % 6)
		in[i].hint.Kind, in[i].hint.VoiceBurst = layer2.KindVoice, v
	}
	got := feed(t, newReceiver(t, nil), in, -1)
	if p := positions(got); p != "ABCDEFABCDEF" {
		t.Errorf("transport positions %s", p)
	}

	in = network(bursts, 0)
	var b layer2.Burst
	if err := b.DecodeFromBytes(bursts[7]); err != nil || b.VoiceBurst != enums.VoiceBurstA {
		t.Fatalf("burst 7: %v %v", err, b.VoiceBurst)
	}
	b.SyncPattern, b.VoiceBurst = enums.SyncNone, enums.VoiceBurstUnknown
	b.EmbeddedSignalling.FEC.Uncorrectable = true
	in[7].decoded = &b
	got = feed(t, newReceiver(t, nil), in, -1)
	if p := positions(got); p != "ABCDEFBBCDEF" {
		t.Errorf("positions %s; a burst without its SYNC where the sequence number predicts A is B", p)
	}
}

// TestReceiver_WeakEnd sets the IPSC weak end flag on a burst in the middle
// of a call. The call goes on, and the next burst is placed by its own
// evidence only.
func TestReceiver_WeakEnd(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 3191868), 2, nil)
	in := network(bursts, 0)
	in[3].hint.End = layer2.EndWeak
	got := feed(t, newReceiver(t, nil), in, -1)
	if k := kinds(got); k != "S"+repeat("V", 12)+"E" {
		t.Fatalf("events %s", k)
	}
	if p := positions(got); p != "ABC?EFABCDEF" {
		t.Errorf("positions %s", p)
	}
}

// TestReceiver_Faults drops each burst of voice.bin in turn, and checks
// that the call still starts and ends once, that the lost burst is
// counted, and that every other burst keeps its position.
func TestReceiver_Faults(t *testing.T) {
	t.Parallel()

	bursts := loadBin(t, "voice.bin")
	base := feed(t, newReceiver(t, nil), timed(bursts, 0, layer2.Hint{}), -1)
	want := positions(base)
	for drop := 2; drop < len(bursts)-2; drop++ {
		in := timed(bursts, 0, layer2.Hint{})
		in = append(in[:drop:drop], in[drop+1:]...)
		got := feed(t, newReceiver(t, nil), in, -1)
		if len(find(got, voice.EventCallStart)) != 1 || len(find(got, voice.EventCallEnd)) != 1 {
			t.Errorf("drop %d: events %s", drop, kinds(got))
			continue
		}
		p := positions(got)
		w := want[:drop-1] + want[drop:]
		if p != w && !strings.Contains(p, "?") {
			t.Errorf("drop %d: positions\n%s, want\n%s", drop, p, w)
		}
		baseLost := find(base, voice.EventCallEnd)[0].Stats.Lost
		end := find(got, voice.EventCallEnd)[0]
		if end.Stats.Lost != baseLost+1 && (!strings.Contains(p, "?") || end.Stats.Lost != baseLost) {
			t.Errorf("drop %d: lost %d, base %d", drop, end.Stats.Lost, baseLost)
		}
	}
}

// TestReceiver_Fuzz feeds random bursts, hints and times, and checks that
// no call appends more than MaxEvents.
func TestReceiver_Fuzz(t *testing.T) {
	t.Parallel()

	src := loadBin(t, "voice.bin")
	src = append(src, transmission(t, groupLC(t, 91, 3191868), 2, nil)...)
	r := rand.New(rand.NewSource(1))
	rx := newReceiver(t, &voice.ReceiverConfig{Lost: 200 * time.Millisecond, Hang: 300 * time.Millisecond})
	out := make([]voice.Event, 0, voice.MaxEvents)
	var now time.Duration
	for range 20000 {
		now += time.Duration(r.Intn(100)) * time.Millisecond
		raw := src[r.Intn(len(src))]
		if r.Intn(4) == 0 {
			raw[r.Intn(33)] ^= byte(1 << r.Intn(8))
		}
		h := layer2.Hint{
			Seq: uint16(r.Intn(8)), HasSeq: r.Intn(2) == 0, Stream: uint32(r.Intn(3)), HasStream: r.Intn(3) == 0,
			End: layer2.EndHint(r.Intn(3)), SyncLost: r.Intn(20) == 0, VoiceBurst: enums.VoiceBurstType(r.Intn(7)),
		}
		switch r.Intn(10) {
		case 0:
			out = rx.Lost(now, out[:0])
		case 1:
			out = rx.Tick(now, out[:0])
		case 2:
			rx.Expect(now, voice.Call{Target: uint32(r.Intn(3))}, now+time.Duration(r.Intn(500))*time.Millisecond)
			continue
		default:
			out = rx.Burst(now, &raw, h, out[:0])
		}
		if len(out) > voice.MaxEvents {
			t.Fatalf("%d events: %s", len(out), kinds(out))
		}
	}
}
