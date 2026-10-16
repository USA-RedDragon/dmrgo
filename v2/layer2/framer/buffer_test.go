package framer

import (
	"math/rand"
	"os"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// TestFramer_BufferBound checks that fewer than a CACH, a burst and a byte
// stay in the buffer between writes, that the buffer never grows, and that a
// write finds the bursts that the same bytes written one at a time find.
func TestFramer_BufferBound(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../testdata/voice.bin")
	if err != nil {
		t.Fatal(err)
	}
	r := rand.New(rand.NewSource(5))
	var stream []byte
	n := 0
	put := func(v int) {
		if n%8 == 0 {
			stream = append(stream, 0)
		}
		stream[n/8] |= byte(v&1) << (7 - n%8)
		n++
	}
	for i := 0; i+len(layer2.RawBurst{}) <= len(data); i += len(layer2.RawBurst{}) {
		for j := range layer2.BurstBits {
			put(int(data[i+j/8] >> (7 - j%8)))
		}
		for range r.Intn(2 * SlotPeriod) {
			put(r.Intn(2))
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
			n := min(1+r.Intn(3*f.cfg.Period/8), len(rest))
			got = f.AppendBursts(got, rest[:n])
			rest = rest[n:]
			if len(f.buf)*8 >= f.pre+layer2.BurstBits+8 {
				t.Fatalf("%+v: %d bytes stay in the buffer", cfg, len(f.buf))
			}
			if cap(f.buf) != c {
				t.Fatalf("%+v: buffer grew from %d to %d for %d bytes", cfg, c, cap(f.buf), n)
			}
		}
		one := New(cfg)
		var want []Burst
		for i := range stream {
			want = one.AppendBursts(want, stream[i:i+1])
		}
		if len(got) == 0 || !slices.Equal(got, want) {
			t.Errorf("%+v: %d bursts, want the %d of single bytes", cfg, len(got), len(want))
		}
		if all := New(cfg).AppendBursts(nil, stream); !slices.Equal(all, want) {
			t.Errorf("%+v: %d bursts from one write of the stream, want %d", cfg, len(all), len(want))
		}
	}
}
