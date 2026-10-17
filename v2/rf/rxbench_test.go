package rf

import (
	"encoding/json"
	"math"
	"math/bits"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// rxbenchPath holds the sweep results that cmd/rxbench renders into the
// README. Regenerate with DMRRF_RXBENCH=full go test -run TestRxBench .
const rxbenchPath = "testdata/rxbench.json"

// benchPayloads lists the payload types of a sweep, in the order of the
// README tables. rate1_bit counts the payload bits of the rate 1 blocks
// instead of frames, each bit of a block that the receiver misses as wrong.
func benchPayloads() []string {
	return []string{"ambe", "ambe_c0c1", "rate12", "rate34", "rate1", "csbk", "lc", "header", "elc", "emb", "slottype", "sync", "rate1_bit"}
}

// benchSlot is one slot of the sweep stream and what a receiver should
// recover from it.
type benchSlot struct {
	burst layer2.RawBurst
	kind  string
	pos   enums.VoiceBurstType
	dt    enums.DataType
	sf    int
}

// benchStream builds a BS outbound stream: a group voice call of n
// superframes on TS2 and data on TS1 (rate 1/2, rate 3/4 and rate 1
// packets and preamble CSBKs).
func benchStream(t testing.TB, n int) ([]rftx.BSSlot, []benchSlot) {
	t.Helper()
	cfg := rftx.SeqConfig{ColorCode: 7, Source: 3191868, Dest: 9, BS: 314926, Group: true, Wakeups: 1, Message: "DMRGO RXBENCH", Superframes: n, BSSourced: true, Slot: 1, Preambles: 2}
	voice, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	slots := voice.Slots
	var data []layer2.RawBurst
	for len(data) < len(slots)/2+1 {
		for _, seq := range []string{"data12", "data34", "data1", "preamble"} {
			c := cfg
			c.Slot, c.Group, c.Dest = 0, false, 3140598
			b, err := rftx.Sequence(seq, c)
			if err != nil {
				t.Fatal(err)
			}
			for i, s := range b.Slots {
				if x, err := layer2.NewBurstFromBytes(s.Burst); i%2 == 0 && err == nil && x.Kind == layer2.KindData && x.SlotType.DataType != enums.DataTypeIdle {
					data = append(data, s.Burst)
				}
			}
		}
	}
	truth := make([]benchSlot, len(slots))
	sf, pos := -1, enums.VoiceBurstUnknown
	for i := range slots {
		ts := &truth[i]
		if i%2 == 0 {
			slots[i].Burst = data[i/2]
		}
		ts.burst = slots[i].Burst
		b, err := layer2.NewBurstFromBytes(ts.burst)
		switch {
		case i%2 == 1 && err == nil && b.Kind == layer2.KindVoice && b.SyncPattern == enums.SyncBSSourcedVoice:
			sf++
			pos = enums.VoiceBurstA
			ts.kind, ts.pos, ts.sf = "voice", pos, sf
		case i%2 == 1 && pos != enums.VoiceBurstUnknown && pos < enums.VoiceBurstF && (err != nil || b.Kind != layer2.KindData):
			pos++
			ts.kind, ts.pos, ts.sf = "voice", pos, sf
		case err == nil && b.Kind == layer2.KindData:
			if i%2 == 1 {
				pos = enums.VoiceBurstUnknown
			}
			ts.kind, ts.dt = "data", b.SlotType.DataType
			if ts.dt == enums.DataTypeIdle {
				ts.kind = "idle"
			}
		default:
			ts.kind = "idle"
		}
	}
	return slots, truth
}

// benchScore counts the frames of one payload type.
type benchScore struct {
	Truth int `json:"truth"`
	Exact int `json:"exact"`
}

// benchPoint is the score of all payload types at one C/N.
type benchPoint struct {
	CN       float64                `json:"cn"`
	Seeds    int                    `json:"seeds,omitempty"`
	Payloads map[string]*benchScore `json:"payloads"`
}

// benchTier is the sweep of one tier.
type benchTier struct {
	Tier         int          `json:"tier"`
	Fast         []benchPoint `json:"fast"`
	Full         []benchPoint `json:"full,omitempty"`
	NsPerSecond  float64      `json:"ns_per_second,omitempty"`
	RAMBytes     int          `json:"ram_bytes"`
	Allocs       float64      `json:"allocs"`
	FalsePerHour float64      `json:"false_per_hour,omitempty"`
}

// benchData is the content of rxbenchPath.
type benchData struct {
	Machine string      `json:"machine"`
	Tiers   []benchTier `json:"tiers"`
}

// benchPad is the noise before and after the stream, in seconds.
const benchPad = 0.3

// benchScorer accumulates the scores of one sweep case.
type benchScorer struct {
	sc    map[string]*benchScore
	frags map[int][4][4]byte
	nfrag map[int]int
}

// add counts one truth item of payload type k, exact when ok.
func (s *benchScorer) add(k string, ok bool) {
	s.sc[k].Truth++
	if ok {
		s.sc[k].Exact++
	}
}

// voice scores the voice burst i of truth against data.
func (s *benchScorer) voice(truth []benchSlot, i int, r Burst, data layer2.RawBurst, have bool) {
	ts := truth[i]
	want, _ := ts.burst.AMBEFrames()
	gotF, _ := data.AMBEFrames()
	for k := range want {
		s.add("ambe", have && gotF[k] == want[k])
		s.add("ambe_c0c1", have && slices.Equal(gotF[k][:3], want[k][:3]))
	}
	if ts.pos != enums.VoiceBurstA {
		e, w := data.EMB(), ts.burst.EMB()
		s.add("emb", have && !e.FEC.Uncorrectable && e.ColorCode == w.ColorCode && e.LCSS == w.LCSS && e.PreemptionAndPowerControlIndicator == w.PreemptionAndPowerControlIndicator)
	} else {
		s.add("sync", have && r.Distance >= 0 && r.Sync == enums.SyncBSSourcedVoice)
	}
	if have && ts.pos >= enums.VoiceBurstB && ts.pos <= enums.VoiceBurstE {
		f := s.frags[ts.sf]
		f[ts.pos-enums.VoiceBurstB] = data.EmbeddedData()
		s.frags[ts.sf] = f
		s.nfrag[ts.sf]++
	}
	if ts.pos == enums.VoiceBurstE {
		var wf [4][4]byte
		for j := range wf {
			wf[j] = truth[i-2*(3-j)].burst.EmbeddedData()
		}
		wantLC, _ := layer2.DecodeEmbeddedLC(wf, true)
		lc, res := layer2.DecodeEmbeddedLC(s.frags[ts.sf], true)
		s.add("elc", s.nfrag[ts.sf] == 4 && !res.Uncorrectable && lc == wantLC)
	}
}

// benchDataKey returns the payload type name of data type dt, or "" when
// the sweep does not score its payload.
func benchDataKey(dt enums.DataType) string {
	switch dt {
	case enums.DataTypeCSBK:
		return "csbk"
	case enums.DataTypeRate34:
		return "rate34"
	case enums.DataTypeRate1:
		return "rate1"
	case enums.DataTypeDataHeader:
		return "header"
	case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC:
		return "lc"
	case enums.DataTypeRate12:
		return "rate12"
	default:
		return ""
	}
}

// data scores the data burst ts against data.
func (s *benchScorer) data(ts benchSlot, r Burst, data layer2.RawBurst, have bool) {
	okST := have && data.SlotType() == ts.burst.SlotType()
	s.add("slottype", okST)
	s.add("sync", have && r.Distance >= 0 && r.Sync == enums.SyncBSSourcedData)
	k := benchDataKey(ts.dt)
	if k == "" {
		return
	}
	ok := okST
	switch ts.dt {
	case enums.DataTypeRate34:
		a, _ := data.Rate34Payload()
		b, _ := ts.burst.Rate34Payload()
		ok = ok && a == b
	case enums.DataTypeRate1:
		a, _ := data.Rate1Payload()
		b, _ := ts.burst.Rate1Payload()
		ok = ok && a == b
		s.sc["rate1_bit"].Truth += 8 * len(b)
		if have {
			for i := range b {
				s.sc["rate1_bit"].Exact += 8 - bits.OnesCount8(a[i]^b[i])
			}
		}
	default:
		a, _ := data.BPTCPayload()
		b, _ := ts.burst.BPTCPayload()
		ok = ok && a == b
	}
	s.add(k, ok)
}

// scoreBench scores the bursts got against the truth of the stream.
func scoreBench(truth []benchSlot, got []Burst) map[string]*benchScore {
	s := benchScorer{sc: map[string]*benchScore{}, frags: map[int][4][4]byte{}, nfrag: map[int]int{}}
	for _, n := range benchPayloads() {
		s.sc[n] = &benchScore{}
	}
	first := (slotSeconds-burstSeconds)/2 + float64(cachSymbols)/symbolRate + benchPad
	byIndex := map[int]Burst{}
	for _, r := range got {
		i := int(math.Round((r.Seconds - first) / slotSeconds))
		if i >= 0 && i < len(truth) && math.Abs(r.Seconds-first-float64(i)*slotSeconds) < 0.002 {
			byIndex[i] = r
		}
	}
	for i, ts := range truth {
		r, have := byIndex[i]
		data, _ := r.Decoded()
		switch ts.kind {
		case "voice":
			s.voice(truth, i, r, data, have)
		case "data":
			s.data(ts, r, data, have)
		}
	}
	return s.sc
}

// benchCase is one run of a sweep.
type benchCase struct {
	cn   float64
	seed uint64
}

// benchAudioHz is the sample rate of the discriminator audio of Tier 0,
// which the sweep runs through a DiscReceiver.
const benchAudioHz = 9600

// benchReceive runs the baseband x through the receiver of tier t: for
// TierDisc a DiscReceiver on the FM discriminator audio of x at
// benchAudioHz, for the other tiers a Receiver on x.
func benchReceive(t testing.TB, x []complex64, tier Tier) []Burst {
	if tier == TierDisc {
		return receiveAudio(t, discAudio(x, benchAudioHz), benchAudioHz, nil)
	}
	return receiveBaseband(x, tier)
}

// runBench receives the stream at each case with tier t on all cores and
// returns the points, summed over seeds.
func runBench(t testing.TB, slots []rftx.BSSlot, truth []benchSlot, tier Tier, cases []benchCase) []benchPoint {
	t.Helper()
	res := make([]map[string]*benchScore, len(cases))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i, c := range cases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res[i] = scoreBench(truth, benchReceive(t, noisyBaseband(slots, 1400, c.cn, c.seed), tier))
		}()
	}
	wg.Wait()
	byCN := map[float64]map[string]*benchScore{}
	seeds := map[float64]int{}
	for i, c := range cases {
		seeds[c.cn]++
		if byCN[c.cn] == nil {
			byCN[c.cn] = map[string]*benchScore{}
		}
		for k, v := range res[i] {
			a := byCN[c.cn][k]
			if a == nil {
				a = &benchScore{}
				byCN[c.cn][k] = a
			}
			a.Truth += v.Truth
			a.Exact += v.Exact
		}
	}
	out := make([]benchPoint, 0, len(byCN))
	for cn, p := range byCN {
		out = append(out, benchPoint{CN: cn, Seeds: seeds[cn], Payloads: p})
	}
	slices.SortFunc(out, func(a, b benchPoint) int { return int(math.Copysign(1, a.CN-b.CN)) })
	return out
}

