package rf

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const extcmpPath = "testdata/extcmp.json"

const (
	extIFLen  = 95
	extIFHz   = 6000.0
	extIFBeta = 8.5
)

const (
	extDSDFME    = "dsd-fme"
	extMMDVM     = "mmdvm"
	extMMDVMFix  = "mmdvm-fixed"
	extMMDVMGain = -2.4
)

func extBin(name string) string {
	switch name {
	case extDSDFME:
		return os.Getenv("DMRRF_DSDFME")
	case extMMDVM:
		return os.Getenv("DMRRF_MMDVM")
	case extMMDVMFix:
		return os.Getenv("DMRRF_MMDVMFIX")
	}
	return ""
}

func extIFTaps() []float64 {
	h := make([]float64, extIFLen)
	for i, v := range kaiserLowpass(extIFLen, extIFHz/basebandRate, extIFBeta) {
		h[i] = float64(v)
	}
	return h
}

func extDisc(x []complex64) []float64 {
	h := extIFTaps()
	lead := (extIFLen - audioIFLen) / 2
	n := max(len(x)-len(h)+1, 0)
	out := make([]float64, lead, lead+n)
	var pr, pi float64
	for i := range n {
		var yr, yi float64
		for k, t := range h {
			v := x[i+k]
			yr += float64(real(v)) * t
			yi += float64(imag(v)) * t
		}
		out = append(out, discHz(yr, yi, pr, pi))
		pr, pi = yr, yi
	}
	return out
}

func extDecimate(disc []float64, hz int) []float64 {
	step := basebandRate / hz
	aa := lowpassTaps(63, min(6250, 0.42*float64(hz))/basebandRate)
	out := make([]float64, 0, len(disc)/step)
	for i := 0; i+len(aa) <= len(disc); i += step {
		var v float64
		for k, t := range aa {
			v += disc[i+k] * t
		}
		out = append(out, v)
	}
	return out
}

func extInt16(hz []float64, gain, offset float64) []int16 {
	out := make([]int16, len(hz))
	for i, v := range hz {
		out[i] = int16(max(min(math.Round(v*gain+offset), math.MaxInt16), math.MinInt16))
	}
	return out
}

func extS16(a []int16) []byte {
	b := make([]byte, 2*len(a))
	for i, v := range a {
		b[2*i], b[2*i+1] = byte(v), byte(uint16(v)>>8)
	}
	return b
}

type extStream struct {
	name   string
	offset float64
	sig    benchSignal
	truth  []benchSlot
	first  float64
	ms     bool
}

func extTruth(bursts []layer2.RawBurst, voiceSync enums.SyncPattern) []benchSlot {
	truth := make([]benchSlot, len(bursts))
	sf, pos := -1, enums.VoiceBurstUnknown
	for i, rb := range bursts {
		ts := &truth[i]
		ts.burst = rb
		b, err := layer2.NewBurstFromBytes(rb)
		switch {
		case err == nil && b.Kind == layer2.KindVoice && b.SyncPattern == voiceSync:
			sf++
			pos = enums.VoiceBurstA
			ts.kind, ts.pos, ts.sf = "voice", pos, sf
		case pos != enums.VoiceBurstUnknown && pos < enums.VoiceBurstF && (err != nil || b.Kind != layer2.KindData):
			pos++
			ts.kind, ts.pos, ts.sf = "voice", pos, sf
		case err == nil && b.Kind == layer2.KindData:
			pos = enums.VoiceBurstUnknown
			ts.kind, ts.dt = "data", b.SlotType.DataType
			if ts.dt == enums.DataTypeIdle {
				ts.kind = "idle"
			}
		default:
			ts.kind = "idle"
		}
	}
	return truth
}

func extMSBursts(t testing.TB, n int) []layer2.RawBurst {
	t.Helper()
	cfg := rftx.SeqConfig{ColorCode: 7, Source: 3009101, Dest: 9, Group: true, Message: "DMRGO RXBENCH", Superframes: n, Preambles: 2}
	v, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := slices.Clone(v.Bursts)
	for range 2 {
		for _, seq := range []string{"data12", "data34", "data1", "preamble"} {
			c := cfg
			c.Group, c.Dest = false, 3009201
			b, err := rftx.Sequence(seq, c)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, b.Bursts...)
		}
	}
	return out
}

