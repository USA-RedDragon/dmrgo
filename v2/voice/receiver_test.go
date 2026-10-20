package voice_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestReceiver_Call(t *testing.T) {
	t.Parallel()

	rx := newReceiver(t, nil)
	in := timed(transmission(t, groupLC(t, 91, 3191868), 3, nil), 0, layer2.Hint{})
	got := feed(t, rx, in, -1)
	if k := kinds(got); k != "S"+repeat("V", 18)+"E" {
		t.Fatalf("events %s", k)
	}
	if p := positions(got); p != "ABCDEFABCDEFABCDEF" {
		t.Errorf("positions %s", p)
	}
	start, end := got[0], got[len(got)-1]
	c := start.Call
	if !c.Group || c.Target != 91 || c.Source != 3191868 || c.Origin != voice.OriginHeader || c.LC == [9]byte{} {
		t.Errorf("call %+v", c)
	}
	if end.Reason != voice.EndTerminator || end.Stats.Bursts != 18 || end.Stats.Lost != 0 || end.Stats.Duration != 19*slot ||
		end.Stats.BitsChecked == 0 {
		t.Errorf("end %v %+v", end.Reason, end.Stats)
	}
	for _, e := range got[1 : len(got)-1] {
		if e.Call != c || e.Lost != 0 || e.Frames[0].Bad || e.Burst == (layer2.RawBurst{}) {
			t.Fatalf("voice event %+v", e)
		}
	}
	if rx.State() != voice.StateHang {
		t.Fatalf("state %v, want Hang", rx.State())
	}
	at, ok := rx.Deadline()
	if !ok || at != 19*slot+3*time.Second {
		t.Fatalf("deadline %v %v", at, ok)
	}
	if ev := rx.Tick(at-time.Millisecond, nil); len(ev) != 0 {
		t.Fatalf("early tick: %s", kinds(ev))
	}
	ev := rx.Tick(at, nil)
	if kinds(ev) != "e" || ev[0].Call.Target != 91 || rx.State() != voice.StateIdle {
		t.Fatalf("hang end: %s, state %v", kinds(ev), rx.State())
	}
	if _, ok := rx.Deadline(); ok {
		t.Error("deadline in Idle")
	}
}

func repeat(s string, n int) string {
	out := ""
	for range n {
		out += s
	}
	return out
}

func TestReceiver_LateEntry(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 9, 3191868), 3, nil)
	rx := newReceiver(t, nil)
	got := feed(t, rx, timed(bursts[4:], 0, layer2.Hint{}), -1)
	if k := kinds(got); k != repeat("V", 7)+"SV"+repeat("V", 7)+"E" {
		t.Fatalf("events %s", k)
	}
	if p := positions(got); p != "?EFABCDEFABCDEF" {
		t.Errorf("positions %s", p)
	}
	if got[0].Call.Origin != voice.OriginUnknown || got[7].Call.Origin != voice.OriginEmbedded || got[7].Call.Target != 9 || got[8].Call != got[7].Call {
		t.Errorf("origins %v %+v", got[0].Call.Origin, got[7].Call)
	}
	if end := got[len(got)-1]; end.Stats.Bursts != 15 {
		t.Errorf("stats %+v", end.Stats)
	}
}

func TestReceiver_Golden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, kinds, positions string
		origin                 voice.Origin
	}{
		{"voice", "SVVVVVVE", "ABCDEF", voice.OriginHeader},
		{"voice-late", "VVVVSVVVVVVVVE", "ABCDEFABCDEF", voice.OriginEmbedded},
	}
	for _, tt := range tests {
		rx := newReceiver(t, nil)
		got := feed(t, rx, timed(loadGolden(t, tt.name), 0, layer2.Hint{}), -1)
		if kinds(got) != tt.kinds || positions(got) != tt.positions {
			t.Errorf("%s: events %s, positions %s", tt.name, kinds(got), positions(got))
			continue
		}
		c := find(got, voice.EventCallStart)[0].Call
		if c.Origin != tt.origin || c.Source != 3191868 || !c.Group {
			t.Errorf("%s: call %+v", tt.name, c)
		}
		if end := find(got, voice.EventCallEnd)[0]; end.Reason != voice.EndTerminator {
			t.Errorf("%s: end %v", tt.name, end.Reason)
		}
	}
}

func TestReceiver_Captures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		file, positions string
		lost            map[int]int
	}{
		{"voice.bin", repeat("ABCDEF", 8) + "BCDEF" + repeat("ABCDEF", 4), map[int]int{48: 1}},
		{"parrot_kerchunk.bin", "ABCDEFAABCDEF", map[int]int{}},
	}
	for _, tt := range tests {
		rx := newReceiver(t, nil)
		got := feed(t, rx, timed(loadBin(t, tt.file), 0, layer2.Hint{}), -1)
		starts, ends := find(got, voice.EventCallStart), find(got, voice.EventCallEnd)
		if len(starts) != 1 || len(ends) != 1 || starts[0].Call.Origin != voice.OriginHeader || ends[0].Reason != voice.EndTerminator {
			t.Errorf("%s: events %s", tt.file, kinds(got))
			continue
		}
		if p := positions(got); p != tt.positions {
			t.Errorf("%s: positions %s, want %s", tt.file, p, tt.positions)
		}
		if l := lostCounts(got); !mapsEqual(l, tt.lost) {
			t.Errorf("%s: lost %v, want %v", tt.file, l, tt.lost)
		}
	}
}

func mapsEqual(a, b map[int]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestReceiver_WeakCapture(t *testing.T) {
	t.Parallel()

	rx := newReceiver(t, nil)
	got := feed(t, rx, loadWeak(t), -1)
	if k := kinds(got); k != "SVVVVVVE" {
		t.Fatalf("events %s", k)
	}
	if p := positions(got); p != "ABCDEF" {
		t.Errorf("positions %s", p)
	}
	c, end := got[0].Call, got[len(got)-1]
	if c.Target != 9 || c.Source != 3191868 || !c.Group {
		t.Errorf("call %+v", c)
	}
	if end.Reason != voice.EndTerminator || end.Stats.Duplicates != 2 || end.Stats.Bursts != 6 || end.Stats.Lost != 0 {
		t.Errorf("end %v %+v", end.Reason, end.Stats)
	}
}

func TestReceiver_ConfigError(t *testing.T) {
	t.Parallel()

	cc := enums.ColorCode(16)
	_, err := voice.NewReceiver(&voice.ReceiverConfig{ColorCode: &cc})
	var ce *voice.ConfigError
	if !errors.As(err, &ce) || ce.Field != "ColorCode" || ce.Error() == "" {
		t.Fatalf("err %v", err)
	}
}
