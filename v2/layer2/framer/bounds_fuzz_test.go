package framer_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func idleBurst(tb testing.TB) [33]byte {
	tb.Helper()
	b, err := layer2.BuildDataBurst(pdu.PRFillBits(), enums.DataTypeIdle, 1, layer2.SyncSourceBS)
	if err != nil {
		tb.Fatal(err)
	}
	return b
}

func framerBoundStream(t *testing.T) []byte {
	t.Helper()
	b := idleBurst(t)
	s := make([]byte, 0, 8*(3+len(b)))
	for range 8 {
		s = append(s, 0, 0, 0)
		s = append(s, b[:]...)
	}
	return s
}

func runFramerBounds(t *testing.T, stream []byte, cuts []byte, period uint8) int {
	t.Helper()
	f := newFramer(t, &framer.Config{Period: fuzzPeriod(period), CACH: period&1 == 0, Threshold: int(period % 5)})
	total := 0
	for off, i := 0, 0; off < len(stream); i++ {
		n := 1
		if len(cuts) > 0 {
			n = int(cuts[i%len(cuts)])%97 + 1
		}
		n = min(n, len(stream)-off)
		nbits := 8*n - int(period%8)
		out := f.AppendBursts(nil, stream[off:off+n], nbits)
		if bound := 2 * (max(0, min(nbits, 8*n))/layer2.BurstBits + 1); len(out) > bound {
			t.Fatalf("AppendBursts of %d bits returned %d bursts", nbits, len(out))
		}
		total += len(out)
		if w := f.Write(stream[off:off+n], -1); len(w) != 0 {
			t.Fatalf("Write of no bits returned %d bursts", len(w))
		}
		off += n
	}
	return total
}

func TestFramerBounds(t *testing.T) {
	t.Parallel()
	s := framerBoundStream(t)
	for _, p := range []uint8{0, 1, 2, 7, 64, 255} {
		if n := runFramerBounds(t, s, []byte{1, 5, 40, 96}, p); p == 0 && n == 0 {
			t.Error("the test stream gave no bursts")
		}
	}
}

func FuzzFramerBounds(f *testing.F) {
	pool := [][33]byte{idleBurst(f)}
	f.Add([]byte{0x05, 0x75, 0x5f, 0xd7, 0xdf, 0x75, 0xf7}, []byte{}, []byte{1}, uint8(1))
	f.Add([]byte{0x06, 0xd5, 0xd7, 0xf7, 0x7f, 0xd7, 0x57, 0x00}, []byte{}, []byte{3, 2}, uint8(2))
	f.Fuzz(func(t *testing.T, script, flips, cuts []byte, period uint8) {
		_ = runFramerBounds(t, fuzztest.Stream(pool, script, flips), cuts[:min(len(cuts), maxCuts)], period)
	})
}

func fuzzPeriod(p uint8) int {
	if p == 0 {
		return 0
	}
	return layer2.BurstBits + int(p)
}
