package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

const maxFuzzOps = 256

type burstPool []layer2.RawBurst

func newPool(t testing.TB) burstPool {
	t.Helper()
	cdef := layer3.CdefParms{Channel: 42, TXMHz: 456, RXMHz: 451}
	mbc := mbcGrant(t, 9, cdef, 2)
	return burstPool{aloha(t), tvGrant(t, 9, 5, false), tvGrant(t, 10, 6, true), pClear(t, 9), terminator(t, 9), mbc[0], mbc[1]}
}

func runFollower(t *testing.T, pool burstPool, in []byte) {
	cfg := trunk.FollowerConfig{Hunt: huntList(t, 1, 2, 3), NoFollow: false}
	if len(in) > 0 {
		cfg.Hunt = huntList(t, []uint16{1, 2, 3}[:in[0]&3]...)
		cfg.Individual = in[0]&4 != 0
		cfg.Data = in[0]&8 != 0
		cfg.NoFollow = in[0]&16 != 0
		in = in[1:]
	}
	f, err := trunk.NewFollower(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	now := time.Duration(0)
	for op := 0; op < maxFuzzOps && len(in) >= 2; op++ {
		kind, arg := in[0], in[1]
		in = in[2:]
		now += time.Duration(arg) * 10 * time.Millisecond
		switch kind % 6 {
		case 0, 1:
			raw := pool[int(arg)%len(pool)]
			out = f.Burst(now, &raw, layer2.Hint{Slot: enums.Slot(kind >> 6)}, nil, out[:0])
		case 2:
			var raw layer2.RawBurst
			n := copy(raw[:], in)
			in = in[n:]
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

func fuzzSeeds() [][]byte {
	return [][]byte{
		{0, 0, 0, 0, 1, 1, 3, 3, 0},
		{1, 3, 0, 0, 0, 1, 1, 3, 0, 4, 0, 3, 200},
		{7, 0, 0, 1, 5, 1, 6, 3, 100, 3, 250, 5, 0, 0, 0},
		{16, 0, 0, 1, 1, 2, 9, 0xFF, 0xFF, 0xFF},
	}
}

func FuzzFollower(f *testing.F) {
	for _, s := range fuzzSeeds() {
		f.Add(s)
	}
	pool := newPool(f)
	f.Fuzz(func(t *testing.T, in []byte) {
		runFollower(t, pool, in)
	})
}
