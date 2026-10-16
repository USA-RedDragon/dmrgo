package voice

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// slotPeriod is the time between two bursts of one logical channel (ETSI
// TS 102 361-1 clause 4.2.2).
const slotPeriod = 60 * time.Millisecond

// placement is what tracker.place found for a voice burst.
type placement uint8

const (
	placed placement = iota
	duplicate
	late
)

// lateWindow is the most sequence numbers by which a burst may be behind
// the last one and count as late. A burst further behind starts the
// sequence again, as after a transport restart.
const lateWindow = 32

// tracker keeps the superframe position of the last voice burst of a call.
type tracker struct {
	known  bool
	pos    int
	at     time.Duration
	hasAt  bool
	seq    uint16
	hasSeq bool
	raw    layer2.RawBurst
	hasRaw bool
}

// place returns the superframe index of the voice burst b, whose bits are
// raw, 0 for burst A to 5 for burst F, or -1 when no evidence places it,
// and the number of voice bursts lost before it. The evidence, strongest
// first, is the transport position, the voice SYNC, the EMB LCSS of bursts
// B, E and F, and the position predicted from the sequence number or,
// without one, from the time since the last burst. A prediction never
// places a burst at A, which needs its SYNC. Without a sequence number, a
// burst that comes less than half a slot period after the last one is a
// duplicate when it has the same bits, and else the next burst, which
// arrived early.
func (t *tracker) place(b *layer2.Burst, raw *layer2.RawBurst, h *layer2.Hint, now time.Duration) (idx, lost int, p placement) {
	gap := 0
	if h.HasSeq && t.hasSeq {
		bits := uint(h.SeqBits)
		if bits == 0 || bits > 16 {
			bits = 16
		}
		size := 1 << bits
		d := int(h.Seq-t.seq) & (size - 1)
		switch {
		case d == 0:
			return -1, 0, duplicate
		case d < size/2:
			gap = d
		case size-d <= lateWindow:
			return -1, 0, late
		}
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

	idx = -1
	if pos, ok := strongPosition(b, h); ok {
		idx = pos
	} else if t.known {
		pred := (t.pos + steps) % layer2.VoiceBurstsPerSuperframe
		cont := b.HasEmbeddedSignalling() && !b.EmbeddedSignalling.FEC.Uncorrectable &&
			b.EmbeddedSignalling.LCSS == enums.LCSSContinuationFragment
		if pred != 0 && (!cont || pred == 2 || pred == 3) {
			idx = pred
		}
	}

	switch {
	case gap > 0:
		lost = gap - 1
	case idx >= 0 && t.known:
		lost = (idx - t.pos - 1 + 2*layer2.VoiceBurstsPerSuperframe) % layer2.VoiceBurstsPerSuperframe
		if !h.HasSeq {
			lost = nearestLost(lost, steps-1)
		}
	case !h.HasSeq && t.hasAt:
		lost = steps - 1
	}

	t.known, t.pos = idx >= 0, idx
	t.at, t.hasAt = at, true
	t.raw, t.hasRaw = *raw, true
	if h.HasSeq {
		t.seq, t.hasSeq = h.Seq, true
	}
	return idx, lost, placed
}

// strongPosition returns the superframe index of b from the transport, the
// voice SYNC or an EMB whose LCSS fits one position only.
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

// nearestLost returns the count of lost bursts that the superframe
// positions give, gap plus a whole number of superframes, that is nearest
// to the count est that the time gives. When the positions give more than
// the time allows, the transmitter started a new superframe early, and the
// time wins.
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

// forget drops the position, so that the next burst is placed only by its
// own evidence, but keeps the sequence number for duplicate checks.
func (t *tracker) forget() {
	t.known = false
}
