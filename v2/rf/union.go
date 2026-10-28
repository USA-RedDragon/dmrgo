//go:build !tinygo

package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	unionHorizon = 3.0
	unionSlot    = 0.015
	unionCap     = 512
	unionHist    = 256
	unionHeads   = 128
	unionBlock   = 2 * slotSeconds
	unionTol     = 0.005
	unionLag     = 0.25
	unionVoices  = 128
)

const (
	unionUnknown = iota
	unionWrong
	unionRight
)

type unionVoice struct {
	at    float64
	voice enums.VoiceBurstType
	emb   [4]byte
}

type unionHead struct {
	at float64
	bf int
}

type discUnion struct {
	r        *discRx
	twins    []*discRx
	e        *softExt
	user     func(*Burst)
	hz       float64
	n        int64
	limit    float64
	inSec    float64
	main     []Burst
	head     Burst
	fill     []bool
	extra    []Burst
	hist     [unionHist]float64
	hn       int
	heads    [unionHeads]unionHead
	headN    int
	voices   [unionVoices]unionVoice
	voiceN   int
	late     int
	maxLag   float64
	dropped  int
	filled   int
	replaced int
	elc      int
}

func (r *discRx) addUnion(opt *options) {
	hz, user, dec, pol := r.hz(), r.onBurst, r.f.dec, r.f.polBits&polOn != 0
	u := &discUnion{r: r, user: user, hz: float64(hz), limit: math.Inf(-1)}
	u.main = make([]Burst, 0, unionCap)
	u.fill = make([]bool, 0, unionCap)
	u.extra = make([]Burst, 0, unionCap)
	for i := range u.hist {
		u.hist[i] = math.Inf(-1)
	}
	for i := range u.heads {
		u.heads[i].at = math.Inf(-1)
	}
	for i := range u.voices {
		u.voices[i].at = math.Inf(-1)
	}
	t2 := discTierOptions(Tier2)
	if opt.UnionTwins&0x80 != 0 {
		t2.CohBudget = 0
	}
	r.setup(discSetup{Hz: hz, OnBurst: u.onMain, opt: &t2, pol: pol}, dec)
	r.addExt(&t2)
	set := opt.UnionTwins &^ 0x80
	if set == 0 {
		set = 1
	}
	for k := range 7 {
		if set&(1<<k) == 0 {
			continue
		}
		o := *opt
		switch k {
		case 1:
			o = tier2Options(discOptions())
			o.CohDataPasses = 4
		case 2:
			o = discTierOptions(Tier2)
			o.CohBudget, o.CohDataPasses = 0, 4
		case 3:
			o = discTierOptions(Tier2)
		}
		o.UnionTwins = 0
		w := &discRx{}
		w.setup(discSetup{Hz: hz, OnBurst: u.onTwin, opt: &o, pol: pol}, false)
		w.addExt(&o)
		u.twins = append(u.twins, w)
	}
	e, ok := r.ext.(*softExt)
	if !ok {
		return
	}
	u.e = e
	u.e.union = u
}

func (u *discUnion) onMain(b *Burst) {
	if !math.IsInf(u.inSec, 1) {
		u.maxLag = max(u.maxLag, u.inSec-b.Seconds)
	}
	u.noteHead(b)
	if b.Seconds < u.limit+unionSlot {
		u.late++
	}
	if b.Seconds < u.limit-unionLag {
		u.remember(b.Seconds)
		u.emit(b)
		return
	}
	if len(u.main) == cap(u.main) {
		u.release(u.main[0].Seconds + unionLag + 1e-9)
	}
	if len(u.main) == cap(u.main) {
		u.popMain()
	}
	n := len(u.main)
	u.main, u.fill = u.main[:n+1], u.fill[:n+1]
	u.main[n], u.fill[n] = *b, false
}

func (u *discUnion) onTwin(b *Burst) {
	u.noteHead(b)
	if b.Seconds < u.limit || len(u.extra) == cap(u.extra) {
		u.dropped++
		return
	}
	n := len(u.extra)
	u.extra = u.extra[:n+1]
	u.extra[n] = *b
}

