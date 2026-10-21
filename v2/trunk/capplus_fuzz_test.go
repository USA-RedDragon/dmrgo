package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func newCapPlusPool(t testing.TB) burstPool {
	t.Helper()
	return burstPool{
		capPlusStatusBurst(t, false, 1),
		capPlusStatusBurst(t, true, 4, 0x48, 9, 14, 0x00, 0x80, 0x10, 0x52, 0x09),
		capPlusStatusBurst(t, false, 16, 0x00, 0x01, 7),
		capPlusGrantBurst(t, 14, 3),
		capPlusGrantBurst(t, 9, 6),
		terminator(t, 14),
	}
}

func runCapPlusFollower(t *testing.T, pool burstPool, in []byte) {
	cfg := trunk.CapacityPlusConfig{Plan: capPlusPlan(t)}
	if len(in) > 0 {
		cfg.Individual = in[0]&1 != 0
		cfg.NoFollow = in[0]&2 != 0
		if in[0]&4 != 0 {
			cfg.Plan = nil
		}
		if in[0]&8 != 0 {
			cfg.TalkgroupsLen, cfg.Talkgroups[0] = 1, 14
		}
		in = in[1:]
	}
	f, err := trunk.NewCapacityPlusFollower(&cfg)
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
		if lsn, ok := f.RestLSN(); ok && (lsn == 0 || lsn > 16) {
			t.Fatalf("op %d: rest LSN %d", op, lsn)
		}
		for _, o := range out {
			if o.Kind == trunk.OutputTune && o.Tune.Channel > 8 {
				t.Fatalf("op %d: tune %+v", op, o.Tune)
			}
		}
	}
}

func capPlusFuzzSeeds() [][]byte {
	return [][]byte{
		{0, 3, 0, 0, 1, 0, 1, 1, 3, 3, 0, 0, 5, 3, 250},
		{1, 0, 0, 1, 2, 3, 0, 1, 4, 0, 5, 3, 200},
		{10, 0, 0, 0, 3, 0, 4, 0, 5, 4, 0, 3, 255, 3, 255},
		{6, 2, 1, 0xFF, 0xFF, 0xFF, 0x0F},
	}
}

func FuzzCapacityPlusFollower(f *testing.F) {
	for _, s := range capPlusFuzzSeeds() {
		f.Add(s)
	}
	pool := newCapPlusPool(f)
	f.Fuzz(func(t *testing.T, in []byte) {
		runCapPlusFollower(t, pool, in)
	})
}
