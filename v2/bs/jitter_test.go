package bs_test

import (
	"math/rand/v2"
	"sort"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

type jpkt struct {
	at  time.Duration
	raw layer2.RawBurst
	h   layer2.Hint
}

func netCall(t *testing.T, superframes int, seq0 uint16, bits uint8, stream uint32) []jpkt {
	t.Helper()
	in := call(t, 1, 9, 3191868, superframes, enums.Slot1, bs.Source{Peer: 1}, time.Second)
	out := make([]jpkt, len(in))
	for i := range in {
		out[i] = jpkt{at: in[i].at, raw: in[i].raw, h: layer2.Hint{Slot: enums.Slot1, Seq: seq0 + uint16(i), HasSeq: true, SeqBits: bits, Stream: stream, HasStream: stream != 0}}
		if bits == 8 {
			out[i].h.Seq &= 0xFF
		}
	}
	return out
}

func sentSlots(in []jpkt) [][9]byte {
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

func runNet(t *testing.T, r *bs.Repeater, in []jpkt, end time.Duration) []bs.Output {
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
	for now := time.Duration(0); now <= end; now += 5 * time.Millisecond {
		for len(in) > 0 && in[0].at <= now {
			h := in[0].h
			h.At = in[0].at
			out = r.Inbound(in[0].at, &in[0].raw, h, bs.Source{Peer: 1}, out[:0])
			keep()
			in = in[1:]
		}
		for {
			at, ok := r.Deadline()
			if !ok || at > now {
				break
			}
			out = r.Tick(at, out[:0])
			keep()
		}
	}
	return got
}

type relayed struct {
	slots    [][9]byte
	fills    int
	header   time.Duration
	hasHead  bool
	term     time.Duration
	cadence  bool
	starts   int
	ends     int
	terminal int
}

func summarize(out []bs.Output) relayed { return summarizeSlot(out, enums.Slot1) }

func summarizeSlot(out []bs.Output, slot enums.Slot) relayed {
	var r relayed
	r.cadence = true
	sil := vocoder.AMBESilenceSlot()
	var prev time.Duration
	n := 0
	for _, o := range out {
		switch o.Kind {
		case bs.OutputCallStart:
			if o.Slot == slot {
				r.starts++
			}
		case bs.OutputCallEnd:
			if o.Slot == slot {
				r.ends++
			}
		case bs.OutputRejected, bs.OutputKey, bs.OutputDekey:
		case bs.OutputBurst:
			if o.Slot != slot {
				continue
			}
			if n > 0 && o.Due-prev != 60*time.Millisecond {
				r.cadence = false
			}
			prev, n = o.Due, n+1
			switch {
			case o.Hint.Kind == layer2.KindVoice:
				for _, s := range o.Burst.VoiceSlots() {
					if s == sil {
						r.fills++
						continue
					}
					r.slots = append(r.slots, s)
				}
			case o.Hint.DataType == enums.DataTypeVoiceLCHeader && !r.hasHead:
				r.header, r.hasHead = o.Due, true
			case o.Hint.DataType == enums.DataTypeTerminatorWithLC:
				r.term = o.Due
				r.terminal++
			}
		}
	}
	return r
}

func same(a, b [][9]byte) int {
	m := map[[9]byte]int{}
	for _, s := range b {
		m[s]++
	}
	n := 0
	for _, s := range a {
		if m[s] > 0 {
			m[s]--
			n++
		}
	}
	return n
}

func TestJitterInOrder(t *testing.T) {
	t.Parallel()
	for _, bits := range []uint8{8, 0} {
		in := netCall(t, 10, 250, bits, 7)
		want := sentSlots(in)
		first := in[0].at
		r := rand.New(rand.NewPCG(1, 2))
		for i := range in {
			in[i].at += time.Duration(r.Int64N(int64(59 * time.Millisecond)))
		}
		got := summarize(runNet(t, newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}), in, 10*time.Second))
		arrival := in[0].at
		if got.fills != 0 || len(got.slots) != len(want) || same(want, got.slots) != len(want) {
			t.Fatalf("bits %d: %d fills, %d of %d slots", bits, got.fills, same(want, got.slots), len(want))
		}
		for i := range want {
			if got.slots[i] != want[i] {
				t.Fatalf("bits %d: slot %d out of order", bits, i)
			}
		}
		if !got.hasHead || got.header > arrival+voice.DefaultJitter || got.header < first+voice.DefaultJitter {
			t.Fatalf("bits %d: header due %v, arrival %v, sent %v", bits, got.header, arrival, first)
		}
		if !got.cadence || got.term != got.header+time.Duration(len(want)/3+1)*60*time.Millisecond {
			t.Fatalf("bits %d: cadence %v, terminator due %v after header %v", bits, got.cadence, got.term, got.header)
		}
	}
}

