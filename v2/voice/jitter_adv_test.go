package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

type advJ struct {
	t   *testing.T
	j   *voice.Jitter
	now time.Duration
	out []jout
	src []uint32
}

func (a *advJ) push(seq uint16, src uint32, term bool, id int) voice.JitterResult {
	var raw layer2.RawBurst
	raw[0], raw[1] = byte(id>>8), byte(id)
	h := layer2.Hint{Seq: seq, HasSeq: true, SeqBits: 8, At: a.now}
	if term {
		h.Kind, h.DataType, h.HasDataType = layer2.KindData, enums.DataTypeTerminatorWithLC, true
	}
	return a.j.Push(a.now, &raw, h, src)
}

func (a *advJ) drain() {
	var p voice.JitterPacket
	for {
		at, ok := a.j.Deadline()
		if !ok || at > a.now {
			return
		}
		if !a.j.Pop(a.now, &p) {
			a.t.Fatalf("Pop failed at %v deadline %v", a.now, at)
		}
		a.out = append(a.out, jout{id: idOf(&p.Burst), due: p.Due})
		a.src = append(a.src, p.Source)
	}
}

func (a *advJ) call(src uint32, n int, id0 int) []voice.JitterResult {
	rs := make([]voice.JitterResult, 0, n)
	for i := range n {
		rs = append(rs, a.push(uint16(i), src, i == n-1, id0+i))
		a.drain()
		a.now += burst
	}
	for range 20 {
		a.now += burst
		a.drain()
	}
	return rs
}

func TestJitterAdvRestartAfterOtherCall(t *testing.T) {
	t.Parallel()
	a := &advJ{t: t, j: newJitter(t, nil), now: time.Second}
	a.call(9, 20, 0)
	a.call(5, 10, 100)
	rs := a.call(9, 20, 200)
	bad := 0
	for i, r := range rs {
		if r != voice.JitterBuffered {
			bad++
			if bad == 1 {
				t.Errorf("third call packet %d: %v", i, r)
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d of 20 packets of the restarted call dropped; played %d of 50; stats %+v", bad, len(a.out), a.j.Stats())
	}
}

func TestJitterAdvDupTerminatorAfterEnd(t *testing.T) {
	t.Parallel()
	a := &advJ{t: t, j: newJitter(t, nil), now: time.Second}
	a.call(9, 20, 0)
	n := len(a.out)
	r := a.push(19, 9, true, 19)
	for range 20 {
		a.now += burst
		a.drain()
	}
	if r != voice.JitterDuplicate || len(a.out) != n {
		t.Errorf("copy of the played terminator: Push %v, played again %d times", r, len(a.out)-n)
	}
	n = len(a.out)
	r = a.push(15, 9, false, 15)
	for range 20 {
		a.now += burst
		a.drain()
	}
	if r != voice.JitterDuplicate || len(a.out) != n {
		t.Errorf("copy of played seq 15 after end: Push %v, played again %d times", r, len(a.out)-n)
	}
}

func TestJitterAdvPreemptLiveStream(t *testing.T) {
	t.Parallel()
	in := make([]jin, 0, 20)
	for i := range 10 {
		at := time.Second + time.Duration(i)*burst
		if i >= 8 {
			at += voice.DefaultJitter - 20*time.Millisecond
		}
		in = append(in, jin{at: at, seq: uint16(i), src: 1, id: i})
	}
	bStart := time.Second + 7*burst + voice.DefaultJitter + 10*time.Millisecond
	for i := range 10 {
		in = append(in, jin{at: bStart + time.Duration(i)*burst, seq: uint16(100 + i), src: 2, id: 100 + i})
	}
	j := newJitter(t, nil)
	out, res := pump(t, j, in, 8, time.Millisecond)
	ids := make([]int, 0, len(out))
	for _, o := range out {
		ids = append(ids, o.id)
	}
	t.Logf("results %v, played %v", res, ids)
	if len(ids) != 20 || ids[8] != 8 || ids[9] != 9 || ids[10] != 100 {
		t.Errorf("A's seq 8 and 9 came within their Delay, but B preempted A: order %v", ids)
	}
}

func TestJitterAdvStaleFarBehind(t *testing.T) {
	t.Parallel()
	for _, waiting := range []bool{false, true} {
		j := newJitter(t, nil)
		var p voice.JitterPacket
		now := time.Second
		var ids []int
		push := func(seq uint16, id int) voice.JitterResult {
			var raw layer2.RawBurst
			raw[0], raw[1] = byte(id>>8), byte(id)
			return j.Push(now, &raw, layer2.Hint{Seq: seq, HasSeq: true, At: now}, 1)
		}
		drain := func() {
			for {
				at, ok := j.Deadline()
				if !ok || at > now || !j.Pop(now, &p) {
					return
				}
				ids = append(ids, idOf(&p.Burst))
			}
		}
		res := map[voice.JitterResult]int{}
		for i := range 200 {
			if !waiting && i == 121 {
				now += voice.DefaultJitter
				drain()
			}
			res[push(uint16(1000+i), i)]++
			if i == 120 {
				if !waiting {
					now += voice.DefaultJitter
					drain()
				}
				r := push(1000, 0)
				t.Logf("waiting=%v: copy of seq 1000 (played, 120 behind) -> %v", waiting, r)
			}
			drain()
			now += burst
		}
		for range 30 {
			now += burst
			drain()
		}
		seen := map[int]int{}
		for _, id := range ids {
			seen[id]++
		}
		t.Logf("waiting=%v: %d played of 200, packet 0 played %d times, results %v, %+v", waiting, len(ids), seen[0], res, j.Stats())
		if seen[0] != 1 || len(ids) < 199 {
			t.Errorf("waiting=%v: one stale copy: packet 0 played %d times, %d of 200 played", waiting, seen[0], len(ids))
		}
	}
}

func TestJitterAdvStickyEnded(t *testing.T) {
	t.Parallel()
	a := &advJ{t: t, j: newJitter(t, &voice.JitterConfig{Delay: 120 * time.Millisecond}), now: time.Second}
	for i := range 30 {
		a.push(uint16(i), 4, i == 9, i)
		a.drain()
		a.now += burst
		a.drain()
		if i == 20 {
			for range 4 {
				a.now += burst
				a.drain()
			}
			if a.j.Len() != 0 {
				t.Fatalf("len %d", a.j.Len())
			}
			r := a.push(18, 4, false, 18)
			t.Logf("copy of played seq 18 in the second call -> %v", r)
			for range 4 {
				a.now += burst
				a.drain()
			}
		}
	}
	for range 10 {
		a.now += burst
		a.drain()
	}
	t.Logf("out %v stats %+v", a.out, a.j.Stats())
	seen := map[int]int{}
	for _, o := range a.out {
		seen[o.id]++
	}
	if seen[18] != 1 {
		t.Errorf("seq 18 played %d times; terminator at seq 9 left the ring ended for the rest of the stream", seen[18])
	}
}
