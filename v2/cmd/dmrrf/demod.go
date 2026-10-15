package main

import (
	"math"
	"math/bits"
	"sort"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/framer"
)

// Received is one burst recovered from I/Q.
type Received struct {
	Data    layer2.RawBurst
	Seconds float64
	// Distance is the Hamming distance of the SYNC field from Sync, or -1
	// for a burst placed by timing without a SYNC (voice bursts B to F).
	Distance int
	Sync     enums.SyncPattern
	// Voice is the superframe position of a voice burst whose position
	// comes from the slot timing, or VoiceBurstUnknown when the SYNC field
	// alone classifies the burst.
	Voice enums.VoiceBurstType
	// CACH is the 24 bits before the burst in transmit order. BS is set
	// when the burst is BS-sourced, so that those bits are a CACH.
	CACH    [layer2.CACHBits]bit.Bit
	HasCACH bool
	BS      bool
}

// Timed reports whether the burst was placed by timing instead of a SYNC.
func (r Received) Timed() bool { return r.Distance < 0 }

// Demodulated is the result of Demodulate.
type Demodulated struct {
	Bursts      []Received
	OffsetHz    float64
	DeviationHz float64
}

func sampleOf(r Received) int { return int(math.Round(r.Seconds * basebandRate)) }

// Demodulate recovers DMR bursts from complex samples at rate. Bursts with a
// SYNC pattern are found by search, twice: the second search slices the
// symbols after removing the carrier offset that the first one measured
// over its SYNC fields. The slots between them are cut on the
// slot timing of the SYNC bursts, and recoverVoice finds the voice
// superframes among them.
func Demodulate(x []complex128, rate float64) Demodulated {
	bb := channelize(x, rate)
	present := presence(power(bb))
	coarse := carrierOffset(bb)
	bb = lowpass(shift(bb, coarse), 6000)
	d := discriminate(bb)
	sym := filter(d, rrcTaps())

	res := Demodulated{OffsetHz: coarse}
	var cut func(int) (Received, bool)
	for pass := range 2 {
		phaseBits := make([][]bit.Bit, samplesPerSy)
		for phase := range samplesPerSy {
			for i := phase; i < len(sym); i += samplesPerSy {
				b0, b1 := dibitFromSymbol(sym[i])
				phaseBits[phase] = append(phaseBits[phase], b0, b1)
			}
		}
		cut = func(s int) (Received, bool) {
			if s < 0 {
				return Received{}, false
			}
			bits := phaseBits[s%samplesPerSy]
			first := 2 * (s / samplesPerSy)
			if first+layer2.BurstBits > len(bits) {
				return Received{}, false
			}
			var b [264]bit.Bit
			copy(b[:], bits[first:first+layer2.BurstBits])
			r := Received{Data: bit.Pack264(b), Seconds: float64(s) / basebandRate}
			if first >= layer2.CACHBits {
				copy(r.CACH[:], bits[first-layer2.CACHBits:first])
				r.HasCACH = true
			}
			return r, true
		}
		res.Bursts = pickBursts(searchSync(phaseBits, cut, present), func(s int) float64 { return eye(sym, s) })
		var off float64
		off, res.DeviationHz = measure(res.Bursts, sym)
		res.OffsetHz += off
		if pass == 0 {
			for i := range sym {
				sym[i] -= off
			}
		}
	}
	near := func(s float64) (Received, bool) {
		at := int(math.Round(s))
		best := at
		for c := at - 1; c <= at+1; c++ {
			if eye(sym, c) > eye(sym, best) {
				best = c
			}
		}
		return cut(best)
	}
	res.Bursts = recoverVoice(fillSlots(res.Bursts, near, present), near, present)
	return res
}