func (u *discUnion) write(x []int16) {
	u.inSec = float64(u.n+int64(len(x))) / u.hz
	u.e.union = nil
	u.r.writeCore(x)
	u.e.union = u
	for _, w := range u.twins {
		w.write(x)
	}
	u.n += int64(len(x))
	u.release(u.inSec - unionHorizon)
}

func (r *discRx) unionBacklog() int {
	e, _ := r.ext.(*softExt)
	if e == nil || e.union == nil {
		return 0
	}
	return cap(e.union.main) + cap(e.union.extra)
}

func (r *discRx) unionLag() float64 {
	if e, _ := r.ext.(*softExt); e == nil || e.union == nil {
		return 0
	}
	return unionHorizon + (extQueue+4)*slotSeconds
}

func (r *discRx) unionFlush() bool {
	e, _ := r.ext.(*softExt)
	if e == nil || e.union == nil {
		return false
	}
	u := e.union
	u.inSec = math.Inf(1)
	e.union = nil
	r.flushSolo()
	for _, w := range u.twins {
		w.flush()
	}
	e.union = u
	u.release(math.Inf(1))
	return true
}

func unionData(b *Burst) (layer2.RawBurst, pdu.SlotType, bool) {
	_, kind, ok := layer2.SyncSourceOf(b.Sync)
	if !ok || kind != layer2.KindData || b.Voice != enums.VoiceBurstUnknown {
		return layer2.RawBurst{}, pdu.SlotType{}, false
	}
	d, _ := b.Decoded()
	st := d.SlotType()
	return d, st, !st.FEC.Uncorrectable
}

func (u *discUnion) noteHead(b *Burst) {
	d, st, ok := unionData(b)
	if !ok || st.DataType != enums.DataTypeDataHeader {
		return
	}
	p, _ := d.BPTCPayload()
	h := pdu.DataHeaderBytes(p)
	if !h.CRCOK() || h.Format() != pdu.FormatConfirmed {
		return
	}
	bf := int(p[8] & 0x7F)
	for _, x := range u.heads {
		if x.bf == bf && math.Abs(x.at-b.Seconds) < unionSlot {
			return
		}
	}
	u.heads[u.headN] = unionHead{at: b.Seconds, bf: bf}
	u.headN = (u.headN + 1) % unionHeads
}

func (u *discUnion) confirmed(s float64) bool {
	for _, h := range u.heads {
		k := (s - h.at) / unionBlock
		n := math.Round(k)
		if n >= 1 && int(n) <= h.bf && math.Abs(k-n)*unionBlock < unionTol {
			return true
		}
	}
	return false
}

func (u *discUnion) status(b *Burst) int {
	d, st, ok := unionData(b)
	if !ok {
		return unionUnknown
	}
	var pass bool
	switch dt := st.DataType; dt {
	case enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1:
		if !u.confirmed(b.Seconds) {
			return unionUnknown
		}
		switch dt {
		case enums.DataTypeRate12:
			p, _ := d.BPTCPayload()
			pass = (&pdu.Rate12Data{Data: p}).ConfirmedCRC9OK()
		case enums.DataTypeRate34:
			p, _ := d.Rate34Payload()
			pass = (&pdu.Rate34Data{Data: p}).ConfirmedCRC9OK()
		default:
			p, _ := d.Rate1Payload()
			pass = (&pdu.Rate1Data{Data: p}).ConfirmedCRC9OK()
		}
	default:
		check := u.e.checks[dt&0xF]
		if check == nil {
			return unionUnknown
		}
		p, _ := d.BPTCPayload()
		pass = check(p)
	}
	if pass {
		return unionRight
	}
	return unionWrong
}

func (u *discUnion) remember(s float64) {
	u.hist[u.hn] = s
	u.hn = (u.hn + 1) % unionHist
}

func (u *discUnion) near(s float64) (int, bool) {
	for _, h := range u.hist {
		if math.Abs(h-s) < unionSlot {
			return -1, true
		}
	}
	at := -1
	for i := range u.main {
		if math.Abs(u.main[i].Seconds-s) < unionSlot {
			if at >= 0 {
				return -1, true
			}
			at = i
		}
	}
	return at, at >= 0
}

