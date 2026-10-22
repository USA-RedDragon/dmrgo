package bs_test

import (
	"sort"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

type advPkt struct {
	at   time.Duration
	raw  layer2.RawBurst
	h    layer2.Hint
	peer uint32
}

func advNetCall(t *testing.T, superframes int, seq0 uint16, stream uint32, peer uint32, start time.Duration, src uint32) []advPkt {
	t.Helper()
	slot := enums.Slot1
	in := call(t, 1, 9, src, superframes, slot, bs.Source{Peer: peer}, start)
	out := make([]advPkt, len(in))
	for i := range in {
		out[i] = advPkt{at: in[i].at, raw: in[i].raw, peer: peer, h: layer2.Hint{Slot: slot, Seq: (seq0 + uint16(i)) & 0xFF, HasSeq: true, SeqBits: 8, Stream: stream, HasStream: stream != 0}}
	}
	return out
}

func advSlots(in []advPkt) [][9]byte {
	var s [][9]byte
	b := layer2.NewBurst()
	for _, p := range in {
		if b.DecodeFromBytes(p.raw) == nil && b.Kind == layer2.KindVoice {
			v := p.raw.VoiceSlots()
			s = append(s, v[:]...)
		}
	}
	return s
}

func advRun(t *testing.T, r *bs.Repeater, in []advPkt, end time.Duration, tick func(now time.Duration) time.Duration) []bs.Output {
	t.Helper()
	sort.SliceStable(in, func(a, b int) bool { return in[a].at < in[b].at })
	out := make([]bs.Output, 0, bs.MaxOutputs)
	var got []bs.Output
	keep := func() {
		if cap(out) != bs.MaxOutputs {
			t.Fatalf("outputs grew to %d", cap(out))
		}
		got = append(got, out...)
	}
	now := time.Duration(0)
	for now <= end {
		for len(in) > 0 && in[0].at <= now {
			h := in[0].h
			h.At = in[0].at
			out = r.Inbound(now, &in[0].raw, h, bs.Source{Peer: in[0].peer}, out[:0])
			keep()
			in = in[1:]
		}
		for n := 0; ; n++ {
			if n > 1000 {
				t.Fatalf("Tick loop at %v", now)
			}
			at, ok := r.Deadline()
			if !ok || at > now {
				break
			}
			out = r.Tick(now, out[:0])
			keep()
		}
		now = tick(now)
	}
	return got
}

func TestJitterAdvRestartSeqAfterOtherPeer(t *testing.T) {
	t.Parallel()
	a := advNetCall(t, 3, 0, 0, 1, time.Second, 3191868)
	b := advNetCall(t, 1, 0, 0, 2, a[len(a)-1].at+3*time.Second, 3191869)
	c := advNetCall(t, 3, 0, 0, 1, b[len(b)-1].at+3*time.Second, 3191870)
	wc := advSlots(c)
	r := newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1})
	in := append(append(a, b...), c...)
	out := advRun(t, r, in, 20*time.Second, func(now time.Duration) time.Duration { return now + time.Millisecond })
	var cur uint32
	var cSlots int
	starts := map[uint32]int{}
	for _, o := range out {
		if o.Slot != enums.Slot1 {
			continue
		}
		switch o.Kind {
		case bs.OutputCallStart:
			cur = o.Call.Source
			starts[cur]++
		case bs.OutputBurst:
			if cur == 3191870 && o.Hint.Kind == layer2.KindVoice {
				cSlots += 3
			}
		case bs.OutputCallEnd, bs.OutputRejected, bs.OutputKey, bs.OutputDekey:
		}
	}
	t.Logf("starts %v, third call relayed %d of %d voice slots, jitter %+v", starts, cSlots, len(wc), r.JitterStats(enums.Slot1))
	if starts[3191870] != 1 || cSlots != len(wc) {
		t.Errorf("third call (peer 1 again, sequence restarted at 0, no stream ID as Hytera) lost: %d of %d slots", cSlots, len(wc))
	}
}

func TestJitterAdvBackToBackCalls(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		mode  bs.Mode
		tight bool
	}{{bs.ModeNetwork, false}, {bs.ModeRF, false}, {bs.ModeNetwork, true}, {bs.ModeRF, true}} {
		mode := tc.mode
		a := advNetCall(t, 3, 10, 11, 1, time.Second, 3191868)
		last := a[len(a)-1].at
		bAt := last + 30*time.Millisecond
		if tc.tight {
			bAt = a[len(a)-3].at + voice.DefaultJitter + 10*time.Millisecond
		}
		b := advNetCall(t, 3, 200, 22, 2, bAt, 3191869)
		wa, wb := advSlots(a), advSlots(b)
		for i := len(a) - 2; i < len(a); i++ {
			if tc.tight {
				a[i].at += voice.DefaultJitter - 20*time.Millisecond
			} else {
				a[i].at += 150 * time.Millisecond
			}
		}
		r := newRepeater(t, &bs.Config{Mode: mode, ColorCode: 1, CACH: true})
		out := advRun(t, r, append(a, b...), 10*time.Second, func(now time.Duration) time.Duration { return now + time.Millisecond })
		var order []string
		var aSlots, bSlots [][9]byte
		var cur uint32
		for _, o := range out {
			if o.Slot != enums.Slot1 {
				continue
			}
			switch o.Kind {
			case bs.OutputCallStart:
				order = append(order, "start")
				cur = o.Call.Source
			case bs.OutputCallEnd:
				order = append(order, "end:"+o.End.String())
			case bs.OutputRejected:
				order = append(order, "reject:"+o.Reason.String())
			case bs.OutputKey, bs.OutputDekey:
			case bs.OutputBurst:
				if o.Hint.Kind == layer2.KindVoice {
					v := o.Burst.VoiceSlots()
					if cur == 3191868 {
						aSlots = append(aSlots, v[:]...)
					} else {
						bSlots = append(bSlots, v[:]...)
					}
				}
			}
		}
		t.Logf("%v tight=%v: events %v; A %d/%d slots, B %d/%d slots; jitter %+v", mode, tc.tight, order, len(aSlots), len(wa), len(bSlots), len(wb), r.JitterStats(enums.Slot1))
		if len(order) != 4 || order[1] != "end:"+voice.EndTerminator.String() || order[3] != "end:"+voice.EndTerminator.String() {
			t.Errorf("%v tight=%v: want start, end by terminator, start, end by terminator; got %v", mode, tc.tight, order)
		}
	}
}
