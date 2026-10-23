package bs

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

const (
	expressCap     = 16
	expressKeys    = 4
	expressHistory = 64
	expressHold    = burstPeriod
)

type expressKey struct {
	src       uint32
	stream    uint32
	hasStream bool
	used      bool
	started   bool
	skips     bool
	voice     bool
	mask      uint16
	next      uint16
	seen      uint64
	last      time.Duration
	relAt     time.Duration
	dataSince time.Duration
}

type expressPkt struct {
	raw    layer2.RawBurst
	h      layer2.Hint
	src    uint32
	arr    time.Duration
	need   uint32
	key    int
	burst  bool
	behind bool
	used   bool
}

type voiceTrack struct {
	src     uint32
	pushed  uint32
	popped  uint32
	pending uint32
	start   time.Duration
	last    time.Duration
	open    bool
	used    bool
}

type express struct {
	pkts   [expressCap]expressPkt
	n      int
	keys   [expressKeys]expressKey
	voice  [expressKeys]voiceTrack
	popAt  time.Duration
	now    time.Duration
	hasNow bool
}

type route uint8

const (
	routeTake route = iota
	routeDone
)

func expressSeqDelta(seq, ref, mask uint16) int {
	size := int(mask) + 1
	d := int(seq-ref) & int(mask)
	if d >= size/2 {
		d -= size
	}
	return d
}

func (e *express) reset() {
	for i := range len(e.pkts) {
		e.pkts[i].used = false
	}
	for i := range len(e.keys) {
		e.keys[i] = expressKey{}
		e.voice[i] = voiceTrack{}
	}
	e.n, e.popAt, e.now, e.hasNow = 0, 0, 0, false
}

func (e *express) clock(now, window time.Duration) {
	if e.hasNow && now < e.now-window {
		d := now - e.now
		e.popAt += d
		for i := range len(e.pkts) {
			e.pkts[i].arr += d
		}
		for i := range len(e.keys) {
			k := &e.keys[i]
			k.last, k.relAt, k.dataSince = k.last+d, k.relAt+d, k.dataSince+d
			t := &e.voice[i]
			t.start, t.last = t.start+d, t.last+d
		}
	}
	if !e.hasNow || now > e.now || now < e.now-window {
		e.now, e.hasNow = now, true
	}
}

func (e *express) key(src uint32, h *layer2.Hint, now, idle time.Duration) int {
	pick := -1
	for i := range len(e.keys) {
		k := &e.keys[i]
		if k.used && k.src == src && k.hasStream == h.HasStream && (!h.HasStream || k.stream == h.Stream) {
			if now-k.last > idle && !e.holds(i) {
				k.started, k.seen, k.skips, k.voice = false, 0, false, false
			}
			k.last = now
			return i
		}
		if e.holds(i) {
			continue
		}
		if pick < 0 || !k.used || e.keys[pick].used && k.last < e.keys[pick].last {
			pick = i
		}
	}
	if pick < 0 {
		return -1
	}
	bits := h.SeqBits
	if bits == 0 || bits > 16 {
		bits = 16
	}
	e.keys[pick] = expressKey{src: src, hasStream: h.HasStream, used: true, mask: uint16(1<<bits - 1), last: now}
	if h.HasStream {
		e.keys[pick].stream = h.Stream
	}
	return pick
}

func (e *express) holds(k int) bool {
	for i := range len(e.pkts) {
		if e.pkts[i].used && e.pkts[i].key == k {
			return true
		}
	}
	return false
}

func (e *express) held(k int, seq uint16) bool {
	for i := range len(e.pkts) {
		if p := &e.pkts[i]; p.used && p.key == k && p.h.Seq&e.keys[k].mask == seq&e.keys[k].mask {
			return true
		}
	}
	return false
}

func (k *expressKey) data(now time.Duration, h *layer2.Hint, raw *layer2.RawBurst) {
	if !k.started || k.voice {
		k.dataSince = now
	}
	if !k.started {
		k.started, k.next, k.seen = true, h.Seq&k.mask, 0
		if dt, ok := dataTypeOf(h, raw); !ok || dt != enums.DataTypeDataHeader && dt != enums.DataTypeCSBK && dt != enums.DataTypeMBCHeader && dt != enums.DataTypeUnifiedSingleBlock {
			k.next = (h.Seq - 1) & k.mask
		}
	}
	k.voice = false
}

func (k *expressKey) delta(seq uint16) int {
	return expressSeqDelta(seq&k.mask, k.next, k.mask)
}

