package voice_test

import (
	"errors"
	"math/rand/v2"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

const burst = 60 * time.Millisecond

type jin struct {
	at  time.Duration
	seq uint16
	src uint32
	str uint32
	id  int
}

type jout struct {
	id  int
	due time.Duration
	at  time.Duration
}

func newJitter(t *testing.T, cfg *voice.JitterConfig) *voice.Jitter {
	t.Helper()
	j, err := voice.NewJitter(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func stream(n int, start time.Duration, seq0 uint16, bits uint8, str uint32, id0 int) []jin {
	in := make([]jin, n)
	for i := range in {
		s := seq0 + uint16(i)
		if bits == 8 {
			s &= 0xFF
		}
		in[i] = jin{at: start + time.Duration(i)*burst, seq: s, str: str, id: id0 + i}
	}
	return in
}

func idOf(raw *layer2.RawBurst) int { return int(raw[0])<<8 | int(raw[1]) }

func pump(t *testing.T, j *voice.Jitter, in []jin, bits uint8, step time.Duration) ([]jout, map[voice.JitterResult]int) {
	t.Helper()
	sort.SliceStable(in, func(a, b int) bool { return in[a].at < in[b].at })
	res := map[voice.JitterResult]int{}
	var out []jout
	var p voice.JitterPacket
	end := in[len(in)-1].at + 3*time.Second
	for now := time.Duration(0); now <= end; now += step {
		for len(in) > 0 && in[0].at <= now {
			var raw layer2.RawBurst
			raw[0], raw[1] = byte(in[0].id>>8), byte(in[0].id)
			h := layer2.Hint{Slot: enums.Slot1, Seq: in[0].seq, HasSeq: true, SeqBits: bits, At: in[0].at, Stream: in[0].str, HasStream: in[0].str != 0}
			res[j.Push(now, &raw, h, in[0].src)]++
			in = in[1:]
		}
		for {
			at, ok := j.Deadline()
			if !ok || at > now {
				break
			}
			if !j.Pop(now, &p) {
				t.Fatalf("Pop at %v failed with Deadline %v", now, at)
			}
			if p.Hint.At != p.Due || !p.HasBurst {
				t.Fatalf("At %v Due %v HasBurst %v", p.Hint.At, p.Due, p.HasBurst)
			}
			out = append(out, jout{id: idOf(&p.Burst), due: p.Due, at: now})
		}
	}
	return out, res
}

func checkOrder(t *testing.T, out []jout, want int) {
	t.Helper()
	if len(out) != want {
		t.Fatalf("%d packets out, want %d", len(out), want)
	}
	for i := range out {
		if out[i].id != i {
			t.Fatalf("packet %d is %d", i, out[i].id)
		}
		if i > 0 && out[i].due-out[i-1].due != burst {
			t.Fatalf("packet %d due %v after %v", i, out[i].due, out[i-1].due)
		}
	}
}

func TestJitterInOrderNoDelayAdded(t *testing.T) {
	t.Parallel()
	for _, bits := range []uint8{8, 16} {
		for _, seq0 := range []uint16{0, 250, 65530} {
			in := stream(400, time.Second, seq0, bits, 1, 0)
			r := rand.New(rand.NewPCG(uint64(seq0), uint64(bits)))
			for i := range in {
				in[i].at += time.Duration(r.Int64N(int64(59 * time.Millisecond)))
			}
			first := in[0].at
			j := newJitter(t, nil)
			out, res := pump(t, j, in, bits, time.Millisecond)
			checkOrder(t, out, 400)
			if out[0].due > first+voice.DefaultJitter || out[0].due < time.Second+voice.DefaultJitter || res[voice.JitterBuffered] != 400 || j.Stats() != (voice.JitterStats{}) {
				t.Fatalf("bits %d seq %d: first due %v, arrival %v; %v %+v", bits, seq0, out[0].due, first, res, j.Stats())
			}
		}
	}
}

func TestJitterReorderAndClumps(t *testing.T) {
	t.Parallel()
	for name, f := range map[string]func(r *rand.Rand, in []jin){
		"random": func(r *rand.Rand, in []jin) {
			for i := range in {
				in[i].at += time.Duration(r.Int64N(int64(voice.DefaultJitter)))
			}
		},
		"clump6": func(_ *rand.Rand, in []jin) {
			at := make([]time.Duration, len(in))
			for i := range in {
				at[i] = in[i].at
			}
			for i := range in {
				in[i].at = at[min(i/6*6+5, len(in)-1)]
			}
		},
		"reverse6": func(_ *rand.Rand, in []jin) {
			for i := 0; i+5 < len(in); i += 6 {
				for k := range 6 {
					in[i+k].at = in[i+5].at + time.Duration(5-k)*time.Millisecond
				}
			}
		},
	} {
		for _, bits := range []uint8{8, 0} {
			in := stream(300, 0, 200, bits, 7, 0)
			f(rand.New(rand.NewPCG(9, 9)), in)
			out, res := pump(t, newJitter(t, nil), in, bits, 5*time.Millisecond)
			if len(out) != 300 || res[voice.JitterBuffered] != 300 {
				t.Fatalf("%s bits %d: %d out, %v", name, bits, len(out), res)
			}
			checkOrder(t, out, 300)
		}
	}
}

func TestJitterSharedCounter(t *testing.T) {
	t.Parallel()
	in := stream(200, time.Second, 0, 8, 1, 0)
	r := rand.New(rand.NewPCG(5, 6))
	for i := range in {
		in[i].seq = uint16(2*i) & 0xFF
		in[i].at += time.Duration(r.Int64N(int64(59 * time.Millisecond)))
	}
	arr := map[int]time.Duration{}
	for _, p := range in {
		arr[p.id] = p.at
	}
	j := newJitter(t, nil)
	out, res := pump(t, j, in, 8, time.Millisecond)
	if len(out) != 200 || res[voice.JitterBuffered] != 200 || j.Stats() != (voice.JitterStats{}) {
		t.Fatalf("%d out, %v %+v", len(out), res, j.Stats())
	}
	for i, o := range out {
		if o.id != i || i > 10 && o.due-out[i-1].due != burst {
			t.Fatalf("packet %d is %d, due %v after %v", i, o.id, o.due, out[i-1].due)
		}
		if d := o.due - arr[o.id]; d > voice.DefaultJitter+2*burst {
			t.Fatalf("packet %d plays %v after it arrived", o.id, d)
		}
	}
}

func TestJitterLateAndDuplicate(t *testing.T) {
	t.Parallel()
	in := stream(50, 0, 0, 8, 1, 0)
	in[10].at += voice.DefaultJitter + 30*time.Millisecond
	in = append(in, in[5], in[20])
	in[len(in)-2].at = in[5].at + 100*time.Millisecond
	in[len(in)-1].at = in[20].at + 5*time.Millisecond
	j := newJitter(t, nil)
	out, res := pump(t, j, in, 8, 5*time.Millisecond)
	if len(out) != 49 || res[voice.JitterLate] != 1 || res[voice.JitterDuplicate] != 2 {
		t.Fatalf("%d out, %v", len(out), res)
	}
	if s := j.Stats(); s.Late != 1 || s.Duplicates != 2 {
		t.Fatalf("stats %+v", s)
	}
	for i, o := range out {
		want := i
		if i >= 10 {
			want++
		}
		if o.id != want {
			t.Fatalf("packet %d is %d, want %d", i, o.id, want)
		}
	}
	if out[10].due-out[9].due != 2*burst {
		t.Fatalf("lost packet not skipped in time: %v", out[10].due-out[9].due)
	}
}

func TestJitterHeaderAfterVoice(t *testing.T) {
	t.Parallel()
	in := stream(20, time.Second, 100, 8, 1, 0)
	in[0].at += 200 * time.Millisecond
	in[1].at += 100 * time.Millisecond
	out, res := pump(t, newJitter(t, nil), in, 8, 5*time.Millisecond)
	checkOrder(t, out, 20)
	if res[voice.JitterBuffered] != 20 || out[0].due != time.Second+voice.DefaultJitter {
		t.Fatalf("%v, first due %v", res, out[0].due)
	}
}

func TestJitterStreamChange(t *testing.T) {
	t.Parallel()
	a := stream(30, 0, 10, 8, 1, 0)
	b := stream(30, a[29].at+20*time.Millisecond, 200, 8, 2, 30)
	r := rand.New(rand.NewPCG(4, 4))
	in := slices.Concat(a, b)
	for i := range in {
		in[i].at += time.Duration(r.Int64N(int64(200 * time.Millisecond)))
	}
	out, res := pump(t, newJitter(t, nil), in, 8, 5*time.Millisecond)
	if res[voice.JitterBuffered] != 60 {
		t.Fatalf("%v", res)
	}
	for i := range out {
		if out[i].id != i {
			t.Fatalf("packet %d is %d", i, out[i].id)
		}
		if i > 0 && out[i].due-out[i-1].due < burst {
			t.Fatalf("packet %d due %v after %v", i, out[i].due, out[i-1].due)
		}
	}
}

func TestJitterSources(t *testing.T) {
	t.Parallel()
	a := stream(20, 0, 5, 8, 0, 0)
	b := stream(20, 30*time.Millisecond, 5, 8, 0, 100)
	c := stream(20, 40*time.Millisecond, 5, 8, 0, 200)
	for i := range b {
		b[i].src, c[i].src = 2, 3
	}
	j := newJitter(t, nil)
	out, res := pump(t, j, slices.Concat(a, b, c), 8, 5*time.Millisecond)
	if res[voice.JitterBuffered] != 40 || res[voice.JitterBypass] != 20 || len(out) != 40 {
		t.Fatalf("%v, %d out", res, len(out))
	}
	for i := range out {
		if want := i%20 + i/20*100; out[i].id != want {
			t.Fatalf("packet %d is %d, want %d", i, out[i].id, want)
		}
	}
}

func TestJitterStallResync(t *testing.T) {
	t.Parallel()
	in := stream(60, 0, 0, 8, 1, 0)
	for i := 30; i < 60; i++ {
		in[i].at += 700 * time.Millisecond
	}
	resumed := in[30].at
	j := newJitter(t, nil)
	out, _ := pump(t, j, in, 8, time.Millisecond)
	if len(out) != 60 || j.Stats().Resyncs != 1 {
		t.Fatalf("%d out, %+v", len(out), j.Stats())
	}
	if d := out[30].due - (resumed + voice.DefaultJitter); d < 0 || d >= burst || (out[30].due-out[29].due)%burst != 0 {
		t.Fatalf("resumed due %v, arrival %v, last %v", out[30].due, resumed, out[29].due)
	}
	for i := 30; i < 60; i++ {
		if out[i].id != i || out[i].due-out[i-1].due != burst && i > 30 {
			t.Fatalf("packet %d is %d at %v", i, out[i].id, out[i].due)
		}
	}
}

func TestJitterClockReset(t *testing.T) {
	t.Parallel()
	j := newJitter(t, nil)
	var p voice.JitterPacket
	var raw layer2.RawBurst
	push := func(now time.Duration, seq uint16) {
		if r := j.Push(now, &raw, layer2.Hint{Seq: seq, HasSeq: true, SeqBits: 8}, 0); r != voice.JitterBuffered {
			t.Fatalf("seq %d: %v", seq, r)
		}
	}
	push(time.Hour, 1)
	push(time.Hour+burst, 2)
	push(5*time.Second, 3)
	at, ok := j.Deadline()
	if !ok || at != 5*time.Second-2*burst+voice.DefaultJitter {
		t.Fatalf("deadline %v %v", at, ok)
	}
	for _, want := range []time.Duration{0, burst, 2 * burst} {
		at, _ := j.Deadline()
		if !j.Pop(at, &p) || p.Due != at || at-(5*time.Second-2*burst+voice.DefaultJitter) != want {
			t.Fatalf("pop at %v due %v", at, p.Due)
		}
	}
	push(10*time.Hour, 4)
	at, _ = j.Deadline()
	if !j.Pop(at+time.Hour, &p) || p.Due != at+time.Hour {
		t.Fatalf("forward jump: due %v", p.Due)
	}
}

func TestJitterSeqJump(t *testing.T) {
	t.Parallel()
	a := stream(20, 0, 10, 16, 1, 0)
	b := stream(20, a[19].at+burst, 20000, 16, 1, 20)
	out, res := pump(t, newJitter(t, nil), slices.Concat(a, b), 16, 5*time.Millisecond)
	if len(out) != 40 || res[voice.JitterBuffered] != 40 {
		t.Fatalf("%d out, %v", len(out), res)
	}
	for i := range out {
		if out[i].id != i {
			t.Fatalf("packet %d is %d", i, out[i].id)
		}
	}
}

func TestJitterBigClump(t *testing.T) {
	t.Parallel()
	j := newJitter(t, &voice.JitterConfig{Delay: 60 * time.Millisecond})
	var raw layer2.RawBurst
	for i := range 100 {
		if r := j.Push(0, &raw, layer2.Hint{Seq: uint16(i), HasSeq: true}, 0); r != voice.JitterBuffered && r != voice.JitterOverflow {
			t.Fatalf("packet %d: %v", i, r)
		}
	}
	var p voice.JitterPacket
	played := 0
	for {
		at, ok := j.Deadline()
		if !ok || !j.Pop(at, &p) {
			break
		}
		played++
	}
	if s := j.Stats(); played != 2 || j.Len() != 0 || s.Late != 98 || s.Overflows != 0 {
		t.Fatalf("%d played, %d buffered, %+v", played, j.Len(), s)
	}
}

func TestJitterEarlyCatchUp(t *testing.T) {
	t.Parallel()
	in := stream(50, 0, 0, 16, 1, 0)
	for i := 10; i < 50; i++ {
		in[i].at = 600 * time.Millisecond
	}
	j := newJitter(t, nil)
	out, res := pump(t, j, in, 16, 5*time.Millisecond)
	if res[voice.JitterOverflow] != 0 || j.Stats().Resyncs == 0 {
		t.Fatalf("%v %+v", res, j.Stats())
	}
	for i := 1; i < len(out); i++ {
		if out[i].id <= out[i-1].id || out[i].due-out[i-1].due < burst {
			t.Fatalf("packet %d is %d at %v after %d at %v", i, out[i].id, out[i].due, out[i-1].id, out[i-1].due)
		}
	}
	if last := out[len(out)-1]; last.id != 49 || last.due > 600*time.Millisecond+voice.DefaultJitter+26*burst {
		t.Fatalf("last %d due %v", last.id, last.due)
	}
}

func TestJitterOffAndZero(t *testing.T) {
	t.Parallel()
	var raw layer2.RawBurst
	h := layer2.Hint{Seq: 1, HasSeq: true}
	var p voice.JitterPacket
	var zero voice.Jitter
	var nilJ *voice.Jitter
	off := newJitter(t, &voice.JitterConfig{Delay: -1})
	on := newJitter(t, nil)
	for _, j := range []*voice.Jitter{&zero, nilJ, off} {
		if j.Push(0, &raw, h, 0) != voice.JitterBypass || j.Pop(time.Hour, &p) || j.Len() != 0 {
			t.Fatal("not a pass-through")
		}
		if _, ok := j.Deadline(); ok {
			t.Fatal("deadline")
		}
		j.Reset()
		_ = j.Stats()
	}
	if on.Push(0, &raw, layer2.Hint{}, 0) != voice.JitterBypass {
		t.Fatal("packet without a sequence number buffered")
	}
	if on.Push(0, nil, h, 0) != voice.JitterBuffered || !on.Pop(time.Hour, &p) || p.HasBurst {
		t.Fatal("frame without a burst")
	}
	if on.Pop(time.Hour, nil) {
		t.Fatal("nil packet")
	}
	var ce *dmrgo.ConfigError
	for _, cfg := range []voice.JitterConfig{{Delay: voice.MaxJitter + 1}, {Lead: -1}} {
		if _, err := voice.NewJitter(&cfg); !errors.As(err, &ce) {
			t.Fatalf("%+v: %v", cfg, err)
		}
	}
}

func TestJitterLead(t *testing.T) {
	t.Parallel()
	j := newJitter(t, &voice.JitterConfig{Lead: 100 * time.Millisecond})
	var raw layer2.RawBurst
	var p voice.JitterPacket
	j.Push(time.Second, &raw, layer2.Hint{Seq: 7, HasSeq: true}, 0)
	at, _ := j.Deadline()
	if at != time.Second+voice.DefaultJitter || !j.Pop(at, &p) || p.Due != at+100*time.Millisecond {
		t.Fatalf("deadline %v due %v", at, p.Due)
	}
}

func TestJitterSeqRestartAfterEnd(t *testing.T) {
	t.Parallel()
	j := newJitter(t, nil)
	var p voice.JitterPacket
	var raw layer2.RawBurst
	now := time.Duration(0)
	got := 0
	drain := func() {
		for {
			at, ok := j.Deadline()
			if !ok || at > now {
				return
			}
			j.Pop(now, &p)
			got++
		}
	}
	for call := range 2 {
		for i := range 20 {
			h := layer2.Hint{Seq: uint16(i), HasSeq: true, SeqBits: 8}
			switch i {
			case 0:
				h.Kind, h.DataType, h.HasDataType = layer2.KindData, enums.DataTypeVoiceLCHeader, true
			case 19:
				h.Kind, h.DataType, h.HasDataType = layer2.KindData, enums.DataTypeTerminatorWithLC, true
			}
			if r := j.Push(now, &raw, h, 9); r != voice.JitterBuffered {
				t.Fatalf("call %d packet %d: %v", call, i, r)
			}
			drain()
			now += burst
		}
		for range 20 {
			now += burst
			drain()
		}
	}
	if got != 40 {
		t.Fatalf("%d played", got)
	}
}
