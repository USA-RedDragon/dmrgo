package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// leanExt is a stage that a tier above Tier 0 adds after the detection of
// the core: it takes each burst that the core detects and places, reads
// the log-likelihood ratios of the core for it, and emits it through
// leanCore.emit. Tier 0 never sets one, so that TinyGo links none of its
// code into a DiscReceiver.
type leanExt interface {
	// burst decodes and emits burst b, which the core placed.
	burst(c *leanCore, b *Burst)
	// end emits the bursts that the stage holds, at the end of a stream.
	end(c *leanCore)
	// kept returns the ring of kept samples that the stage keeps in place
	// of the scratch of the core, or nil.
	kept() []int16
	// locked runs when the core locks to a stream, at a SYNC that it
	// sampled again d samples from its timing.
	locked(c *leanCore, d float32)
	// firstSlot runs as the first slot of a stream ends, before the core
	// handles it.
	firstSlot(c *leanCore)
}

// extQueue is the most bursts that a softExt holds: the bursts B to D of a
// superframe and the bursts of the other channel between them.
const extQueue = 8

// extChan is the embedded LC state of one channel of a softExt.
type extChan struct {
	// frag holds the ratios of the embedded signalling fragments of bursts
	// B to E of the superframe whose burst B is in slot b, and have marks
	// the fragments it holds.
	frag [4][embBits]int16
	have uint8
	b    int64
	// acc sums the ratios of the embedded LC of the earlier superframes of
	// the call that decoded, accN of them.
	acc  [4][embBits]int16
	accN int
}

// softExt decodes every FEC field of a burst with the soft decoders of the
// coherent tiers, from the ratios of the core: C0 and C1 of the voice
// frames by ML, BPTC(196,96) by ordered statistics and its grid, and the
// embedded LC of each superframe by ordered statistics over its four
// fragments, summed with the earlier superframes of the call. It holds the
// bursts B to D of a superframe until burst E, to write the corrected
// fragments into them.
type softExt struct {
	soft   *softDecoder
	checks [16]func([12]byte) bool
	llr    [layer2.BurstBits]int16
	ch     [2]extChan
	q      [extQueue]Burst
	// held marks the bursts of q that wait for their embedded LC.
	held [extQueue]bool
	head int
	n    int
	sum  [4][embBits]int16
	out  [4]layer2.RawBurst
	// acq is the ring of matched filter samples that the core keeps while
	// it searches, two to an element, long enough for lookback slots
	// before the burst of the SYNC that it locks to, and ring holds the
	// symbols of the core while those slots run.
	acq      []int16
	lookback int
	ring     [leanRing]int8
	// lockSym is the last symbol at the lock, the end of its SYNC, and
	// lockAt its time from the last kept sample, in samples.
	lockSym int64
	lockAt  float32
	// stages selects the decodes that replace those of Tier 0.
	stages uint8
}

// The stages of a softExt.
const (
	// extAMBE decodes C0 and C1 of each voice frame by soft ML.
	extAMBE = 1 << iota
	// extOSD decodes a BPTC(196,96) block whose check fails by ordered
	// statistics, and again over its grid.
	extOSD
	// extELC decodes the embedded LC of each superframe over its four
	// fragments, summed with the earlier superframes of the call.
	extELC
	// extAll is every stage.
	extAll = extAMBE | extOSD | extELC
)

// newSoftExt returns a softExt with every stage that decodes lookback
// slots before the SYNC of each lock, at sps matched filter samples per
// symbol.
func newSoftExt(lookback int, sps float32) *softExt {
	return newSoftExtStages(extAll, lookback, sps)
}

// softExtOf returns the softExt of the settings opt at sps matched filter
// samples per symbol.
func softExtOf(opt *options, sps float32) *softExt {
	var st uint8
	if opt.ExtAMBE {
		st |= extAMBE
	}
	if opt.ExtOSD {
		st |= extOSD
	}
	if opt.ExtELC {
		st |= extELC
	}
	return newSoftExtStages(st, opt.ExtLookback, sps)
}

// newSoftExtStages returns a softExt with the stages st that decodes
// lookback slots before the SYNC of each lock, at sps matched filter
// samples per symbol.
func newSoftExtStages(st uint8, lookback int, sps float32) *softExt {
	x := &softExt{soft: newSoftDecoder(), lookback: lookback, stages: st}
	if lookback > 0 {
		n := int(float32((lookback+1)*slotSymbols)*sps) + leanAcqSamples
		x.acq = make([]int16, (n+1)/2)
	}
	for dt := range x.checks {
		x.checks[dt] = softBPTCCheck(enums.DataType(dt))
	}
	return x
}

