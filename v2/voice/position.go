package voice

import (
	"time"

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

type tracker struct {
	pos    layer2.PositionTracker
	at     time.Duration
	hasAt  bool
	raw    layer2.RawBurst
	hasRaw bool
}

func (t *tracker) afterHeader(h *layer2.Hint, now time.Duration) {
	if t.hasRaw {
		return
	}
	t.pos.Header(h)
	if !h.HasSeq {
		t.at, t.hasAt = now, true
	}
}

func (t *tracker) place(b *layer2.Burst, raw *layer2.RawBurst, h *layer2.Hint, now time.Duration) (idx, lost int, p placement) {
	steps, at := 1, now
	if !h.HasSeq && t.hasAt {
		steps = int((now - t.at + slotPeriod/2) / slotPeriod)
		if steps < 1 {
			if t.hasRaw && *raw == t.raw {
				return -1, 0, duplicate
			}
			steps, at = 1, t.at+slotPeriod
		}
	}
	lost, pl := t.pos.PlaceDecoded(b, h, steps)
	switch pl {
	case layer2.PlacementDuplicate:
		return -1, 0, duplicate
	case layer2.PlacementLate:
		return -1, 0, late
	case layer2.PlacementInOrder:
	}
	idx = -1
	if i, ok := h.VoiceBurst.Index(); ok {
		idx = i
	}
	t.at, t.hasAt = at, true
	t.raw, t.hasRaw = *raw, true
	return idx, lost, placed
}

func (t *tracker) forget() {
	t.pos.Forget()
}
