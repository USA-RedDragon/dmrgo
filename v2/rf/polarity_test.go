//go:build !tinygo

package rf

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func (r *discRx) inverted() bool {
	return r.f.polBits&polInv != 0
}

func (r *AudioReceiver) acOf() *acStage {
	if e, _ := r.ext.(*softExt); e != nil {
		return e.ac
	}
	return nil
}

func (r *AudioReceiver) plain() {
	r.f.polBits = 0
	if e, _ := r.ext.(*softExt); e != nil {
		e.ac = nil
		if e.union != nil {
			for _, w := range e.union.twins {
				w.f.polBits = 0
			}
		}
	}
}

func receivePlain(t testing.TB, a []int16, hz int, tier Tier) []Burst {
	t.Helper()
	out, _ := receivePol(t, a, hz, tier, true)
	return out
}

func receiveFixed(t testing.TB, a []int16, hz int, tier Tier) ([]Burst, *AudioReceiver) {
	t.Helper()
	return receivePol(t, a, hz, tier, false)
}

func receivePol(t testing.TB, a []int16, hz int, tier Tier, plain bool) ([]Burst, *AudioReceiver) {
	t.Helper()
	var out []Burst
	r, err := NewAudioReceiver(&AudioRxConfig{Rate: hz, Tier: tier, OnBursts: each(func(b *Burst) { out = append(out, *b) })})
	if err != nil {
		t.Fatal(err)
	}
	if plain {
		r.plain()
	}
	for k := 0; k < len(a); k += 1024 {
		must(r.writeAll(a[k:min(k+1024, len(a))]))
	}
	r.flushAll()
	return out, r
}

func TestDiscInverted(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 20, 3)
	for _, hz := range []int{24000, 12000, 9600} {
		a := discAudio(x, hz)
		for i, v := range a {
			a[i] = -max(v, -math.MaxInt16)
		}
		for _, tier := range []Tier{Tier0, Tier1, Tier2} {
			base := scoreBench(truth, receivePlain(t, a, hz, tier))
			got, r := receiveFixed(t, a, hz, tier)
			sc := scoreBench(truth, got)
			if !r.inverted() {
				t.Errorf("%d Hz %v: inverted audio not detected", hz, tier)
			}
			for _, k := range []string{"csbk", "rate12", "rate34", "ambe"} {
				if b := base[k]; b.Exact > b.Truth/10 {
					t.Errorf("%d Hz %v, %s: %d of %d without the polarity check", hz, tier, k, b.Exact, b.Truth)
				}
				if s := sc[k]; s.Truth == 0 || s.Exact < s.Truth*7/10 {
					t.Errorf("%d Hz %v, %s: %d of %d recovered from inverted audio", hz, tier, k, s.Exact, s.Truth)
				}
			}
		}
	}
}

func TestDiscPolarityStays(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 3)
	for _, hz := range []int{24000, 12000, 9600} {
		for _, in := range [][]complex64{noisyBaseband(slots, 1400, 30, 1), noisyBaseband(slots, -900, 3, 2)} {
			a := discAudio(in, hz)
			want := receivePlain(t, a, hz, Tier1)
			got, r := receiveFixed(t, a, hz, Tier1)
			if r.inverted() {
				t.Errorf("%d Hz: audio of the right polarity taken as inverted", hz)
			}
			if len(got) != len(want) {
				t.Fatalf("%d Hz: %d bursts with the polarity check, %d without", hz, len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("%d Hz: burst %d differs with the polarity check", hz, i)
				}
			}
		}
	}
}

func msVoiceAudio(t *testing.T, hz int, inv bool) []int16 {
	t.Helper()
	b, err := rftx.Sequence("voice", rftx.SeqConfig{ColorCode: 7, Source: 3191868, Dest: 9, Group: true, Superframes: 12, Slot: 1})
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(b.Bursts, rftx.Schedule{Slot: 1})
	pad := int(0.3 * basebandRate)
	x := make([]complex64, len(fm)+2*pad)
	rng := rand.New(rand.NewPCG(5, 6))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, 2.5)*12500)) / math.Sqrt2
	var ph float64
	for i := range x {
		var re, im float64
		if k := i - pad; k >= 0 && k < len(fm) {
			ph += 2 * math.Pi * (fm[k] + 700) / basebandRate
			re, im = env[k]*math.Cos(ph), env[k]*math.Sin(ph)
		}
		x[i] = complex(float32(re+rng.NormFloat64()*sigma), float32(im+rng.NormFloat64()*sigma))
	}
	a := discAudio(x, hz)
	for i, v := range a {
		if inv {
			a[i] = -max(v, -math.MaxInt16)
		}
	}
	return a
}

func TestDiscInvertedVoiceOnly(t *testing.T) {
	t.Parallel()
	for _, hz := range []int{24000, 9600} {
		a := msVoiceAudio(t, hz, true)
		for _, tier := range []Tier{Tier0, Tier1, Tier2} {
			if _, r := receiveFixed(t, msVoiceAudio(t, hz, false), hz, tier); r.inverted() {
				t.Errorf("%d Hz %v: voice of the right polarity taken as inverted", hz, tier)
			}
		}
		for _, tier := range []Tier{Tier0, Tier1, Tier2} {
			voice := 0
			for _, b := range receivePlain(t, a, hz, tier) {
				if b.Sync == enums.SyncMSSourcedVoice {
					voice++
				}
			}
			got, r := receiveFixed(t, a, hz, tier)
			fixed := 0
			for _, b := range got {
				if b.Sync == enums.SyncMSSourcedVoice {
					fixed++
				}
			}
			if !r.inverted() || voice > 2 || fixed < 8 {
				t.Errorf("%d Hz %v: inverted %v, voice SYNCs %d without the check and %d with it", hz, tier, r.inverted(), voice, fixed)
			}
		}
	}
}

func TestDiscInvertedInit(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 3)
	a := discAudio(noisyBaseband(slots, 1400, 20, 3), 9600)
	for i, v := range a {
		a[i] = -max(v, -math.MaxInt16)
	}
	var r AudioReceiver
	n := 0
	if err := r.Init(AudioRxConfig{Rate: 9600, OnBursts: each(func(b *Burst) {
		if b.Sync == enums.SyncBSSourcedData {
			n++
		}
	})}); err != nil {
		t.Fatal(err)
	}
	for k := 0; k < len(a); k += 1024 {
		if _, err := r.Write(a[k:min(k+1024, len(a))]); err != nil {
			t.Fatal(err)
		}
	}
	if !r.inverted() || n == 0 {
		t.Errorf("Init: inverted %v, %d data SYNCs from inverted audio", r.inverted(), n)
	}
}
