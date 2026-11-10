//go:build !tinygo

package rf

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testscen"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type relockPart struct {
	bursts []layer2.RawBurst
	t0, hz float64
}

func relockSynth(parts []relockPart, fade [][2]float64, end float64) []complex64 {
	n := int(end * basebandRate)
	x := make([]complex64, n)
	for _, p := range parts {
		f, e := rftx.Modulate(p.bursts, rftx.Schedule{Slot: 0})
		k0 := int(math.Round(p.t0 * basebandRate))
		var ph float64
		for i := range f {
			ph += 2 * math.Pi * (f[i] + p.hz) / basebandRate
			if k := k0 + i; k >= 0 && k < n {
				x[k] += complex64(complex(e[i]*math.Cos(ph), e[i]*math.Sin(ph)))
			}
		}
	}
	for k := range x {
		t := float64(k) / basebandRate
		for _, f := range fade {
			if t >= f[0] && t < f[1] {
				x[k] = 0
			}
		}
	}
	return x
}

func relockStarts(t0 float64, n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = t0 + float64(2*i)*slotSeconds + (slotSeconds-burstSeconds)/2
	}
	return w
}

func relockCheck(t *testing.T, label string, x []complex64, want []float64) {
	t.Helper()
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		for _, audio := range []bool{false, true} {
			var got []Burst
			delay := 0.0
			if audio {
				r := mustAudio(AudioRxConfig{Rate: 48000, Tier: tier})
				got = r.AppendFlush(r.AppendBursts(nil, discAudio(x, 48000)))
				delay = secondsAudioLag / basebandRate
			} else {
				r := mustNew(RxConfig{Rate: basebandRate, Tier: tier, Format: IQFormatCS16})
				got = r.AppendFlush(r.AppendBursts(nil, iqBytes(x, IQFormatCS16)))
			}
			found := 0
			for _, w := range want {
				for _, b := range got {
					if math.Abs(b.Seconds+delay-w) < 20e-6 {
						found++
						break
					}
				}
			}
			if found != len(want) {
				t.Errorf("%s t%d audio %v: %d of %d bursts", label, tier-Tier0, audio, found, len(want))
			}
		}
	}
}

func TestLeanPayloadSync(t *testing.T) {
	t.Parallel()
	checkPayloadSync(t, randomizedN(t, 1234567, payloadSyncConfig))
}

var payloadSyncConfig = rftx.SeqConfig{ColorCode: 1, Source: 1234567, Dest: 9990, BS: 990911, Superframes: 1}

// TestLeanFadeKeepsPhase pins a payload whose burst after a two-slot fade
// was stamped 0.06 symbol early: the timing loop ran on the noise of the
// faded slots of the talker and moved the strobe phase.
func TestLeanFadeKeepsPhase(t *testing.T) {
	t.Parallel()
	checkPayloadSync(t, testscen.Of(17341948031809861460).Apply(payloadSyncConfig))
}

func checkPayloadSync(t *testing.T, cfg rftx.SeqConfig) {
	t.Helper()
	v, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	const lead = 0.05
	var p [8]byte
	binary.BigEndian.PutUint64(p[:], uint64(enums.SyncMSSourcedData)<<16)
	for lost := 1; lost <= 2; lost++ {
		vb := slices.Clone(v.Bursts)
		copy(vb[2+lost][0:6], p[:6])
		var fade [][2]float64
		var want []float64
		for i, w := range relockStarts(lead, len(vb)) {
			if i >= 2 && i < 2+lost {
				s := lead + float64(2*i)*slotSeconds
				fade = append(fade, [2]float64{s, s + slotSeconds})
				continue
			}
			want = append(want, w)
		}
		x := relockSynth([]relockPart{{vb, lead, 0}}, fade, lead+float64(2*len(vb))*slotSeconds+0.1)
		rng := rand.New(rand.NewPCG(uint64(lost), 9))
		for i := range x {
			x[i] += complex(float32(rng.NormFloat64()*0.01), float32(rng.NormFloat64()*0.01))
		}
		relockCheck(t, fmt.Sprintf("%d lost", lost), x, want)
	}
}

func TestLeanTalkerAfterDrop(t *testing.T) {
	t.Parallel()
	checkTalkerAfterDrop(t, randomizedN(t, 1234567, dropTalkers[0]), randomizedN(t, 1234568, dropTalkers[1]))
}