func TestJitterReorder(t *testing.T) {
	t.Parallel()
	cases := map[string]func(r *rand.Rand, in []jpkt){
		"swap": func(_ *rand.Rand, in []jpkt) {
			for i := 0; i+1 < len(in); i += 3 {
				in[i].at, in[i+1].at = in[i+1].at, in[i].at
			}
		},
		"random300": func(r *rand.Rand, in []jpkt) {
			for i := range in {
				in[i].at += time.Duration(r.Int64N(int64(300 * time.Millisecond)))
			}
		},
		"clump6": func(_ *rand.Rand, in []jpkt) {
			at := make([]time.Duration, len(in))
			for i := range in {
				at[i] = in[i].at
			}
			for i := range in {
				in[i].at = at[min(i/6*6+5, len(in)-1)]
			}
		},
		"headerlast": func(_ *rand.Rand, in []jpkt) {
			in[0].at += 150 * time.Millisecond
		},
	}
	for name, f := range cases {
		for _, mode := range []bs.Mode{bs.ModeNetwork, bs.ModeRF} {
			in := netCall(t, 10, 65530, 0, 9)
			want := sentSlots(in)
			f(rand.New(rand.NewPCG(3, 4)), in)
			got := summarize(runNet(t, newRepeater(t, &bs.Config{Mode: mode, ColorCode: 1, CACH: true}), in, 10*time.Second))
			if got.fills != 0 || same(want, got.slots) != len(want) || len(got.slots) != len(want) {
				t.Errorf("%s %v: %d fills, %d of %d slots", name, mode, got.fills, same(want, got.slots), len(want))
				continue
			}
			for i := range want {
				if got.slots[i] != want[i] {
					t.Errorf("%s %v: slot %d out of order", name, mode, i)
					break
				}
			}
			if got.starts != 1 || got.ends != 1 || got.terminal == 0 {
				t.Errorf("%s %v: %d starts, %d ends, %d terminators", name, mode, got.starts, got.ends, got.terminal)
			}
		}
	}
}

func TestJitterStall(t *testing.T) {
	t.Parallel()
	in := netCall(t, 10, 0, 8, 3)
	want := sentSlots(in)
	stall := in[20].at + 700*time.Millisecond
	for i := 20; i < len(in) && in[i].at < stall; i++ {
		in[i].at = stall
	}
	term := in[len(in)-1].at
	got := summarize(runNet(t, newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}), in, 10*time.Second))
	if n := same(want, got.slots); n < len(want)-6*3 || n > len(want)-5*3 || got.starts != 1 || got.ends != 1 {
		t.Fatalf("%d of %d slots, %d starts, %d ends", n, len(want), got.starts, got.ends)
	}
	for i := range 19 * 3 {
		if got.slots[i] != want[i] {
			t.Fatalf("slot %d lost before the stall", i)
		}
	}
	if !got.cadence {
		t.Fatalf("cadence broken")
	}
	if d := got.term - term - voice.DefaultJitter; d < 0 || d >= 60*time.Millisecond {
		t.Fatalf("terminator due %v, arrival %v: the delay did not come back", got.term, term)
	}
}

func TestJitterOff(t *testing.T) {
	t.Parallel()
	in := netCall(t, 2, 0, 8, 3)
	got := summarize(runNet(t, newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1, Jitter: -1}), in, 5*time.Second))
	if got.header != in[0].at {
		t.Fatalf("header due %v, want %v", got.header, in[0].at)
	}
	got = summarize(runNet(t, newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1}), netCall(t, 2, 0, 8, 3), 5*time.Second))
	if got.header != in[0].at+voice.DefaultJitter {
		t.Fatalf("default: header due %v, want %v", got.header, in[0].at+voice.DefaultJitter)
	}
}

func TestJitterBothSlots(t *testing.T) {
	t.Parallel()
	for _, mode := range []bs.Mode{bs.ModeNetwork, bs.ModeRF} {
		a := netCall(t, 10, 10, 8, 1)
		b := netCall(t, 9, 200, 8, 2)
		r := rand.New(rand.NewPCG(7, 8))
		for i := range b {
			b[i].at += 25 * time.Millisecond
			b[i].h.Slot = enums.Slot2
			b[i].raw = call(t, 1, 9, 3191868, 9, enums.Slot2, bs.Source{Peer: 1}, 0)[i].raw
		}
		in := append(append([]jpkt(nil), a...), b...)
		wantA, wantB := sentSlots(a), sentSlots(b)
		for i := range in {
			in[i].at += time.Duration(r.Int64N(int64(250 * time.Millisecond)))
		}
		out := runNet(t, newRepeater(t, &bs.Config{Mode: mode, ColorCode: 1, CACH: true}), in, 10*time.Second)
		for _, c := range []struct {
			slot enums.Slot
			want [][9]byte
		}{{enums.Slot1, wantA}, {enums.Slot2, wantB}} {
			got := summarizeSlot(out, c.slot)
			if got.fills != 0 || len(got.slots) != len(c.want) || same(c.want, got.slots) != len(c.want) || got.starts != 1 || got.ends != 1 {
				t.Errorf("%v %v: %d fills, %d of %d slots, %d starts, %d ends", mode, c.slot, got.fills, same(c.want, got.slots), len(c.want), got.starts, got.ends)
			}
		}
	}
}
