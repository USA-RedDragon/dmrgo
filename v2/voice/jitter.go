package voice

import (
	"fmt"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Limits and default of JitterConfig.
const (
	// DefaultJitter is the playout delay of a Jitter when JitterConfig.Delay
	// is 0: 360 ms, the default Jitter of the DMR Network section of
	// MMDVMHost.
	DefaultJitter = 360 * time.Millisecond
	// MaxJitter is the longest playout delay. A Jitter holds two streams of
	// 3 packets for each started 60 ms of its delay, plus 8: 220 packets at
	// MaxJitter and 52 at DefaultJitter, about 80 bytes each.
	MaxJitter = 2 * time.Second
)

const jitterHistory = 64

var errJitterDelay error = &dmrgo.ConfigError{Type: pkgName, Field: "Delay"}

// JitterConfig configures NewJitter. A nil config means the zero
// JitterConfig.
type JitterConfig struct {
	// Delay is the playout delay: the time from the arrival of the packet
	// that came first for its place in the stream to the Pop that returns
	// it. A packet that comes up to Delay later than that, in any order,
	// plays in its place. 0
	// means DefaultJitter, and a negative value turns the buffer off: Push
	// then returns JitterBypass for every packet. At most MaxJitter.
	Delay time.Duration
	// Lead is the time from a Pop to the Due time of the packet it returns,
	// for a Transmitter that relays the call with this
	// TransmitterConfig.Lead, so that the packet is there before the
	// Transmitter sends its burst. It must not be negative.
	Lead time.Duration
}

// JitterResult is what Push did with a packet.
type JitterResult uint8

// The JitterResult values.
const (
	// JitterBuffered is a packet that Pop returns later.
	JitterBuffered JitterResult = iota + 1
	// JitterBypass is a packet that the Jitter does not take: the buffer is
	// off, the packet has no Hint.HasSeq, or two other streams wait. The
	// caller handles it at once.
	JitterBypass
	// JitterLate is a packet dropped because its Due time has passed, or
	// because a later packet has played.
	JitterLate
	// JitterDuplicate is a packet dropped as a copy of one buffered or
	// played.
	JitterDuplicate
	// JitterOverflow is a packet dropped because it is too far ahead of the
	// packet that plays next for the buffer to hold it.
	JitterOverflow
)

// String returns the name of the result. It allocates only for a value
// without a name.
func (r JitterResult) String() string {
	switch r {
	case JitterBuffered:
		return "Buffered"
	case JitterBypass:
		return "Bypass"
	case JitterLate:
		return "Late"
	case JitterDuplicate:
		return "Duplicate"
	case JitterOverflow:
		return "Overflow"
	}
	return fmt.Sprintf("JitterResult(%d)", uint8(r))
}

// JitterStats counts what a Jitter dropped and how often it started its
// playout again. The counts stop at the largest uint32.
type JitterStats struct {
	// Late is the number of packets dropped as late: those that Push
	// returned JitterLate for, and buffered packets that a later packet made
	// late, at the start of the playout or when it moved earlier.
	Late uint32
	// Duplicates is the number of packets that Push returned
	// JitterDuplicate for.
	Duplicates uint32
	// Overflows is the number of packets that Push returned JitterOverflow
	// for.
	Overflows uint32
	// Resyncs is the number of times the playout started again after a
	// stall longer than the delay, a jump of the sequence number, or a Pop
	// that came more than the delay after a Due time.
	Resyncs uint32
}

// JitterPacket is a packet that Pop returns.
type JitterPacket struct {
	// Burst is the burst of the packet. HasBurst reports whether it is set.
	Burst layer2.RawBurst
	// HasBurst is false for a transport frame without a burst, that Push
	// took with a nil raw.
	HasBurst bool
	// Hint is the Hint that Push took, with At set to Due.
	Hint layer2.Hint
	// Source is the src that Push took.
	Source uint32
	// Due is the time at which the packet plays: the caller hands it to a
	// Receiver or a Repeater with this time as now.
	Due time.Duration
}

type jitterEntry struct {
	raw   layer2.RawBurst
	h     layer2.Hint
	arr   time.Duration
	src   uint32
	full  bool
	burst bool
}

type jitterKey struct {
	src       uint32
	stream    uint32
	hasStream bool
}

func keyOf(src uint32, h *layer2.Hint) jitterKey {
	k := jitterKey{src: src, hasStream: h.HasStream}
	if h.HasStream {
		k.stream = h.Stream
	}
	return k
}

type jitterRing struct {
	entries []jitterEntry
	key     jitterKey
	used    bool
	closed  bool
	ended   bool
	mask    uint16
	refSeq  uint16
	refV    int
	head    int
	low     int
	count   int
	played  uint64
	base    time.Duration
}

func (q *jitterRing) is(k jitterKey) bool { return q.used && q.key == k }

func seqDelta(seq, ref, mask uint16) int {
	size := int(mask) + 1
	d := int(seq-ref) & int(mask)
	if d >= size/2 {
		d -= size
	}
	return d
}

// Jitter is a network jitter buffer for the packets of one slot, like the
// Jitter of the DMR Network section of MMDVMHost. It puts the packets of a
// stream back in the order of Hint.Seq and plays them a fixed delay after
// they were sent, one per 60 ms burst period, so that a Receiver, and a
// Transmitter or a bs.Repeater that relays the call, get the stream as the
// sender sent it. A Receiver drops a packet that comes after a later one
// as late, and counts the bursts lost from Hint.Seq, so the Jitter goes in
// front of it. Push takes a packet and Pop returns the next one at its
// time; time comes only from their now and from Hint.At.
//
// The rules are these.
//   - Each packet of a stream implies the time of the first sequence number:
//     its arrival less 60 ms for each sequence number it is ahead. The
//     earliest of these, from the packets that came before the playout
//     starts, is the clock of the stream, so that a late or bunched packet
//     does not move it. The packet with the lowest sequence number plays
//     Delay after its time on that clock, and each later sequence number 60
//     ms after the one before. So the first packet plays at most Delay
//     after it arrived. The delay stays as the start set it: it does not
//     shrink when the network delay falls later in the stream.
//   - A sender whose clock runs fast fills the buffer until it holds no more;
//     the playout then moves earlier as for an early packet.
//   - A sequence number that is missing when a later packet plays is lost.
//     Pop skips it, and the Receiver counts it from Hint.Seq and fills it.
//     A packet after a gap plays no later than Delay after it arrived, on
//     the 60 ms grid, so that a sender whose sequence number also counts
//     other packets, such as one counter for both slots, does not fall
//     behind.
//   - A packet that comes after its time to play, or behind a packet that
//     has played, is late and dropped. A copy of a packet that is buffered
//     or among the last 64 played is a duplicate and dropped.
//   - A packet so early that the buffer cannot hold it moves the playout
//     earlier, by whole burst periods, so that it plays at most Delay after
//     it arrived, and the Jitter counts a resync; the packets that this
//     makes late are dropped as late. A packet that is still too early is
//     dropped as an overflow.
//   - A stall longer than the delay empties the buffer. The packets that
//     come after it start the playout again by the first rule, on the 60 ms
//     grid of the packets before, and the Jitter counts a resync. Those of
//     them that came more than the delay late are dropped as late, so the
//     delay stays the same; the stall becomes fills. A packet that comes late
//     while others wait is dropped.
//   - A new stream, another src or Hint.Stream, waits behind the one that
//     plays and starts by the first rule, but not before 60 ms after the last
//     packet of the stream before it. A jump of the sequence number of more
//     than the buffer holds starts a new stream in the same way. A third
//     stream while two wait passes: Push returns JitterBypass. Until Delay
//     and Lead after its last packet played, a packet of the stream before
//     that is up to 64 sequence numbers behind
//     its last one, or ahead by no more than the buffer holds, is late,
//     unless it is a Voice LC Header or PI Header. A new stream takes the
//     place of one whose buffer is empty only when that one has ended or
//     its next packet is overdue.
//   - After a Terminator with LC or a Hint.End of EndStrong has played and
//     the buffer is empty, a Voice LC Header or PI Header of the stream
//     starts a new stream, as for a transport whose sequence number starts
//     again with each call; so does any packet that is not a copy of one
//     played, more than Delay and Lead after the last packet played. A
//     packet more than 64 sequence numbers behind the last one is late
//     unless it is such a header.
//   - Hint.SeqBits gives the width at which the sequence number wraps: 8
//     for HomeBrew DMRD, 0 or 16 for RTP.
//   - A now more than Delay, Lead and 1 s earlier than the last one is a
//     clock reset, and the Jitter moves its times by the jump; Deadline
//     gives the old times until the next Push or Pop. A now that is less
//     earlier, as from a caller that ticks late, changes nothing. A Pop more than Delay after the time of the
//     next packet, as after the clock jumped forward, plays it at once and
//     the rest 60 ms apart.
//
// A Jitter delays each packet that it takes by the playout delay, which
// only voice needs. A caller with data, CSBK or MBC bursts passes them
// around it, as bs.Repeater does.
//
// The zero Jitter passes every packet through. Make it with NewJitter. A
// Jitter is not safe for concurrent use and must not be copied.
type Jitter struct {
	delay    time.Duration
	lead     time.Duration
	margin   int
	rings    [2]jitterRing
	cur      *jitterRing
	nxt      *jitterRing
	prev     jitterKey
	prevRef  uint16
	prevMask uint16
	prevEnd  time.Duration
	hasPrev  bool
	running  bool
	resync   bool
	vT       int
	tT       time.Duration
	lastDue  time.Duration
	hasLast  bool
	now      time.Duration
	hasNow   bool
	stats    JitterStats
	noCopy   noCopy
}

// NewJitter returns a Jitter for cfg. A nil cfg means the zero
// JitterConfig. It returns a *dmrgo.ConfigError for a field out of range.
// It allocates the Jitter and its storage, two streams of 3 packets for
// each started 60 ms of Delay, plus 8.
func NewJitter(cfg *JitterConfig) (*Jitter, error) {
	var conf JitterConfig
	if cfg != nil {
		conf = *cfg
	}
	j := &Jitter{delay: conf.Delay, lead: conf.Lead}
	if j.delay == 0 {
		j.delay = DefaultJitter
	}
	switch {
	case conf.Lead < 0:
		return nil, errLead
	case j.delay > MaxJitter:
		return nil, errJitterDelay
	case j.delay < 0:
		return j, nil
	}
	n := int((j.delay + slotPeriod - 1) / slotPeriod)
	j.margin = n
	for i := range j.rings {
		j.rings[i].entries = make([]jitterEntry, 3*n+8)
	}
	j.cur, j.nxt = &j.rings[0], &j.rings[1]
	return j, nil
}

// Reset drops every packet and clears the streams and the counts. It keeps
// the config.
func (j *Jitter) Reset() {
	if j == nil || j.cur == nil {
		return
	}
	for i := range j.rings {
		q := &j.rings[i]
		for k := range q.entries {
			q.entries[k].full = false
		}
		*q = jitterRing{entries: q.entries}
	}
	j.hasPrev, j.running, j.resync, j.hasLast, j.hasNow = false, false, false, false, false
	j.stats = JitterStats{}
}

// Stats returns the counts since NewJitter or Reset.
func (j *Jitter) Stats() JitterStats {
	if j == nil {
		return JitterStats{}
	}
	return j.stats
}

// Len returns the number of packets that wait.
func (j *Jitter) Len() int {
	if j == nil || j.cur == nil {
		return 0
	}
	return j.cur.count + j.nxt.count
}

func (j *Jitter) clock(now time.Duration) {
	if j.hasNow && now < j.now && now >= j.now-j.delay-j.lead-time.Second {
		return
	}
	if j.hasNow && now < j.now {
		d := now - j.now
		j.tT += d
		j.lastDue += d
		j.prevEnd += d
		for i := range j.rings {
			q := &j.rings[i]
			q.base += d
			for k := range q.entries {
				q.entries[k].arr += d
			}
		}
	}
	j.now, j.hasNow = now, true
}

func bursts(n int) time.Duration { return time.Duration(n) * slotPeriod }

func ceilBursts(d time.Duration) time.Duration {
	if d <= 0 {
		return bursts(int(d / slotPeriod))
	}
	return bursts(int((d + slotPeriod - 1) / slotPeriod))
}

func (q *jitterRing) at(v int) *jitterEntry {
	n := len(q.entries)
	return &q.entries[(v%n+n)%n]
}

func satInc(n *uint32) {
	if *n < ^uint32(0) {
		*n++
	}
}

func (j *Jitter) drop(r JitterResult) JitterResult {
	switch r {
	case JitterLate:
		satInc(&j.stats.Late)
	case JitterDuplicate:
		satInc(&j.stats.Duplicates)
	case JitterOverflow:
		satInc(&j.stats.Overflows)
	case JitterBuffered, JitterBypass:
	}
	return r
}

func (j *Jitter) begin(q *jitterRing, k jitterKey, h *layer2.Hint, arr time.Duration) {
	bits := h.SeqBits
	if bits == 0 || bits > 16 {
		bits = 16
	}
	*q = jitterRing{entries: q.entries, key: k, used: true, mask: uint16(1<<bits - 1), refV: j.margin, base: arr}
	q.refSeq = h.Seq & q.mask
}

func (j *Jitter) retire() {
	j.prev, j.prevRef, j.prevMask, j.hasPrev = j.cur.key, j.cur.refSeq, j.cur.mask, j.cur.used
	j.prevEnd = j.lastDue + j.delay + j.lead
}

// Push takes the packet raw with Hint h from the sender src, such as a
// peer ID, at the time now, and returns what it did with it. A nil raw is a
// transport frame without a burst, which plays in its place as the others
// do. The arrival time is h.At when it is set and not after now or more
// than the delay before it, else now. A nil j, the zero Jitter, a Jitter
// whose buffer is off, and a packet without h.HasSeq return JitterBypass.
func (j *Jitter) Push(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, src uint32) JitterResult {
	if j == nil || j.cur == nil {
		return JitterBypass
	}
	j.clock(now)
	if !h.HasSeq {
		return JitterBypass
	}
	arr := now
	if h.At != 0 && h.At <= now && now-h.At <= j.delay {
		arr = h.At
	}
	k := keyOf(src, &h)
	start := starts(&h, raw)
	if j.hasPrev && j.prev == k && !start && now <= j.prevEnd && !j.cur.is(k) && !j.nxt.is(k) {
		if d := seqDelta(h.Seq, j.prevRef, j.prevMask); d >= -jitterHistory && d <= len(j.cur.entries) {
			return j.drop(JitterLate)
		}
	}
	q := j.ring(k, &h, arr, now, start)
	if q == nil {
		return JitterBypass
	}
	q, v, r, ahead := j.place(q, k, &h, arr, start)
	if ahead && j.evict(q, v, arr, now) {
		r = JitterBuffered
	}
	if r != JitterBuffered {
		return j.drop(r)
	}
	if q == j.cur && j.running {
		if due := j.runDue(v, arr); due-j.lead < now {
			if j.Len() > 0 {
				return j.drop(JitterLate)
			}
			j.running, j.resync = false, true
			q.base = arr - j.offset(v)
			satInc(&j.stats.Resyncs)
		}
	} else if b := arr - j.offset(v); b < q.base {
		q.base = b
	}
	e := q.at(v)
	e.full, e.burst, e.src, e.h, e.arr = true, raw != nil, src, h, arr
	if raw != nil {
		e.raw = *raw
	}
	if q.count == 0 || v < q.low {
		q.low = v
	}
	q.count++
	if d := seqDelta(h.Seq, q.refSeq, q.mask); d > 0 {
		q.refSeq, q.refV = h.Seq&q.mask, v
	}
	return JitterBuffered
}

func (j *Jitter) ring(k jitterKey, h *layer2.Hint, arr, now time.Duration, start bool) *jitterRing {
	switch {
	case j.nxt.is(k):
		return j.nxt
	case j.cur.is(k) && !j.cur.closed && !j.restarts(h, now, start):
		return j.cur
	case !j.cur.used || j.Len() == 0 && j.idle(now):
		j.retire()
		j.nxt.used = false
		j.begin(j.cur, k, h, arr)
		j.running, j.resync = false, false
		return j.cur
	case !j.nxt.used:
		j.begin(j.nxt, k, h, arr)
		return j.nxt
	}
	return nil
}

func (j *Jitter) restarts(h *layer2.Hint, now time.Duration, start bool) bool {
	if !j.cur.ended || j.Len() > 0 {
		return false
	}
	d := seqDelta(h.Seq, j.cur.refSeq, j.cur.mask)
	if back := j.cur.head - 1 - (j.cur.refV + d); !start && back >= 0 && back < jitterHistory && j.cur.played>>back&1 != 0 {
		return false
	}
	return start || now > j.lastDue+j.delay+j.lead
}

func (j *Jitter) idle(now time.Duration) bool {
	return j.cur.ended || j.cur.closed || !j.running || j.tT+bursts(j.cur.head-j.vT)-j.lead < now
}

func (j *Jitter) place(q *jitterRing, k jitterKey, h *layer2.Hint, arr time.Duration, start bool) (*jitterRing, int, JitterResult, bool) {
	d := seqDelta(h.Seq, q.refSeq, q.mask)
	if d < -jitterHistory && !start {
		return q, 0, JitterLate, false
	}
	if d < -jitterHistory || d > len(q.entries) {
		switch {
		case q == j.cur && j.Len() == 0:
			j.begin(j.cur, k, h, arr)
			j.running, j.resync = false, true
			satInc(&j.stats.Resyncs)
		case q == j.cur && !j.nxt.used:
			j.cur.closed = true
			j.begin(j.nxt, k, h, arr)
			q = j.nxt
		default:
			return q, 0, JitterOverflow, false
		}
		d = 0
	}
	v := q.refV + d
	switch {
	case v < q.head:
		if back := q.head - 1 - v; back >= 0 && back < jitterHistory && q.played>>back&1 != 0 {
			return q, 0, JitterDuplicate, false
		}
		return q, 0, JitterLate, false
	case v-q.head >= len(q.entries):
		return q, v, JitterOverflow, true
	case q.at(v).full:
		return q, 0, JitterDuplicate, false
	}
	return q, v, JitterBuffered, false
}

func dataTypeOf(h *layer2.Hint, raw *layer2.RawBurst) (enums.DataType, bool) {
	switch {
	case h.HasDataType:
		return h.DataType, true
	case raw == nil || h.Kind == layer2.KindVoice:
		return 0, false
	}
	switch s, d := raw.Sync(); {
	case d > layer2.DefaultSyncThreshold:
		return 0, false
	case s != enums.SyncBSSourcedData && s != enums.SyncMSSourcedData && s != enums.SyncTDMA1Data && s != enums.SyncTDMA2Data:
		return 0, false
	}
	st := raw.SlotType()
	return st.DataType, !st.FEC.Uncorrectable
}

func starts(h *layer2.Hint, raw *layer2.RawBurst) bool {
	dt, ok := dataTypeOf(h, raw)
	return ok && (dt == enums.DataTypeVoiceLCHeader || dt == enums.DataTypePIHeader)
}

func ends(e *jitterEntry) bool {
	raw := &e.raw
	if !e.burst {
		raw = nil
	}
	dt, ok := dataTypeOf(&e.h, raw)
	return e.h.End == layer2.EndStrong || ok && dt == enums.DataTypeTerminatorWithLC
}

func (j *Jitter) evict(q *jitterRing, v int, arr, now time.Duration) bool {
	running := q == j.cur && j.running
	base := min(q.base, arr-j.offset(v))
	grid := q == j.cur && j.resync
	if running {
		k := int((j.tT + bursts(v-j.vT) - arr - j.delay - j.lead) / slotPeriod)
		if k <= 0 {
			return false
		}
		j.tT -= bursts(k)
		satInc(&j.stats.Resyncs)
	}
	for q.count > 0 {
		e := q.at(q.low)
		if running {
			due := j.tT + bursts(q.low-j.vT)
			if due-j.lead >= now && (!j.hasLast || due >= j.lastDue+slotPeriod) {
				break
			}
		} else if j.startAt(base+j.offset(q.low), grid)-j.lead >= e.arr {
			break
		}
		e.full = false
		q.count--
		satInc(&j.stats.Late)
		q.nextLow()
	}
	head := v
	if q.count > 0 && q.low < v {
		head = q.low
	}
	if head < q.head || v-head >= len(q.entries) {
		return false
	}
	q.shift(head - q.head)
	q.head = head
	return true
}

func (q *jitterRing) shift(k int) {
	if k >= jitterHistory {
		q.played = 0
	} else if k > 0 {
		q.played <<= k
	}
}

func (q *jitterRing) nextLow() {
	for i := 1; q.count > 0 && i < len(q.entries); i++ {
		if q.at(q.low + i).full {
			q.low += i
			return
		}
	}
}

func (j *Jitter) runDue(v int, arr time.Duration) time.Duration {
	due := j.tT + bursts(v-j.vT)
	if late := arr + j.delay + j.lead; late < due {
		due = j.tT + ceilBursts(late-j.tT)
	}
	return max(due, j.lastDue+slotPeriod)
}

func (j *Jitter) offset(v int) time.Duration {
	return bursts(v - j.margin)
}

func (j *Jitter) next() (v int, cross bool, due time.Duration, ok bool) {
	q := j.cur
	switch {
	case q.count > 0 && j.running:
		return q.low, false, j.runDue(q.low, q.at(q.low).arr), true
	case q.count == 0 && j.nxt.count == 0:
		return 0, false, 0, false
	case q.count == 0:
		q, cross = j.nxt, true
	}
	grid := !cross && j.resync
	for i := range len(q.entries) {
		e := q.at(q.low + i)
		if !e.full {
			continue
		}
		v, due = q.low+i, j.startAt(q.base+j.offset(q.low+i), grid)
		if due-j.lead >= e.arr {
			break
		}
	}
	return v, cross, due, true
}

func (j *Jitter) startAt(implied time.Duration, grid bool) time.Duration {
	at := implied + j.delay + j.lead
	switch {
	case !j.hasLast:
	case at < j.lastDue+slotPeriod:
		at = j.lastDue + slotPeriod
	case grid:
		at = j.lastDue + ceilBursts(at-j.lastDue)
	}
	return at
}

// Deadline returns the time at which the next packet is to be popped: its
// Due time less Lead. ok is false when no packet waits.
func (j *Jitter) Deadline() (at time.Duration, ok bool) {
	if j == nil || j.cur == nil {
		return 0, false
	}
	_, _, due, ok := j.next()
	if !ok {
		return 0, false
	}
	return due - j.lead, true
}

// Pop returns in p the next packet when now has reached its Due time less
// Lead, and reports whether it did. Each Pop returns at most one packet; a
// late caller pops until Deadline is in the future. A nil j or p returns
// false.
func (j *Jitter) Pop(now time.Duration, p *JitterPacket) bool {
	if j == nil || p == nil || j.cur == nil {
		return false
	}
	j.clock(now)
	v, cross, due, ok := j.next()
	if !ok || due-j.lead > now {
		return false
	}
	if now-(due-j.lead) > j.delay {
		due = now + j.lead
		satInc(&j.stats.Resyncs)
	}
	if cross {
		j.retire()
		j.cur, j.nxt = j.nxt, j.cur
		j.nxt.used = false
	}
	q := j.cur
	for u := q.low; u < v; u++ {
		if e := q.at(u); e.full {
			e.full = false
			q.count--
			satInc(&j.stats.Late)
		}
	}
	q.shift(v + 1 - q.head)
	q.played |= 1
	q.head = v + 1
	j.running, j.resync, j.vT, j.tT = true, false, v, due
	j.lastDue, j.hasLast = due, true
	e := q.at(v)
	p.Burst, p.HasBurst, p.Hint, p.Source, p.Due = e.raw, e.burst, e.h, e.src, due
	p.Hint.At = due
	q.ended = q.ended || ends(e)
	e.full = false
	q.count--
	q.low = v
	q.nextLow()
	return true
}
