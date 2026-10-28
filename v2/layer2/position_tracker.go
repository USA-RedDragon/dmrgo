package layer2

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const lateWindow = 32

const maxPlaceSteps = 1 << 16

// ETSI TS 102 361-1 clause 4.2.2
const burstPeriod = 60 * time.Millisecond

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
// superframe, packet by packet, as voice.Receiver does: from the position in
// the Hint, the voice SYNC of burst A, the LCSS of the EMB of bursts B to F,
// and the sequence number of the Hint, or one step per burst without one.
// The position in the Hint and the voice SYNC come first. Once the Hint, the
// voice SYNC or the LCSS has placed a burst of the call, a sequence number
// that follows the last one places the burst over its LCSS, because a
// repeater sends a wrong EMB on a weak signal; the LCSS places a burst only
// without that evidence, or where the sequence number predicts a burst A
// that has no SYNC. After a header, the LCSS comes first until it or the
// SYNC places a burst, because the sequence number also counts lost headers.
// The number of bursts lost before a placed burst is the count that fits its
// position and is nearest to an estimate, with ties to the smaller count.
// The estimate is the missing sequence numbers. With Hint.At, the 60 ms burst
// periods since the burst before it, less one, take its place after a
// SyncLost Hint, and lower it when the position of the burst before is
// known. The periods run on a clock that the earliest arrivals of the call
// set, so that a late or bunched packet does not move it. After a header the
// burst can also follow bursts that came before the header, as after the
// header that a repeater makes for a late entry; then the count is the
// estimate. A SyncLost Hint with Hint.At keeps the position for the count. A
// bridge that forwards each packet as it comes, such as IPSC to HomeBrew
// DMRD, calls Place for each packet of a slot, so that the voice bursts that
// the source does not mark get a position. The zero PositionTracker is ready
// to use. It is not safe for concurrent use.
// (ETSI TS 102 361-1 clause 5.1.2.1, clause 9.3.5)
type PositionTracker struct {
	off       time.Duration
	count     int
	stream    uint32
	seq       uint16
	pos       int8
	known     bool
	header    bool
	started   bool
	anchored  bool
	hasSeq    bool
	hasStream bool
	signal    bool
	clock     bool
}

// Reset clears the call.
func (t *PositionTracker) Reset() {
	if t == nil {
		return
	}

	if t != nil {
		*t = PositionTracker{}
	}
}

// Forget clears the superframe position and keeps the sequence number, so
// that the next burst is placed only by its SYNC or EMB, as after a weak
// burst.
func (t *PositionTracker) Forget() {
	if t == nil {
		return
	}

	if t != nil {
		t.known = false
	}
}

// Place takes the burst raw of one packet with its Hint h, as a transport
// gives them, and sets h.VoiceBurst of a voice burst to its superframe
// position, or to VoiceBurstUnknown when it cannot place it. A new stream
// ID starts a new call. A voice LC header or PI header puts the next burst
// at A, a terminator or an EndStrong hint ends the call, and a SyncLost or
// EndWeak hint clears the position, which a SyncLost hint with At keeps
// only to count the bursts lost. h.Kind KindUnknown takes a burst with
// a data SYNC as data and any other as voice. Place returns the number of
// bursts lost before raw and how it took raw. A duplicate or late burst
// gets the position that its sequence number gives, and does not move the
// tracker. A nil raw, for a packet without a burst, applies only the
// stream, SyncLost and End of h. A nil t or h is a no-op.
func (t *PositionTracker) Place(raw *RawBurst, h *Hint) (lost int, p Placement) {
	if t == nil {
		return 0, 0
	}

	if t == nil || h == nil {
		return 0, PlacementInOrder
	}
	t.hint(h)
	if raw == nil {
		if t.signal && h.HasSeq && t.hasSeq {
			if _, p := t.seqGap(h); p == PlacementInOrder {
				t.seq = h.Seq
			}
		}
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
		lost, p = t.place(rawCue(raw, h), h, 1)
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
	case KindReverseChannel, KindUnknown:
	}
	if h.End == EndStrong {
		t.end()
	}
	return lost, p
}

