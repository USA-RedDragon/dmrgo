package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const lateWindow = 32

// Placement is how a PositionTracker took a burst.
type Placement uint8

// The Placement values.
const (
	// PlacementInOrder is a burst that follows the one before it, or a
	// gap after it.
	PlacementInOrder Placement = iota
	// PlacementDuplicate is a burst with the sequence number of the burst
	// before it.
	PlacementDuplicate
	// PlacementLate is a burst whose sequence number is at most 32 behind
	// the last one.
	PlacementLate
)

// String returns the name of the placement. It allocates only for a value
// without a name.
func (p Placement) String() string {
	switch p {
	case PlacementInOrder:
		return "InOrder"
	case PlacementDuplicate:
		return "Duplicate"
	case PlacementLate:
		return "Late"
	}
	return "Placement(" + text.Int(int64(p)) + ")"
}

// PositionTracker places the voice bursts of one logical channel in their
// superframe, packet by packet, as voice.Receiver does: from the position
// in the Hint, the voice SYNC of burst A, the LCSS of the EMB of bursts B
// to F, and the sequence number of the Hint, or one step per burst without
// one. A bridge that forwards each packet as it comes, such as IPSC to
// HomeBrew DMRD, calls Place for each packet of a slot, so that the voice
// bursts that the source does not mark get a position. The zero
// PositionTracker is ready to use. It is not safe for concurrent use.
// (ETSI TS 102 361-1 clause 5.1.2.1, clause 9.3.5)
type PositionTracker struct {
	stream    uint32
	seq       uint16
	pos       int8
	known     bool
	header    bool
	started   bool
	hasSeq    bool
	hasStream bool
}

// Reset forgets the call.
func (t *PositionTracker) Reset() {
	if t != nil {
		*t = PositionTracker{}
	}
}

// Forget forgets the superframe position and keeps the sequence number, so
// that the next burst is placed only by its SYNC or EMB, as after a weak
// burst.
func (t *PositionTracker) Forget() {
	if t != nil {
		t.known = false
	}
}

// Place takes the burst raw of one packet with its Hint h, as a transport
// gives them, and sets h.VoiceBurst of a voice burst to its superframe
// position, or to VoiceBurstUnknown when it cannot place it. A new stream
// ID starts a new call. A voice LC header or PI header puts the next burst
// at A, a terminator or an EndStrong hint ends the call, and a SyncLost or
// EndWeak hint forgets the position. h.Kind KindUnknown takes a burst with
// a data SYNC as data and any other as voice. Place returns the number of
// bursts lost before raw and how it took raw. A duplicate or late burst
// gets the position that its sequence number gives, and does not move the
// tracker. A nil raw, for a packet without a burst, applies only the
// stream, SyncLost and End of h. A nil t or h is a no-op.
func (t *PositionTracker) Place(raw *RawBurst, h *Hint) (lost int, p Placement) {
	if t == nil || h == nil {
		return 0, PlacementInOrder
	}
	if h.HasStream {
		if t.hasStream && h.Stream != t.stream {
			*t = PositionTracker{}
		}
		t.stream, t.hasStream = h.Stream, true
	}
	if h.SyncLost {
		t.known = false
	}
	if raw == nil {
		if h.End == EndStrong {
			t.end()
		}
		return 0, PlacementInOrder
	}
	kind, dt, hasDT := h.Kind, h.DataType, h.HasDataType
	if kind == KindUnknown {
		kind = KindVoice
		if s, d := raw.Sync(); d <= DefaultSyncThreshold && isDataSync(s) {
			kind, dt, hasDT = KindData, raw.SlotType().DataType, true
		}
	}
	switch kind {
	case KindVoice:
		pos, ok, cont := rawPosition(raw, h)
		lost, p = t.place(pos, ok, cont, h, 1)
		if p == PlacementInOrder && h.End == EndWeak {
			t.known = false
		}
	case KindData:
		switch {
		case hasDT && (dt == enums.DataTypeVoiceLCHeader || dt == enums.DataTypePIHeader):
			t.Header(h)
		case hasDT && dt == enums.DataTypeTerminatorWithLC:
			t.end()
		}
	case KindReverseChannel:
	}
	if h.End == EndStrong {
		t.end()
	}
	return lost, p
}

func (t *PositionTracker) end() {
	stream, has := t.stream, t.hasStream
	*t = PositionTracker{stream: stream, hasStream: has}
}

// Header tells the tracker that a voice LC header or PI header with Hint h
// came before the first voice burst of the call, so that the next burst is
// burst A. It does nothing after the first voice burst, or for a nil t or
// h.
func (t *PositionTracker) Header(h *Hint) {
	if t == nil || h == nil || t.started {
		return
	}
	t.pos = VoiceBurstsPerSuperframe - 1
	if h.HasSeq {
		t.known, t.seq, t.hasSeq = true, h.Seq, true
		return
	}
	t.header = true
}

