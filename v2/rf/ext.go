package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
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
}

// newSoftExt returns a softExt.
func newSoftExt() *softExt {
	x := &softExt{soft: newSoftDecoder()}
	for dt := range x.checks {
		x.checks[dt] = softBPTCCheck(enums.DataType(dt))
	}
	return x
}

func (x *softExt) burst(c *leanCore, b *Burst) {
	for p := range x.llr {
		x.llr[p] = c.llrAt(p)
	}
	cc, ccOK := c.colorCode()
	b.Fixed, b.FixedCACH, b.HasFixed = b.Data, b.CACH, true
	out := &b.Fixed
	ch := &x.ch[b.Slot&1]
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	hold := false
	switch {
	case b.Voice != enums.VoiceBurstUnknown || known && kind == layer2.KindVoice:
		x.soft.decodeAMBE(&x.llr, out)
		if b.Voice != enums.VoiceBurstUnknown && b.Voice != enums.VoiceBurstA {
			x.soft.decodeEMB(&x.llr, cc, ccOK, out)
		}
		hold = x.fragment(ch, b)
	case known && kind == layer2.KindData:
		ch.have, ch.accN = 0, 0
		st, res := x.soft.decodeSlotType(&x.llr, cc, ccOK, out)
		if res.Uncorrectable {
			break
		}
		switch dt := st.DataType(); dt {
		case enums.DataTypeRate34:
			x.soft.decodeRate34(&x.llr, out)
		case enums.DataTypeRate1, enums.DataTypeReserved, enums.DataTypeIdle:
		default:
			check := x.checks[dt&0xF]
			saved := *out
			var r fec.Result
			if _, r = x.soft.decodeBPTCOSD(&x.llr, check, out); check != nil && r.Uncorrectable {
				saved = *out
				if _, r = x.soft.decodeBPTCGrid(&x.llr, check, out); r.Uncorrectable {
					*out = saved
				}
			}
		}
	default:
		ch.have = 0
	}
	if known && c.anchor == b.Sync {
		_ = out.SetSync(b.Sync)
	}
	if b.HasCACH {
		c.fixTACT(&c.fec, b)
	}
	x.push(c, b, hold)
	if b.Voice == enums.VoiceBurstE && ch.have == 0xf {
		x.embeddedLC(ch, b.Slot)
		ch.have = 0
	}
	x.drain(c, false)
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

func (x *softExt) end(c *leanCore) {
	for i := range x.held {
		x.held[i] = false
	}
	x.ch[0].have, x.ch[1].have = 0, 0
	x.ch[0].accN, x.ch[1].accN = 0, 0
	x.drain(c, false)
}