// searchSync returns every burst with a SYNC pattern at any sampling phase.
func searchSync(phaseBits [][]bit.Bit, cut func(int) (Received, bool), present func(float64) bool) []Received {
	var all []Received
	for phase, bits := range phaseBits {
		for start := 0; ; {
			m, ok := framer.Search(bits, start, -1)
			if !ok {
				break
			}
			start = m.Offset + 1
			first := m.Offset - framer.SyncOffset
			if first < 0 || first%2 != 0 {
				continue
			}
			r, ok := cut(phase + first/2*samplesPerSy)
			if !ok || !present(r.Seconds) {
				continue
			}
			src, _, known := layer2.SyncSourceOf(m.Pattern)
			r.Distance, r.Sync, r.BS = m.Distance, m.Pattern, known && src == layer2.SyncSourceBS
			all = append(all, r)
		}
	}
	return all
}

// eye returns the mean distance of the symbols of the burst at sample s from
// the nearest decision threshold, which peaks at the best sampling time.
func eye(sym []float64, s int) float64 {
	var m float64
	for k := range burstSymbols {
		i := s + k*samplesPerSy
		if i < 0 || i >= len(sym) {
			return 0
		}
		v := math.Abs(sym[i])
		m += math.Min(v, math.Abs(v-2*deviationHz))
	}
	return m / burstSymbols
}

// presence returns a test for whether the carrier is on during the burst
// that starts at a time in seconds: its mean power is at least a quarter of
// the strongest burst-long stretch of pw.
func presence(pw []float64) func(float64) bool {
	sum, ref := burstPower(pw)
	n := int(burstSeconds * basebandRate)
	return func(t float64) bool {
		lo := int(t * basebandRate)
		hi := min(lo+n, len(pw))
		if lo < 0 || hi <= lo {
			return false
		}
		return (sum[hi]-sum[lo])/float64(hi-lo) > ref/4
	}
}

// pickBursts keeps one candidate per burst, in time order: of the sampling
// phases with the least SYNC distance, the one with the widest eye.
func pickBursts(all []Received, eye func(int) float64) []Received {
	sort.SliceStable(all, func(i, j int) bool { return all[i].Distance < all[j].Distance })
	var out []Received
	used := make([]bool, len(all))
	for i := range all {
		if used[i] {
			continue
		}
		pick := all[i]
		for j := i + 1; j < len(all); j++ {
			if !used[j] && math.Abs(all[j].Seconds-all[i].Seconds) < burstSeconds {
				used[j] = true
				if all[j].Distance == all[i].Distance && eye(sampleOf(all[j])) > eye(sampleOf(pick)) {
					pick = all[j]
				}
			}
		}
		out = append(out, pick)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seconds < out[j].Seconds })
	return out
}

// measure returns the residual carrier offset and the outer deviation over
// the SYNC fields of bursts, against the SYNC patterns they matched.
func measure(bursts []Received, sym []float64) (offset, deviation float64) {
	var resid, outer float64
	var nr, no int
	var pattern [framer.SyncBits]bit.Bit
	for _, r := range bursts {
		bit.PutUint(pattern[:], uint64(r.Sync))
		ideal := symbolsFromBits(pattern[:])
		first := sampleOf(r) + framer.SyncOffset/2*samplesPerSy
		for k := range ideal {
			idx := first + k*samplesPerSy
			if idx >= len(sym) {
				break
			}
			resid += sym[idx] - float64(ideal[k])*deviationHz
			nr++
			if ideal[k] == 3 || ideal[k] == -3 {
				outer += math.Abs(sym[idx])
				no++
			}
		}
	}
	if nr > 0 {
		offset = resid / float64(nr)
	}
	if no > 0 {
		deviation = outer / float64(no)
	}
	return offset, deviation
}

// Limits of the slot timing.
const (
	slotSamples = slotSeconds * basebandRate
	// matchSamples is how far a burst can be from a slot time and still
	// fill that slot.
	matchSamples = slotSamples / 4
	// damagedSyncDistance is the largest SYNC distance of a burst A or a
	// data burst that only the slot timing finds.
	damagedSyncDistance = 8
	// voiceSyncDistance is the largest SYNC distance of a burst A at a slot
	// where the timing of the call expects one.
	voiceSyncDistance = 16
	// maxGapSeconds is the longest time between two SYNC bursts whose
	// slot timing fills the slots between them.
	maxGapSeconds = 10
)

