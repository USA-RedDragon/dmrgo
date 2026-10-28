package rf

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"runtime"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

const (
	fuzzBursts   = 4096
	fuzzMaxBytes = 1 << 16
)

type fuzzSink struct {
	b []Burst
	n int
}

func newFuzzSink() *fuzzSink { return &fuzzSink{b: make([]Burst, fuzzBursts)} }

func (s *fuzzSink) add(bs []Burst) {
	for i := range bs {
		s.one(&bs[i])
	}
}

func (s *fuzzSink) one(b *Burst) {
	if s.n < len(s.b) {
		s.b[s.n] = *b
	}
	s.n++
}

func (s *fuzzSink) bursts() []Burst { return s.b[:min(s.n, len(s.b))] }

func mallocs(f func()) uint64 {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	f()
	runtime.ReadMemStats(&b)
	return b.Mallocs - a.Mallocs
}

func quiet(run func() uint64) uint64 {
	n := run()
	for i := 0; i < 2 && n != 0; i++ {
		n = min(n, run())
	}
	return n
}

func fuzzChunks(n, most int, seed uint64) []int {
	var out []int
	rng := rand.New(rand.NewPCG(seed, seed>>1|1))
	for n > 0 {
		k := most
		if seed != 0 {
			k = 1 + rng.IntN(most)
			if rng.IntN(4) == 0 {
				k = 1 + rng.IntN(min(most, 7))
			}
		}
		k = min(k, n)
		out = append(out, k)
		n -= k
	}
	return out
}

func fuzzConfig(tier, half, flags uint8, rate uint16, lookback uint8) (RxConfig, *options) {
	cfg := RxConfig{Tier: Tier0 + Tier(tier%4), Format: iqFormatOf(flags&1 != 0), Rate: basebandRate}
	bb := basebandRate
	if half%2 == 1 && cfg.Tier <= Tier1 {
		bb = discRate
	}
	cfg.Rate = bb
	if flags&2 != 0 {
		cfg.Rate = int(float64(bb) * (1 + float64(rate%4000)/1000))
	}
	if cfg.Tier != Tier0 {
		cfg.LookbackSlots = int(lookback % 33)
	}
	if bb == discRate && cfg.Rate >= basebandRate {
		o := tierOptions(cfg.Tier)
		if cfg.LookbackSlots > 0 {
			o.ExtLookback = cfg.LookbackSlots
		}
		return cfg, halfOpt(&o)
	}
	return cfg, nil
}

func fuzzMost(flags uint8, n uint32, most int) int {
	if flags&4 != 0 {
		return 1 + int(n%uint32(2*most))
	}
	return most
}

func fuzzRun(t *testing.T, cfg RxConfig, opt *options, stream bool, in []byte, most int, seed uint64, sink *fuzzSink) uint64 {
	if stream {
		cfg.OnBursts = sink.add
	}
	r, err := newReceiver(cfg, opt)
	if err != nil {
		t.Fatal(err)
	}
	chunks := fuzzChunks(len(in), most, seed)
	dst := make([]Burst, 0, max(fuzzBursts, r.MaxOutput(most/r.SampleBytes()+1), r.MaxOutput(0)))
	return mallocs(func() {
		p := 0
		for _, k := range chunks {
			if stream {
				if _, err := r.Write(in[p : p+k]); err != nil {
					panic(err)
				}
			} else {
				dst = r.AppendBursts(dst[:0], in[p:p+k])
				sink.add(dst)
			}
			p += k
		}
		if stream {
			if err := r.Flush(); err != nil {
				panic(err)
			}
		} else {
			sink.add(r.AppendFlush(dst[:0]))
		}
	})
}

