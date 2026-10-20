package codec2

import (
	"math/bits"
	"math/rand/v2"
	"testing"
)

func allModes() []Mode { return []Mode{Mode2400, Mode3200} }

func randomFrame(rng *rand.Rand, mode Mode) [9]byte {
	for {
		var s [9]byte
		for i := range mode.frameBytes() {
			s[i] = byte(rng.Uint32())
		}
		if mode == Mode3200 && bits.OnesCount16(uint16(s[6])<<8|uint16(s[7])) < 3 {
			continue
		}
		if bits.OnesCount64(uint64(s[0])|uint64(s[1])<<8|uint64(s[2])<<16|uint64(s[3])<<24|uint64(s[4])<<32|uint64(s[5])<<40) <= detectQuiet {
			continue
		}
		return s
	}
}

func TestDetectorCleanFrames(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(3, 4))
	for _, mode := range allModes() {
		var d detector
		for i := range 20 {
			s := randomFrame(rng, mode)
			if got := d.add(&s); got != mode {
				t.Fatalf("mode %d frame %d: decoded as %d", mode, i, got)
			}
			want := ModeAuto
			if i+1 >= detectDecide {
				want = mode
			}
			if got := d.mode(); got != want {
				t.Fatalf("mode %d frame %d: mode() = %d, want %d", mode, i, got, want)
			}
		}
		d.reset()
		if d.mode() != ModeAuto {
			t.Errorf("mode %d: mode() after reset = %d", mode, d.mode())
		}
	}
}

func TestDetectorQuietFrames(t *testing.T) {
	t.Parallel()

	var d detector
	zero := [9]byte{}
	if got := d.add(&zero); got != Mode2400 || d.mode() != ModeAuto {
		t.Fatalf("zero frame: %d, mode() %d", got, d.mode())
	}
	rng := rand.New(rand.NewPCG(5, 6))
	for range detectLimit {
		s := randomFrame(rng, Mode3200)
		d.add(&s)
	}
	for i := range 100 {
		if got := d.add(&zero); got != Mode3200 || d.mode() != Mode3200 {
			t.Fatalf("zero frame %d after Mode3200: %d, mode() %d", i, got, d.mode())
		}
	}
}

func TestDetectorFollowsModeChange(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(7, 8))
	for _, from := range allModes() {
		to := Mode2400
		if from == Mode2400 {
			to = Mode3200
		}
		var d detector
		for range 50 {
			s := randomFrame(rng, from)
			d.add(&s)
		}
		for i := range 20 {
			s := randomFrame(rng, to)
			want := from
			if i >= detectLimit {
				want = to
			}
			if got := d.add(&s); got != want {
				t.Fatalf("%d to %d, frame %d: decoded as %d, want %d", from, to, i, got, want)
			}
		}
	}
}

func TestDetectorBitErrors(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(9, 10))
	for _, mode := range allModes() {
		var wrong int
		for range 2000 {
			var d detector
			for range 50 {
				s := randomFrame(rng, mode)
				for b := range 72 {
					if rng.IntN(20) == 0 {
						s[b/8] ^= 0x80 >> (b % 8)
					}
				}
				d.add(&s)
				if m := d.mode(); m != ModeAuto && m != mode {
					wrong++
					break
				}
			}
		}
		if wrong != 0 {
			t.Errorf("mode %d at a bit error rate of 5%%: %d of 2000 calls decided the other mode", mode, wrong)
		}
	}
}

func FuzzDetector(f *testing.F) {
	f.Add([]byte{0xB9, 0xE8, 0x81, 0x52, 0x61, 0x73, 0, 0, 0})
	f.Add([]byte{0xB9, 0xE8, 0x81, 0x52, 0x61, 0x73, 0x21, 0x08, 0})
	f.Fuzz(func(t *testing.T, raw []byte) {
		var d detector
		for off := 0; off < len(raw); off += 9 {
			var s [9]byte
			copy(s[:], raw[off:])
			got := d.add(&s)
			if d.score < -detectLimit || d.score > detectLimit {
				t.Fatalf("score %d out of range", d.score)
			}
			if got != Mode2400 && got != Mode3200 {
				t.Fatalf("add returned %d", got)
			}
			if m := d.mode(); m != ModeAuto && m != got {
				t.Fatalf("mode() %d, add %d", m, got)
			}
		}
	})
}
