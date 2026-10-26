package framer_test

import (
	"math/bits"
	"reflect"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	ibits "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
)

const (
	maxCuts  = 128
	maxNoise = 128
)

func syncAt(stream []byte, i int) uint64 {
	var v uint64
	for k := range framer.SyncBits {
		v = v<<1 | uint64(ibits.Bit(stream, i+k))
	}
	return v
}

func nearestAt(stream []byte, i int) (enums.SyncPattern, int) {
	v := syncAt(stream, i)
	return enums.NearestSyncPattern([6]byte{byte(v >> 40), byte(v >> 32), byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}

func refSearch(stream []byte, nbits, start, threshold int) (int, int, bool) {
	n := max(0, min(nbits, 8*len(stream)))
	switch {
	case threshold == 0:
		threshold = layer2.DefaultSyncThreshold
	case threshold < 0:
		threshold = 0
	}
	for i := max(start, 0); i+framer.SyncBits <= n; i++ {
		if _, d := nearestAt(stream, i); d <= threshold {
			return i, d, true
		}
	}
	return 0, 0, false
}

type searchSeed struct {
	stream       []byte
	nbits, start int16
	threshold    int8
}

func searchSeeds(tb testing.TB) []searchSeed {
	tb.Helper()
	var seeds []searchSeed
	for _, b := range loadVoice(tb)[:3] {
		seeds = append(seeds, searchSeed{append([]byte{0x5a, 0x3c}, b[:]...), 8 * 35, 0, 0})
	}
	return append(seeds, searchSeed{[]byte{0xff}, -3, -9, 0})
}

func checkSearch(t *testing.T, stream []byte, nbits, start int16, threshold int8) {
	m, ok := framer.Search(stream, int(nbits), int(start), int(threshold))
	off, d, want := refSearch(stream, int(nbits), int(start), int(threshold))
	if ok != want || ok && (m.Offset != off || m.Distance != d) {
		t.Fatalf("Search = %+v, %v, want offset %d, distance %d, %v", m, ok, off, d, want)
	}
	if ok && bits.OnesCount64(syncAt(stream, m.Offset)^uint64(m.Pattern)) != m.Distance {
		t.Fatalf("Search pattern %v is not %d bits from the field", m.Pattern, m.Distance)
	}
}

func FuzzSearch(f *testing.F) {
	for _, s := range searchSeeds(f) {
		f.Add(s.stream, s.nbits, s.start, s.threshold)
	}
	f.Fuzz(checkSearch)
}

func fuzzConfig(c byte) framer.Config {
	cfg := framer.Config{Threshold: int(c&7) - 1, CACH: c&0x10 != 0, MaxMisses: int(c >> 5)}
	if c&8 != 0 {
		cfg.Period = framer.SlotPeriod
	}
	return cfg
}

func threshold(cfg framer.Config) int {
	switch {
	case cfg.Threshold == 0:
		return layer2.DefaultSyncThreshold
	case cfg.Threshold < 0:
		return 0
	}
	return cfg.Threshold
}

func checkBursts(t *testing.T, cfg framer.Config, stream []byte, got []framer.Burst) {
	t.Helper()
	pre := 0
	if cfg.CACH {
		pre = layer2.CACHBits
	}
	thr := threshold(cfg)
	m, ok := framer.Search(stream, 8*len(stream), pre+framer.SyncOffset, cfg.Threshold)
	if ok && m.Offset-framer.SyncOffset+layer2.BurstBits <= 8*len(stream) {
		if len(got) == 0 || got[0].Offset != int64(m.Offset-framer.SyncOffset) {
			t.Fatalf("SYNC at bit %d, but the first burst is %v", m.Offset, got[:min(len(got), 1)])
		}
	} else if len(got) != 0 {
		t.Fatalf("no SYNC with a whole burst, but %d bursts", len(got))
	}
	last := int64(-1)
	for i, b := range got {
		off := int(b.Offset)
		if b.Offset <= last || off+layer2.BurstBits > 8*len(stream) {
			t.Fatalf("burst %d at %d after %d in %d bits", i, b.Offset, last, 8*len(stream))
		}
		last = b.Offset
		var want layer2.RawBurst
		ibits.Copy(want[:], 0, stream, off, layer2.BurstBits)
		if b.Data != want {
			t.Fatalf("burst %d at %d holds %x, stream has %x", i, off, b.Data, want)
		}
		p, d := nearestAt(stream, off+framer.SyncOffset)
		if b.Distance != d || (d <= thr) != (b.Sync != enums.SyncNone) ||
			b.Sync != enums.SyncNone && bits.OnesCount64(syncAt(stream, off+framer.SyncOffset)^uint64(b.Sync)) != d {
			t.Fatalf("burst %d at %d: SYNC %v at %d, nearest %v at %d, threshold %d", i, off, b.Sync, b.Distance, p, d, thr)
		}
		if b.HasCACH != (cfg.CACH && off >= layer2.CACHBits) {
			t.Fatalf("burst %d at %d: HasCACH %v", i, off, b.HasCACH)
		}
		if b.HasCACH {
			var c [3]byte
			ibits.Copy(c[:], 0, stream, off-layer2.CACHBits, layer2.CACHBits)
			if layer2.DecodeCACHField(c) != b.CACH {
				t.Fatalf("burst %d at %d: CACH %+v, stream has %x", i, off, b.CACH, c)
			}
		}
		if b.Slot > enums.Slot2 {
			t.Fatalf("burst %d: slot %v", i, b.Slot)
		}
	}
}

type framerSeed struct {
	script, cuts []byte
	cfg          byte
}

func framerSeeds(pool [][33]byte) []framerSeed {
	script := make([]byte, 0, 40)
	for k := range 8 {
		script = append(script, 0x02, 0x12, 0x34, 0x56, 0xc0|byte(k))
	}
	tail := append([]byte{0x1e}, pool[0][2:]...)
	return []framerSeed{
		{script, []byte{}, 0x10},
		{script, []byte{1, 7, 200, 0, 13}, 0x01},
		{append(tail, script[5:]...), []byte{255}, 0xe8},
	}
}

func checkFramer(t *testing.T, pool [][33]byte, script, flips, cuts []byte, c byte) {
	cfg := fuzzConfig(c)
	stream := fuzztest.Stream(pool, script, flips)
	cuts = cuts[:min(len(cuts), maxCuts)]
	whole := newFramer(t, &cfg).Write(stream, 8*len(stream))
	checkBursts(t, cfg, stream, whole)
	if !slices.ContainsFunc(cuts, func(b byte) bool { return b != 0 }) {
		cuts = nil
	}

	fr := newFramer(t, &cfg)
	var pieces []framer.Burst
	var piece []byte
	for i, from := 0, 0; from < 8*len(stream); i++ {
		n := 8*len(stream) - from
		if len(cuts) > 0 {
			n = min(n, int(cuts[i%len(cuts)]))
		}
		piece = append(piece[:0], make([]byte, (n+7)/8+1)...)
		ibits.Copy(piece, 0, stream, from, n)
		piece[len(piece)-1] = 0xA5
		pieces = fr.AppendBursts(pieces, piece, n)
		from += n
	}
	if !reflect.DeepEqual(whole, pieces) {
		t.Fatalf("pieces %v give\n%+v\nat once\n%+v", cuts, pieces, whole)
	}
}

func FuzzFramer(f *testing.F) {
	pool := loadVoice(f)
	for _, s := range framerSeeds(pool) {
		f.Add(s.script, []byte{}, s.cuts, s.cfg)
	}
	f.Fuzz(func(t *testing.T, script, flips, cuts []byte, c byte) {
		checkFramer(t, pool, script, flips, cuts, c)
	})
}

type findsBurstSeed struct {
	noise  []byte
	offset uint16
	which  byte
	flips  []byte
	cfg    byte
}

var findsBurstSeeds = []findsBurstSeed{
	{[]byte{0x01, 0x02, 0x03, 0x04}, 0, 0, []byte{}, 0},
	{[]byte{0xff, 0x00, 0x77, 0x31, 0x99}, 37, 6, []byte{3, 40}, 0x13},
	{[]byte{0xaa}, 300, 12, []byte{1}, 0x0a},
}

type findsBurst struct {
	synced [][33]byte
}

func newFindsBurst(tb testing.TB) *findsBurst {
	tb.Helper()
	fb := &findsBurst{}
	for _, b := range loadVoice(tb) {
		if _, d := layer2.RawBurst(b).Sync(); d == 0 {
			fb.synced = append(fb.synced, b)
		}
	}
	return fb
}

func (fb *findsBurst) check(t *testing.T, noise []byte, offset uint16, which byte, flips []byte, c byte) {
	cfg := fuzzConfig(c)
	noise = noise[:min(len(noise), maxNoise)]
	burst := fb.synced[int(which)%len(fb.synced)]
	thr := threshold(cfg)
	for _, fl := range flips[:min(len(flips), thr)] {
		i := framer.SyncOffset + int(fl)%framer.SyncBits
		burst[i/8] ^= 0x80 >> (i % 8)
	}
	if _, d := layer2.RawBurst(burst).Sync(); d > thr {
		return
	}
	off := int(offset) % 2048
	n := off + layer2.BurstBits + 8*len(noise)
	stream := make([]byte, (n+7)/8)
	for i := range stream {
		if len(noise) > 0 {
			stream[i] = noise[i%len(noise)]
		}
	}
	ibits.Copy(stream, off, burst[:], 0, layer2.BurstBits)
	pre := 0
	if cfg.CACH {
		pre = layer2.CACHBits
	}
	m, ok := framer.Search(stream, n, pre+framer.SyncOffset, cfg.Threshold)
	if !ok || m.Offset != off+framer.SyncOffset {
		return
	}
	got := newFramer(t, &cfg).Write(stream, n)
	if len(got) == 0 || got[0].Offset != int64(off) || got[0].Data != layer2.RawBurst(burst) || got[0].Sync == enums.SyncNone {
		t.Fatalf("burst at %d not found: %+v", off, got[:min(len(got), 1)])
	}
}

func FuzzFramerFindsBurst(f *testing.F) {
	for _, s := range findsBurstSeeds {
		f.Add(s.noise, s.offset, s.which, s.flips, s.cfg)
	}
	f.Fuzz(newFindsBurst(f).check)
}
