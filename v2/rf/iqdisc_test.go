package rf

import (
	"math"
	"strconv"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// TestIQDiscBurstyOffset receives MS voice calls, whose carrier is on in
// one slot of two, at a carrier offset of 900 Hz through the I/Q tiers
// that run iqRx, and checks that the parts C0 and C1 of every voice frame
// of every call decode: the oscillator moves while the core searches,
// often right before the first SYNC of a call, and the core must follow
// the move.
func TestIQDiscBurstyOffset(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.ColorCode, cfg.Superframes, cfg.Wakeups, cfg.Slot = 1, 2, 0, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	const calls = 3
	gap := make([]float64, int(0.3*basebandRate))
	var fmAll, envAll []float64
	want := make([]uint64, 0, calls*18*cfg.Superframes)
	for c := range calls {
		if c > 0 {
			fmAll, envAll = append(fmAll, gap...), append(envAll, gap...)
		}
		fmAll, envAll = append(fmAll, fm...), append(envAll, env...)
		for i := range 18 * cfg.Superframes {
			want = append(want, rftx.AMBEValue(rftx.TestAMBEFrame(i)))
		}
	}
	for _, tier := range []Tier{Tier0, Tier1} {
		for seed := range uint64(4) {
			t.Run(strconv.Itoa(int(tier-Tier0))+"/"+strconv.Itoa(int(seed)), func(t *testing.T) {
				t.Parallel()
				x := msBaseband(fmAll, envAll, 900, 30, 100+seed)
				if n := c0c1InOrder(want, voiceFrames(receiveBaseband(x, tier))); n < len(want) {
					t.Errorf("C0 and C1 of %d of %d voice frames exact", n, len(want))
				}
			})
		}
	}
}

// TestIQDiscDataBias receives MS rate 1 data calls on the carrier through
// the I/Q tiers that run iqRx, and checks that the oscillator stays on the
// carrier and that every rate 1 block is exact. Rate 1 data has no
// whitening, and the ASCII of its payload moves the mean frequency of a
// block by about 360 Hz: a move of the oscillator on that mean put a step
// into the middle of a burst, where a block without FEC lost a bit.
func TestIQDiscDataBias(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.ColorCode, cfg.Wakeups, cfg.Slot, cfg.Preambles = 1, 0, 1, 2
	cfg.Message = "DMRGO SIMCMP CALL 11 THE QUICK BROWN FOX JUMPS OVER THE LAZY DOG 0123456789"
	built, err := rftx.Sequence("data1", cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := map[[24]byte]bool{}
	wantN := 0
	for _, b := range built.Bursts {
		if x, err := layer2.NewBurstFromBytes(b); err == nil && x.Kind == layer2.KindData && x.SlotType.DataType == enums.DataTypeRate1 {
			p, _ := b.Rate1Payload()
			want[p] = true
			wantN++
		}
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	const calls = 4
	gap := make([]float64, int(0.3*basebandRate))
	fmAll, envAll := make([]float64, 0, calls*(len(gap)+len(fm))), make([]float64, 0, calls*(len(gap)+len(env)))
	for range calls {
		fmAll, envAll = append(fmAll, gap...), append(envAll, gap...)
		fmAll, envAll = append(fmAll, fm...), append(envAll, env...)
	}
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		t.Run(strconv.Itoa(int(tier-Tier0)), func(t *testing.T) {
			t.Parallel()
			x := msBaseband(fmAll, envAll, 0, 30, 7)
			r := mustNew(Config{Rate: basebandRate, Tier: tier})
			var got []Burst
			var worst float32
			for k := 0; k < len(x); k += 960 {
				got = append(got, r.WriteBaseband(x[k:min(k+960, len(x))])...)
				worst = max(worst, abs32(r.iqd.conv.hz))
			}
			got = append(got, r.Flush()...)
			if worst > iqAFCLockHz {
				t.Errorf("oscillator %.0f Hz off the carrier", worst)
			}
			n := 0
			for i := range got {
				d, _ := got[i].Decoded()
				x, err := layer2.NewBurstFromBytes(d)
				if err != nil || x.Kind != layer2.KindData || x.SlotType.DataType != enums.DataTypeRate1 {
					continue
				}
				if p, _ := d.Rate1Payload(); want[p] {
					n++
				}
			}
			if total := calls * wantN; n != total {
				t.Errorf("%d of %d rate 1 blocks exact", n, total)
			}
		})
	}
}

// voiceFrames returns the AMBE+2 frames of the voice bursts of bs.
func voiceFrames(bs []Burst) []uint64 {
	var out []uint64
	for i := range bs {
		var opts layer2.DecodeOptions
		if bs[i].Voice != 0 {
			opts.Kind, opts.VoiceBurst = layer2.KindVoice, bs[i].Voice
		}
		data, _ := bs[i].Decoded()
		var b layer2.Burst
		if err := b.DecodeFromBytesWith(data, &opts); err != nil || b.Kind != layer2.KindVoice {
			continue
		}
		frames, _ := data.AMBEFrames()
		for _, f := range frames {
			out = append(out, rftx.AMBEValue(f))
		}
	}
	return out
}

// c0c1InOrder returns the number of frames of want whose parts C0 and C1,
// the first 24 bits, which the Golay codes protect, got holds in order.
func c0c1InOrder(want, got []uint64) int {
	n, next := 0, 0
	for _, v := range got {
		for k := next; k < len(want); k++ {
			if want[k]>>25 == v>>25 {
				n, next = n+1, k+1
				break
			}
		}
	}
	return n
}

// TestIQDiscAdjacent receives MS voice calls, whose carrier is on in one
// slot of two and 300 Hz above the channel, next to a continuous BS
// stream 12.5 kHz above them and 40 dB stronger, through iqRx. In the gaps
// between the bursts the edge of the channel filter holds only the other
// channel, which pulls the mean frequency onto it. The test checks that
// the parts C0 and C1 of every voice frame decode, that the oscillator,
// held within 2500 Hz and moved only by the SYNC fits while locked, stays
// within iqAFCLockHz of the carrier, and that without the hold the mean
// moves it farther toward the other channel.
func TestIQDiscAdjacent(t *testing.T) {
	t.Parallel()
	for _, held := range []bool{false, true} {
		o := discOptions()
		o.IQDisc, o.IQHz = true, 12000
		if held {
			o.IQTuneMaxHz, o.IQTuneSyncOnly = 2500, true
		}
		n, want, off := adjacentReceive(t, Tier0, &o, 300)
		if n < want {
			t.Errorf("held %v: C0 and C1 of %d of %d voice frames exact", held, n, want)
		}
		switch {
		case held && off > iqAFCLockHz:
			t.Errorf("held: oscillator %.0f Hz off the carrier while locked", off)
		case !held && off <= iqAFCLockHz:
			t.Errorf("not held: oscillator at most %.0f Hz off the carrier; the test no longer shows the pull", off)
		}
	}
}

// TestEngineAdjacent receives the streams of TestIQDiscAdjacent on Tier3,
// with its channel held within 2500 Hz and a channel filter to 6 kHz, and
// checks that C0 and C1 of every voice frame decode; its own channel
// filter, flat to 7.5 kHz, lets the other channel in.
func TestEngineAdjacent(t *testing.T) {
	t.Parallel()
	for _, held := range []bool{false, true} {
		o := tierOptions(Tier3)
		if held {
			o.IQTuneMaxHz, o.IQCutHz, o.IQKaiser = 2500, 6000, 6
		}
		n, want, _ := adjacentReceive(t, Tier3, &o, 0)
		switch {
		case held && n < want:
			t.Errorf("held: C0 and C1 of %d of %d voice frames exact", n, want)
		case !held && n == want:
			t.Errorf("not held: all %d voice frames exact; the test no longer shows the other channel", n)
		}
	}
}

// adjacentReceive receives the MS voice calls of TestIQDiscAdjacent at
// offsetHz next to the stronger BS stream on tier with the settings o. It
// returns the number of voice frames whose C0 and C1 decode in order, the
// number sent, and the largest distance in Hz of the I/Q oscillator from
// the carrier while the core is locked, 0 without iqRx.
func adjacentReceive(t *testing.T, tier Tier, o *options, offsetHz float64) (n, want int, off float32) {
	t.Helper()
	cfg := testConfig()
	cfg.ColorCode, cfg.Superframes, cfg.Wakeups, cfg.Slot = 1, 2, 0, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	const calls = 3
	gap := make([]float64, int(0.3*basebandRate))
	var fmAll, envAll []float64
	frames := make([]uint64, 0, calls*18*cfg.Superframes)
	for c := range calls {
		if c > 0 {
			fmAll, envAll = append(fmAll, gap...), append(envAll, gap...)
		}
		fmAll, envAll = append(fmAll, fm...), append(envAll, env...)
		for i := range 18 * cfg.Superframes {
			frames = append(frames, rftx.AMBEValue(rftx.TestAMBEFrame(i)))
		}
	}
	ic := testConfig()
	ic.ColorCode, ic.Superframes, ic.Wakeups, ic.Slot, ic.BSSourced = 5, 4, 0, 0, true
	ib, err := rftx.Sequence("voice", ic)
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := rftx.ModulateBS(ib.Slots)
	x := msBaseband(fmAll, envAll, offsetHz, 20, 7)
	var ph float64
	for i := range x {
		ph += 2 * math.Pi * (fi[i%len(fi)] + 12500) / basebandRate
		x[i] += complex(float32(100*math.Cos(ph)), float32(100*math.Sin(ph)))
	}
	var out []Burst
	r := mustNew(Config{Rate: basebandRate, Tier: tier, opt: o, OnBurst: func(b *Burst) { out = append(out, *b) }})
	for k := 0; k < len(x); k += 960 {
		r.WriteBaseband(x[k:min(k+960, len(x))])
		if r.iqd != nil && r.iqd.d.locked {
			off = max(off, abs32(r.iqd.conv.hz-float32(offsetHz)))
		}
	}
	r.Flush()
	return c0c1InOrder(frames, voiceFrames(out)), len(frames), off
}

// iqDiscReceive runs the baseband x through iqRx at hz with OnBurst.
func iqDiscReceive(x []complex64, hz int) []Burst {
	o := discOptions()
	o.IQDisc, o.IQHz = true, hz
	var out []Burst
	r := mustNew(Config{Rate: basebandRate, Tier: Tier0, opt: &o, OnBurst: func(b *Burst) { out = append(out, *b) }})
	for k := 0; k < len(x); k += 4096 {
		r.WriteBaseband(x[k:min(k+4096, len(x))])
	}
	r.Flush()
	return out
}

// TestIQDiscLoopback receives a strong BS stream with a 1400 Hz carrier
// offset through iqRx at both discriminator rates and checks that every
// voice frame and data block after the first slot decodes, and that the
// oscillator found the offset.
func TestIQDiscLoopback(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 30, 3)
	for _, hz := range []int{discRate, discRate / 2} {
		sc := scoreBench(truth, iqDiscReceive(x, hz))
		for _, k := range []string{"ambe", "rate12", "rate34", "csbk", "emb", "slottype"} {
			// The receiver locks to the SYNC of the first slot, which it
			// does not decode.
			if s := sc[k]; s.Truth-s.Exact > 3 {
				t.Errorf("%d Hz: %s %d of %d exact", hz, k, s.Exact, s.Truth)
			}
		}
	}
	o := discOptions()
	o.IQDisc = true
	r := mustNew(Config{Rate: basebandRate, Tier: Tier0, opt: &o, OnBurst: func(*Burst) {}})
	r.WriteBaseband(x)
	if got := float64(r.iqd.conv.hz); got < 1300 || got > 1500 {
		t.Errorf("oscillator at %.0f Hz, want about 1400", got)
	}
}