func checkBursts(t *testing.T, bs []Burst, seconds float64) {
	t.Helper()
	type key struct {
		stream int
		ch     int64
	}
	last := map[key]Burst{}
	for i, b := range bs {
		known := slices.Contains([]enums.SyncPattern{enums.SyncNone, enums.SyncBSSourcedVoice, enums.SyncBSSourcedData, enums.SyncMSSourcedVoice,
			enums.SyncMSSourcedData, enums.SyncMSSourcedRC, enums.SyncTDMA1Voice, enums.SyncTDMA1Data, enums.SyncTDMA2Voice, enums.SyncTDMA2Data}, b.Sync)
		switch {
		case b.Distance < -1 || b.Distance > 48:
			t.Fatalf("burst %d: Distance %d", i, b.Distance)
		case !known:
			t.Fatalf("burst %d: Sync %#x", i, uint64(b.Sync))
		case b.Voice < enums.VoiceBurstUnknown || b.Voice > enums.VoiceBurstF:
			t.Fatalf("burst %d: Voice %d", i, b.Voice)
		case b.Stream < 1:
			t.Fatalf("burst %d: Stream %d", i, b.Stream)
		case math.IsNaN(b.Seconds) || b.Seconds < -slotSeconds || b.Seconds > seconds+2*slotSeconds:
			t.Fatalf("burst %d: Seconds %v of a %v s input", i, b.Seconds, seconds)
		}
		k := key{b.Stream, b.Slot & 1}
		if p, ok := last[k]; ok {
			d := b.Seconds - p.Seconds
			if d < 0 || b.Slot <= p.Slot || math.Abs(d-float64(b.Slot-p.Slot)*slotSeconds) > 0.002+0.001*d {
				t.Fatalf("burst %d: stream %d slot %d at %.6f s after slot %d at %.6f s", i, b.Stream, b.Slot, b.Seconds, p.Slot, p.Seconds)
			}
		}
		last[k] = b
	}
}

func sameBursts(t *testing.T, what string, a, b []Burst) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("%s: %d bursts, %d in one chunk", what, len(b), len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("%s: burst %d differs:\n%+v\n%+v", what, i, b[i], a[i])
		}
	}
}

func fuzzSeedIQ(tb testing.TB, signed bool) []byte {
	slots, _ := benchStream(tb, 1)
	x := noisyBaseband(slots, 900, 20, 9)
	out := make([]byte, 2*len(x))
	for i, v := range x {
		re, im := float64(real(v))*0.5*127.5, float64(imag(v))*0.5*127.5
		if signed {
			out[2*i], out[2*i+1] = byte(int8(max(-127, min(127, math.Round(re))))), byte(int8(max(-127, min(127, math.Round(im)))))
		} else {
			out[2*i], out[2*i+1] = byte(max(0, min(255, math.Round(re+127.5)))), byte(max(0, min(255, math.Round(im+127.5))))
		}
	}
	return out
}

func FuzzReceiverWrite(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint8(0), uint16(0), uint32(0), uint8(0), uint64(0), fuzzSeedIQ(f, false))
	f.Add(uint8(1), uint8(1), uint8(7), uint16(1000), uint32(4095), uint8(4), uint64(3), fuzzSeedIQ(f, true))
	f.Add(uint8(2), uint8(0), uint8(2), uint16(0), uint32(0), uint8(8), uint64(5), fuzzSeedIQ(f, false))
	f.Add(uint8(3), uint8(0), uint8(4), uint16(0), uint32(77), uint8(0), uint64(9), fuzzSeedIQ(f, false)[:20000])
	f.Add(uint8(0), uint8(0), uint8(6), uint16(3999), uint32(1), uint8(0), uint64(1), []byte{1, 2, 3})
	f.Add(uint8(3), uint8(0), uint8(4), uint16(0), uint32(0), uint8(0), uint64(1), fuzzSeedIQ(f, false)[:9001])
	f.Fuzz(func(t *testing.T, tier, half, flags uint8, rate uint16, maxWrite uint32, lookback uint8, chunkSeed uint64, iq []byte) {
		iq = iq[:min(len(iq), fuzzMaxBytes)]
		cfg, opt := fuzzConfig(tier, half, flags, rate, lookback)
		stream := flags&8 != 0
		most := fuzzMost(flags, maxWrite, 1<<16)
		run := func(seed uint64, sink *fuzzSink) uint64 {
			return fuzzRun(t, cfg, opt, stream, iq, most, seed, sink)
		}
		hz := float64(cfg.Rate)
		var one, chunked *fuzzSink
		if n := quiet(func() uint64 { one = newFuzzSink(); return run(0, one) }); n != 0 {
			t.Fatalf("%d allocations after New", n)
		}
		if n := quiet(func() uint64 { chunked = newFuzzSink(); return run(chunkSeed|1, chunked) }); n != 0 {
			t.Fatalf("%d allocations after New, in chunks", n)
		}
		checkBursts(t, one.bursts(), float64(len(iq)/2)/hz)
		sameBursts(t, "chunks", one.bursts(), chunked.bursts())
	})
}