func extMSSignal(bursts []layer2.RawBurst, offsetHz float64) benchSignal {
	fm, env := rftx.Modulate(bursts, rftx.Schedule{Slot: 1})
	pad := int(benchPad * basebandRate)
	s := benchSignal{make([]float64, len(fm)+2*pad), make([]float64, len(fm)+2*pad)}
	var ph float64
	for k := range fm {
		ph += 2 * math.Pi * (fm[k] + offsetHz) / basebandRate
		s.re[pad+k], s.im[pad+k] = env[k]*math.Cos(ph), env[k]*math.Sin(ph)
	}
	return s
}

func extStreams(t testing.TB) []extStream {
	t.Helper()
	slots, truth := benchStream(t, 6)
	msb := extMSBursts(t, 6)
	mt := make([]benchSlot, 2*len(msb))
	for i, ts := range extTruth(msb, enums.SyncMSSourcedVoice) {
		mt[2*i] = benchSlot{kind: "idle"}
		mt[2*i+1] = ts
	}
	bsFirst := (slotSeconds-burstSeconds)/2 + float64(cachSymbols)/symbolRate + benchPad
	msFirst := (slotSeconds-burstSeconds)/2 + benchPad
	out := make([]extStream, 0, 4)
	for _, off := range []float64{1400, 0} {
		out = append(out,
			extStream{name: "bs", offset: off, sig: cleanBaseband(slots, off), truth: truth, first: bsFirst},
			extStream{name: "ms", offset: off, sig: extMSSignal(msb, off), truth: mt, first: msFirst, ms: true})
	}
	return out
}

func extScore(truth []benchSlot, got []Burst, first float64) map[string]*benchScore {
	s := benchScorer{sc: map[string]*benchScore{}, frags: map[int][4][4]byte{}, nfrag: map[int]int{}}
	for _, n := range benchPayloads() {
		s.sc[n] = &benchScore{}
	}
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

func extAlign(truth []benchSlot, got []Burst, first float64) float64 {
	best, shift := -1, 0.0
	for _, g := range got[:min(len(got), 20)] {
		for i, ts := range truth {
			if ts.kind == "idle" || g.Data != ts.burst {
				continue
			}
			d := g.Seconds - first - float64(i)*slotSeconds
			n := 0
			for _, h := range got {
				j := int(math.Round((h.Seconds - d - first) / slotSeconds))
				if j >= 0 && j < len(truth) && h.Data == truth[j].burst {
					n++
				}
			}
			if n > best {
				best, shift = n, d
			}
		}
	}
	return shift
}

func runMMDVMBursts(ctx context.Context, bin string, a []int16, cc int) ([]Burst, time.Duration, error) {
	cmd := exec.CommandContext(ctx, bin, "/dev/stdin", strconv.Itoa(cc))
	cmd.Stdin = bytes.NewReader(extS16(a))
	out, err := cmd.Output()
	if err != nil {
		return nil, 0, err
	}
	var bs []Burst
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 3 {
			continue
		}
		n, _ := strconv.Atoi(f[0])
		b, err := hex.DecodeString(f[2])
		if err != nil || len(b) != 33 {
			continue
		}
		var r layer2.RawBurst
		copy(r[:], b)
		bs = append(bs, Burst{Data: r, Seconds: float64(n) / 24000})
	}
	return bs, cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime(), nil
}

type dsdItem struct {
	kind string
	data []byte
	ok   bool
}

func runDSDFME(ctx context.Context, bin string, a []int16) ([]dsdItem, time.Duration, error) {
	cmd := exec.CommandContext(ctx, bin, "-fs", "-i", "-", "-o", "null", "-Z")
	cmd.Stdin = bytes.NewReader(extS16(a))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return nil, 0, err
	}
	var items []dsdItem
	sc := bufio.NewScanner(&out)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		switch {
		case len(f) >= 2 && f[0] == "AMBE":
			if b, err := hex.DecodeString(f[1]); err == nil && len(b) == 7 {
				items = append(items, dsdItem{kind: "ambe", data: b})
			}
		case len(f) == 5 && f[0] == "ZZFEC":
			t, _ := strconv.ParseUint(f[1], 16, 8)
			b, err := hex.DecodeString(f[4])
			if err != nil {
				continue
			}
			items = append(items, dsdItem{kind: dsdKind(uint8(t)), data: b, ok: f[3] == "1"})
		}
	}
	return items, cmd.ProcessState.UserTime() + cmd.ProcessState.SystemTime(), nil
}

func dsdKind(t uint8) string {
	switch t {
	case 0x01, 0x02:
		return "lc"
	case 0x03:
		return "csbk"
	case 0x06:
		return "header"
	case 0x07:
		return "rate12"
	case 0x08:
		return "rate34"
	case 0x0A:
		return "rate1"
	case 0xEB:
		return "elc"
	}
	return ""
}

