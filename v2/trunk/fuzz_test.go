package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

const (
	maxFuzzOps   = 256
	maxScript    = 128
	maxRawScript = 4 * 33
)

type opSeed struct {
	cfg            uint8
	ops, raw       []byte
	from, to, reps uint8
}

type opScript struct {
	ops                  []byte
	lo, hi, total, width int
}

func (s opSeed) script() opScript {
	in := s.ops[:min(len(s.ops), maxScript)]
	n := len(in) / 2
	lo := min(int(s.from), n)
	hi := max(min(int(s.to), n), lo)
	w := int(s.reps) * (hi - lo)
	return opScript{ops: in, lo: lo, hi: hi, total: min(n+w, maxFuzzOps), width: w}
}

func (s *opScript) op(i int) (kind, arg byte) {
	if i >= s.hi {
		i -= s.hi
		if i < s.width {
			i = s.lo + i%(s.hi-s.lo)
		} else {
			i = s.hi + i - s.width
		}
	}
	return s.ops[2*i], s.ops[2*i+1]
}

type burstPool []layer2.RawBurst

func newPool(t testing.TB) burstPool {
	t.Helper()
	cdef := layer3.CdefParms{Channel: 42, TXMHz: 456, RXMHz: 451}
	mbc := mbcGrant(t, 9, cdef, 2)
	return burstPool{aloha(t), tvGrant(t, 9, 5, false), tvGrant(t, 10, 6, true), pClear(t, 9), terminator(t, 9), mbc[0], mbc[1]}
}

func runFollower(t *testing.T, pool burstPool, s opSeed) {
	cfg := trunk.FollowerConfig{
		Hunt:       huntList(t, []uint16{1, 2, 3}[:s.cfg&3]...),
		Individual: s.cfg&4 != 0,
		Data:       s.cfg&8 != 0,
		NoFollow:   s.cfg&16 != 0,
	}
	script := s.script()
	rawIn := s.raw[:min(len(s.raw), maxRawScript)]
	f, err := trunk.NewFollower(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	now := time.Duration(0)
	for op := range script.total {
		kind, arg := script.op(op)
		now += time.Duration(arg) * 10 * time.Millisecond
		switch kind % 6 {
		case 0, 1:
			raw := pool[int(arg)%len(pool)]
			out = f.Burst(now, &raw, layer2.Hint{Slot: enums.Slot(kind >> 6)}, nil, out[:0])
		case 2:
			var raw layer2.RawBurst
			n := copy(raw[:], rawIn)
			rawIn = rawIn[n:]
			cach := [3]byte{arg, kind, byte(n)}
			out = f.Burst(now, &raw, layer2.Hint{}, &cach, out[:0])
		case 3:
			if at, ok := f.Deadline(); ok && at > now {
				now = at
			}
			out = f.Tick(now, out[:0])
		case 4:
			out = f.Lost(now, out[:0])
		case 5:
			f.Reset()
			out = out[:0]
		}
		if len(out) > trunk.MaxOutputs || cap(out) != trunk.MaxOutputs {
			t.Fatalf("op %d: %d outputs, cap %d", op, len(out), cap(out))
		}
	}
}

func fuzzSeeds() []opSeed {
	return []opSeed{
		{cfg: 0, ops: []byte{0, 0, 0, 1, 1, 3, 3, 0}},
		{cfg: 1, ops: []byte{3, 0, 0, 0, 1, 1, 3, 0, 4, 0, 3, 200}},
		{cfg: 7, ops: []byte{0, 0, 1, 5, 1, 6, 3, 100, 3, 250, 5, 0, 0, 0}},
		{cfg: 16, ops: []byte{0, 0, 1, 1, 2, 9}, raw: []byte{0xFF, 0xFF, 0xFF}},
		{cfg: 3, ops: []byte{0, 0, 1, 1, 0, 3, 3, 0, 0, 4}, from: 2, to: 4, reps: 40},
	}
}

func FuzzFollower(f *testing.F) {
	for _, s := range fuzzSeeds() {
		f.Add(s.cfg, s.ops, s.raw, s.from, s.to, s.reps)
	}
	pool := newPool(f)
	f.Fuzz(func(t *testing.T, cfg uint8, ops, raw []byte, from, to, reps uint8) {
		runFollower(t, pool, opSeed{cfg: cfg, ops: ops, raw: raw, from: from, to: to, reps: reps})
	})
}
