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

func runCapPlusFollower(t *testing.T, pool burstPool, s opSeed) {
	cfg := trunk.CapacityPlusConfig{Plan: capPlusPlan(t)}
	cfg.Individual = s.cfg&1 != 0
	cfg.NoFollow = s.cfg&2 != 0
	if s.cfg&4 != 0 {
		cfg.Plan = nil
	}
	if s.cfg&8 != 0 {
		cfg.TalkgroupsLen, cfg.Talkgroups[0] = 1, 14
	}
	script := s.script()
	rawIn := s.raw[:min(len(s.raw), maxRawScript)]
	f, err := trunk.NewCapacityPlusFollower(&cfg)
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

func capPlusFuzzSeeds() []opSeed {
	return []opSeed{
		{cfg: 0, ops: []byte{3, 0, 0, 1, 0, 1, 1, 3, 3, 0, 0, 5, 3, 250}},
		{cfg: 1, ops: []byte{0, 0, 1, 2, 3, 0, 1, 4, 0, 5, 3, 200}},
		{cfg: 10, ops: []byte{0, 0, 0, 3, 0, 4, 0, 5, 4, 0, 3, 255, 3, 255}},
		{cfg: 6, ops: []byte{2, 1}, raw: []byte{0xFF, 0xFF, 0xFF, 0x0F}},
		{cfg: 0, ops: []byte{3, 0, 0, 1, 0, 3, 1, 3, 3, 0, 0, 5, 3, 250}, from: 2, to: 5, reps: 20},
	}
}

func FuzzCapacityPlusFollower(f *testing.F) {
	for _, s := range capPlusFuzzSeeds() {
		f.Add(s.cfg, s.ops, s.raw, s.from, s.to, s.reps)
	}
	pool := newCapPlusPool(f)
	f.Fuzz(func(t *testing.T, cfg uint8, ops, raw []byte, from, to, reps uint8) {
		runCapPlusFollower(t, pool, opSeed{cfg: cfg, ops: ops, raw: raw, from: from, to: to, reps: reps})
	})
}