// slot is a burst of the slot timing: a SYNC burst, decoded, or a burst cut
// at a slot time between SYNC bursts (filled).
type slot struct {
	r      Received
	s      float64
	src    layer2.SyncSource
	filled bool
	burst  *layer2.Burst
}

// voiceDistance returns the voice SYNC pattern of the source of sl and the
// distance of its SYNC field from it, or framer.SyncBits when the field is
// nearer the data SYNC pattern.
func (sl *slot) voiceDistance() (enums.SyncPattern, int) {
	voice, data, ok := sl.src.SyncPatterns()
	if !ok {
		return enums.SyncNone, framer.SyncBits
	}
	v, d := syncDistance(sl.r.Data, voice), syncDistance(sl.r.Data, data)
	if v >= d {
		return voice, framer.SyncBits
	}
	return voice, v
}

func syncDistance(b layer2.RawBurst, p enums.SyncPattern) int {
	all := bit.Unpack264(b)
	return bits.OnesCount64(bit.Uint(all[framer.SyncOffset:framer.SyncOffset+framer.SyncBits]) ^ uint64(p))
}

// fillSlots returns the SYNC bursts of found together with the slots between
// them, in time order. A SYNC burst and the next one of the same channel, a
// whole number of slots later, give the time of every slot between them: 30
// ms apart in a BS outbound stream, which is continuous, and 60 ms apart
// otherwise, at most maxGapSeconds apart. The match allows one symbol plus
// 200 ppm of clock error. A slot is cut only where the carrier is on.
func fillSlots(found []Received, cut func(float64) (Received, bool), present func(float64) bool) []slot {
	var out []slot
	for i, a := range found {
		src, _, known := layer2.SyncSourceOf(a.Sync)
		out = append(out, slot{r: a, s: float64(sampleOf(a)), src: src})
		if !known {
			continue
		}
		for _, b := range found[i+1:] {
			period := float64(frameSamples)
			if a.BS && b.BS {
				period = slotSamples
			}
			dt := float64(sampleOf(b) - sampleOf(a))
			if dt > maxGapSeconds*basebandRate {
				break
			}
			k := math.Round(dt / period)
			if k < 1 || math.Abs(dt-k*period) > samplesPerSy+dt*200e-6 {
				continue
			}
			for j := 1; j < int(k); j++ {
				r, ok := cut(float64(sampleOf(a)) + float64(j)*dt/k)
				if !ok || !present(r.Seconds) {
					continue
				}
				r.Distance, r.Sync, r.BS = -1, enums.SyncNone, a.BS
				out = append(out, slot{r: r, s: float64(sampleOf(r)), src: src, filled: true})
			}
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].s < out[j].s })
	for i := range out {
		if !out[i].filled {
			out[i].burst, _ = layer2.NewBurstFromBytes(out[i].r.Data)
		}
	}
	return out
}

// recoverVoice finds the voice superframes in slots and returns the bursts
// to report, in time order. A superframe starts at a burst A with a voice
// SYNC, or at a filled slot that is within damagedSyncDistance of the voice
// SYNC. It also starts at a filled slot within voiceSyncDistance of the
// voice SYNC 60 ms after a voice LC or PI header, 60 ms or 240 ms before a
// burst B or E with a clean EMB, or every 360 ms between two superframes of
// a call. Its bursts B to F are the slots at 60 ms steps, or bursts cut by
// timing where there is no slot. Superframes that start at a SYNC go first, and a slot
// belongs to one superframe only. A filled slot outside a superframe is kept
// only as a data burst within damagedSyncDistance of a data SYNC.
func recoverVoice(slots []slot, cut func(float64) (Received, bool), present func(float64) bool) []Received {
	tl := &timeline{slots: slots, cut: cut, present: present, keep: make([]bool, len(slots))}
	for _, s := range tl.starts() {
		tl.superframe(s)
	}
	return tl.bursts()
}

