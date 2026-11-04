package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

var nullParties = []struct {
	name           string
	target, source uint32
}{
	{"both", 0, 0},
	{"target", 0, 1002},
	{"source", 92, 0},
}

func TestReceiver_NullAddressNoSupersede(t *testing.T) {
	t.Parallel()

	a := transmission(t, groupLC(t, 91, 1001), 1, nil)
	for _, p := range nullParties {
		b := transmission(t, groupLC(t, p.target, p.source), 1, nil)
		for _, header := range []bool{true, false} {
			from := 1
			if header {
				from = 0
			}
			in := append(timed(a[:4], 0, layer2.Hint{}), timed(b[from:], 4*slot, layer2.Hint{})...)
			got := feed(t, newReceiver(t, nil), in, -1)
			starts, ends := find(got, voice.EventCallStart), find(got, voice.EventCallEnd)
			if len(starts) != 1 || starts[0].Call.Target != 91 || starts[0].Call.Source != 1001 {
				t.Errorf("%s header=%v: events %s, starts %+v", p.name, header, kinds(got), starts)
				continue
			}
			if len(ends) != 1 || ends[0].Reason != voice.EndTerminator || ends[0].Call.Target != 91 {
				t.Errorf("%s header=%v: events %s, ends %+v", p.name, header, kinds(got), ends)
			}
			if n := len(find(got, voice.EventVoice)); n != 3+layer2.VoiceBurstsPerSuperframe {
				t.Errorf("%s header=%v: %d voice events, want %d", p.name, header, n, 3+layer2.VoiceBurstsPerSuperframe)
			}
			if e := find(got, voice.EventEmbedded); len(e) != 0 {
				t.Errorf("%s header=%v: embedded events %+v", p.name, header, e)
			}
		}
	}
}

func TestReceiver_NullAddressNoCall(t *testing.T) {
	t.Parallel()

	for _, p := range nullParties {
		b := transmission(t, groupLC(t, p.target, p.source), 2, nil)
		for _, from := range []int{0, 1} {
			rx := newReceiver(t, nil)
			got := feed(t, rx, timed(b[from:], 0, layer2.Hint{}), -1)
			for _, e := range got {
				if e.Kind != voice.EventVoice || e.Call.Origin != voice.OriginUnknown {
					t.Errorf("%s from %d: events %s", p.name, from, kinds(got))
					break
				}
			}
			if rx.State() == voice.StateInCall || rx.State() == voice.StateHang {
				t.Errorf("%s from %d: state %v", p.name, from, rx.State())
			}
		}
	}
}

func TestReceiver_NullAddressLateEntryWaits(t *testing.T) {
	t.Parallel()

	null := transmission(t, groupLC(t, 0, 0), 1, nil)
	live := transmission(t, groupLC(t, 91, 1001), 1, nil)
	in := timed(null[1:1+layer2.VoiceBurstsPerSuperframe], 0, layer2.Hint{})
	in = append(in, timed(live[1:], slot*layer2.VoiceBurstsPerSuperframe, layer2.Hint{})...)
	got := feed(t, newReceiver(t, nil), in, -1)
	starts := find(got, voice.EventCallStart)
	if len(starts) != 1 || starts[0].Call.Target != 91 || starts[0].Call.Source != 1001 || starts[0].Call.Origin != voice.OriginEmbedded {
		t.Fatalf("events %s, starts %+v", kinds(got), starts)
	}
	for _, e := range got {
		if e.Kind == voice.EventEmbedded && e.LC.FLCO == enums.FLCOGroupVoiceChannelUser {
			t.Errorf("embedded voice LC event %+v", e.LC)
		}
	}
}

func TestReceiver_EdgeAddressesStartCalls(t *testing.T) {
	t.Parallel()

	for _, p := range []struct{ target, source uint32 }{{1, 1}, {0xFFFFFF, 0xFFFFFF}, {0xFFFCE0, 0xFFFEE0}} {
		b := transmission(t, groupLC(t, p.target, p.source), 1, nil)
		for _, from := range []int{0, 1} {
			starts := find(feed(t, newReceiver(t, nil), timed(b[from:], 0, layer2.Hint{}), -1), voice.EventCallStart)
			if len(starts) != 1 || starts[0].Call.Target != p.target || starts[0].Call.Source != p.source {
				t.Errorf("%06X from %06X, from %d: starts %+v", p.target, p.source, from, starts)
			}
		}
	}
}

func TestReceiver_ExpectNullAddress(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 1001), 1, nil)
	for _, p := range nullParties {
		rx := newReceiver(t, nil)
		rx.Expect(0, voice.Call{Group: true, Target: p.target, Source: p.source}, time.Second)
		got := feed(t, rx, timed(bursts[1:3], 0, layer2.Hint{}), -1)
		if k := kinds(got); k != "VV" || got[0].Call.Origin != voice.OriginUnknown {
			t.Errorf("%s: events %s, call %+v", p.name, k, got[0].Call)
		}

		rx = newReceiver(t, nil)
		rx.Expect(0, voice.Call{Group: true, Target: 91, Source: 1001}, time.Second)
		rx.Expect(0, voice.Call{Group: true, Target: p.target, Source: p.source}, time.Second)
		got = feed(t, rx, timed(bursts[1:3], 0, layer2.Hint{}), -1)
		if len(got) == 0 || got[0].Kind != voice.EventCallStart || got[0].Call.Origin != voice.OriginExpected || got[0].Call.Target != 91 {
			t.Errorf("%s after a valid Expect: events %s", p.name, kinds(got))
		}
	}
}