func fuzzSamples(b []byte, scale uint8, raw bool) []complex64 {
	if raw {
		x := make([]complex64, len(b)/8)
		for i := range x {
			re := math.Float32frombits(binary.LittleEndian.Uint32(b[8*i:]))
			im := math.Float32frombits(binary.LittleEndian.Uint32(b[8*i+4:]))
			x[i] = complex(re, im)
		}
		return x
	}
	g := float32(math.Ldexp(1, -int(scale%40)))
	x := make([]complex64, len(b)/4)
	for i := range x {
		x[i] = complex(float32(int16(binary.LittleEndian.Uint16(b[4*i:])))*g, float32(int16(binary.LittleEndian.Uint16(b[4*i+2:])))*g)
	}
	return x
}

func fuzzSeedBaseband(tb testing.TB) []byte {
	slots, _ := benchStream(tb, 1)
	x := noisyBaseband(slots, -700, 20, 11)
	out := make([]byte, 4*len(x))
	for i, v := range x {
		binary.LittleEndian.PutUint16(out[4*i:], uint16(int16(max(-32767, min(32767, math.Round(float64(real(v))*4096))))))
		binary.LittleEndian.PutUint16(out[4*i+2:], uint16(int16(max(-32767, min(32767, math.Round(float64(imag(v))*4096))))))
	}
	return out
}

func FuzzReceiverBaseband(f *testing.F) {
	seed := fuzzSeedBaseband(f)
	f.Add(uint8(0), uint8(0), uint8(0), uint8(12), false, uint64(0), seed)
	f.Add(uint8(1), uint8(1), uint8(8), uint8(12), false, uint64(7), seed)
	f.Add(uint8(2), uint8(0), uint8(0), uint8(0), false, uint64(2), seed)
	f.Add(uint8(3), uint8(0), uint8(8), uint8(12), false, uint64(3), seed[:40000])
	f.Add(uint8(0), uint8(0), uint8(0), uint8(0), true, uint64(1), []byte{0, 0, 0xc0, 0x7f, 0, 0, 0x80, 0x7f})
	f.Fuzz(func(t *testing.T, tier, half, flags, scale uint8, raw bool, chunkSeed uint64, b []byte) {
		cfg, opt := fuzzConfig(tier, half, flags&^2, 0, flags>>4)
		cfg.Format = IQFormatCF32
		x := fuzzSamples(b[:min(len(b), 2*fuzzMaxBytes)], scale, raw)
		in := cf32(x)
		stream := flags&8 != 0
		most := 8 << 14
		if flags&4 != 0 {
			most = 8 * (1 + int(uint32(chunkSeed>>32)%(1<<13)))
		}
		run := func(seed uint64, sink *fuzzSink) uint64 {
			return fuzzRun(t, cfg, opt, stream, in, most, seed, sink)
		}
		hz := float64(cfg.Rate)
		var one, chunked *fuzzSink
		if n := quiet(func() uint64 { one = newFuzzSink(); return run(0, one) }); n != 0 {
			t.Fatalf("%d allocations after New", n)
		}
		if n := quiet(func() uint64 { chunked = newFuzzSink(); return run(chunkSeed|1, chunked) }); n != 0 {
			t.Fatalf("%d allocations after New, in chunks", n)
		}
		checkBursts(t, one.bursts(), float64(len(x))/hz)
		sameBursts(t, "chunks", one.bursts(), chunked.bursts())
	})
}

func fuzzSeedAudio(tb testing.TB, hz int) []byte {
	slots, _ := benchStream(tb, 1)
	a := discAudio(noisyBaseband(slots, 500, 20, 13), hz)
	out := make([]byte, 2*len(a))
	for i, v := range a {
		binary.LittleEndian.PutUint16(out[2*i:], uint16(v))
	}
	return out
}

