package rf

import (
	"encoding/json"
	"math"
	"math/bits"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

const (
	acsPath     = "testdata/rxbench_acs.json"
	acsStartDB  = 60
	acsStepDB   = 2
	acsTopDB    = 120
	acsMarginDB = 70
	acsMaxBER   = 1e-2
	acsToneHz   = 400
	acsDevHz    = 1500
	acsSeeds    = 4
)

var acsWantedCN = map[Tier]float64{Tier0: 11, Tier1: 11, Tier2: 7, Tier3: 7}

func acsBER(t *testing.T, tier Tier, o options, cn, irDB, irHz float64, seed uint64) float64 {
	t.Helper()
	cfg := testConfig(t)
	cfg.ColorCode, cfg.Superframes, cfg.Wakeups, cfg.Slot = 1, 2, 0, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	x := msBaseband(fm, env, 0, cn, seed)
	a := math.Pow(10, irDB/20)
	var ph float64
	for i := range x {
		ph += 2 * math.Pi * (irHz + acsDevHz*math.Sin(2*math.Pi*acsToneHz*float64(i)/basebandRate)) / basebandRate
		x[i] += complex(float32(a*math.Cos(ph)), float32(a*math.Sin(ph)))
	}
	var out []Burst
	r := mustNewOpt(RxConfig{Rate: basebandRate, Tier: tier, OnBursts: each(func(b *Burst) { out = append(out, *b) }), Format: IQFormatCF32}, o)
	for k := 0; k < len(x); k += 960 {
		must(r.writeBB(x[k:min(k+960, len(x))]))
	}
	r.flushAll()
	errs := make([]int, len(built.Bursts))
	for i := range errs {
		errs[i] = len(built.Bursts[i]) * 8 / 2
	}
	for _, b := range out {
		i := int(math.Round((b.Seconds - msPad - slotSeconds - (slotSeconds-burstSeconds)/2) / (2 * slotSeconds)))
		if i < 0 || i >= len(errs) {
			continue
		}
		e := 0
		for j := range b.Data {
			e += bits.OnesCount8(b.Data[j] ^ built.Bursts[i][j])
		}
		errs[i] = min(errs[i], e)
	}
	var sum int
	for _, e := range errs {
		sum += e
	}
	return float64(sum) / float64(len(errs)*len(built.Bursts[0])*8)
}

// TestIQAdjacentSelectivity measures the adjacent channel selectivity of EN
// 300 113 V2.2.1 clause 8.6: the wanted signal 3 dB above the sensitivity
// limit (BER 1e-2) and an unwanted A-M3 signal (FM, 400 Hz tone, 1.5 kHz
// deviation) on the channel above or below. The standard asks for 60 dB.
func TestIQAdjacentSelectivity(t *testing.T) {
	t.Parallel()
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		for _, side := range []float64{12500, -12500} {
			t.Run(strconv.Itoa(int(tier-Tier0))+"/"+strconv.FormatFloat(side, 'f', 0, 64), func(t *testing.T) {
				t.Parallel()
				cn := acsWantedCN[tier]
				var ber float64
				for s := range uint64(acsSeeds) {
					ber += acsBER(t, tier, tierOptions(tier), cn, acsMarginDB, side, 100+s) / acsSeeds
				}
				if ber > acsMaxBER {
					t.Errorf("BER %.4f with the A-M3 signal %d dB above the wanted, over %.0e", ber, acsMarginDB, acsMaxBER)
				}
			})
		}
	}
}

// TestIQAdjacentPull runs each I/Q tier next to a DMR channel 40 dB stronger
// at +12.5 kHz whose spectrum in this channel is legal (rftx.ModulateBS,
// -62.7 dBc), with a noise lead before the wanted call. The converter must
// not follow the other channel before the lock.
func TestIQAdjacentPull(t *testing.T) {
	t.Parallel()
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		for _, off := range []float64{0, 300} {
			if tier == Tier3 && off == 0 {
				continue
			}
			t.Run(strconv.Itoa(int(tier-Tier0))+"/"+strconv.FormatFloat(off, 'f', 0, 64), func(t *testing.T) {
				t.Parallel()
				o := tierOptions(tier)
				n, want, pull := adjacentReceiveSeed(t, tier, &o, off, 3)
				if n < want {
					t.Errorf("C0 and C1 of %d of %d voice frames exact", n, want)
				}
				if pull > iqAFCLockHz {
					t.Errorf("oscillator %.0f Hz off the carrier while locked", pull)
				}
			})
		}
	}
}

type acsPoint struct {
	Tier   int     `json:"tier"`
	SideHz float64 `json:"side_hz"`
	DB     float64 `json:"acs_db"`
}

type acsData struct {
	MaxBER float64            `json:"max_ber"`
	Seeds  int                `json:"seeds"`
	StepDB float64            `json:"step_db"`
	CN     map[string]float64 `json:"wanted_cn"`
	Points []acsPoint         `json:"points"`
}

// TestRxACS measures the adjacent channel selectivity of every I/Q tier as
// in TestIQAdjacentSelectivity: the highest A-M3 level, in steps of
// acsStepDB from acsStartDB, at which the BER over acsSeeds draws stays at
// most acsMaxBER; acsStartDB - acsStepDB stands for a tier that fails at
// acsStartDB. It writes testdata/rxbench_acs.json for cmd/rxbench.
func TestRxACS(t *testing.T) {
	t.Parallel()
	if os.Getenv("DMRRF_RXACS") != "1" {
		t.Skip("set DMRRF_RXACS=1 to measure testdata/rxbench_acs.json")
	}
	d := acsData{MaxBER: acsMaxBER, Seeds: acsSeeds, StepDB: acsStepDB, CN: map[string]float64{}}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		d.CN[strconv.Itoa(int(tier-Tier0))] = acsWantedCN[tier]
		for _, side := range []float64{-12500, 12500} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				best := float64(acsStartDB - acsStepDB)
				for db := float64(acsStartDB); db <= acsTopDB; db += acsStepDB {
					var ber float64
					for s := range uint64(acsSeeds) {
						ber += acsBER(t, tier, tierOptions(tier), acsWantedCN[tier], db, side, 100+s) / acsSeeds
					}
					if ber > acsMaxBER {
						break
					}
					best = db
				}
				mu.Lock()
				d.Points = append(d.Points, acsPoint{Tier: int(tier - Tier0), SideHz: side, DB: best})
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	slices.SortFunc(d.Points, func(a, b acsPoint) int {
		if a.Tier != b.Tier {
			return a.Tier - b.Tier
		}
		return int(a.SideHz - b.SideHz)
	})
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(acsPath, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