// fastCases returns the cases of the CI sweep of tier t: a few C/N points
// near where its voice frames start to decode.
func fastCases(t Tier) []benchCase {
	cns := map[Tier][]float64{TierDisc: {5, 7, 9}, TierCoherent: {1, 3, 5}, TierExact: {0, 2, 4}}[t]
	out := make([]benchCase, len(cns))
	for i, cn := range cns {
		out[i] = benchCase{cn, 11}
	}
	return out
}

// fullCases returns the cases of the README sweep.
func fullCases() []benchCase {
	var out []benchCase
	for cn := -2.0; cn <= 12; cn += 0.5 {
		for seed := range uint64(4) {
			out = append(out, benchCase{cn, 100 + seed})
		}
	}
	return out
}

// deepPayloads lists the payload types of the README graphs, which the
// generator sweep follows down to low error rates.
func deepPayloads() []string {
	return []string{"ambe", "rate12", "rate34", "csbk", "elc", "rate1_bit"}
}

// Limits of the deep points of the generator sweep.
const (
	// deepErrors is the number of errors at which a payload type of a point
	// needs no more seeds.
	deepErrors = 20
	// deepBatch is the number of seeds that each round of a deep point adds.
	deepBatch = 32
	// deepFirstSeed is the first seed of the deep rounds, after the seeds of
	// fullCases.
	deepFirstSeed = 1000
)

