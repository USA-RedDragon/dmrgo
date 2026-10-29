//go:build !tinygo

package rf

import (
	"errors"
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func slcKey(sec float64) int64 {
	ramp := (slotSeconds - burstSeconds) / 2
	return int64(math.Round((sec - 0.3 - ramp - float64(cachSymbols)/symbolRate) / slotSeconds))
}

func slcWant(slots []rftx.BSSlot) map[int64]pdu.ShortLC {
	m := map[int64]pdu.ShortLC{}
	for g := 0; 4*g+3 < len(slots); g++ {
		var w [4]uint32
		for j := range w {
			w[j] = cachPayloadWord(layer2.DecodeCACHField(slots[4*g+j].CACH).Payload)
		}
		v, _ := bptc.DecodeCACH(w)
		v <<= 4
		m[int64(4*g+3)] = pdu.DecodeShortLC([5]byte{byte(v >> 32), byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
	}
	return m
}

func slcReceive(t testing.TB, x []complex64, e benchEntry) ([]Burst, []ShortLC) {
	t.Helper()
	var bursts []Burst
	var lcs []ShortLC
	on := func(l []ShortLC) { lcs = append(lcs, l...) }
	switch e.mode {
	case benchAudio:
		a := discAudio(x, benchAudioHz)
		r := mustAudio(AudioRxConfig{Rate: benchAudioHz, Tier: e.tier, OnBursts: each(func(b *Burst) { bursts = append(bursts, *b) }), OnShortLC: on})
		for k := 0; k < len(a); k += 1024 {
			must(r.writeAll(a[k:min(k+1024, len(a))]))
		}
		r.flushAll()
	case benchIQLow:
		y := lowIQ(x, iq0LowHz)
		r := mustNew(RxConfig{Tier: Tier0, Rate: iq0LowHz, Format: IQFormatCU8, OnShortLC: on})
		for k := 0; k < len(y); k += 1024 {
			bursts = append(bursts, must(r.writeBB(y[k:min(k+1024, len(y))]))...)
		}
		bursts = append(bursts, r.flushAll()...)
	default:
		r := mustNew(RxConfig{Rate: basebandRate, Tier: e.tier, Format: IQFormatCU8, OnShortLC: on})
		for k := 0; k < len(x); k += 4096 {
			bursts = append(bursts, must(r.writeBB(x[k:min(k+4096, len(x))]))...)
		}
		bursts = append(bursts, r.flushAll()...)
	}
	return bursts, lcs
}

func slcHardOf(bursts []Burst) []ShortLC {
	var a layer2.ShortLCAssembler
	var out []ShortLC
	stream := 0
	for i := range bursts {
		b := &bursts[i]
		if !b.HasCACH {
			continue
		}
		if b.Stream != stream {
			a.Reset()
			stream = b.Stream
		}
		_, c := b.Decoded()
		if lc, done, err := a.Add(layer2.DecodeCACHField(c)); err == nil && done && !lc.FEC.Uncorrectable {
			out = append(out, ShortLC{LC: lc, Stream: b.Stream, Slot: b.Slot, Seconds: b.Seconds})
		}
	}
	return out
}

func slcSame(a, b pdu.ShortLC) bool {
	return a.SLCO == b.SLCO && a.RawPayload == b.RawPayload
}

func slcCount(lcs []ShortLC, want map[int64]pdu.ShortLC) (good, bad int) {
	for _, l := range lcs {
		if w, ok := want[slcKey(l.Seconds)]; ok && slcSame(w, l.LC) {
			good++
		} else {
			bad++
		}
	}
	return good, bad
}

func TestShortLCClean(t *testing.T) {
	slots, _ := benchStream(t, 6)
	want := slcWant(slots)
	x := cleanBaseband(slots, 1400).noisy(40, 1)
	for _, e := range benchEntries() {
		t.Run(e.name(), func(t *testing.T) {
			bursts, lcs := slcReceive(t, x, e)
			hard := slcHardOf(bursts)
			good, bad := slcCount(lcs, want)
			if bad != 0 || good < len(want)-1 {
				t.Errorf("Short LCs: %d right, %d wrong, of %d", good, bad, len(want))
			}
			var hl []ShortLC
			for _, l := range lcs {
				if !l.Soft {
					hl = append(hl, l)
				}
			}
			if len(hl) != len(hard) {
				t.Fatalf("got %d hard Short LCs, hard decode gives %d", len(hl), len(hard))
			}
			for i := range hl {
				if !slcSame(hl[i].LC, hard[i].LC) || hl[i].Seconds != hard[i].Seconds || hl[i].Slot != hard[i].Slot || hl[i].Stream != hard[i].Stream {
					t.Errorf("LC %d: got %+v, hard %+v", i, hl[i], hard[i])
				}
			}
			if s := len(lcs) - len(hl); s > 0 {
				t.Logf("%d soft Short LCs where the hard decode fails", s)
			}
		})
	}
}

func TestShortLCSoft(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	slots, _ := benchStream(t, 30)
	want := slcWant(slots)
	sig := cleanBaseband(slots, 1400)
	for _, e := range []benchEntry{{benchAudio, Tier1}, {benchIQ, Tier1}} {
		t.Run(e.name(), func(t *testing.T) {
			var good, bad, hGood, hBad, soft int
			for _, cn := range []float64{3, 4, 5} {
				for s := range uint64(2) {
					bursts, lcs := slcReceive(t, sig.noisy(cn, 900+s), e)
					g, b := slcCount(lcs, want)
					hg, hb := slcCount(slcHardOf(bursts), want)
					good, bad, hGood, hBad = good+g, bad+b, hGood+hg, hBad+hb
					for _, l := range lcs {
						if l.Soft {
							soft++
						}
					}
				}
			}
			t.Logf("%s: OnShortLC %d right %d wrong (%d soft), hard %d right %d wrong, of %d", e.name(), good, bad, soft, hGood, hBad, 6*len(want))
			if good <= hGood || bad > hBad {
				t.Errorf("OnShortLC %d right %d wrong, hard %d right %d wrong", good, bad, hGood, hBad)
			}
		})
	}
}

func TestShortLCConfig(t *testing.T) {
	on := func([]ShortLC) {}
	var r Receiver
	var ce *dmrgo.ConfigError
	if err := r.Init(RxConfig{Rate: iq0LowHz, OnShortLC: on}); !errors.As(err, &ce) || ce.Field != "RxConfig.OnShortLC" {
		t.Errorf("Receiver.Init: %v", err)
	}
	var a AudioReceiver
	if err := a.Init(AudioRxConfig{OnShortLC: on}); !errors.As(err, &ce) || ce.Field != "AudioRxConfig.OnShortLC" {
		t.Errorf("AudioReceiver.Init: %v", err)
	}
	for tier := Tier0; tier <= Tier3; tier++ {
		r, err := NewReceiver(&RxConfig{Tier: tier, OnShortLC: on})
		if err != nil || r.sink.slc == nil || (r.sink.slc.soft != nil) != (tier != Tier0) {
			t.Errorf("NewReceiver %v: %v", tier, err)
		}
		ar, err := NewAudioReceiver(&AudioRxConfig{Tier: tier, OnShortLC: on})
		if err != nil || ar.bursts.slc == nil || (ar.bursts.slc.soft != nil) != (tier != Tier0) {
			t.Errorf("NewAudioReceiver %v: %v", tier, err)
		}
	}
	if r, _ := NewReceiver(nil); r.sink.slc != nil {
		t.Error("NewReceiver(nil) set up a Short LC output")
	}
}