const (
	frame      = float64(frameSamples)
	superframe = layer2.VoiceBurstsPerSuperframe * frame
)

// timeline holds the slots of a capture in time order while recoverVoice
// assigns them to superframes.
type timeline struct {
	slots   []slot
	cut     func(float64) (Received, bool)
	present func(float64) bool
	// keep marks the slots that belong to a superframe.
	keep []bool
	// extra holds the bursts cut by timing where there is no slot.
	extra []Received
}

// at returns the index of the slot within matchSamples of sample s, or -1.
func (tl *timeline) at(s float64) int {
	i := sort.Search(len(tl.slots), func(i int) bool { return tl.slots[i].s >= s-matchSamples })
	if i < len(tl.slots) && math.Abs(tl.slots[i].s-s) <= matchSamples {
		return i
	}
	return -1
}

// starts returns the samples at which superframes can start: those at a
// voice SYNC, then all of them in time order.
func (tl *timeline) starts() []float64 {
	cc, haveCC := colorCode(tl.slots)
	var synced, all []float64
	for i := range tl.slots {
		sl := &tl.slots[i]
		if _, k, _ := layer2.SyncSourceOf(sl.r.Sync); !sl.filled && k == layer2.KindVoice {
			synced = append(synced, sl.s)
			all = append(all, sl.s)
		} else if s, ok := tl.inferredStart(sl, cc, haveCC); ok {
			all = append(all, s)
		}
	}
	sort.Float64s(all)
	all = append(all, tl.between(all)...)
	sort.Float64s(all)
	return append(synced, all...)
}

// inferredStart returns the start of the superframe that sl shows without a
// voice SYNC of its own: the slot after a voice LC or PI header, a filled
// slot near the voice SYNC, or the burst A before a burst B or E whose EMB
// decodes with at most one correction and color code cc.
func (tl *timeline) inferredStart(sl *slot, cc enums.ColorCode, haveCC bool) (float64, bool) {
	if !sl.filled {
		b := sl.burst
		if b.Kind != layer2.KindData || b.SlotType.FEC.Uncorrectable ||
			b.SlotType.DataType != enums.DataTypeVoiceLCHeader && b.SlotType.DataType != enums.DataTypePIHeader {
			return 0, false
		}
		j := tl.at(sl.s + frame)
		return tl.slots[max(j, 0)].s, j >= 0 && tl.slots[j].filled
	}
	if _, d := sl.voiceDistance(); d <= damagedSyncDistance {
		return sl.s, true
	}
	var b layer2.Burst
	_ = b.DecodeFromBytesWith(sl.r.Data, &layer2.DecodeOptions{Kind: layer2.KindVoice, VoiceBurst: enums.VoiceBurstB})
	emb := b.EmbeddedSignalling
	if !haveCC || emb.FEC.Uncorrectable || emb.FEC.ErrorsCorrected > 1 || emb.ColorCode != cc {
		return 0, false
	}
	switch emb.LCSS {
	case enums.LCSSFirstFragment:
		return sl.s - frame, true
	case enums.LCSSLastFragment:
		return sl.s - 4*frame, true
	case enums.LCSSSingleFragment, enums.LCSSContinuationFragment:
	}
	return 0, false
}

// between returns the filled slots near the voice SYNC that are a whole
// number of superframes after one start and before the next start that is
// a whole number of superframes later.
func (tl *timeline) between(starts []float64) []float64 {
	var out []float64
	for i, a := range starts {
		for _, b := range starts[i+1:] {
			k := math.Round((b - a) / superframe)
			if k < 1 || math.Abs(b-a-k*superframe) > matchSamples {
				continue
			}
			for n := 1; n < int(k); n++ {
				x := tl.at(a + float64(n)*(b-a)/k)
				if x < 0 || !tl.slots[x].filled {
					continue
				}
				if _, d := tl.slots[x].voiceDistance(); d <= voiceSyncDistance {
					out = append(out, tl.slots[x].s)
				}
			}
			break
		}
	}
	return out
}