// deepStreams returns the most streams that a deep point of tier t runs,
// set by its compute cost.
func deepStreams(t Tier) int {
	return map[Tier]int{TierDisc: 4096, TierCoherent: 2048, TierExact: 1024}[t]
}

// deepen adds seeds to the points of a full sweep where a graphed payload
// type has fewer than deepErrors errors, so that the error rate curves
// reach about 1e-4. It adds rounds of deepBatch seeds to a point until each
// such payload type has deepErrors errors or the point has run
// deepStreams streams. A payload type that ends a point with no errors
// needs no deeper points above it.
func deepen(t testing.TB, slots []rftx.BSSlot, truth []benchSlot, tier Tier, pts []benchPoint) []benchPoint {
	t.Helper()
	done := map[string]bool{}
	for i := range pts {
		p := &pts[i]
		seed := uint64(deepFirstSeed) + uint64(i)<<20
		for {
			need := false
			for _, k := range deepPayloads() {
				sc := p.Payloads[k]
				if sc == nil || done[k] || sc.Truth == 0 || sc.Truth-sc.Exact >= deepErrors {
					continue
				}
				if i == 0 || pts[i-1].Payloads[k] == nil || pts[i-1].Payloads[k].Exact < pts[i-1].Payloads[k].Truth || sc.Exact < sc.Truth {
					need = true
				}
			}
			if !need || p.Seeds >= deepStreams(tier) {
				break
			}
			cases := make([]benchCase, deepBatch)
			for j := range cases {
				cases[j] = benchCase{p.CN, seed}
				seed++
			}
			add := runBench(t, slots, truth, tier, cases)[0]
			p.Seeds += add.Seeds
			for k, v := range add.Payloads {
				a := p.Payloads[k]
				if a == nil {
					a = &benchScore{}
					p.Payloads[k] = a
				}
				a.Truth += v.Truth
				a.Exact += v.Exact
			}
		}
		for _, k := range deepPayloads() {
			if sc := p.Payloads[k]; sc != nil && sc.Truth > 0 && sc.Exact == sc.Truth {
				done[k] = true
			}
		}
	}
	return pts
}

