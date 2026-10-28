package rf

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func monitorBursts(t *testing.T, slot int) []Burst {
	t.Helper()
	built, err := rftx.Sequence("voice", rftx.SeqConfig{ColorCode: 7, Source: 3191868, Dest: 9, BS: 314926, Group: true, Wakeups: 1,
		Superframes: 2, BSSourced: true, Slot: slot})
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.ModulateBS(built.Slots)
	x := rftx.Baseband(fm, env, 800)
	var out []Burst
	rx, err := NewReceiver(&RxConfig{Tier: Tier0, OnBursts: func(bs []Burst) { out = append(out, bs...) }, Rate: basebandRate, Format: IQFormatCF32})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rx.Write(cf32(x)); err != nil {
		t.Fatal(err)
	}
	if err := rx.Flush(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMonitorCall(t *testing.T) {
	t.Parallel()
	for slot, want := range []enums.Slot{enums.Slot1, enums.Slot2} {
		var m Monitor
		lb := layer2.NewBurst()
		voice, ends, other := 0, 0, 0
		for _, b := range monitorBursts(t, slot) {
			r, err := m.Burst(lb, &b)
			if err != nil {
				t.Fatalf("slot %d: %v", slot, err)
			}
			if r.Hint.Slot != b.Hint().Slot {
				t.Errorf("slot %d: Monitor gives %v, the CACH of the burst %v", slot, r.Hint.Slot, b.Hint().Slot)
			}
			if r.Hint.Slot != want {
				if lb.Kind == layer2.KindVoice {
					t.Errorf("slot %d: voice burst on %v", slot, r.Hint.Slot)
				}
				other++
				continue
			}
			if lb.Kind == layer2.KindVoice {
				voice++
				if !r.HasLC {
					t.Errorf("slot %d: voice burst without the call LC", slot)
				}
			}
			if r.HasLC {
				if g, dst, src, ok := r.LC.Call(); !ok || !g || dst != 9 || src != 3191868 {
					t.Errorf("slot %d: LC %x", slot, r.LC)
				}
			}
			if r.End {
				ends++
			}
		}
		if voice != 12 || ends == 0 || other == 0 {
			t.Errorf("slot %d: %d voice bursts, %d terminators, %d bursts on the other slot", slot, voice, ends, other)
		}
	}
}

func TestMonitorMisuse(t *testing.T) {
	t.Parallel()
	var nilM *Monitor
	nilM.Reset()
	lb := layer2.NewBurst()
	if r, err := nilM.Burst(lb, &Burst{}); !errors.Is(err, ErrNilBurst) || r != (MonitorBurst{}) {
		t.Errorf("nil Monitor: %+v, %v", r, err)
	}
	var m Monitor
	if _, err := m.Burst(lb, nil); !errors.Is(err, ErrNilBurst) {
		t.Errorf("nil Burst: %v", err)
	}
	if _, err := m.Burst(nil, &Burst{}); !errors.Is(err, ErrNilBurst) {
		t.Errorf("nil layer2.Burst: %v", err)
	}
	for _, b := range []Burst{{}, {BS: true}, {BS: true, HasCACH: true, Distance: 48, Stream: 3, Slot: -1}, {Voice: enums.VoiceBurstF, Distance: -1}} {
		_, _ = m.Burst(lb, &b)
	}
	m.Reset()
	var nilB *Burst
	if h := nilB.Hint(); h != (layer2.Hint{}) {
		t.Errorf("nil Burst Hint %+v", h)
	}
}

func TestMonitorAllocs(t *testing.T) {
	bursts := monitorBursts(t, 1)
	var m Monitor
	lb := layer2.NewBurst()
	i := 0
	allocs := testing.AllocsPerRun(len(bursts), func() {
		_, _ = m.Burst(lb, &bursts[i%len(bursts)])
		i++
	})
	if allocs != 0 {
		t.Errorf("%.1f allocations per burst", allocs)
	}
}