func FuzzDiscReceiver(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint8(0), uint16(0), uint64(0), fuzzSeedAudio(f, 24000))
	f.Add(uint8(1), uint8(1), uint8(1), uint16(100), uint64(3), fuzzSeedAudio(f, 12000))
	f.Add(uint8(2), uint8(2), uint8(2), uint16(4095), uint64(5), fuzzSeedAudio(f, 9600))
	f.Add(uint8(3), uint8(0), uint8(7), uint16(1), uint64(1), []byte{0xff, 0x7f, 0, 0x80, 1})
	f.Fuzz(func(t *testing.T, hzSel, tierSel, mode uint8, maxWrite uint16, chunkSeed uint64, b []byte) {
		cfg := AudioRxConfig{Rate: [4]int{0, 24000, 12000, 9600}[hzSel%4], Tier: [3]Tier{TierDefault, Tier0, Tier1}[tierSel%3]}
		most := fuzzMost(mode, uint32(maxWrite), 1<<12)
		init := mode&8 != 0 && cfg.Tier != Tier1
		b = b[:min(len(b), fuzzMaxBytes)]
		x := make([]int16, len(b)/2)
		for i := range x {
			x[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
		}
		var hold AudioReceiver
		run := func(seed uint64, sink *fuzzSink) uint64 {
			c := cfg
			stream := mode%3 == 0
			if stream {
				c.OnBursts = sink.add
			}
			r := &hold
			if init {
				if err := r.Init(c); err != nil {
					t.Fatal(err)
				}
			} else {
				var err error
				if r, err = NewAudioReceiver(&c); err != nil {
					t.Fatal(err)
				}
			}
			chunks := fuzzChunks(len(x), most, seed)
			dst := make([]Burst, 0, fuzzBursts)
			return mallocs(func() {
				p := 0
				for _, k := range chunks {
					if stream {
						if _, err := r.Write(x[p : p+k]); err != nil {
							panic(err)
						}
					} else {
						dst = r.AppendBursts(dst[:0], x[p:p+k])
						sink.add(dst)
					}
					p += k
				}
				if stream {
					if err := r.Flush(); err != nil {
						panic(err)
					}
				} else {
					sink.add(r.AppendFlush(dst[:0]))
				}
			})
		}
		hz := cfg.Rate
		if hz == 0 {
			hz = 2 * discRate
		}
		var one, chunked *fuzzSink
		if n := quiet(func() uint64 { one = newFuzzSink(); return run(0, one) }); n != 0 {
			t.Fatalf("%d allocations after setup", n)
		}
		if n := quiet(func() uint64 { chunked = newFuzzSink(); return run(chunkSeed|1, chunked) }); n != 0 {
			t.Fatalf("%d allocations after setup, in chunks", n)
		}
		checkBursts(t, one.bursts(), float64(len(x))/float64(hz))
		sameBursts(t, "chunks", one.bursts(), chunked.bursts())
	})
}

func FuzzRxSignal(f *testing.F) {
	f.Add(uint8(0), uint8(2), int16(0), int8(0), int8(0), int8(0), uint8(30), uint8(0), uint8(0), int8(0), uint64(1))
	f.Add(uint8(1), uint8(4), int16(1200), int8(20), int8(-50), int8(10), uint8(20), uint8(5), uint8(3), int8(2), uint64(2))
	f.Add(uint8(0), uint8(5), int16(-2500), int8(-40), int8(90), int8(-20), uint8(15), uint8(2), uint8(10), int8(-15), uint64(3))
	streams := rxStreams(f)
	f.Fuzz(func(t *testing.T, ms, recv uint8, offsetHz int16, driftHz, ppm, levelDB int8, cn, cw, gap uint8, jumpMs int8, seed uint64) {
		sn := "bs"
		if ms%2 == 1 {
			sn = "ms"
		}
		es := rxRecvs()
		e := es[int(recv)%len(es)]
		c := rxCond{name: "fuzz", pad: 0.3, audioGain: 1,
			offsetHz: float64(offsetHz%2000) * 1, driftHz: float64(driftHz) * 100 / 128, ppm: float64(ppm) * 100 / 128,
			level: math.Pow(10, float64(levelDB%41)/20), cw: float64(cw%11) / 10, gap: float64(gap%21) / 20,
			jump: float64(jumpMs%16) / 1000}
		snr := 15 + float64(cn%26)
		audio := e.mode == benchAudio
		in := rxBuild(streams[sn], c, snr, audio, seed)
		clean := rxBuild(streams[sn], rxConds()[0], snr, audio, seed)
		base := rxMatch(clean, rxReceive(t, clean, e))
		sc := rxMatch(rxEligible(in), rxReceive(t, in, e))
		ratio, br := float64(sc.exact)/float64(max(sc.total, 1)), float64(base.exact)/float64(max(base.total, 1))
		if ratio < rxFloor*br {
			t.Errorf("%s %s %+v at %g dB: %d of %d payloads (%.3f, clean %.3f)", e.name(), sn, c, snr, sc.exact, sc.total, ratio, br)
		}
		for i := range sc.found {
			if sc.found[i] > sc.bound[i] {
				t.Errorf("%s %s %+v at %g dB: found %s, bound %s", e.name(), sn, c, snr, rxSecs(sc.found), rxSecs(sc.bound))
			}
		}
	})
}