// falsePerHour returns the bursts that tier t reports from 5 minutes of
// noise, per hour.
func falsePerHour(t Tier) float64 {
	const seconds = 300
	n, total := 0, 0
	if t == TierDisc {
		r, err := NewDisc(DiscConfig{Hz: benchAudioHz, OnBurst: func(*Burst) { n++ }})
		if err != nil {
			panic(err)
		}
		for seed := uint64(1); total < seconds*benchAudioHz; seed++ {
			a := discAudio(noisyBaseband(nil, 0, 0, seed), benchAudioHz)
			r.Write(a)
			total += len(a)
		}
		r.Flush()
		return float64(n) * 3600 * benchAudioHz / float64(total)
	}
	r := mustNew(Config{Rate: basebandRate, Tier: t})
	for seed := uint64(1); total < seconds*basebandRate; seed++ {
		x := noisyBaseband(nil, 0, 0, seed)
		n += len(r.WriteBaseband(x))
		total += len(x)
	}
	n += len(r.Flush())
	return float64(n) * 3600 * basebandRate / float64(total)
}

// receiverRAM returns the heap bytes that a receiver of tier t keeps, with
// OnBurst set, as a microcontroller runs it: for TierDisc a DiscReceiver at
// benchAudioHz, for the other tiers a Receiver at the baseband input rate.
func receiverRAM(t Tier) int {
	var a, b runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&a)
	var r any
	if t == TierDisc {
		d, err := NewDisc(DiscConfig{Hz: benchAudioHz, OnBurst: func(*Burst) {}})
		if err != nil {
			panic(err)
		}
		r = d
	} else {
		r = mustNew(Config{Rate: basebandRate, Tier: t, OnBurst: func(*Burst) {}})
	}
	runtime.GC()
	runtime.ReadMemStats(&b)
	runtime.KeepAlive(r)
	return int(b.HeapAlloc) - int(a.HeapAlloc)
}

// steadyAllocs returns the allocations per WriteBaseband of 16 KiB of x
// with tier t, after the first chunk. It counts the mallocs of the whole
// process, so the full sweep calls it while nothing else runs.
func steadyAllocs(x []complex64, t Tier) float64 {
	const chunk = 1 << 14
	if t == TierDisc {
		return discSteadyAllocs(discAudio(x, benchAudioHz))
	}
	r := mustNew(Config{Rate: basebandRate, Tier: t})
	r.WriteBaseband(x[:chunk])
	n := len(x)/chunk - 1
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	for i := 1; i <= n; i++ {
		r.WriteBaseband(x[i*chunk : (i+1)*chunk])
	}
	runtime.ReadMemStats(&b)
	return float64(b.Mallocs-a.Mallocs) / float64(n)
}

