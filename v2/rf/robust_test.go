package rf

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type rxItem struct {
	burst layer2.RawBurst
	at    float64
	ch    int
	voice bool
	sync  bool
}

type rxTx struct {
	fm, env []float64
	items   []rxItem
	starts  []float64
}

func rxBS(t testing.TB, n int) rxTx {
	slots, truth := benchStream(t, n)
	fm, env := rftx.ModulateBS(slots)
	first := (slotSeconds-burstSeconds)/2 + float64(cachSymbols)/symbolRate
	var items []rxItem
	for i, ts := range truth {
		if ts.kind != "voice" && (ts.kind != "data" || benchDataKey(ts.dt) == "") {
			continue
		}
		items = append(items, rxItem{burst: ts.burst, at: first + float64(i)*slotSeconds, ch: i % 2, voice: ts.kind == "voice",
			sync: ts.kind == "data" || ts.pos == enums.VoiceBurstA})
	}
	return rxTx{fm: fm, env: env, items: items}
}

func rxMS(t testing.TB, n int) rxTx {
	return rxMSCall(t, n, "data34")
}

func rxMSCall(t testing.TB, n int, data string) rxTx {
	t.Helper()
	cfg := rftx.SeqConfig{ColorCode: 3, Source: 3191868, Dest: 9, Group: true, Message: "DMRGO MS", Superframes: n, Slot: 1}
	voice, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := rftx.Sequence(data, cfg)
	if err != nil {
		t.Fatal(err)
	}
	bursts := append(slices.Clone(voice.Bursts), pkt.Bursts...)
	fm, env := rftx.Modulate(bursts, rftx.Schedule{Slot: 1})
	var items []rxItem
	pos := enums.VoiceBurstUnknown
	for i, raw := range bursts {
		at := float64(2*i+1)*slotSeconds + (slotSeconds-burstSeconds)/2
		b, err := layer2.NewBurstFromBytes(raw)
		switch {
		case err == nil && b.Kind == layer2.KindVoice && b.SyncPattern == enums.SyncMSSourcedVoice:
			pos = enums.VoiceBurstA
			items = append(items, rxItem{burst: raw, at: at, voice: true, sync: true})
		case pos != enums.VoiceBurstUnknown && pos < enums.VoiceBurstF && (err != nil || b.Kind != layer2.KindData):
			pos++
			items = append(items, rxItem{burst: raw, at: at, voice: true})
		case err == nil && b.Kind == layer2.KindData:
			pos = enums.VoiceBurstUnknown
			if benchDataKey(b.SlotType.DataType) != "" {
				items = append(items, rxItem{burst: raw, at: at, sync: true})
			}
		}
	}
	return rxTx{fm: fm, env: env, items: items}
}

func rxMSCalls(t testing.TB, n int) rxTx {
	one := rxMSCall(t, 1, "data1")
	var out rxTx
	for c := range n {
		k := int(0.3*basebandRate) + (3*c+7)%samplesPerSy
		if c == 0 {
			k = 0
		}
		at := float64(len(out.fm)+k) / basebandRate
		if c > 0 {
			out.starts = append(out.starts, at)
		}
		out.fm = append(append(out.fm, make([]float64, k)...), one.fm...)
		out.env = append(append(out.env, make([]float64, k)...), one.env...)
		for _, it := range one.items {
			it.at += at
			it.ch = c
			out.items = append(out.items, it)
		}
	}
	return out
}

type rxCond struct {
	name                      string
	cw, zero, pad             float64
	offsetHz, driftHz         float64
	ppm                       float64
	stepDB, fadeDB            float64
	late                      int
	gap                       float64
	jump                      float64
	level                     float64
	bytes                     bool
	dc                        complex128
	audioGain, audioDC        float64
	iqOnly, audioOnly, highCN bool
}

const (
	rxByteRate  = 240000
	rampSamples = (slotSamplesN - burstSymbols*samplesPerSy) / 2
)

