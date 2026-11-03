//go:build !tinygo

package rf

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestRxEmbeddedLCTalkerChange(t *testing.T) {
	t.Parallel()
	checkELCTalkerChange(t, map[float64][]uint64{25: {9300}, 8: {9301}, 4: {9301}, 2: {9301}})
}

func benchELCTalkerChange(t *testing.T) {
	four := []uint64{9300, 9301, 9302, 9303}
	checkELCTalkerChange(t, map[float64][]uint64{25: {9300}, 8: four, 4: four, 2: four})
}

func checkELCTalkerChange(t *testing.T, seeds map[float64][]uint64) {
	type rx struct {
		name string
		run  func(x []complex64) []Burst
	}
	var rxs []rx
	for _, e := range benchEntries() {
		rxs = append(rxs, rx{e.name(), func(x []complex64) []Burst { return benchReceive(t, x, e) }})
	}
	rxs = append(rxs, rx{"core I/Q Tier 3", func(x []complex64) []Burst { return receiveCore3(t, x) }})
	talkers := []callTalker{
		{src: 3002068, superframes: 3, header: true},
		{src: 990126, lead: -1, superframes: 3},
		{src: 990023, lead: -1, superframes: 3, term: true},
	}
	old := make([]map[enums.VoiceBurstType]layer2.RawBurst, len(talkers))
	for k, tk := range talkers {
		old[k] = map[enums.VoiceBurstType]layer2.RawBurst{}
		pos := enums.VoiceBurstUnknown
		for _, b := range callBursts(t, tk) {
			x, err := layer2.NewBurstFromBytes(b)
			switch {
			case err == nil && x.Kind == layer2.KindVoice && x.SyncPattern != enums.SyncNone:
				pos = enums.VoiceBurstA
			case pos != enums.VoiceBurstUnknown && pos < enums.VoiceBurstF:
				pos++
				old[k][pos] = b
			default:
				pos = enums.VoiceBurstUnknown
			}
		}
		if len(old[k]) != 5 {
			t.Fatalf("talker %d: %d of the bursts B to F", k, len(old[k]))
		}
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	stale := map[string]int{}
	for _, cn := range []float64{25, 8, 4, 2} {
		for _, seed := range seeds[cn] {
			s := callMix(t, cn, seed, 0, talkers...)
			for _, r := range rxs {
				wg.Add(1)
				go func() {
					defer wg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					first := (slotSeconds-burstSeconds)/2 + benchPad
					n := 0
					for _, b := range r.run(s.x) {
						j := int(math.Round((b.Seconds - first) / slotSeconds))
						if j < 0 || j >= len(s.truth) || s.who[j] < 1 || b.Voice < enums.VoiceBurstB || b.Voice > enums.VoiceBurstE {
							continue
						}
						d, _ := b.Decoded()
						got, want := d.EmbeddedData(), s.truth[j].EmbeddedData()
						for _, o := range old[:s.who[j]] {
							if got != want && got == o[b.Voice].EmbeddedData() {
								n++
								break
							}
						}
					}
					mu.Lock()
					stale[fmt.Sprintf("%s at %.0f dB", r.name, cn)] += n
					mu.Unlock()
				}()
			}
		}
	}
	wg.Wait()
	for _, cn := range []float64{25, 8, 4, 2} {
		for _, r := range rxs {
			k := fmt.Sprintf("%s at %.0f dB", r.name, cn)
			if n := stale[k]; n > 0 {
				t.Errorf("%s: %d bursts of a later talker carry an earlier talker's embedded LC fragment", k, n)
			}
		}
	}
}