// PlaceDecoded places the decoded voice burst b with Hint h by the same
// rules as Place, for a receiver that has decoded the burst already. It
// takes b as steps bursts after the one before it when h has no sequence
// number, as a receiver that counts 60 ms slots does; Place takes 1. It
// sets h.VoiceBurst, and returns the bursts lost before b and how it took
// b. A nil t, b or h is a no-op, and steps below 1 count as 1.
func (t *PositionTracker) PlaceDecoded(b *Burst, h *Hint, steps int) (lost int, p Placement) {
	if t == nil || b == nil || h == nil {
		return 0, PlacementInOrder
	}
	pos, ok, cont := decodedPosition(b, h)
	return t.place(pos, ok, cont, h, steps)
}

func (t *PositionTracker) place(strong int, ok, cont bool, h *Hint, steps int) (lost int, p Placement) {
	gap, p := t.seqGap(h)
	if p != PlacementInOrder {
		h.VoiceBurst = t.behind(strong, ok, cont, gap)
		return 0, p
	}
	steps = max(steps, 1)
	if gap > 0 {
		steps = gap
	}
	idx := t.position(strong, ok, cont, steps, gap > 0)
	missedA := gap > 0 && t.known && idx == 1 && (int(t.pos)+steps)%VoiceBurstsPerSuperframe == 0
	switch {
	case gap > 0 && missedA:
		lost = gap
	case gap > 0:
		lost = gap - 1
	case idx >= 0 && (t.known || t.header):
		lost = (idx - int(t.pos) - 1 + 2*VoiceBurstsPerSuperframe) % VoiceBurstsPerSuperframe
		if !h.HasSeq {
			lost = nearestLost(lost, steps-1)
		}
	case !h.HasSeq:
		lost = steps - 1
	}
	t.known, t.header, t.started = idx >= 0, false, true
	t.pos = int8(idx & 7)
	if h.HasSeq {
		t.seq, t.hasSeq = h.Seq, true
	}
	h.VoiceBurst = enums.VoiceBurstUnknown
	if v, ok := enums.VoiceBurstAt(idx); ok {
		h.VoiceBurst = v
	}
	return lost, PlacementInOrder
}

func (t *PositionTracker) seqGap(h *Hint) (int, Placement) {
	if !h.HasSeq || !t.hasSeq {
		return 0, PlacementInOrder
	}
	bits := uint(h.SeqBits)
	if bits == 0 || bits > 16 {
		bits = 16
	}
	size := 1 << bits
	d := int(h.Seq-t.seq) & (size - 1)
	switch {
	case d == 0:
		return 0, PlacementDuplicate
	case d < size/2:
		return d, PlacementInOrder
	case size-d <= lateWindow:
		return size - d, PlacementLate
	}
	return 0, PlacementInOrder
}

func (t *PositionTracker) behind(strong int, ok, cont bool, back int) enums.VoiceBurstType {
	if ok {
		v, _ := enums.VoiceBurstAt(strong)
		return v
	}
	if !t.known {
		return enums.VoiceBurstUnknown
	}
	idx := ((int(t.pos)-back)%VoiceBurstsPerSuperframe + VoiceBurstsPerSuperframe) % VoiceBurstsPerSuperframe
	if idx == 0 || cont && idx != 2 && idx != 3 {
		return enums.VoiceBurstUnknown
	}
	v, _ := enums.VoiceBurstAt(idx)
	return v
}

func (t *PositionTracker) position(strong int, ok, cont bool, steps int, bySeq bool) int {
	if ok {
		return strong
	}
	if !t.known {
		return -1
	}
	pred := (int(t.pos) + steps) % VoiceBurstsPerSuperframe
	if pred == 0 && bySeq {
		pred = 1
	}
	if pred == 0 || cont && pred != 2 && pred != 3 {
		return -1
	}
	return pred
}

func rawPosition(raw *RawBurst, h *Hint) (pos int, ok, cont bool) {
	if i, ok := h.VoiceBurst.Index(); ok {
		return i, true, false
	}
	s, d := raw.Sync()
	if d <= DefaultSyncThreshold && isVoiceSync(s) {
		return 0, true, false
	}
	if d <= DefaultSyncThreshold && h.Kind != KindVoice {
		return 0, false, false
	}
	return embPosition(raw.EMB())
}

func decodedPosition(b *Burst, h *Hint) (pos int, ok, cont bool) {
	if i, ok := h.VoiceBurst.Index(); ok {
		return i, true, false
	}
	if b.VoiceBurst == enums.VoiceBurstA {
		return 0, true, false
	}
	if !b.HasEmbeddedSignalling() {
		return 0, false, false
	}
	return embPosition(b.EmbeddedSignalling)
}

func embPosition(emb pdu.EmbeddedSignalling) (pos int, ok, cont bool) {
	if emb.FEC.Uncorrectable {
		return 0, false, false
	}
	switch emb.LCSS {
	case enums.LCSSFirstFragment:
		return 1, true, false
	case enums.LCSSLastFragment:
		return 4, true, false
	case enums.LCSSSingleFragment:
		return 5, true, false
	case enums.LCSSContinuationFragment:
	}
	return 0, false, true
}

func nearestLost(gap, est int) int {
	if gap > est+2 {
		return est
	}
	n := gap
	for n+VoiceBurstsPerSuperframe/2 < est {
		n += VoiceBurstsPerSuperframe
	}
	return n
}