func rxConds() []rxCond {
	base := rxCond{pad: 0.3, level: 1, audioGain: 1}
	var out []rxCond
	add := func(name string, f func(c *rxCond)) {
		c := base
		c.name = name
		f(&c)
		out = append(out, c)
	}
	add("clean", func(*rxCond) {})
	for _, s := range []float64{0.1, 0.5, 2} {
		add(fmt.Sprintf("cw %.1f s", s), func(c *rxCond) { c.cw = s })
	}
	add("zeros 1 s", func(c *rxCond) { c.zero = 1 })
	add("step +20 dB", func(c *rxCond) { c.stepDB = 20 })
	add("step -20 dB", func(c *rxCond) { c.stepDB = -20 })
	add("fade -20 dB", func(c *rxCond) { c.fadeDB, c.highCN = -20, true })
	add("level 1e-4", func(c *rxCond) { c.level, c.iqOnly = 1e-4, true })
	add("level 1e4", func(c *rxCond) { c.level, c.iqOnly = 1e4, true })
	add("u8 0.25", func(c *rxCond) { c.bytes, c.level, c.iqOnly = true, 0.25, true })
	add("u8 3 LSB", func(c *rxCond) { c.bytes, c.level, c.iqOnly = true, 3.0/127.5, true })
	add("u8 clip x2", func(c *rxCond) { c.bytes, c.level, c.iqOnly = true, 2, true })
	add("u8 dc", func(c *rxCond) { c.bytes, c.level, c.dc, c.iqOnly = true, 0.25, complex(0.2, -0.15), true })
	add("u8 cw 0.5 s", func(c *rxCond) { c.bytes, c.level, c.cw, c.iqOnly = true, 0.25, 0.5, true })
	add("u8 dc cw 0.5 s", func(c *rxCond) { c.bytes, c.level, c.dc, c.cw, c.iqOnly = true, 0.25, complex(0.2, -0.15), 0.5, true })
	add("u8 step +20 dB", func(c *rxCond) { c.bytes, c.level, c.stepDB, c.iqOnly = true, 0.04, 20, true })
	add("u8 step -20 dB", func(c *rxCond) { c.bytes, c.level, c.stepDB, c.iqOnly = true, 0.4, -20, true })
	add("audio x0.02", func(c *rxCond) { c.audioGain, c.audioOnly = 0.02, true })
	add("audio x25", func(c *rxCond) { c.audioGain, c.audioOnly = 25, true })
	add("audio dc", func(c *rxCond) { c.audioDC, c.audioOnly = 12000, true })
	for _, hz := range []float64{1000, 3000, -3000} {
		add(fmt.Sprintf("offset %+.0f Hz", hz), func(c *rxCond) { c.offsetHz = hz })
	}
	add("drift 100 Hz/s", func(c *rxCond) { c.offsetHz, c.driftHz = -150, 100 })
	for _, p := range []float64{50, -50, 200, -200} {
		add(fmt.Sprintf("clock %+.0f ppm", p), func(c *rxCond) { c.ppm = p })
	}
	for k := range 6 {
		add("late "+string(rune('A'+k)), func(c *rxCond) { c.late = k + 1 })
	}
	for _, g := range []float64{0.05, 0.15, 0.5, 1, 2} {
		add(fmt.Sprintf("rekey %.2f s", g), func(c *rxCond) { c.gap = g })
	}
	for _, j := range []float64{0.002, -0.002, 0.015} {
		add(fmt.Sprintf("jump %+.0f ms", 1000*j), func(c *rxCond) { c.jump = j })
	}
	return out
}

type rxInput struct {
	x      []complex64
	iq     []byte
	audio  []int16
	items  []rxItem
	events []float64
	jump   float64
}