func (x *softExt) burst(c *leanCore, b *Burst) {
	// The decode of Tier 0 first; the stages then replace the fields that
	// they decode better.
	cc, ccOK := c.colorCode()
	c.repair(&c.fec, b, cc, ccOK)
	out := &b.Fixed
	ch := &x.ch[b.Slot&1]
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	hold := false
	switch {
	case b.Voice != enums.VoiceBurstUnknown || known && kind == layer2.KindVoice:
		if x.stages&(extAMBE|extELC) != 0 {
			x.fill(c)
		}
		if x.stages&extAMBE != 0 {
			x.soft.decodeAMBE(&x.llr, out)
		}
		if x.stages&extELC != 0 {
			hold = x.fragment(ch, b)
		}
	case known && kind == layer2.KindData:
		ch.have, ch.accN = 0, 0
		if x.stages&extOSD == 0 {
			break
		}
		st := out.SlotType()
		if st.FEC.Uncorrectable {
			break
		}
		dt := st.DataType
		check := x.checks[dt&0xF]
		if check == nil {
			break
		}
		if p, _ := out.BPTCPayload(); check(p) {
			break
		}
		x.fill(c)
		saved := *out
		if _, r := x.soft.decodeBPTCOSD(&x.llr, check, out); r.Uncorrectable {
			if _, r = x.soft.decodeBPTCGrid(&x.llr, check, out); r.Uncorrectable {
				*out = saved
			}
		}
	default:
		ch.have = 0
	}
	x.push(c, b, hold)
	if b.Voice == enums.VoiceBurstE && ch.have == 0xf {
		x.embeddedLC(ch, b.Slot)
		ch.have = 0
	}
	x.drain(c, false)
}

// fill sets llr to the log-likelihood ratios of the burst under detection.
func (x *softExt) fill(c *leanCore) {
	for p := range x.llr {
		x.llr[p] = c.llrAt(p)
	}
}

// fragment keeps the embedded signalling fragment of voice burst b of
// channel ch, and reports whether b waits for burst E of its superframe.
func (x *softExt) fragment(ch *extChan, b *Burst) bool {
	if b.Voice < enums.VoiceBurstB || b.Voice > enums.VoiceBurstE {
		if b.Voice != enums.VoiceBurstF {
			ch.have = 0
		}
		return false
	}
	i := int(b.Voice - enums.VoiceBurstB)
	if i == 0 {
		ch.have, ch.b = 0, b.Slot
	}
	if b.Slot != ch.b+2*int64(i) || ch.have != 1<<i-1 {
		ch.have = 0
		x.release(ch)
		return false
	}
	copy(ch.frag[i][:], x.llr[embFirstBit:embFirstBit+embBits])
	ch.have |= 1 << i
	return i < 3
}

// embeddedLC decodes the embedded LC of the superframe of ch whose burst E
// is in slot e, and writes the corrected fragments into its bursts.
func (x *softExt) embeddedLC(ch *extChan, e int64) {
	var ps [4]*Burst
	for k := range x.n {
		b := &x.q[(x.head+k)%extQueue]
		if i := (b.Slot - ch.b) / 2; b.Slot&1 == e&1 && b.Slot >= ch.b && b.Slot <= e && (b.Slot-ch.b)%2 == 0 {
			ps[i] = b
		}
	}
	for i, b := range ps {
		if b == nil {
			x.release(ch)
			return
		}
		x.out[i] = b.Fixed
	}
	ok := false
	if ch.accN > 0 {
		for i := range x.sum {
			x.sum[i] = ch.acc[i]
			softCombine(x.sum[i][:], ch.frag[i][:])
		}
		if _, res := x.soft.decodeEmbeddedFrags(&x.sum, &x.out); !res.Uncorrectable {
			ok = true
			ch.acc, ch.accN = x.sum, min(ch.accN+1, maxCombine)
		}
	}
	if !ok {
		_, res := x.soft.decodeEmbeddedFrags(&ch.frag, &x.out)
		ok = !res.Uncorrectable
		ch.acc, ch.accN = ch.frag, 1
	}
	for i, b := range ps {
		if ok {
			b.Fixed = x.out[i]
		}
	}
	x.release(ch)
}