// superframe assigns the bursts of the superframe that starts at sample
// start, up to the first slot that cannot be part of it.
func (tl *timeline) superframe(start float64) {
	a := tl.at(start)
	if a < 0 || tl.keep[a] {
		return
	}
	for n := range layer2.VoiceBurstsPerSuperframe {
		pos := enums.VoiceBurstA + enums.VoiceBurstType(n)
		p := start + float64(n)*frame
		x := tl.at(p)
		if x < 0 {
			r, ok := tl.cut(p)
			if !ok || !tl.present(r.Seconds) {
				return
			}
			c := slot{r: r, src: tl.slots[a].src}
			if p, d := c.dataDistance(); d <= damagedSyncDistance {
				r.Distance, r.Sync, r.BS = d, p, tl.slots[a].r.BS
				tl.extra = append(tl.extra, r)
				return
			}
			r.Distance, r.Sync, r.BS, r.Voice = -1, enums.SyncNone, tl.slots[a].r.BS, pos
			tl.extra = append(tl.extra, r)
			continue
		}
		if tl.keep[x] || !tl.slots[x].takes(pos) {
			return
		}
		tl.keep[x] = true
	}
}

// takes reports whether sl can be the burst at pos of a superframe, and
// sets the position of a filled slot and the SYNC of a filled burst A.
func (sl *slot) takes(pos enums.VoiceBurstType) bool {
	if !sl.filled {
		_, k, _ := layer2.SyncSourceOf(sl.r.Sync)
		return pos == enums.VoiceBurstA && k == layer2.KindVoice
	}
	if pos == enums.VoiceBurstA {
		p, d := sl.voiceDistance()
		if d > voiceSyncDistance {
			return false
		}
		sl.r.Distance, sl.r.Sync = d, p
	} else if _, d := sl.dataDistance(); d <= damagedSyncDistance {
		return false
	}
	sl.r.Voice = pos
	return true
}

// bursts returns the SYNC bursts, the slots of superframes, and the filled
// slots near a data SYNC, in time order.
func (tl *timeline) bursts() []Received {
	out := make([]Received, 0, len(tl.slots)+len(tl.extra))
	for i := range tl.slots {
		sl := &tl.slots[i]
		if !sl.filled || tl.keep[i] {
			out = append(out, sl.r)
		} else if p, d := sl.dataDistance(); d <= damagedSyncDistance {
			sl.r.Distance, sl.r.Sync = d, p
			out = append(out, sl.r)
		}
	}
	out = append(out, tl.extra...)
	sort.Slice(out, func(i, j int) bool { return out[i].Seconds < out[j].Seconds })
	return out
}

// dataDistance returns the data SYNC pattern of the source of sl and the
// distance of its SYNC field from it.
func (sl *slot) dataDistance() (enums.SyncPattern, int) {
	_, data, ok := sl.src.SyncPatterns()
	if !ok {
		return enums.SyncNone, framer.SyncBits
	}
	return data, syncDistance(sl.r.Data, data)
}

// colorCode returns the color code that most SYNC data bursts of slots carry
// in a slot type without errors. ok is false when there is none.
func colorCode(slots []slot) (cc enums.ColorCode, ok bool) {
	var votes [16]int
	for _, sl := range slots {
		b := sl.burst
		if !sl.filled && b.Kind == layer2.KindData && b.SlotType.FEC.ErrorsCorrected == 0 && !b.SlotType.FEC.Uncorrectable {
			votes[b.SlotType.ColorCode&0xF]++
		}
	}
	best := 0
	for i, v := range votes {
		if v > votes[best] {
			best = i
		}
	}
	return enums.ColorCode(best), votes[best] > 0
}
