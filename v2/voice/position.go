package voice

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// ETSI TS 102 361-1 clause 4.2.2
const slotPeriod = 60 * time.Millisecond

type placement uint8

const (
	placed placement = iota
	duplicate
	late
)

const lateWindow = 32

type tracker struct {
	known  bool
	header bool
	pos    int
	at     time.Duration
	hasAt  bool
	seq    uint16
	hasSeq bool
	raw    layer2.RawBurst
	hasRaw bool
}

func (t *tracker) afterHeader(h *layer2.Hint, now time.Duration) {
	if t.hasRaw {
		return
	}
	t.pos = layer2.VoiceBurstsPerSuperframe - 1
	if h.HasSeq {
		t.known, t.seq, t.hasSeq = true, h.Seq, true
		return
	}
	t.header, t.at, t.hasAt = true, now, true
}

func (t *tracker) place(b *layer2.Burst, raw *layer2.RawBurst, h *layer2.Hint, now time.Duration) (idx, lost int, p placement) {
	gap, p := t.seqGap(h)
	if p != placed {
		return -1, 0, p
	}
	steps, at := 1, now
	switch {
	case gap > 0:
		steps = gap
	case !h.HasSeq && t.hasAt:
		steps = int((now - t.at + slotPeriod/2) / slotPeriod)
		if steps < 1 {
			if t.hasRaw && *raw == t.raw {
				return -1, 0, duplicate
			}
			steps, at = 1, t.at+slotPeriod
		}
	}

	idx = t.position(b, h, steps, gap > 0)
	missedA := gap > 0 && t.known && idx == 1 && (t.pos+steps)%layer2.VoiceBurstsPerSuperframe == 0
	switch {
	case gap > 0 && missedA:
		lost = gap
	case gap > 0:
		lost = gap - 1
	case idx >= 0 && (t.known || t.header):
		lost = (idx - t.pos - 1 + 2*layer2.VoiceBurstsPerSuperframe) % layer2.VoiceBurstsPerSuperframe
		if !h.HasSeq {
			lost = nearestLost(lost, steps-1)
		}
	case !h.HasSeq && t.hasAt:
		lost = steps - 1
	}

	t.known, t.header, t.pos = idx >= 0, false, idx
	t.at, t.hasAt = at, true
	t.raw, t.hasRaw = *raw, true
	if h.HasSeq {
		t.seq, t.hasSeq = h.Seq, true
	}
	return idx, lost, placed
}

func (t *tracker) seqGap(h *layer2.Hint) (int, placement) {
	if !h.HasSeq || !t.hasSeq {
		return 0, placed
	}
	bits := uint(h.SeqBits)
	if bits == 0 || bits > 16 {
		bits = 16
	}
	size := 1 << bits
	d := int(h.Seq-t.seq) & (size - 1)
	switch {
	case d == 0:
		return 0, duplicate
	case d < size/2:
		return d, placed
	case size-d <= lateWindow:
		return 0, late
	}
	return 0, placed
}

func (t *tracker) position(b *layer2.Burst, h *layer2.Hint, steps int, bySeq bool) int {
	if pos, ok := strongPosition(b, h); ok {
		return pos
	}
	if !t.known {
		return -1
	}
	pred := (t.pos + steps) % layer2.VoiceBurstsPerSuperframe
	if pred == 0 && bySeq {
		pred = 1
	}
	cont := b.HasEmbeddedSignalling() && !b.EmbeddedSignalling.FEC.Uncorrectable &&
		b.EmbeddedSignalling.LCSS == enums.LCSSContinuationFragment
	if pred == 0 || cont && pred != 2 && pred != 3 {
		return -1
	}
	return pred
}

func strongPosition(b *layer2.Burst, h *layer2.Hint) (int, bool) {
	if i, ok := h.VoiceBurst.Index(); ok {
		return i, true
	}
	if b.VoiceBurst == enums.VoiceBurstA {
		return 0, true
	}
	if !b.HasEmbeddedSignalling() || b.EmbeddedSignalling.FEC.Uncorrectable {
		return 0, false
	}
	switch b.EmbeddedSignalling.LCSS {
	case enums.LCSSFirstFragment:
		return 1, true
	case enums.LCSSLastFragment:
		return 4, true
	case enums.LCSSSingleFragment:
		return 5, true
	case enums.LCSSContinuationFragment:
	}
	return 0, false
}

func nearestLost(gap, est int) int {
	if gap > est+2 {
		return est
	}
	n := gap
	for n+layer2.VoiceBurstsPerSuperframe/2 < est {
		n += layer2.VoiceBurstsPerSuperframe
	}
	return n
}

func (t *tracker) forget() {
	t.known = false
}
