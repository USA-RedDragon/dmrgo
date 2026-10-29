package rf

import (
	"fmt"
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type onsetTruth struct {
	at   float64
	data layer2.RawBurst
	cach [3]byte
	bs   bool
}

func onsetReceivers() []levelsReceiver {
	return append(levelsReceivers(), levelsReceiver{0, Tier2})
}

func onsetBits(t *testing.T, what string, truth []onsetTruth, got []Burst, period float64) (lockBad bool) {
	t.Helper()
	byIndex := map[int]Burst{}
	for _, b := range got {
		i := int(math.Round((b.Seconds - truth[0].at) / period))
		if i >= 0 && i < len(truth) && math.Abs(b.Seconds-truth[i].at) < 0.002 {
			byIndex[i] = b
		}
	}
	if len(byIndex) < len(truth)-1 {
		t.Errorf("%s: %d of %d bursts", what, len(byIndex), len(truth))
	}
	first := true
	for i, w := range truth {
		b, ok := byIndex[i]
		if !ok {
			continue
		}
		bad := b.Data != w.data
		if w.bs && b.HasCACH && b.CACH != w.cach {
			t.Errorf("%s: burst %d CACH %x, want %x", what, i, b.CACH, w.cach)
		}
		if first {
			lockBad, first = bad, false
		} else if bad {
			t.Errorf("%s: burst %d raw bits differ", what, i)
		}
	}
	return lockBad
}

func TestRxRawBitsAfterCarrier(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 2)
	fm, env := rftx.ModulateBS(slots)
	for i := 0; env[i] < 1; i++ {
		env[i] = 1
	}
	for _, cn := range []float64{math.Inf(1)} {
		for _, lr := range onsetReceivers() {
			t.Run(fmt.Sprintf("%s/%.0f", lr.name(), cn), func(t *testing.T) {
				t.Parallel()
				for _, base := range []float64{0.5, 2} {
					for k := 0; k < 30; k += leadStride() {
						lead := base + float64(k)*0.0009
						start := benchPad + lead
						carrier := make([]float64, int(lead*basebandRate))
						for i := range carrier {
							carrier[i] = 1
						}
						x := signalBaseband([]signalPart{{benchPad, nil, carrier}, {start, fm, env}}, start+float64(len(fm))/basebandRate+0.3, cn, 1)
						truth := make([]onsetTruth, len(slots))
						for i, s := range slots {
							at := start + float64(i)*slotSeconds + (slotSeconds-burstSeconds)/2 + float64(cachSymbols)/symbolRate
							truth[i] = onsetTruth{at: at, data: s.Burst, cach: s.CACH, bs: true}
						}
						if onsetBits(t, fmt.Sprintf("%.4f s of carrier", lead), truth, lr.receive(t, x), slotSeconds) {
							t.Errorf("%.4f s of carrier: lock burst raw bits differ", lead)
						}
					}
				}
			})
		}
	}
}

func TestRxRawBitsAtGapEdge(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
	for _, seq := range []string{"data12", "voice"} {
		built, err := rftx.Sequence(seq, cfg)
		if err != nil {
			t.Fatal(err)
		}
		fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
		for _, cn := range []float64{math.Inf(1), 40} {
			for _, lr := range onsetReceivers() {
				t.Run(fmt.Sprintf("%s/%s/%.0f", seq, lr.name(), cn), func(t *testing.T) {
					t.Parallel()
					bad := 0
					for k := 0; k < 30; k += 3 {
						start := benchPad + float64(k)*0.0009
						x := signalBaseband([]signalPart{{start, fm, env}}, start+float64(len(fm))/basebandRate+0.3, cn, 1)
						truth := make([]onsetTruth, len(built.Bursts))
						for i, b := range built.Bursts {
							truth[i] = onsetTruth{at: start + float64(2*i)*slotSeconds + (slotSeconds-burstSeconds)/2, data: b}
						}
						if onsetBits(t, fmt.Sprintf("start %.4f s", start), truth, lr.receive(t, x), 2*slotSeconds) {
							bad++
						}
					}
					if bad > 1 {
						t.Errorf("lock burst raw bits differ at %d starts", bad)
					}
				})
			}
		}
	}
}
