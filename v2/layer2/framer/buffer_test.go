package framer

import (
	"math/rand"
	"os"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func testStream(t *testing.T, r *rand.Rand) []byte {
	t.Helper()
	data, err := os.ReadFile("../testdata/voice.bin")
	if err != nil {
		t.Fatal(err)
	}
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
	return stream
}

var testConfigs = []Config{
	{Threshold: -1, Period: SlotPeriod},
	{Threshold: -1, CACH: true},
	{Threshold: 0, MaxMisses: 1},
}

func TestFramer_BufferBound(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(5))
	stream := testStream(t, r)
	for _, cfg := range testConfigs {
		f := mustNew(t, &cfg)
		c := cap(f.buf)
		var got []Burst
		for rest := stream; len(rest) > 0; {
			n := min(1+r.Intn(3*f.cfg.Period/8), len(rest))
			got = f.AppendBursts(got, rest[:n], 8*n)
			rest = rest[n:]
			if f.n >= f.pre+layer2.BurstBits+8 {
				t.Fatalf("%+v: %d bits stay in the buffer", cfg, f.n)
			}
			if cap(f.buf) != c {
				t.Fatalf("%+v: buffer grew from %d to %d for %d bytes", cfg, c, cap(f.buf), n)
			}
		}
		one := mustNew(t, &cfg)
		var want []Burst
		for i := range stream {
			want = one.AppendBursts(want, stream[i:i+1], 8)
		}
		if len(got) == 0 || !slices.Equal(got, want) {
			t.Errorf("%+v: %d bursts, want the %d of single bytes", cfg, len(got), len(want))
		}
		if all := mustNew(t, &cfg).AppendBursts(nil, stream, 8*len(stream)); !slices.Equal(all, want) {
			t.Errorf("%+v: %d bursts from one write of the stream, want %d", cfg, len(all), len(want))
		}
	}
}

func TestFramer_BitPieces(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(6))
	stream := testStream(t, r)
	for _, cfg := range testConfigs {
		all := mustNew(t, &cfg).AppendBursts(nil, stream, 8*len(stream))
		for end := 8*len(stream) - 8; end <= 8*len(stream); end++ {
			var want []Burst
			for _, b := range all {
				if b.Offset+layer2.BurstBits <= int64(end) {
					want = append(want, b)
				}
			}
			f := mustNew(t, &cfg)
			c := cap(f.buf)
			var got []Burst
			for from := 0; from < end; {
				k := min(1+r.Intn(3*f.cfg.Period), end-from)
				piece := make([]byte, (k+7)/8+r.Intn(2))
				for i := range piece {
					piece[i] = byte(r.Intn(256))
				}
				bits.Copy(piece, 0, stream, from, k)
				got = f.AppendBursts(got, piece, k)
				from += k
				if f.n >= f.pre+layer2.BurstBits+8 || cap(f.buf) != c {
					t.Fatalf("%+v: %d bits stay in the buffer of %d bytes", cfg, f.n, cap(f.buf))
				}
			}
			if len(want) == 0 || !slices.Equal(got, want) {
				t.Fatalf("%+v, stream of %d bits: %d bursts, want the %d of the whole bytes", cfg, end, len(got), len(want))
			}
		}
	}
}

func mustNew(t *testing.T, cfg *Config) *Framer {
	t.Helper()
	f, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return f
}