func rxSeed(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func rxLate(tx rxTx, late int) float64 {
	var a []float64
	for _, it := range tx.items {
		if it.voice && it.sync {
			a = append(a, it.at)
		}
	}
	return a[1] + float64(late-1)*2*slotSeconds - (slotSeconds-burstSeconds)/2 - 0.0113
}

type rxLine struct {
	fm, env []float64
	items   []rxItem
	events  []float64
}

func (l *rxLine) shift(from int, d float64) {
	for i := range l.items {
		if l.items[i].at*basebandRate >= float64(from) {
			l.items[i].at += d
		}
	}
}

func (l *rxLine) jump(d float64) {
	mid := (len(l.fm)/2)/slotSamplesN*slotSamplesN + rampSamples
	n := int(math.Abs(d) * basebandRate)
	if d > 0 {
		l.fm = slices.Insert(l.fm, mid, make([]float64, n)...)
		ins := make([]float64, n)
		for i := range ins {
			ins[i] = l.env[mid]
		}
		l.env = slices.Insert(l.env, mid, ins...)
		l.shift(mid, d)
	} else {
		l.fm, l.env = slices.Delete(l.fm, mid, mid+n), slices.Delete(l.env, mid, mid+n)
		l.items = slices.DeleteFunc(l.items, func(it rxItem) bool {
			return it.at*basebandRate+burstSymbols*samplesPerSy > float64(mid) && it.at*basebandRate < float64(mid+n)
		})
		l.shift(mid+n, d)
	}
	l.events = append(l.events, float64(mid)/basebandRate)
}

func (l *rxLine) rekey(gap float64, starts []float64) {
	g := int(gap*basebandRate) + 337
	n := len(l.fm)
	second := slices.Clone(l.items)
	for i := range second {
		second[i].at += float64(n+g) / basebandRate
		second[i].ch += 1000
	}
	l.fm = append(append(l.fm, make([]float64, g)...), l.fm[:n]...)
	l.env = append(append(l.env, make([]float64, g)...), l.env[:n]...)
	l.items = append(l.items, second...)
	l.events = append(l.events, float64(n+g)/basebandRate)
	for _, a := range starts {
		l.events = append(l.events, a+float64(n+g)/basebandRate)
	}
}

func (l *rxLine) lead(cw float64) {
	k := int(cw * basebandRate)
	l.fm = append(make([]float64, k), l.fm...)
	env := make([]float64, k, k+len(l.env))
	for i := range env {
		env[i] = 1
	}
	l.env = append(env, l.env...)
	for i := k; i < k+2*rampSamples && l.env[i] < 1; i++ {
		l.env[i] = 1
	}
}

func rxBuild(tx rxTx, c rxCond, cn float64, audio bool, seed uint64) rxInput {
	l := rxLine{fm: slices.Clone(tx.fm), env: slices.Clone(tx.env), items: slices.Clone(tx.items)}
	if c.jump != 0 {
		l.jump(c.jump)
	}
	if c.gap > 0 {
		l.rekey(c.gap, tx.starts)
	}
	l.events = append(l.events, tx.starts...)
	if c.cw > 0 {
		l.lead(c.cw)
	}
	fm, env, items, events := l.fm, l.env, l.items, l.events
	pre := c.pad
	if c.late > 0 {
		pre = 0
	}
	start := 0.0
	if c.late > 0 {
		start = rxLate(tx, c.late)
	}
	scale := 1 + c.ppm*1e-6
	rx := func(at float64) float64 { return c.zero + pre + (c.cw+at)/scale - start }
	for i := range items {
		items[i].at = rx(items[i].at)
	}
	items = slices.DeleteFunc(items, func(it rxItem) bool { return it.at < c.zero })
	for i := range events {
		events[i] = rx(events[i])
	}
	events = append([]float64{max(rx(0), c.zero)}, events...)
	rate := float64(basebandRate)
	if c.bytes {
		rate = rxByteRate
	}
	dur := float64(len(fm))/basebandRate + pre + c.pad - start
	n := int(dur * rate)
	nz := int(c.zero * rate)
	stepAt := rx((float64(len(fm))/basebandRate - c.cw) / 2)
	if c.stepDB != 0 || c.fadeDB != 0 {
		events = append(events, stepAt)
	}
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	sigma := math.Sqrt(rate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	var ph float64
	in := rxInput{items: items, events: events}
	if c.jump != 0 {
		in.jump = events[1]
	}
	if c.bytes {
		in.iq = make([]byte, 2*(nz+n))
		for i := range 2 * nz {
			in.iq[i] = 128
		}
	} else {
		in.x = make([]complex64, nz+n)
	}
	for i := range n {
		t := float64(i) / rate
		pos := (t + start - pre) * scale * basebandRate
		var f, a float64
		if k := int(math.Floor(pos)); k >= 0 && k+1 < len(fm) {
			u := pos - float64(k)
			f, a = fm[k]+u*(fm[k+1]-fm[k]), env[k]+u*(env[k+1]-env[k])
		}
		ph += 2 * math.Pi * (f + c.offsetHz + c.driftHz*t) / rate
		if ph > math.Pi {
			ph -= 2 * math.Pi
		}
		gain := c.level
		if c.fadeDB != 0 && t+c.zero >= stepAt {
			a *= math.Pow(10, c.fadeDB/20)
		}
		if c.stepDB != 0 && t+c.zero >= stepAt {
			gain *= math.Pow(10, c.stepDB/20)
		}
		v := complex(a*math.Cos(ph)+sigma*rng.NormFloat64(), a*math.Sin(ph)+sigma*rng.NormFloat64())
		v = v*complex(gain, 0) + c.dc
		if c.bytes {
			in.iq[2*(nz+i)] = byte(max(0, min(255, math.Round(real(v)*127.5+127.5))))
			in.iq[2*(nz+i)+1] = byte(max(0, min(255, math.Round(imag(v)*127.5+127.5))))
		} else {
			in.x[nz+i] = complex64(v)
		}
	}
	if audio {
		in.audio = discAudio(in.x, benchAudioHz)
		for i, v := range in.audio {
			in.audio[i] = int16(max(min(math.Round((float64(v)-audioOffset)*c.audioGain+audioOffset+c.audioDC), math.MaxInt16), math.MinInt16))
		}
		in.x = nil
	}
	return in
}

type rxRecv struct {
	benchEntry
	half bool
}

func (r rxRecv) name() string {
	if r.half {
		return r.benchEntry.name() + "h"
	}
	return r.benchEntry.name()
}

func rxRecvs() []rxRecv {
	var out []rxRecv
	for _, e := range benchEntries() {
		out = append(out, rxRecv{benchEntry: e})
	}
	return append(out, rxRecv{benchEntry{benchIQ, Tier0}, true}, rxRecv{benchEntry{benchIQ, Tier1}, true})
}

func rxReceive(t testing.TB, in rxInput, e rxRecv) []Burst {
	cfg := Config{Tier: e.tier}
	if e.half {
		cfg.BasebandHz = BasebandRate / 2
	}
	switch {
	case e.mode == benchAudio:
		return receiveAudioTier(t, in.audio, e.tier)
	case in.iq != nil:
		cfg.Rate = rxByteRate
		r := mustNew(cfg)
		var out []Burst
		for k := 0; k < len(in.iq); k += 1 << 14 {
			out = append(out, r.Write(in.iq[k:min(k+1<<14, len(in.iq))])...)
		}
		return append(out, r.Flush()...)
	case e.half:
		h := make([]complex64, len(in.x)/2)
		for i := range h {
			h[i] = (in.x[2*i] + in.x[2*i+1]) / 2
		}
		r := mustNew(cfg)
		var out []Burst
		for k := 0; k < len(h); k += 2048 {
			out = append(out, r.WriteBaseband(h[k:min(k+2048, len(h))])...)
		}
		return append(out, r.Flush()...)
	}
	return receiveBaseband(in.x, e.tier)
}

type rxScore struct {
	exact, total int
	found, bound []float64
}

func rxMatch(in rxInput, got []Burst) rxScore {
	got = slices.Clone(got)
	slices.SortFunc(got, func(a, b Burst) int { return int(math.Copysign(1, a.Seconds-b.Seconds)) })
	ok := make([]bool, len(in.items))
	var sc rxScore
	for i, it := range in.items {
		k, _ := slices.BinarySearchFunc(got, it.at-0.002, func(b Burst, v float64) int { return int(math.Copysign(1, b.Seconds-v)) })
		n := 1
		if it.voice {
			n = 3
		}
		sc.total += n
		best := 0
		for ; k < len(got) && got[k].Seconds < it.at+0.002; k++ {
			best = max(best, rxPayloads(it, &got[k]))
		}
		sc.exact += best
		ok[i] = best == n
	}
	for _, ev := range in.events {
		found, bound, syncs := math.Inf(1), math.Inf(1), 0
		for i, it := range in.items {
			if it.at < ev {
				continue
			}
			if ok[i] && math.IsInf(found, 1) {
				found = it.at - ev
			}
			if it.sync {
				if syncs++; syncs == 3 {
					bound = it.at - ev
				}
			}
		}
		if !math.IsInf(bound, 1) {
			sc.found, sc.bound = append(sc.found, found), append(sc.bound, bound)
		}
	}
	return sc
}

func rxPayloads(it rxItem, b *Burst) int {
	data, _ := b.Decoded()
	if it.voice {
		want, _ := it.burst.AMBEFrames()
		got, _ := data.AMBEFrames()
		n := 0
		for k := range want {
			if got[k] == want[k] {
				n++
			}
		}
		return n
	}
	if data.SlotType() != it.burst.SlotType() {
		return 0
	}
	var ok bool
	switch it.burst.SlotType().DataType {
	case enums.DataTypeRate34:
		a, _ := data.Rate34Payload()
		w, _ := it.burst.Rate34Payload()
		ok = a == w
	case enums.DataTypeRate1:
		a, _ := data.Rate1Payload()
		w, _ := it.burst.Rate1Payload()
		ok = a == w
	default:
		a, _ := data.BPTCPayload()
		w, _ := it.burst.BPTCPayload()
		ok = a == w
	}
	if ok {
		return 1
	}
	return 0
}

func rxEligible(in rxInput) rxInput {
	seen := map[int]bool{}
	out := in
	out.items = nil
	jumped := false
	for _, it := range in.items {
		if in.jump != 0 && !jumped && it.at >= in.jump {
			jumped = true
			clear(seen)
		}
		if it.sync {
			seen[it.ch] = true
		}
		if seen[it.ch] {
			out.items = append(out.items, it)
		}
	}
	return out
}

func rxStreams(t testing.TB) map[string]rxTx {
	return map[string]rxTx{"bs": rxBS(t, 3), "ms": rxMS(t, 3), "msj": rxMSCalls(t, 4)}
}

const (
	rxFloor      = 0.9
	rxCleanFloor = 0.95
)

var rxKnown = map[string]string{
	"msj/40 dB/offset -3000 Hz/iq0": rxKnownAFC,
	"msj/12 dB/offset -3000 Hz/iq0": rxKnownAFC,
	"msj/12 dB/late E/iq3":          rxKnownLate,
	"ms/40 dB/late E/iq2":           rxKnownGain,
	"ms/40 dB/jump -2 ms/iq0h":      rxKnownJump,
}

const (
	rxKnownAFC = "the oscillator of Tier 0 at 3 kHz off takes about three moves of 50 ms to pull in, " +
		"and the two SYNCs of the first call of an MS come while it moves"
	rxKnownLate = "a receiver that starts in the call misses the first SYNC for some noise at 12 dB, " +
		"and loses the superframe after it"
	rxKnownGain = "the DiscReceiver of Tier 2 scales its input up for each burst of an MS and down for the noise " +
		"between them before it locks, and a scale in the SYNC of the late entry loses it, as on main"
	rxKnownJump = "the half rate front of Tier 0 locks this seed while its AFC is still 850 Hz off, and after the jump " +
		"the hunt fits the next SYNC at a poor timing phase and locks one superframe late; 40 other seeds lose at most 1 payload"
)

type rxResult struct {
	stream, cond, recv string
	cn                 float64
	ratio, base        float64
	found, bound       []float64
	pass, known        bool
}

func TestRxRobust(t *testing.T) {
	t.Parallel()
	streams := rxStreams(t)
	conds := rxConds()
	var (
		mu  sync.Mutex
		res []rxResult
	)
	type baseKey struct {
		stream string
		cn     float64
		e      rxRecv
		bytes  bool
	}
	var bmu sync.Mutex
	bases := map[baseKey]func() float64{}
	baseline := func(t testing.TB, k baseKey) float64 {
		bmu.Lock()
		f, ok := bases[k]
		if !ok {
			f = sync.OnceValue(func() float64 {
				c := conds[0]
				if k.bytes {
					c = conds[slices.IndexFunc(conds, func(c rxCond) bool { return c.name == "u8 0.25" })]
				}
				in := rxBuild(streams[k.stream], c, k.cn, k.e.mode == benchAudio, rxSeed(k.stream+c.name))
				sc := rxMatch(in, rxReceive(t, in, k.e))
				return float64(sc.exact) / float64(sc.total)
			})
			bases[k] = f
		}
		bmu.Unlock()
		return f()
	}
	if p := os.Getenv("DMRRF_ROBUST"); p != "" {
		t.Cleanup(func() {
			if err := os.WriteFile(p, []byte(rxTable(res)), 0o600); err != nil {
				t.Error(err)
			}
		})
	}
	for _, sn := range []string{"bs", "ms", "msj"} {
		for _, cn := range []float64{40, 12} {
			for _, c := range conds {
				for _, audio := range []bool{false, true} {
					if audio && c.iqOnly || !audio && c.audioOnly || c.highCN && cn < 40 {
						continue
					}
					name := fmt.Sprintf("%s/%g dB/%s", sn, cn, c.name)
					if audio {
						name += "/audio"
					}
					t.Run(name, func(t *testing.T) {
						t.Parallel()
						in := rxBuild(streams[sn], c, cn, audio, rxSeed(sn+c.name))
						for _, e := range rxRecvs() {
							if (e.mode == benchAudio) != audio {
								continue
							}
							got := rxReceive(t, in, e)
							sc := rxMatch(rxEligible(in), got)
							base := baseline(t, baseKey{sn, cn, e, c.bytes})
							r := rxResult{stream: sn, cond: c.name, recv: e.name(), cn: cn, base: base,
								ratio: float64(sc.exact) / float64(sc.total), found: sc.found, bound: sc.bound}
							r.pass = r.ratio >= rxFloor*base && (c.name != conds[0].name || r.ratio >= rxCleanFloor)
							for i := range sc.found {
								if sc.found[i] > sc.bound[i] {
									r.pass = false
								}
							}
							msg := fmt.Sprintf("%s: %d of %d payloads (%.3f, clean %.3f), found %v, bound %v", e.name(), sc.exact, sc.total, r.ratio, base, rxSecs(sc.found), rxSecs(sc.bound))
							switch why, known := rxKnown[name+"/"+e.name()]; {
							case known && r.pass:
								t.Logf("known failure passes, remove it from rxKnown: %s", msg)
							case known:
								r.known = true
								t.Logf("known failure, %s: %s", why, msg)
							case !r.pass:
								t.Error(msg)
							}
							mu.Lock()
							res = append(res, r)
							mu.Unlock()
						}
					})
				}
			}
		}
	}
}

func rxSecs(v []float64) string {
	s := make([]string, len(v))
	for i, x := range v {
		s[i] = fmt.Sprintf("%.3f", x)
	}
	return "[" + strings.Join(s, " ") + "]"
}

func rxTable(res []rxResult) string {
	es := rxRecvs()
	recvs := make([]string, 0, len(es))
	for _, e := range es {
		recvs = append(recvs, e.name())
	}
	type row struct {
		stream, cond string
		cn           float64
	}
	cells := map[row]map[string]rxResult{}
	conds := rxConds()
	rows := make([]row, 0, 6*len(conds))
	for _, c := range conds {
		for _, sn := range []string{"bs", "ms", "msj"} {
			for _, cn := range []float64{40, 12} {
				rows = append(rows, row{sn, c.name, cn})
			}
		}
	}
	for _, r := range res {
		k := row{r.stream, r.cond, r.cn}
		if cells[k] == nil {
			cells[k] = map[string]rxResult{}
		}
		cells[k][r.recv] = r
	}
	var b strings.Builder
	b.WriteString("| stream | C/N | condition | " + strings.Join(recvs, " | ") + " |\n|---|---|---|" + strings.Repeat("---|", len(recvs)) + "\n")
	for _, k := range rows {
		fmt.Fprintf(&b, "| %s | %g | %s |", k.stream, k.cn, k.cond)
		for _, n := range recvs {
			r, ok := cells[k][n]
			switch {
			case !ok:
				b.WriteString(" n/a |")
			case r.pass:
				fmt.Fprintf(&b, " %.3f |", r.ratio)
			case r.known:
				fmt.Fprintf(&b, " KNOWN %.3f/%.3f %s |", r.ratio, r.base, rxSecs(r.found))
			default:
				fmt.Fprintf(&b, " FAIL %.3f/%.3f %s |", r.ratio, r.base, rxSecs(r.found))
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
