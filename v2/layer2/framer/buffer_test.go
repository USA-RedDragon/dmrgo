package framer

import (
	"math/rand"
	"os"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// TestFramer_BufferBound checks that fewer than a CACH and a burst stay in
// the buffer between writes, that the buffer never grows, and that a write
// finds the bursts that the same bits written one at a time find.
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
		c := cap(f.buf)
		var got []Burst
		for rest := stream; len(rest) > 0; {
			n := min(1+r.Intn(3*f.cfg.Period), len(rest))
			got = f.AppendBursts(got, rest[:n]...)
			rest = rest[n:]
			if len(f.buf) >= f.pre+layer2.BurstBits {
				t.Fatalf("%+v: %d bits stay in the buffer", cfg, len(f.buf))
			}
			if cap(f.buf) != c {
				t.Fatalf("%+v: buffer grew from %d to %d for %d bits", cfg, c, cap(f.buf), n)
			}
		}
		one := New(cfg)
		var want []Burst
		for i := range stream {
			want = one.AppendBursts(want, stream[i])
		}
		if len(got) == 0 || !slices.Equal(got, want) {
			t.Errorf("%+v: %d bursts, want the %d of single bits", cfg, len(got), len(want))
		}
		if all := New(cfg).AppendBursts(nil, stream...); !slices.Equal(all, want) {
			t.Errorf("%+v: %d bursts from one write of the stream, want %d", cfg, len(all), len(want))
		}
	}
}