func (t *PositionTracker) hint(h *Hint) (newCall bool) {
	if h.HasStream {
		if t.hasStream && h.Stream != t.stream {
			*t = PositionTracker{}
			newCall = true
		}
		t.stream, t.hasStream = h.Stream, true
	}
	if h.SyncLost {
		t.signal = t.clock && h.At != 0
		t.known = t.known && t.signal
	}
	return newCall
}

func (t *PositionTracker) end() {
	stream, has := t.stream, t.hasStream
	*t = PositionTracker{stream: stream, hasStream: has}
}

// Header records that a voice LC header or PI header with Hint h
// came before the first voice burst of the call, so that the next burst is
// burst A. It does nothing after the first voice burst, or for a nil t or
// h.
func (t *PositionTracker) Header(h *Hint) {
	if t == nil {
		return
	}

	if t == nil || h == nil || t.started {
		return
	}
	t.pos = VoiceBurstsPerSuperframe - 1
	t.count = -1
	if h.At != 0 {
		t.off, t.clock = h.At+burstPeriod, true
	}
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
// b. A nil t, b or h is a no-op, and steps below 1 count as 1. Steps over
// 65536, about 65 minutes of bursts, clear the superframe position first,
// as Forget does.
func (t *PositionTracker) PlaceDecoded(b *Burst, h *Hint, steps int) (lost int, p Placement) {
	if t == nil {
		return 0, 0
	}

	if t == nil || b == nil || h == nil {
		return 0, PlacementInOrder
	}
	return t.place(decodedCue(b, h), h, steps)
}

type cue struct {
	pos   int8
	emb   int8
	fixed bool
	byEMB bool
	cont  bool
}

func (t *PositionTracker) place(c cue, h *Hint, steps int) (lost int, p Placement) {
	gap, p := t.seqGap(h)
	if p != PlacementInOrder {
		h.VoiceBurst = t.behind(c, gap)
		return 0, p
	}
	steps = max(steps, 1)
	if gap > 0 {
		steps = gap
	}
	if steps > maxPlaceSteps {
		t.known, t.header, t.signal = false, false, false
	}
	base, signal := int(t.pos), t.signal
	reliable := t.known && (t.anchored || !t.started)
	if signal {
		t.known = false
	}
	idx := t.position(c, steps, gap > 0)
	lost = t.lostBefore(h, idx, base, gap, steps, reliable, signal)
	t.anchored = idx >= 0 && (c.fixed || c.byEMB && idx == int(c.emb) || t.known && t.anchored)
	t.known, t.header, t.started, t.signal = idx >= 0, false, true, false
	t.tick(h, lost)
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

func (t *PositionTracker) lostBefore(h *Hint, idx, base, gap, steps int, reliable, signal bool) int {
	est, timed := t.elapsed(h)
	want := gap - 1
	if timed && (signal || reliable && est < want) {
		want = est
	}
	switch {
	case gap > 0 && idx >= 0 && reliable:
		return fitLost(idx, base, want, !t.started)
	case gap > 0:
		return max(want, 0)
	case !h.HasSeq && signal && timed && idx >= 0 && reliable:
		return fitLost(idx, base, est, false)
	case idx >= 0 && (t.known || t.header):
		lost := (idx - base - 1 + 2*VoiceBurstsPerSuperframe) % VoiceBurstsPerSuperframe
		if !h.HasSeq {
			lost = nearestLost(lost, steps-1)
		}
		return lost
	case !h.HasSeq:
		return steps - 1
	}
	return 0
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

func (t *PositionTracker) behind(c cue, back int) enums.VoiceBurstType {
	idx := -1
	switch {
	case c.fixed:
		idx = int(c.pos)
	case t.known:
		idx = ((int(t.pos)-back)%VoiceBurstsPerSuperframe + VoiceBurstsPerSuperframe) % VoiceBurstsPerSuperframe
	case c.byEMB:
		idx = int(c.emb)
	}
	if idx == 0 && !c.fixed {
		return enums.VoiceBurstUnknown
	}
	v, _ := enums.VoiceBurstAt(idx)
	return v
}

func (t *PositionTracker) position(c cue, steps int, bySeq bool) int {
	if c.fixed {
		return int(c.pos)
	}
	pred := (int(t.pos) + steps) % VoiceBurstsPerSuperframe
	seq := t.known && bySeq
	switch {
	case c.byEMB && (!seq || !t.anchored || pred == 0):
		return int(c.emb)
	case seq && pred == 0:
		return 1
	case seq:
		return pred
	case !t.known || pred == 0 || c.cont && pred != 2 && pred != 3:
		return -1
	}
	return pred
}

func rawCue(raw *RawBurst, h *Hint) cue {
	if i, ok := h.VoiceBurst.Index(); ok {
		return cue{pos: int8(i & 7), fixed: true}
	}
	s, d := raw.Sync()
	if d <= DefaultSyncThreshold && isVoiceSync(s) {
		return cue{fixed: true}
	}
	if d <= DefaultSyncThreshold && h.Kind != KindVoice {
		return cue{}
	}
	return embCue(raw.EMB())
}

func decodedCue(b *Burst, h *Hint) cue {
	if i, ok := h.VoiceBurst.Index(); ok {
		return cue{pos: int8(i & 7), fixed: true}
	}
	if b.VoiceBurst == enums.VoiceBurstA {
		return cue{fixed: true}
	}
	if !b.HasEmbeddedSignalling() {
		return cue{}
	}
	return embCue(b.EmbeddedSignalling)
}

func embCue(emb pdu.EmbeddedSignalling) cue {
	if emb.FEC.Uncorrectable {
		return cue{}
	}
	switch emb.LCSS {
	case enums.LCSSFirstFragment:
		return cue{emb: 1, byEMB: true}
	case enums.LCSSLastFragment:
		return cue{emb: 4, byEMB: true}
	case enums.LCSSSingleFragment:
		return cue{emb: 5, byEMB: true}
	case enums.LCSSContinuationFragment:
	}
	return cue{cont: true}
}

func (t *PositionTracker) elapsed(h *Hint) (int, bool) {
	if h.At == 0 || !t.clock {
		return 0, false
	}
	d := h.At - t.off + burstPeriod/2
	n := d / burstPeriod
	if d%burstPeriod < 0 {
		n--
	}
	e := n - time.Duration(t.count) - 1
	return int(max(min(e, maxPlaceSteps), -maxPlaceSteps)), true
}

func (t *PositionTracker) tick(h *Hint, lost int) {
	t.count = min(t.count+lost+1, 1<<30)
	if h.At == 0 {
		return
	}
	if o := h.At - time.Duration(t.count)*burstPeriod; !t.clock || o < t.off {
		t.off = o
	}
	t.clock = true
}

func fitLost(idx, base, want int, afterHeader bool) int {
	r := (idx - base - 1 + 2*VoiceBurstsPerSuperframe) % VoiceBurstsPerSuperframe
	if want <= r {
		if afterHeader && r-want >= want-(r-VoiceBurstsPerSuperframe) {
			return max(want, 0)
		}
		return r
	}
	lo := r + VoiceBurstsPerSuperframe*((want-r)/VoiceBurstsPerSuperframe)
	if want-lo <= VoiceBurstsPerSuperframe/2 {
		return lo
	}
	return lo + VoiceBurstsPerSuperframe
}

func nearestLost(gap, est int) int {
	if gap > est+2 {
		return est
	}
	if d := est - VoiceBurstsPerSuperframe/2 - gap; d > 0 {
		return gap + VoiceBurstsPerSuperframe*((d-1)/VoiceBurstsPerSuperframe+1)
	}
	return gap
}