func lcsCount(want, got [][]byte) int {
	prev := make([]int, len(got)+1)
	cur := make([]int, len(got)+1)
	for i := range want {
		for j := range got {
			if bytes.Equal(want[i], got[j]) {
				cur[j+1] = prev[j] + 1
			} else {
				cur[j+1] = max(prev[j+1], cur[j])
			}
		}
		prev, cur = cur, prev
	}
	return prev[len(got)]
}

func dsdTruth(truth []benchSlot) map[string][][]byte {
	w := map[string][][]byte{}
	for i, ts := range truth {
		switch ts.kind {
		case "voice":
			f, _ := ts.burst.AMBEFrames()
			for k := range f {
				w["ambe"] = append(w["ambe"], f[k][:])
				w["ambe_c0c1"] = append(w["ambe_c0c1"], f[k][:3])
			}
			if ts.pos == enums.VoiceBurstE {
				var wf [4][4]byte
				for j := range wf {
					wf[j] = truth[i-2*(3-j)].burst.EmbeddedData()
				}
				lc, _ := strictEmbeddedLC(wf)
				w["elc"] = append(w["elc"], lc[:])
			}
		case "data":
			k := benchDataKey(ts.dt)
			if k == "" {
				continue
			}
			var p []byte
			switch ts.dt {
			case enums.DataTypeRate34:
				a, _ := ts.burst.Rate34Payload()
				p = a[:]
			case enums.DataTypeRate1:
				a, _ := ts.burst.Rate1Payload()
				p = a[:]
			default:
				a, _ := ts.burst.BPTCPayload()
				p = a[:]
			}
			w[k] = append(w[k], p)
		}
	}
	return w
}

func scoreDSD(want map[string][][]byte, items []dsdItem) map[string]*benchScore {
	got := map[string][][]byte{}
	for _, it := range items {
		switch it.kind {
		case "ambe":
			got["ambe"] = append(got["ambe"], it.data)
			got["ambe_c0c1"] = append(got["ambe_c0c1"], it.data[:3])
		case "elc":
			if it.ok {
				got["elc"] = append(got["elc"], it.data)
			}
		case "":
		default:
			got[it.kind] = append(got[it.kind], it.data)
		}
	}
	sc := map[string]*benchScore{}
	for k, w := range want {
		sc[k] = &benchScore{Truth: len(w), Exact: lcsCount(w, got[k])}
	}
	return sc
}

type extPoint struct {
	CN       float64                `json:"cn"`
	Seeds    int                    `json:"seeds"`
	Payloads map[string]*benchScore `json:"payloads"`
}

type extDecoder struct {
	Name     string     `json:"name"`
	CPUNs    float64    `json:"ns_per_second,omitempty"`
	WorstNs  float64    `json:"worst_ns_per_second,omitempty"`
	Points   []extPoint `json:"points"`
	external bool
}

type extPair struct {
	Ours, Ext, Payload string
	CN                 float64
	Sum, SumSq         int64
}

func (p extPair) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{p.Ours, p.Ext, p.Payload, p.CN, p.Sum, p.SumSq})
}

type extStreamData struct {
	Name     string       `json:"name"`
	OffsetHz float64      `json:"offset_hz"`
	Decoders []extDecoder `json:"decoders"`
	Pairs    []extPair    `json:"pairs"`
}

type extData struct {
	Machine  string            `json:"machine"`
	Versions map[string]string `json:"versions"`
	Seeds    int               `json:"seeds"`
	IFTaps   int               `json:"if_taps"`
	Streams  []extStreamData   `json:"streams"`
}

func extPayloads() []string {
	return []string{"ambe", "ambe_c0c1", "rate12", "rate34", "rate1", "csbk", "lc", "header", "elc", "emb", "slottype"}
}

func extCNs() []float64 {
	var out []float64
	for cn := 0.0; cn <= 20; cn++ {
		out = append(out, cn)
	}
	return out
}

func extMachine() string {
	b, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return runtime.GOARCH
	}
	for l := range strings.Lines(string(b)) {
		if k, v, ok := strings.Cut(l, ":"); ok && strings.TrimSpace(k) == "model name" {
			return strings.TrimSpace(v)
		}
	}
	return runtime.GOARCH
}

type extRun struct {
	dec  int
	cn   int
	seed int
	sc   map[string]*benchScore
	cpu  time.Duration
}