func (k *expressKey) mark(seq uint16) {
	seq &= k.mask
	if !k.started {
		k.started, k.next, k.seen = true, (seq+1)&k.mask, 1
		return
	}
	d := k.delta(seq)
	switch {
	case d >= expressHistory:
		k.next, k.seen = (seq+1)&k.mask, 1
	case d >= 0:
		k.seen = k.seen<<(d+1) | 1
		k.next = (seq + 1) & k.mask
	case -d-1 < expressHistory:
		k.seen |= 1 << (-d - 1)
	}
}

func (k *expressKey) dup(seq uint16) bool {
	if !k.started {
		return false
	}
	d := k.delta(seq)
	return d < 0 && -d-1 < expressHistory && k.seen>>(-d-1)&1 != 0
}

func (e *express) track(src uint32) *voiceTrack {
	for i := range len(e.voice) {
		if e.voice[i].used && e.voice[i].src == src {
			return &e.voice[i]
		}
	}
	return nil
}

func (s *slot) holdsVoice(src uint32) bool {
	t := s.ex.track(src)
	return t != nil && t.pending > 0 && s.jit.Len() > 0
}

func (s *slot) newTrack(src uint32) *voiceTrack {
	t := &s.ex.voice[0]
	for i := range len(s.ex.voice) {
		u := &s.ex.voice[i]
		if !u.used {
			t = u
			break
		}
		if u.pending == 0 && (t.pending > 0 || u.last < t.last) {
			t = u
		}
	}
	*t = voiceTrack{src: src, used: true}
	return t
}

func (s *slot) dropped(t *voiceTrack, n uint32) {
	t.pending -= min(n, t.pending)
	if s.jit.Len() == 0 {
		for i := range len(s.ex.voice) {
			s.ex.voice[i].pending = 0
		}
	}
}

func (r *Repeater) pushVoice(now time.Duration, s *slot, k *expressKey, raw *layer2.RawBurst, h *layer2.Hint, from Source) route {
	fresh := !s.holdsVoice(from.Peer)
	late := s.jit.Stats().Late
	res := s.jit.Push(now, raw, *h, from.Peer)
	lost := s.jit.Stats().Late - late
	if k != nil {
		k.mark(h.Seq)
		k.voice = true
	}
	if res == voice.JitterBypass {
		return routeTake
	}
	t := s.ex.track(from.Peer)
	switch res {
	case voice.JitterBuffered:
		if t == nil {
			t, fresh = s.newTrack(from.Peer), true
		}
		if fresh {
			t.start = now
		}
		end := voiceEnds(h, raw)
		t.last, t.open = now, !end
		t.pending++
		if end {
			t.pushed++
		}
	case voice.JitterLate:
		lost -= min(lost, 1)
	case voice.JitterDuplicate, voice.JitterOverflow, voice.JitterBypass:
	}
	if t != nil {
		s.dropped(t, lost)
	}
	return routeDone
}

func (s *slot) poppedVoice(now time.Duration, src uint32, end bool, lost uint32) {
	s.ex.popAt = now
	t := s.ex.track(src)
	if t == nil {
		return
	}
	if end {
		t.popped++
	}
	s.dropped(t, lost+1)
}

func dataTypeOf(h *layer2.Hint, raw *layer2.RawBurst) (enums.DataType, bool) {
	switch {
	case h.HasDataType:
		return h.DataType, true
	case raw == nil || h.Kind == layer2.KindVoice:
		return 0, false
	}
	if !dataSync(raw) {
		return 0, false
	}
	st := raw.SlotType()
	return st.DataType, !st.FEC.Uncorrectable
}

func dataSync(raw *layer2.RawBurst) bool {
	s, d := raw.Sync()
	return d <= layer2.DefaultSyncThreshold && (s == enums.SyncBSSourcedData || s == enums.SyncMSSourcedData || s == enums.SyncTDMA1Data || s == enums.SyncTDMA2Data)
}

func voiceEnds(h *layer2.Hint, raw *layer2.RawBurst) bool {
	dt, ok := dataTypeOf(h, raw)
	return h.End == layer2.EndStrong || ok && dt == enums.DataTypeTerminatorWithLC
}

func isVoice(s *slot, k *expressKey, h *layer2.Hint, raw *layer2.RawBurst, from Source) bool {
	if dt, ok := dataTypeOf(h, raw); ok {
		return dt == enums.DataTypeVoiceLCHeader || dt == enums.DataTypePIHeader || dt == enums.DataTypeTerminatorWithLC
	}
	switch {
	case h.Kind == layer2.KindVoice || raw != nil && h.Kind != layer2.KindData && !dataSync(raw):
		return true
	case s.holdsVoice(from.Peer):
		return true
	case k != nil && k.started && !k.voice:
		return false
	}
	return h.Kind != layer2.KindData && (s.owner != from || s.state != SlotData && s.state != SlotPreamble)
}