// discSteadyAllocs returns the allocations per Write of 4 KiB of the audio
// a with a DiscReceiver at benchAudioHz, after the first chunk.
func discSteadyAllocs(a []int16) float64 {
	const chunk = 1 << 11
	r, err := NewDisc(DiscConfig{Hz: benchAudioHz, OnBurst: func(*Burst) {}})
	if err != nil {
		panic(err)
	}
	r.Write(a[:chunk])
	n := len(a)/chunk - 1
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	for i := 1; i <= n; i++ {
		r.Write(a[i*chunk : (i+1)*chunk])
	}
	runtime.ReadMemStats(&m1)
	return float64(m1.Mallocs-m0.Mallocs) / float64(n)
}

// TestRxBench receives a BS outbound stream with every payload type at a
// few C/N points per tier below TierMax, which is for offline decoding,
// and checks the frame recovery against the baseline in rxbenchPath. With
// DMRRF_RXBENCH=full it runs the README sweep instead and rewrites
// rxbenchPath, only for the tiers in DMRRF_RXBENCH_TIERS when it is set,
// and with DMRRF_RXBENCH=fast it rewrites only the CI points.
func TestRxBench(t *testing.T) {
	t.Parallel()
	full := os.Getenv("DMRRF_RXBENCH") == "full"
	fast := os.Getenv("DMRRF_RXBENCH") == "fast"
	var base benchData
	if b, err := os.ReadFile(rxbenchPath); err == nil {
		if err := json.Unmarshal(b, &base); err != nil {
			t.Fatal(err)
		}
	} else if !full {
		t.Fatal(err)
	}
	slots, truth := benchStream(t, 6)
	out := benchData{Machine: base.Machine}
	for _, tier := range []Tier{TierDisc, TierCoherent, TierExact} {
		bt := benchTier{Tier: int(tier)}
		bt.Fast = runBench(t, slots, truth, tier, fastCases(tier))
		if full && !benchTierSelected(tier) {
			out.Tiers = append(out.Tiers, base.Tiers[tier])
			continue
		}
		if full {
			fs, ft := benchStream(t, 30)
			bt.Full = deepen(t, fs, ft, tier, runBench(t, fs, ft, tier, fullCases()))
			x := noisyBaseband(fs, 1400, 6, 1)
			bt.Allocs = steadyAllocs(x, tier)
			bt.RAMBytes = receiverRAM(tier)
			var cpu float64
			if tier == TierDisc {
				a := discAudio(x, benchAudioHz)
				cpu = cpuSeconds(func() { receiveAudio(t, a, benchAudioHz, nil) })
			} else {
				cpu = cpuSeconds(func() { receiveBaseband(x, tier) })
			}
			bt.NsPerSecond = cpu * 1e9 / (float64(len(x)) / basebandRate)
			bt.FalsePerHour = falsePerHour(tier)
		}
		if int(tier) < len(base.Tiers) {
			want := base.Tiers[tier]
			if !fast {
				checkBench(t, tier, want.Fast, bt.Fast)
			}
			if !full {
				bt.Full, bt.NsPerSecond, bt.FalsePerHour, bt.Allocs, bt.RAMBytes = want.Full, want.NsPerSecond, want.FalsePerHour, want.Allocs, want.RAMBytes
			}
		}
		out.Tiers = append(out.Tiers, bt)
	}
	if full || fast {
		b, err := json.MarshalIndent(out, "", " ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rxbenchPath, append(b, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// benchTierSelected reports whether DMRRF_RXBENCH_TIERS, a comma-separated
// list of tier numbers, names tier t, or is empty.
func benchTierSelected(t Tier) bool {
	v := os.Getenv("DMRRF_RXBENCH_TIERS")
	return v == "" || slices.Contains(strings.Split(v, ","), strconv.Itoa(int(t)))
}

// checkBench fails when the recovery of a payload type at a C/N of the CI
// sweep falls more than 5 percent under the baseline.
func checkBench(t *testing.T, tier Tier, want, got []benchPoint) {
	t.Helper()
	for _, w := range want {
		for _, g := range got {
			if g.CN != w.CN {
				continue
			}
			for k, ws := range w.Payloads {
				gs := g.Payloads[k]
				if ws.Truth == 0 || gs == nil {
					continue
				}
				wr, gr := float64(ws.Exact)/float64(ws.Truth), float64(gs.Exact)/float64(max(gs.Truth, 1))
				if gr < wr-0.05 {
					t.Errorf("tier %d, %s at C/N %.1f dB: %.3f recovered, baseline %.3f", tier, k, w.CN, gr, wr)
				}
			}
		}
	}
}