func extDecode(t *testing.T, st extStream, d extDecoder, e benchEntry, x []complex64, disc []float64, a9 *[]int16) (map[string]*benchScore, time.Duration, error) {
	switch d.Name {
	case extDSDFME:
		items, c, err := runDSDFME(t.Context(), extBin(d.Name), extInt16(extDecimate(disc, 48000), 4.8, 0))
		return scoreDSD(dsdTruth(st.truth), items), c, err
	case extMMDVM, extMMDVMFix:
		got, c, err := runMMDVMBursts(t.Context(), extBin(d.Name), extInt16(extDecimate(disc, 24000), extMMDVMGain, 0), 7)
		sh := extAlign(st.truth, got, st.first)
		for i := range got {
			got[i].Seconds -= sh
		}
		return extScore(st.truth, got, st.first), c, err
	}
	var got []Burst
	if e.mode == benchAudio {
		if *a9 == nil {
			*a9 = extInt16(extDecimate(disc, benchAudioHz), audioGain, audioOffset)
		}
		got = receiveAudioTier(t, *a9, e.tier)
	} else {
		got = benchReceive(t, x, e)
	}
	return extScore(st.truth, got, st.first), 0, nil
}

type extRunner struct {
	t     *testing.T
	ours  []benchEntry
	cns   []float64
	seeds int
}

func (r extRunner) decoders(st extStream) []extDecoder {
	decs := make([]extDecoder, 0, len(r.ours)+3)
	for _, e := range r.ours {
		decs = append(decs, extDecoder{Name: e.name()})
	}
	decs = append(decs, extDecoder{Name: extDSDFME, external: true})
	if st.ms {
		decs = append(decs, extDecoder{Name: extMMDVM, external: true}, extDecoder{Name: extMMDVMFix, external: true})
	}
	return decs
}

func (r extRunner) decodeAll(st extStream, decs []extDecoder) ([][][]map[string]*benchScore, [][]time.Duration) {
	runs := make(chan extRun, 64)
	go func() {
		var wg sync.WaitGroup
		sem := make(chan struct{}, runtime.GOMAXPROCS(0))
		for ci, cn := range r.cns {
			for s := range r.seeds {
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer func() { <-sem; wg.Done() }()
					r.decodeOne(st, decs, ci, cn, s, runs)
				}()
			}
		}
		wg.Wait()
		close(runs)
	}()
	per := make([][][]map[string]*benchScore, len(decs))
	cpus := make([][]time.Duration, len(decs))
	for i := range per {
		per[i] = make([][]map[string]*benchScore, len(r.cns))
		cpus[i] = make([]time.Duration, len(r.cns))
		for j := range per[i] {
			per[i][j] = make([]map[string]*benchScore, r.seeds)
		}
	}
	for x := range runs {
		per[x.dec][x.cn][x.seed] = x.sc
		cpus[x.dec][x.cn] += x.cpu
	}
	return per, cpus
}

func (r extRunner) decodeOne(st extStream, decs []extDecoder, ci int, cn float64, s int, runs chan<- extRun) {
	x := st.sig.noisy(nil, cn, 20000+uint64(s))
	disc := extDisc(x)
	var a9 []int16
	for di, d := range decs {
		var e benchEntry
		if di < len(r.ours) {
			e = r.ours[di]
		}
		sc, cpu, err := extDecode(r.t, st, d, e, x, disc, &a9)
		if err != nil {
			r.t.Error(d.Name, err)
		}
		runs <- extRun{di, ci, s, sc, cpu}
	}
}

func extSum(scs []map[string]*benchScore) map[string]*benchScore {
	out := map[string]*benchScore{}
	for _, sc := range scs {
		for k, v := range sc {
			if !slices.Contains(extPayloads(), k) {
				continue
			}
			a := out[k]
			if a == nil {
				a = &benchScore{}
				out[k] = a
			}
			a.Truth += v.Truth
			a.Exact += v.Exact
		}
	}
	return out
}