var dropTalkers = [2]rftx.SeqConfig{
	{ColorCode: 1, Source: 1234567, Dest: 9990, BS: 990911, Superframes: 1},
	{ColorCode: 1, Source: 1234568, Dest: 9990, BS: 990911, Message: "TALKER TWO DATA"},
}

// TestLeanWeakLockKeeps pins payloads whose voice header was lost on audio
// at Tiers 2 and 3: a weak lock on the header payload stopped keeping the
// acquisition samples, so the replay of the true lock could not reach the
// header.
func TestLeanWeakLockKeeps(t *testing.T) {
	t.Parallel()
	checkTalkerAfterDrop(t, testscen.Of(1394549278350625547).Apply(dropTalkers[0]), testscen.Of(5879030131196329451).Apply(dropTalkers[1]))
}

// TestLeanPayloadHoldsSlot pins payloads where a new talker's first SYNC,
// in the slot of a talker that had stopped, was dropped as a SYNC in that
// talker's payload: the payload at the EMB of the old slot decoded with two
// corrected bits, though the new SYNC showed no noise.
func TestLeanPayloadHoldsSlot(t *testing.T) {
	t.Parallel()
	checkTalkerAfterDrop(t, testscen.Of(5191531046375087081).Apply(dropTalkers[0]), testscen.Of(2832539846698043428).Apply(dropTalkers[1]))
}

func checkTalkerAfterDrop(t *testing.T, c1, c2 rftx.SeqConfig) {
	t.Helper()
	v, err := rftx.Sequence("voice", c1)
	if err != nil {
		t.Fatal(err)
	}
	d, err := rftx.Sequence("data12", c2)
	if err != nil {
		t.Fatal(err)
	}
	const lead = 0.05
	vb := v.Bursts[:len(v.Bursts)-1]
	for _, gap := range []float64{0.0555, 0.0592, 0.1147, 0.1295, 0.21} {
		t2 := lead + float64(2*len(vb))*slotSeconds + gap + 7.3/symbolRate
		x := relockSynth([]relockPart{{vb, lead, 0}, {d.Bursts, t2, 150}}, nil, t2+float64(2*len(d.Bursts))*slotSeconds+0.1)
		want := append(relockStarts(lead, len(vb)), relockStarts(t2, len(d.Bursts))...)
		relockCheck(t, fmt.Sprintf("gap %.4f s", gap), x, want)
	}
}

// TestLeanRCAnchorProof pins a standalone RC SYNC with a bad EMB in the
// flywheel after an MS voice call: the receiver took it as an anchor and
// moved its slot grid onto noise.
func TestLeanRCAnchorProof(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 1234567, Dest: 9990, Superframes: 2}
	call, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := rftx.Sequence("rc", cfg)
	if err != nil {
		t.Fatal(err)
	}
	rc := b.Bursts[len(b.Bursts)-1]
	for k := 84; k < 92 && !rc.StandaloneRCEMB().FEC.Uncorrectable; k++ {
		rc[k/8] ^= 0x80 >> (k % 8)
	}
	voice := call.Bursts[:len(call.Bursts)-1]
	const lead = 0.05
	for gap := 1; gap <= 3; gap++ {
		at := lead + float64(2*(len(voice)+gap))*slotSeconds
		x := relockSynth([]relockPart{{voice, lead, 0}, {[]layer2.RawBurst{rc}, at, 0}}, nil, at+slotSeconds+0.2)
		for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
			r := mustNew(RxConfig{Rate: basebandRate, Tier: tier, Format: IQFormatCS16})
			for _, g := range r.AppendFlush(r.AppendBursts(nil, iqBytes(x, IQFormatCS16))) {
				if g.Sync == enums.SyncMSSourcedRC {
					t.Errorf("gap %d, Tier%d: RC burst with a bad EMB at %.4f s", gap, tier-Tier0, g.Seconds)
				}
			}
		}
	}
}