// release lets the held bursts of the channel of ch go.
func (x *softExt) release(ch *extChan) {
	j := int64(0)
	if ch == &x.ch[1] {
		j = 1
	}
	for k := range x.n {
		i := (x.head + k) % extQueue
		if x.q[i].Slot&1 == j {
			x.held[i] = false
		}
	}
}

// push adds burst b to the queue, held when hold, emitting the oldest
// burst when the queue is full.
func (x *softExt) push(c *leanCore, b *Burst, hold bool) {
	if x.n == extQueue {
		x.held[x.head] = false
		x.drain(c, true)
	}
	i := (x.head + x.n) % extQueue
	x.q[i], x.held[i] = *b, hold
	x.n++
}

// drain emits the bursts at the head of the queue that wait for nothing,
// or only the head when one is true.
func (x *softExt) drain(c *leanCore, one bool) {
	for x.n > 0 && !x.held[x.head] {
		c.emit(&x.q[x.head])
		x.head = (x.head + 1) % extQueue
		x.n--
		if one {
			return
		}
	}
}

func (x *softExt) kept() []int16 { return x.acq }

// locked notes the time of the SYNC that the core locked to, d samples
// from the timing loop, in the kept samples, which the core stops keeping.
func (x *softExt) locked(c *leanCore, d float32) {
	x.lockSym, x.lockAt = c.ns-1, c.lastAt+d
}

// firstSlot decodes the lookback slots before the burst of the SYNC that
// the core locked to, as that burst ends: it samples their symbols from
// the kept samples at the timing of the SYNC and runs each through the
// slot handling of the core, as if the core had been locked there. A
// first run only votes for the color code, with the burst of the SYNC
// too, so that the voice bursts without a SYNC place by their EMB; the
// second decodes. The slots of the stream count from the first of them.
func (x *softExt) firstSlot(c *leanCore) {
	if x.lookback == 0 {
		return
	}
	sps := 2 * c.tm.step
	last := x.lockSym
	// The oldest symbol that a slot reads, one before it for the SYNC
	// search, must lie in the kept samples with the four of the
	// interpolation.
	room := float32(c.acqKept()-4) - x.lockAt
	n := 0
	for n < x.lookback {
		oldest := c.first - int64(n+1)*slotSymbols - 1
		if oldest < 0 || float32(last-oldest)*sps > room {
			break
		}
		n++
	}
	if n == 0 {
		return
	}
	x.ring = c.ring
	first, eAcc, eSyms, hold, slotEnd := c.first, c.eAcc, c.eSyms, c.tm.hold, c.slotEnd
	levels := [3]float32{c.dcHz, c.devScale, c.discVar}
	c.replaying = true
	for _, dry := range [2]bool{true, false} {
		votes := c.votes
		c.resetChans()
		c.votes = votes
		c.dcHz, c.devScale, c.discVar = levels[0], levels[1], levels[2]
		c.next, c.lastAnch, c.lost = 0, leanNoSlot, 0
		c.first = first - int64(n)*slotSymbols
		c.slotEnd = c.first + slotSymbols
		c.replayDry = dry
		// The first run also votes with the slot of the SYNC, whose
		// symbols the ring holds.
		m := n
		if dry {
			m++
		}
		for j := range m {
			if j == n {
				c.ring = x.ring
			} else {
				base := c.first + int64(j)*slotSymbols
				for k := base - 1; k <= base+slotSymbols; k++ {
					v := c.keptAt(x.lockAt - float32(last-k)*sps)
					c.ring[k%leanRing] = int8(max(min(roundf(v), math.MaxInt8), math.MinInt8))
				}
			}
			c.eAcc, c.eSyms = 0, 0
			c.slot()
		}
	}
	c.replaying, c.replayDry = false, false
	c.ring = x.ring
	c.first = first - int64(n)*slotSymbols
	c.slotEnd = slotEnd
	c.eAcc, c.eSyms, c.tm.hold = eAcc, eSyms, hold
}

func (x *softExt) end(c *leanCore) {
	for i := range x.held {
		x.held[i] = false
	}
	x.ch[0].have, x.ch[1].have = 0, 0
	x.ch[0].accN, x.ch[1].accN = 0, 0
	x.drain(c, false)
}