func (r extRunner) stream(st extStream) extStreamData {
	decs := r.decoders(st)
	per, cpus := r.decodeAll(st, decs)
	sigSeconds := float64(len(st.sig.re)) / basebandRate
	sd := extStreamData{Name: st.name, OffsetHz: st.offset}
	for di := range decs {
		d := &decs[di]
		for ci, cn := range r.cns {
			d.Points = append(d.Points, extPoint{CN: cn, Seeds: r.seeds, Payloads: extSum(per[di][ci])})
			if !d.external {
				continue
			}
			ns := float64(cpus[di][ci].Nanoseconds()) / float64(r.seeds) / sigSeconds
			d.WorstNs = max(d.WorstNs, ns)
			if cn == 6 {
				d.CPUNs = ns
			}
		}
	}
	for oi := range r.ours {
		for xi := len(r.ours); xi < len(decs); xi++ {
			for ci, cn := range r.cns {
				for _, k := range extPayloads() {
					if pr, ok := extPairOf(per[oi][ci], per[xi][ci], k); ok {
						pr.Ours, pr.Ext, pr.Payload, pr.CN = decs[oi].Name, decs[xi].Name, k, cn
						sd.Pairs = append(sd.Pairs, pr)
					}
				}
			}
		}
	}
	sd.Decoders = decs
	return sd
}

func extOurs() []benchEntry {
	tiers := os.Getenv("DMRRF_EXTCMP_TIERS")
	var ours []benchEntry
	for _, e := range benchEntries() {
		if tiers == "" && e.tier != Tier3 || slices.Contains(strings.Split(tiers, ","), e.name()) {
			ours = append(ours, e)
		}
	}
	return ours
}

func TestExtCompare(t *testing.T) {
	t.Parallel()
	if os.Getenv("DMRRF_EXTCMP") == "" {
		t.Skip("DMRRF_EXTCMP=full runs the comparison with the external receivers")
	}
	for _, n := range []string{extDSDFME, extMMDVM, extMMDVMFix} {
		if extBin(n) == "" {
			t.Fatal("DMRRF_DSDFME, DMRRF_MMDVM and DMRRF_MMDVMFIX name the dsd-fme and mmdvmrx binaries")
		}
	}
	r := extRunner{t: t, ours: extOurs(), cns: extCNs(), seeds: 192}
	if s, err := strconv.Atoi(os.Getenv("DMRRF_EXTCMP_SEEDS")); err == nil {
		r.seeds = s
	}
	var base extData
	if b, err := os.ReadFile(extcmpPath); err == nil {
		if err := json.Unmarshal(b, &base); err != nil {
			t.Fatal(err)
		}
	}
	data := extData{Machine: extMachine(), Versions: map[string]string{}, Seeds: r.seeds, IFTaps: extIFLen}
	for v := range strings.SplitSeq(os.Getenv("DMRRF_EXT_VERSIONS"), ",") {
		if k, x, ok := strings.Cut(v, "="); ok {
			data.Versions[k] = x
		}
	}
	for _, st := range extStreams(t) {
		sd := r.stream(st)
		if t.Failed() {
			t.FailNow()
		}
		data.Streams = append(data.Streams, sd)
		t.Logf("%s %g Hz done", st.name, st.offset)
	}
	data = mergeExt(base, data)
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(extcmpPath, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func extPairOf(ours, ext []map[string]*benchScore, k string) (extPair, bool) {
	var pr extPair
	var a0, b0, n0 int
	for s := range ours {
		a, b := ours[s][k], ext[s][k]
		if a == nil || b == nil {
			return extPair{}, false
		}
		d := int64(a.Exact - b.Exact)
		pr.Sum += d
		pr.SumSq += d * d
		a0, b0, n0 = a0+a.Exact, b0+b.Exact, n0+a.Truth
	}
	return pr, len(ours) > 0 && (a0 != b0 || a0 != 0 && a0 != n0)
}

func (p *extPair) UnmarshalJSON(b []byte) error {
	var a []json.RawMessage
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	if len(a) != 6 {
		return fmt.Errorf("pair of %d fields", len(a))
	}
	for i, v := range []any{&p.Ours, &p.Ext, &p.Payload, &p.CN, &p.Sum, &p.SumSq} {
		if err := json.Unmarshal(a[i], v); err != nil {
			return err
		}
	}
	return nil
}

func mergeExt(base, cur extData) extData {
	if base.Seeds != cur.Seeds || base.IFTaps != cur.IFTaps {
		return cur
	}
	for si, s := range cur.Streams {
		for _, bs := range base.Streams {
			if bs.Name != s.Name || bs.OffsetHz != s.OffsetHz {
				continue
			}
			have := map[string]bool{}
			for _, d := range s.Decoders {
				have[d.Name] = true
			}
			for _, d := range bs.Decoders {
				if !have[d.Name] {
					s.Decoders = append(s.Decoders, d)
				}
			}
			for _, p := range bs.Pairs {
				if !have[p.Ours] {
					s.Pairs = append(s.Pairs, p)
				}
			}
		}
		cur.Streams[si] = s
	}
	return cur
}