func TestLeanLoneRCProof(t *testing.T) {
	t.Parallel()
	b, err := rftx.Sequence("rc", randomizedN(t, 1234567, rftx.SeqConfig{ColorCode: 1, Source: 1234567, Dest: 9990, BS: 990911}))
	if err != nil {
		t.Fatal(err)
	}
	rc := b.Bursts[len(b.Bursts)-1]
	flip := func(r layer2.RawBurst, from, to int, bad func(layer2.RawBurst) bool) layer2.RawBurst {
		for k := from; k < to && !bad(r); k++ {
			r[k/8] ^= 0x80 >> (k % 8)
		}
		if !bad(r) {
			t.Fatal("corruption left the field valid")
		}
		return r
	}
	badEMB := flip(rc, 84, 92, func(r layer2.RawBurst) bool { return r.StandaloneRCEMB().FEC.Uncorrectable })
	oneEMB := flip(rc, 84, 85, func(r layer2.RawBurst) bool { return r.StandaloneRCEMB().FEC.ErrorsCorrected == 1 })
	const lead = 0.05
	for _, c := range []struct {
		name  string
		burst layer2.RawBurst
		want  int
	}{{"valid", rc, 1}, {"bad EMB", badEMB, 0}, {"EMB with one bit error", oneEMB, 0}} {
		x := relockSynth([]relockPart{{[]layer2.RawBurst{c.burst}, lead, 0}}, nil, lead+slotSeconds+0.1)
		for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
			r := mustNew(RxConfig{Rate: basebandRate, Tier: tier, Format: IQFormatCS16})
			n := 0
			for _, g := range r.AppendFlush(r.AppendBursts(nil, iqBytes(x, IQFormatCS16))) {
				if g.Sync == enums.SyncMSSourcedRC {
					n++
				}
			}
			if n != c.want {
				t.Errorf("%s t%d: %d RC bursts, want %d", c.name, tier-Tier0, n, c.want)
			}
		}
	}
}

func TestLeanFadeNoBursts(t *testing.T) {
	t.Parallel()
	cfg := randomizedN(t, 1234567, rftx.SeqConfig{ColorCode: 1, Source: 1234567, Dest: 9990, BS: 990911, Superframes: 1})
	v, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	const lead = 0.05
	var fade [][2]float64
	for i := 2; i < 4; i++ {
		s := lead + float64(2*i)*slotSeconds
		fade = append(fade, [2]float64{s, s + slotSeconds})
	}
	clean := relockSynth([]relockPart{{v.Bursts, lead, 0}}, fade, lead+float64(2*len(v.Bursts))*slotSeconds+0.1)
	for _, sigma := range []float64{0, 1e-3} {
		x := append([]complex64(nil), clean...)
		rng := rand.New(rand.NewPCG(7, 8))
		for i := range x {
			x[i] += complex(float32(rng.NormFloat64()*sigma), float32(rng.NormFloat64()*sigma))
		}
		for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
			for _, audio := range []bool{false, true} {
				if audio && sigma > 0 {
					continue
				}
				var got []Burst
				delay := 0.0
				if audio {
					r := mustAudio(AudioRxConfig{Rate: 48000, Tier: tier})
					got = r.AppendFlush(r.AppendBursts(nil, discAudio(x, 48000)))
					delay = secondsAudioLag / basebandRate
				} else {
					r := mustNew(RxConfig{Rate: basebandRate, Tier: tier, Format: IQFormatCS16})
					got = r.AppendFlush(r.AppendBursts(nil, iqBytes(x, IQFormatCS16)))
				}
				for _, b := range got {
					at := b.Seconds + delay
					for _, f := range fade {
						if at > f[0]-1e-3 && at < f[1] {
							t.Errorf("sigma %g t%d audio %v: burst at %.4f s in the fade %.3f to %.3f s", sigma, tier-Tier0, audio, at, f[0], f[1])
						}
					}
				}
			}
		}
	}
}

func TestRxTwoTalkerLevelStep(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "mid.none.-1500.ms2.pi.-500ppm.nom.cs16-48k.40dB.step10dB")
	rxs := mxReceivers(row.val(mxFormat), true)
	res, err := mxRun(t, row, rxs)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range rxs {
		if r := res[x]; r.fail["missing"] {
			t.Errorf("%s: %s", x.name(), strings.Join(r.detail, " | "))
		}
	}
}

// TestRxAudioTwoTalkerLevelStep steps the level of an audio input by 10 dB
// between two SYNCs of two talkers on a 2500 Hz offset. Every tier decodes the
// flywheel bursts after the step: the step refits the levels of both slots
// once and gives the phase stage of Tiers 2 and 3 its new unit.
func TestRxAudioTwoTalkerLevelStep(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "gap.none.+2500.ms2.pi.-500ppm.nom.a12000.40dB.step10dB")
	rxs := mxReceivers(row.val(mxFormat), true)
	res, err := mxRun(t, row, rxs)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range rxs {
		if r := res[x]; r.fail["bits"] || r.fail["missing"] {
			t.Errorf("%s: %s", x.name(), strings.Join(r.detail, " | "))
		}
	}
}

