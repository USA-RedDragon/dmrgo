package rf

import (
	"encoding/json"
	"math"
	"math/bits"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const rxbenchPath = "testdata/rxbench.json"

func benchPayloads() []string {
	return []string{"ambe", "ambe_c0c1", "ambe_u", "rate12", "rate34", "rate1", "csbk", "lc", "header", "elc", "emb", "slottype", "sync", "rate1_bit"}
}

type benchSlot struct {
	burst layer2.RawBurst
	kind  string
	pos   enums.VoiceBurstType
	dt    enums.DataType
	sf    int
}

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

type benchScore struct {
	Truth int `json:"truth"`
	Exact int `json:"exact"`
}

type benchPoint struct {
	CN       float64                `json:"cn"`
	Seeds    int                    `json:"seeds,omitempty"`
	Payloads map[string]*benchScore `json:"payloads"`
}

type benchTier struct {
	Mode         string       `json:"mode"`
	Tier         int          `json:"tier"`
	Fast         []benchPoint `json:"fast"`
	Full         []benchPoint `json:"full,omitempty"`
	NsPerSecond  float64      `json:"ns_per_second,omitempty"`
	WorstNs      float64      `json:"worst_ns_per_second,omitempty"`
	WorstCN      float64      `json:"worst_cn,omitempty"`
	RAMBytes     int          `json:"ram_bytes"`
	Allocs       float64      `json:"allocs"`
	FalsePerHour float64      `json:"false_per_hour,omitempty"`
}

type benchData struct {
	Machine string      `json:"machine"`
	Tiers   []benchTier `json:"tiers"`
}

func (d *benchData) find(e benchEntry) (benchTier, bool) {
	for _, t := range d.Tiers {
		if t.Mode == e.mode && t.Tier == e.number() {
			return t, true
		}
	}
	return benchTier{}, false
}

const benchPad = 0.3

type benchScorer struct {
	sc    map[string]*benchScore
	frags map[int][4][4]byte
	nfrag map[int]int
}

func (s *benchScorer) add(k string, ok bool) {
	s.sc[k].Truth++
	if ok {
		s.sc[k].Exact++
	}
}

func (s *benchScorer) voice(truth []benchSlot, i int, r Burst, data layer2.RawBurst, have bool) {
	ts := truth[i]
	want, _ := ts.burst.AMBEFrames()
	gotF, _ := data.AMBEFrames()
	for k := range want {
		s.add("ambe", have && gotF[k] == want[k])
		s.add("ambe_c0c1", have && slices.Equal(gotF[k][:3], want[k][:3]))
		for q := 24; q < 49; q++ {
			m := byte(0x80) >> (q % 8)
			s.add("ambe_u", have && gotF[k][q/8]&m == want[k][q/8]&m)
		}
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
		wantLC, _ := strictEmbeddedLC(wf)
		lc, lcOK := strictEmbeddedLC(s.frags[ts.sf])
		s.add("elc", s.nfrag[ts.sf] == 4 && lcOK && lc == wantLC)
	}
}

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

type benchCase struct {
	cn   float64
	seed uint64
}

const (
	benchAudio = "audio"
	benchIQ    = "iq"
	benchIQLow = "iqlow"
)

type benchEntry struct {
	mode string
	tier Tier
}

func (e benchEntry) number() int { return int(e.tier - Tier0) }

func (e benchEntry) name() string { return e.mode + strconv.Itoa(e.number()) }

func benchEntries() []benchEntry {
	return []benchEntry{
		{benchAudio, Tier0}, {benchAudio, Tier1}, {benchAudio, Tier2}, {benchAudio, Tier3},
		{benchIQLow, Tier0}, {benchIQ, Tier0}, {benchIQ, Tier1}, {benchIQ, Tier2}, {benchIQ, Tier3},
	}
}

func receiveAudioTier(t testing.TB, a []int16, tier Tier) []Burst {
	t.Helper()
	return receiveAudioAt(t, a, benchAudioHz, tier)
}

func receiveAudioAt(t testing.TB, a []int16, hz int, tier Tier) []Burst {
	t.Helper()
	var out []Burst
	r, err := NewAudioReceiver(&AudioRxConfig{Rate: hz, Tier: tier, OnBursts: each(func(b *Burst) { out = append(out, *b) })})
	if err != nil {
		t.Fatal(err)
	}
	for k := 0; k < len(a); k += 1024 {
		must(r.writeAll(a[k:min(k+1024, len(a))]))
	}
	r.flushAll()
	return out
}

const benchAudioHz = 9600

func benchReceive(t testing.TB, x []complex64, e benchEntry) []Burst {
	switch e.mode {
	case benchAudio:
		return receiveAudioTier(t, discAudio(x, benchAudioHz), e.tier)
	case benchIQLow:
		return receiveLowIQ(lowIQ(x, iq0LowHz))
	}
	return receiveBaseband(x, e.tier)
}

func receiveLowIQ(y []complex64) []Burst {
	r := mustNew(RxConfig{Tier: Tier0, Rate: iq0LowHz, Format: IQFormatCU8})
	var out []Burst
	for k := 0; k < len(y); k += 1024 {
		out = append(out, must(r.writeBB(y[k:min(k+1024, len(y))]))...)
	}
	return append(out, r.flushAll()...)
}

type benchPool struct {
	mu   sync.Mutex
	cond *sync.Cond
	q    [][]func()
}

func newBenchPool(workers, levels int) *benchPool {
	p := &benchPool{q: make([][]func(), levels)}
	p.cond = sync.NewCond(&p.mu)
	for range workers {
		go p.work()
	}
	return p
}

func (p *benchPool) work() {
	for {
		p.mu.Lock()
		var f func()
		for f == nil {
			for i, q := range p.q {
				if len(q) > 0 {
					f, p.q[i] = q[0], q[1:]
					break
				}
			}
			if f == nil {
				p.cond.Wait()
			}
		}
		p.mu.Unlock()
		f()
	}
}

func (p *benchPool) do(prio int, f func()) {
	p.mu.Lock()
	p.q[prio] = append(p.q[prio], f)
	p.mu.Unlock()
	p.cond.Signal()
}

type benchShared struct {
	t     testing.TB
	pool  *benchPool
	sig   benchSignal
	truth []benchSlot
	es    []benchEntry
}

const (
	benchGenPrio   = 9
	benchFalsePrio = 10
	benchLevels    = 11
)

func (s benchShared) run(cases [][]benchCase) [][]benchPoint {
	type use struct{ e, i int }
	var order []benchCase
	users := map[benchCase][]use{}
	res := make([][]map[string]*benchScore, len(cases))
	for j, cs := range cases {
		res[j] = make([]map[string]*benchScore, len(cs))
		for i, c := range cs {
			if users[c] == nil {
				order = append(order, c)
			}
			users[c] = append(users[c], use{j, i})
		}
	}
	var wg sync.WaitGroup
	for _, c := range order {
		us := users[c]
		wg.Add(len(us))
		s.pool.do(benchGenPrio, func() {
			x := s.sig.noisy(c.cn, c.seed)
			var a []int16
			if slices.ContainsFunc(us, func(u use) bool { return s.es[u.e].mode == benchAudio }) {
				a = discAudio(x, benchAudioHz)
			}
			for _, u := range us {
				e := s.es[u.e]
				s.pool.do(benchPriority(e), func() {
					defer wg.Done()
					var got []Burst
					if e.mode == benchAudio {
						got = receiveAudioTier(s.t, a, e.tier)
					} else {
						got = benchReceive(s.t, x, e)
					}
					res[u.e][u.i] = scoreBench(s.truth, got)
				})
			}
		})
	}
	wg.Wait()
	out := make([][]benchPoint, len(cases))
	for j := range cases {
		out[j] = benchPoints(cases[j], res[j])
	}
	return out
}

func benchPoints(cases []benchCase, res []map[string]*benchScore) []benchPoint {
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

func fastCases(e benchEntry) []benchCase {
	cns := []float64{5, 7, 9}
	switch {
	case e.mode == benchIQ && e.tier == Tier2:
		cns = []float64{1, 3, 5}
	case e.mode == benchIQ && e.tier == Tier3:
		cns = []float64{0, 2, 4}
	}
	out := make([]benchCase, len(cns))
	for i, cn := range cns {
		out[i] = benchCase{cn, 11}
	}
	return out
}

func fullCases() []benchCase {
	var out []benchCase
	for cn := -2.0; cn <= 12; cn += 0.5 {
		for seed := range uint64(4) {
			out = append(out, benchCase{cn, 100 + seed})
		}
	}
	return out
}

func deepPayloads() []string {
	return []string{"ambe", "rate12", "rate34", "csbk", "elc", "rate1_bit"}
}

const (
	deepErrors    = 20
	deepBatch     = 32
	deepFirstSeed = 1000
	deepFloor     = 1e-4
	deepFrames    = deepErrors / deepFloor
)

func deepStreams(e benchEntry) int {
	switch {
	case e.mode == benchIQ && e.tier == Tier2:
		return 2048
	case e.mode == benchIQ && e.tier == Tier3:
		return 1024
	}
	return 4096
}

func deepNeeds(sc *benchScore) bool {
	return sc != nil && sc.Truth > 0 && sc.Truth-sc.Exact < deepErrors && sc.Truth < deepFrames
}

func (s benchShared) deepen(pts [][]benchPoint) {
	below := make([]map[string]int, len(s.es))
	n := 0
	for j := range s.es {
		below[j] = map[string]int{}
		n = max(n, len(pts[j]))
	}
	for i := range n {
		seed := uint64(deepFirstSeed) + uint64(i)<<20
		for {
			cases := make([][]benchCase, len(s.es))
			more := false
			for j, e := range s.es {
				if i >= len(pts[j]) {
					continue
				}
				p := &pts[j][i]
				if p.Seeds < deepStreams(e) && slices.ContainsFunc(deepPayloads(), func(k string) bool { return below[j][k] < 2 && deepNeeds(p.Payloads[k]) }) {
					cases[j] = make([]benchCase, deepBatch)
					for b := range cases[j] {
						cases[j][b] = benchCase{p.CN, seed + uint64(b)}
					}
					more = true
				}
			}
			if !more {
				break
			}
			seed += deepBatch
			add := s.run(cases)
			for j := range s.es {
				if cases[j] == nil {
					continue
				}
				p, d := &pts[j][i], add[j][0]
				p.Seeds += d.Seeds
				for k, v := range d.Payloads {
					a := p.Payloads[k]
					if a == nil {
						a = &benchScore{}
						p.Payloads[k] = a
					}
					a.Truth += v.Truth
					a.Exact += v.Exact
				}
			}
		}
		for j := range s.es {
			if i >= len(pts[j]) {
				continue
			}
			p := &pts[j][i]
			for _, k := range deepPayloads() {
				sc := p.Payloads[k]
				switch {
				case below[j][k] >= 2:
					delete(p.Payloads, k)
				case sc != nil && sc.Truth > 0 && float64(sc.Truth-sc.Exact) < deepFloor*float64(sc.Truth):
					below[j][k]++
				default:
					below[j][k] = 0
				}
			}
		}
	}
}

func falsePerHour(e benchEntry, noise benchSignal) float64 {
	const seconds = 300
	n, total := 0, 0
	t := e.tier
	switch e.mode {
	case benchAudio:
		hz := benchAudioHz
		r, err := NewAudioReceiver(&AudioRxConfig{Rate: hz, Tier: t, OnBursts: each(func(*Burst) { n++ })})
		if err != nil {
			panic(err)
		}
		for seed := uint64(1); total < seconds*hz; seed++ {
			a := discAudio(noise.noisy(0, seed), hz)
			must(r.writeAll(a))
			total += len(a)
		}
		r.flushAll()
		return float64(n) * 3600 * float64(hz) / float64(total)
	case benchIQLow:
		r := mustNew(RxConfig{Tier: Tier0, Rate: iq0LowHz, Format: IQFormatCU8})
		for seed := uint64(1); total < seconds*iq0LowHz; seed++ {
			y := lowIQ(noise.noisy(0, seed), iq0LowHz)
			n += len(must(r.writeBB(y)))
			total += len(y)
		}
		n += len(r.flushAll())
		return float64(n) * 3600 * iq0LowHz / float64(total)
	}
	r := mustNew(RxConfig{Rate: basebandRate, Tier: t, Format: IQFormatCU8})
	for seed := uint64(1); total < seconds*basebandRate; seed++ {
		x := noise.noisy(0, seed)
		n += len(must(r.writeBB(x)))
		total += len(x)
	}
	n += len(r.flushAll())
	return float64(n) * 3600 * basebandRate / float64(total)
}

func receiverRAM(e benchEntry) int {
	switch {
	case e.mode == benchAudio && e.tier == Tier0:
		return int(unsafe.Sizeof(AudioReceiver{}) - unsafe.Sizeof(audioRate{}))
	case e.mode == benchIQLow:
		return int(unsafe.Sizeof(Receiver{}))
	}
	ramMu.Lock()
	defer ramMu.Unlock()
	rate := runtime.MemProfileRate
	runtime.MemProfileRate = 1
	defer func() { runtime.MemProfileRate = rate }()
	runtime.GC()
	runtime.GC()
	before := liveBytesUnder(newBenchReceiver)
	r := newBenchReceiver(e)
	runtime.GC()
	runtime.GC()
	after := liveBytesUnder(newBenchReceiver)
	runtime.KeepAlive(r)
	return int(after - before)
}

var ramMu sync.Mutex

//go:noinline
func newBenchReceiver(e benchEntry) any {
	if e.mode == benchAudio {
		d, err := NewAudioReceiver(&AudioRxConfig{Rate: benchAudioHz, Tier: e.tier, OnBursts: each(func(*Burst) {})})
		if err != nil {
			panic(err)
		}
		return d
	}
	return mustNew(RxConfig{Rate: basebandRate, Tier: e.tier, OnBursts: each(func(*Burst) {}), Format: IQFormatCU8})
}

func liveBytesUnder(f any) int64 {
	name := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	n, _ := runtime.MemProfile(nil, true)
	var p []runtime.MemProfileRecord
	for {
		p = make([]runtime.MemProfileRecord, n+64)
		var ok bool
		if n, ok = runtime.MemProfile(p, true); ok {
			p = p[:n]
			break
		}
	}
	var live int64
	for i := range p {
		frames := runtime.CallersFrames(p[i].Stack())
		for {
			fr, more := frames.Next()
			if fr.Function == name {
				live += p[i].InUseBytes()
				break
			}
			if !more {
				break
			}
		}
	}
	return live
}

func steadyAllocs(x []complex64, e benchEntry) float64 {
	const chunk = 1 << 14
	switch e.mode {
	case benchAudio:
		return discSteadyAllocs(discAudio(x, benchAudioHz), e.tier)
	case benchIQLow:
		x = lowIQ(x, iq0LowHz)
	}
	cfg := RxConfig{Rate: basebandRate, Tier: e.tier, Format: IQFormatCU8}
	if e.mode == benchIQLow {
		cfg = RxConfig{Tier: Tier0, Rate: iq0LowHz, Format: IQFormatCU8}
	}
	r := mustNew(cfg)
	must(r.writeBB(x[:chunk]))
	n := len(x)/chunk - 1
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	for i := 1; i <= n; i++ {
		must(r.writeBB(x[i*chunk : (i+1)*chunk]))
	}
	runtime.ReadMemStats(&b)
	return float64(b.Mallocs-a.Mallocs) / float64(n)
}

func discSteadyAllocs(a []int16, tier Tier) float64 {
	const chunk = 1 << 11
	r, err := NewAudioReceiver(&AudioRxConfig{Rate: benchAudioHz, Tier: tier, OnBursts: each(func(*Burst) {})})
	if err != nil {
		panic(err)
	}
	must(r.writeAll(a[:chunk]))
	n := len(a)/chunk - 1
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	for i := 1; i <= n; i++ {
		must(r.writeAll(a[i*chunk : (i+1)*chunk]))
	}
	runtime.ReadMemStats(&m1)
	return float64(m1.Mallocs-m0.Mallocs) / float64(n)
}

func TestRxBench(t *testing.T) {
	t.Parallel()
	if testmode.Race {
		t.Skip("compares one goroutine's numbers with the table; the runs without -race check them")
	}
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
	if os.Getenv("DMRRF_RXBENCH") == "cpu" {
		benchCPUOnly(t, base)
		return
	}
	slots, truth := benchStream(t, 6)
	fs, ft := benchStream(t, 30)
	sig, fsig, noise := cleanBaseband(slots, 1400), cleanBaseband(fs, 1400), cleanBaseband(nil, 0)
	entries := benchEntries()
	pool := newBenchPool(runtime.GOMAXPROCS(0), benchLevels)
	tiers := make([]benchTier, len(entries))
	var es []benchEntry
	var at []int
	for i, e := range entries {
		if want, have := base.find(e); full && !benchTierSelected(e) && have {
			tiers[i] = want
			continue
		}
		es, at = append(es, e), append(at, i)
	}
	fph := make([]float64, len(es))
	var wg sync.WaitGroup
	if full {
		for j, e := range es {
			wg.Add(1)
			pool.do(benchFalsePrio, func() {
				defer wg.Done()
				fph[j] = falsePerHour(e, noise)
			})
		}
	}
	fc := make([][]benchCase, len(es))
	for j, e := range es {
		fc[j] = fastCases(e)
	}
	fastPts := benchShared{t, pool, sig, truth, es}.run(fc)
	var deep [][]benchPoint
	if full {
		sh := benchShared{t, pool, fsig, ft, es}
		cs := make([][]benchCase, len(es))
		for j := range cs {
			cs[j] = fullCases()
		}
		deep = sh.run(cs)
		sh.deepen(deep)
	}
	wg.Wait()
	for j, e := range es {
		bt := benchTier{Mode: e.mode, Tier: e.number(), Fast: fastPts[j]}
		if full {
			bt.Full, bt.FalsePerHour = deep[j], fph[j]
		}
		tiers[at[j]] = bt
	}
	out := benchData{Machine: base.Machine}
	for i, e := range entries {
		bt := tiers[i]
		want, have := base.find(e)
		if full && !benchTierSelected(e) && have {
			out.Tiers = append(out.Tiers, want)
			continue
		}
		if full {
			bt.Allocs = steadyAllocs(fsig.noisy(6, 1), e)
			bt.RAMBytes = receiverRAM(e)
			bt.NsPerSecond, bt.WorstNs, bt.WorstCN = benchCPU(t, fsig, e)
		}
		if have {
			if !fast {
				checkBench(t, e, want.Fast, bt.Fast)
			}
			if !full {
				bt.Full, bt.NsPerSecond, bt.FalsePerHour, bt.Allocs, bt.RAMBytes = want.Full, want.NsPerSecond, want.FalsePerHour, want.Allocs, want.RAMBytes
				bt.WorstNs, bt.WorstCN = want.WorstNs, want.WorstCN
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

var benchCPUGrid = []float64{0, 2, 4, 6, 8, 12, 20, 40}

const benchCPURuns = 3

func benchCPU(t *testing.T, sig benchSignal, e benchEntry) (at6, worst, worstCN float64) {
	t.Helper()
	for _, cn := range benchCPUGrid {
		x := sig.noisy(cn, 1)
		var f func()
		switch e.mode {
		case benchAudio:
			a := discAudio(x, benchAudioHz)
			f = func() { receiveAudioTier(t, a, e.tier) }
		case benchIQLow:
			y := lowIQ(x, iq0LowHz)
			f = func() { receiveLowIQ(y) }
		default:
			f = func() { receiveBaseband(x, e.tier) }
		}
		best := time.Duration(math.MaxInt64)
		for range benchCPURuns {
			start := time.Now()
			f()
			best = min(best, time.Since(start))
		}
		ns := float64(best) / (float64(len(x)) / basebandRate)
		if cn == 6 {
			at6 = ns
		}
		if ns > worst {
			worst, worstCN = ns, cn
		}
	}
	return at6, worst, worstCN
}

func benchCPUOnly(t *testing.T, base benchData) {
	fs, _ := benchStream(t, 30)
	fsig := cleanBaseband(fs, 1400)
	out := base
	out.Tiers = slices.Clone(base.Tiers)
	for i := range out.Tiers {
		bt := &out.Tiers[i]
		e := benchEntry{bt.Mode, Tier0 + Tier(bt.Tier)}
		if benchTierSelected(e) {
			bt.NsPerSecond, bt.WorstNs, bt.WorstCN = benchCPU(t, fsig, e)
		}
	}
	b, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rxbenchPath, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func benchPriority(e benchEntry) int {
	return slices.IndexFunc(benchEntriesByCost(), func(x benchEntry) bool { return x == e })
}

func benchEntriesByCost() []benchEntry {
	return []benchEntry{
		{benchIQ, Tier3}, {benchAudio, Tier3}, {benchAudio, Tier2}, {benchIQ, Tier2}, {benchIQ, Tier1},
		{benchIQ, Tier0}, {benchIQLow, Tier0}, {benchAudio, Tier1}, {benchAudio, Tier0},
	}
}

func benchTierSelected(e benchEntry) bool {
	v := os.Getenv("DMRRF_RXBENCH_TIERS")
	return v == "" || slices.Contains(strings.Split(v, ","), e.name())
}

func checkBench(t *testing.T, e benchEntry, want, got []benchPoint) {
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
					t.Errorf("%s, %s at C/N %.1f dB: %.3f recovered, baseline %.3f", e.name(), k, w.CN, gr, wr)
				}
			}
		}
	}
}