func (u *discUnion) release(limit float64) {
	u.limit = max(u.limit, limit)
	k := 0
	for i := range u.extra {
		x := &u.extra[i]
		if x.Seconds >= limit {
			u.extra[k] = *x
			k++
			continue
		}
		j, clash := u.near(x.Seconds)
		u.noteVoice(x)
		if clash {
			if j >= 0 && !u.fill[j] && u.status(&u.main[j]) == unionWrong && u.status(x) == unionRight {
				m := &u.main[j]
				_, c := m.Decoded()
				m.Fixed, _ = x.Decoded()
				m.FixedCACH, m.HasFixed = c, true
				u.replaced++
			}
			u.embedded(x)
			continue
		}
		u.embedded(x)
		u.filled++
		u.insert(x)
	}
	u.extra = u.extra[:k]
	for len(u.main) > 0 && u.main[0].Seconds < limit-unionLag {
		u.popMain()
	}
}

func (u *discUnion) noteVoice(x *Burst) {
	if x.Voice < enums.VoiceBurstB || x.Voice > enums.VoiceBurstE {
		return
	}
	d, _ := x.Decoded()
	u.voices[u.voiceN] = unionVoice{at: x.Seconds, voice: x.Voice, emb: d.EmbeddedData()}
	u.voiceN = (u.voiceN + 1) % unionVoices
}

func (u *discUnion) twinFrag(at float64, v enums.VoiceBurstType) ([4]byte, bool) {
	for _, x := range u.voices {
		if x.voice == v && math.Abs(x.at-at) < unionTol {
			return x.emb, true
		}
	}
	return [4]byte{}, false
}

func (u *discUnion) embedded(x *Burst) {
	if x.Voice != enums.VoiceBurstE {
		return
	}
	var tw, mf [4][4]byte
	var at [4]int
	whole := true
	for k := range tw {
		s := x.Seconds - float64(3-k)*unionBlock
		f, ok := u.twinFrag(s, enums.VoiceBurstB+enums.VoiceBurstType(k))
		if !ok {
			return
		}
		tw[k] = f
		j, near := u.near(s)
		if near && j < 0 {
			return
		}
		if j >= 0 && u.fill[j] {
			j = -1
		}
		at[k] = j
		if j < 0 {
			whole = false
			continue
		}
		if u.main[j].Voice != enums.VoiceBurstB+enums.VoiceBurstType(k) {
			return
		}
		d, _ := u.main[j].Decoded()
		mf[k] = d.EmbeddedData()
	}
	if _, ok := strictEmbeddedLC(tw); !ok {
		return
	}
	if whole {
		if _, ok := strictEmbeddedLC(mf); ok {
			return
		}
	}
	for k, j := range at {
		if j < 0 {
			continue
		}
		m := &u.main[j]
		if !m.HasFixed {
			m.Fixed, m.FixedCACH, m.HasFixed = m.Data, m.CACH, true
		}
		m.Fixed.SetEmbeddedData(tw[k])
	}
	u.elc++
}

func (u *discUnion) insert(x *Burst) {
	if len(u.main) == cap(u.main) {
		u.popMain()
	}
	p := len(u.main)
	for i := range u.main {
		if u.main[i].Seconds > x.Seconds {
			p = i
			break
		}
	}
	n := len(u.main)
	u.main, u.fill = u.main[:n+1], u.fill[:n+1]
	copy(u.main[p+1:], u.main[p:])
	copy(u.fill[p+1:], u.fill[p:])
	u.main[p], u.fill[p] = *x, true
}

func (u *discUnion) popMain() {
	u.head = u.main[0]
	copy(u.main, u.main[1:])
	u.main = u.main[:len(u.main)-1]
	copy(u.fill, u.fill[1:])
	u.fill = u.fill[:len(u.fill)-1]
	u.remember(u.head.Seconds)
	u.emit(&u.head)
}

func (u *discUnion) emit(b *Burst) {
	if u.user != nil {
		u.user(b)
		return
	}
	u.r.sink.put(b)
}