// TestRxTwoTalkerSyncLevelStep steps the level of an audio input by 10 dB
// between two talkers whose bursts all carry a SYNC. The SYNC of each talker
// sees the step, but the phase stage of Tiers 2 and 3 takes it once.
func TestRxTwoTalkerSyncLevelStep(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "mid.none.+700.ms2.data12.+500ppm.nom.a12000.40dB.step10dB")
	rxs := []mxRx{{row.val(mxFormat), Tier2}, {row.val(mxFormat), Tier3}}
	res, err := mxRun(t, row, rxs)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range rxs {
		if r := res[x]; r.fail["bits"] || r.fail["missing"] {
			t.Errorf("%s: %s", x.name(), strings.Join(r.detail, " | "))
		}
	}
}

// TestRxTwoTalkerClipStep raises the level of an audio input by 10 dB just
// before the first burst of the second talker, enough to clip the symbol ring
// of Tiers 0 and 1. The gain drops before the SYNC of that burst, so the burst
// is found.
func TestRxTwoTalkerClipStep(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "gap.none.+2500.ms2.data1.-500ppm.nom.a12000.40dB.step10dB")
	rxs := []mxRx{{row.val(mxFormat), Tier0}, {row.val(mxFormat), Tier1}}
	res, err := mxRun(t, row, rxs)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range rxs {
		if r := res[x]; r.fail["bits"] || r.fail["missing"] {
			t.Errorf("%s: %s", x.name(), strings.Join(r.detail, " | "))
		}
	}
}

// TestRxTwoTalkerSyncLevelStepHead steps the level of an audio input by
// -10 dB a third of the way into a burst of one of two talkers. The phase
// stage of Tiers 2 and 3 must keep the phase of the samples before the step
// when it takes the new level at the next SYNC, so the head of that burst
// decodes at its own level.
func TestRxTwoTalkerSyncLevelStepHead(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "mid.none.+700.ms2.data12.+500ppm.nom.a12000.40dB.step10dB")
	rxs := []mxRx{{row.val(mxFormat), Tier2}, {row.val(mxFormat), Tier3}}
	for _, run := range []uint64{1, 2, 5, 10, 12, 13, 15, 17, 19, 20, 21, 24, 25, 27, 30, 35, 40, 44} {
		res, err := mxRun(pinnedTB{t, run}, row, rxs)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range rxs {
			if r := res[x]; r.fail["bits"] || r.fail["missing"] {
				t.Errorf("run %d %s: %s", run, x.name(), strings.Join(r.detail, " | "))
			}
		}
	}
}

// TestRxAudioTwoTalkerLevelStepBlind steps an audio input of two talkers on a
// 2500 Hz offset by -10 dB just before a voice burst without a SYNC of the
// weaker talker. The step moves the level and the DC of that burst, and the
// core must refit them from the burst's own symbols, whatever the balance of
// its payload.
func TestRxAudioTwoTalkerLevelStepBlind(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "gap.none.+2500.ms2.pi.-500ppm.nom.a12000.40dB.step10dB")
	rxs := mxReceivers(row.val(mxFormat), true)
	for _, run := range []uint64{10, 16, 22, 34, 53, 54, 56, 57, 60, 61} {
		res, err := mxRun(pinnedTB{t, run}, row, rxs)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range rxs {
			if r := res[x]; r.fail["bits"] || r.fail["missing"] {
				t.Errorf("run %d %s: %s", run, x.name(), strings.Join(r.detail, " | "))
			}
		}
	}
}

// TestRxCarrierLeadFirstBurstPhase holds a 500 ms carrier ahead of an MS
// voice call on 9600 Hz audio. The phase stage of Tiers 2 and 3 takes its
// first phase scale from the lock levels, and the carrier fit of the first
// burst must follow the scale that the burst fits, so the bits of the first
// burst come out exact.
func TestRxCarrierLeadFirstBurstPhase(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "gap.cw500ms.-4000.ms.voice.-500ppm.nom.a9600.40dB")
	rxs := []mxRx{{row.val(mxFormat), Tier2}, {row.val(mxFormat), Tier3}}
	for _, run := range []uint64{4, 7, 18, 37, 43, 46, 53, 69, 73, 86} {
		res, err := mxRun(pinnedTB{t, run}, row, rxs)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range rxs {
			if r := res[x]; len(r.symptoms()) > 0 {
				t.Errorf("run %d %s: %v %s", run, x.name(), r.symptoms(), strings.Join(r.detail, " | "))
			}
		}
	}
}
