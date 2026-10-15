package framer

import (
	"math/rand"
	"os"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// TestFramer_BufferBound checks that fewer than a CACH and a burst stay in
// the buffer between writes, and that the buffer grows only for a stream
// longer than Period and than every earlier stream.
func TestFramer_BufferBound(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../testdata/voice.bin")
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewSource(5))
	var stream []bit.Bit
	for i := 0; i+len(layer2.RawBurst{}) <= len(data); i += len(layer2.RawBurst{}) {
		stream = append(stream, bit.Unpack(data[i:i+len(layer2.RawBurst{})])...)
		for range r.Intn(2 * SlotPeriod) {
			stream = append(stream, bit.Bit(r.Intn(2)))
		}
	}
	for _, cfg := range []Config{
		{Threshold: -1, Period: SlotPeriod},
		{Threshold: -1, CACH: true},
		{Threshold: 0, MaxMisses: 1},
	} {
		f := New(cfg)
		longest := f.cfg.Period
		var dst []Burst
		for rest := stream; len(rest) > 0; {
			n := min(1+r.Intn(3*f.cfg.Period), len(rest))
			c := cap(f.buf)
			dst = f.AppendBursts(dst[:0], rest[:n]...)
			rest = rest[n:]
			if len(f.buf) >= f.pre+layer2.BurstBits {
				t.Fatalf("%+v: %d bits stay in the buffer", cfg, len(f.buf))
			}
			if cap(f.buf) != c && n <= longest {
				t.Fatalf("%+v: buffer grew for %d bits, longest earlier %d", cfg, n, longest)
			}
			longest = max(longest, n)
		}
	}
}
