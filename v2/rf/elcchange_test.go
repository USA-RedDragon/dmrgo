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
	rxs := []rx{
		{"I/Q Tier 1", func(x []complex64) []Burst { return receiveBaseband(x, Tier1) }},
		{"I/Q Tier 2", func(x []complex64) []Burst { return receiveBaseband(x, Tier2) }},
		{"I/Q Tier 3", func(x []complex64) []Burst { return receiveBaseband(x, Tier3) }},
		{"core I/Q Tier 3", func(x []complex64) []Burst { return receiveCore3(t, x) }},
		{"audio Tier 2", func(x []complex64) []Burst { return receiveAudioTier(t, discAudio(x, benchAudioHz), Tier2) }},
	}
	talkers := []callTalker{{src: 3191868, superframes: 3, header: true}, {src: 3140598, lead: -1, superframes: 3, term: true}}
	old := map[enums.VoiceBurstType]layer2.RawBurst{}
	pos := enums.VoiceBurstUnknown
	for _, b := range callBursts(t, talkers[0]) {
		x, err := layer2.NewBurstFromBytes(b)
		switch {
		case err == nil && x.Kind == layer2.KindVoice && x.SyncPattern != enums.SyncNone:
			pos = enums.VoiceBurstA
		case pos != enums.VoiceBurstUnknown && pos < enums.VoiceBurstF:
			pos++
			old[pos] = b
		default:
			pos = enums.VoiceBurstUnknown
		}
	}
	if len(old) != 5 {
		t.Fatalf("first talker: %d of the bursts B to F", len(old))
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
						if j < 0 || j >= len(s.truth) || s.who[j] != 1 || b.Voice < enums.VoiceBurstB || b.Voice > enums.VoiceBurstE {
							continue
						}
						d, _ := b.Decoded()
						o := old[b.Voice]
						if got, want := d.EmbeddedData(), s.truth[j].EmbeddedData(); got != want && got == o.EmbeddedData() {
							n++
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
				t.Errorf("%s: %d bursts of the second talker carry the first talker's embedded LC fragment", k, n)
			}
		}
	}
}