func (r *Repeater) network(now time.Duration, s *slot, raw *layer2.RawBurst, h *layer2.Hint, from Source) route {
	if s.jit == nil || !h.HasSeq {
		return routeTake
	}
	s.ex.clock(now, r.jitDelay+r.jitterLead()+time.Second)
	ki := s.ex.key(from.Peer, h, now, r.lost)
	var k *expressKey
	if ki >= 0 {
		k = &s.ex.keys[ki]
	}
	if isVoice(s, k, h, raw, from) {
		return r.pushVoice(now, s, k, raw, h, from)
	}
	if k == nil {
		return routeTake
	}
	if k.dup(h.Seq) || s.ex.held(ki, h.Seq) {
		return routeDone
	}
	k.data(now, h, raw)
	free := -1
	for i := range len(s.ex.pkts) {
		if !s.ex.pkts[i].used {
			free = i
			break
		}
	}
	if free < 0 {
		k.relAt = now
		k.mark(h.Seq)
		return routeTake
	}
	_, at, waits := s.expressNext(r)
	p := &s.ex.pkts[free]
	p.h, p.src, p.arr, p.key, p.burst, p.behind, p.need = *h, from.Peer, now, ki, raw != nil, false, 0
	if raw != nil {
		p.raw = *raw
	}
	if t := s.ex.track(from.Peer); t != nil && t.start <= k.dataSince && s.holdsVoice(from.Peer) {
		p.behind, p.need = true, t.pushed
		if t.open {
			p.need++
		}
	}
	if (!waits || at > now) && r.ready(now, s, p) {
		r.released(now, s, p)
		return routeTake
	}
	p.used = true
	s.ex.n++
	return routeDone
}

func (r *Repeater) ready(now time.Duration, s *slot, p *expressPkt) bool {
	at, ok := r.readyAt(s, p)
	return ok && at <= now && (r.mode == ModeNetwork || s.count < len(s.queue))
}

func (r *Repeater) readyAt(s *slot, p *expressPkt) (time.Duration, bool) {
	k := &s.ex.keys[p.key]
	d := k.delta(p.h.Seq)
	first := p.arr
	for i := range len(s.ex.pkts) {
		q := &s.ex.pkts[i]
		if !q.used || q == p || q.key != p.key {
			continue
		}
		if k.delta(q.h.Seq) < d {
			return 0, false
		}
		first = min(first, q.arr)
	}
	if p.behind {
		if t := s.ex.track(p.src); t != nil && t.popped < p.need && s.holdsVoice(p.src) {
			return 0, false
		}
	}
	at := max(p.arr, min(k.relAt, s.ex.now))
	if d > 0 && d < expressHistory && !k.skips {
		at = max(at, first+expressHold)
	}
	if p.behind {
		at = max(at, min(s.ex.popAt, s.ex.now))
	}
	return at, true
}

func (r *Repeater) released(now time.Duration, s *slot, p *expressPkt) {
	k := &s.ex.keys[p.key]
	k.relAt = now
	if d := k.delta(p.h.Seq); k.started && d > 0 && d < expressHistory {
		k.skips = true
	}
	k.mark(p.h.Seq)
}

func (s *slot) expressNext(r *Repeater) (*expressPkt, time.Duration, bool) {
	var best *expressPkt
	var bestAt time.Duration
	for i := range len(s.ex.pkts) {
		p := &s.ex.pkts[i]
		if !p.used {
			continue
		}
		at, ok := r.readyAt(s, p)
		if !ok {
			continue
		}
		if best == nil || at < bestAt || at == bestAt && p.arr < best.arr {
			best, bestAt = p, at
		}
	}
	return best, bestAt, best != nil
}

func (r *Repeater) expressDeadline(s *slot) (time.Duration, bool) {
	if s.ex.n == 0 || r.mode == ModeRF && s.count >= len(s.queue) {
		return 0, false
	}
	_, at, ok := s.expressNext(r)
	return at, ok
}

func (r *Repeater) expressRelease(now time.Duration, dst []Output) ([]Output, bool) {
	var s *slot
	var at time.Duration
	for i := range len(r.slots) {
		sl := &r.slots[i]
		if sl.ex.n > 0 {
			sl.ex.clock(now, r.jitDelay+r.jitterLead()+time.Second)
		}
		if t, ok := r.expressDeadline(sl); ok && t <= now && (s == nil || t < at) {
			s, at = sl, t
		}
	}
	if s == nil {
		return dst, false
	}
	p, _, _ := s.expressNext(r)
	r.released(now, s, p)
	p.used = false
	s.ex.n--
	var raw *layer2.RawBurst
	if p.burst {
		raw = &p.raw
	}
	h := p.h
	return r.take(now, raw, &h, Source{Peer: p.src}, dst, false), true
}
