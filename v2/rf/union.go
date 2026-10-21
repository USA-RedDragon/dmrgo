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
	unionCap     = 256
	unionHist    = 256
	unionHeads   = 128
	unionBlock   = 2 * slotSeconds
	unionTol     = 0.005
	unionLag     = 0.25
	unionVoices  = 16
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
	r, twin  *DiscReceiver
	e        *softExt
	user     func(*Burst)
	hz       float64
	n        int64
	limit    float64
	inSec    float64
	main     []Burst
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

func (r *DiscReceiver) addUnion(opt *options) {
	hz, out, user := r.hz(), r.out, r.onBurst
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
	_ = r.Init(DiscConfig{Hz: hz, OnBurst: u.onMain, opt: &t2})
	r.out = out
	r.addExt(&t2)
	o := *opt
	o.Union = false
	u.twin = &DiscReceiver{}
	_ = u.twin.Init(DiscConfig{Hz: hz, OnBurst: u.onTwin, opt: &o})
	u.twin.addExt(&o)
	u.e = r.ext.(*softExt)
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
	u.main = append(u.main, *b)
	u.fill = append(u.fill, false)
}

func (u *discUnion) onTwin(b *Burst) {
	u.noteHead(b)
	if b.Seconds < u.limit || len(u.extra) == cap(u.extra) {
		u.dropped++
		return
	}
	u.extra = append(u.extra, *b)
}

func (u *discUnion) write(x []int16) {
	u.inSec = float64(u.n+int64(len(x))) / u.hz
	u.e.union = nil
	u.r.write(x)
	u.e.union = u
	u.twin.write(x)
	u.n += int64(len(x))
	u.release(u.inSec - unionHorizon)
}

func (r *DiscReceiver) unionFlush() bool {
	e, _ := r.ext.(*softExt)
	if e == nil || e.union == nil {
		return false
	}
	u := e.union
	u.inSec = math.Inf(1)
	e.union = nil
	r.flushSolo()
	u.twin.flush()
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
	if _, r := layer2.DecodeEmbeddedLC(tw, true); r.Uncorrectable {
		return
	}
	if whole {
		if _, r := layer2.DecodeEmbeddedLC(mf, true); !r.Uncorrectable {
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
	u.main = append(u.main, Burst{})
	u.fill = append(u.fill, false)
	copy(u.main[p+1:], u.main[p:])
	copy(u.fill[p+1:], u.fill[p:])
	u.main[p], u.fill[p] = *x, true
}

func (u *discUnion) popMain() {
	b := u.main[0]
	copy(u.main, u.main[1:])
	u.main = u.main[:len(u.main)-1]
	copy(u.fill, u.fill[1:])
	u.fill = u.fill[:len(u.fill)-1]
	u.remember(b.Seconds)
	u.emit(&b)
}

func (u *discUnion) emit(b *Burst) {
	if u.user != nil {
		u.user(b)
		return
	}
	r := u.r
	if len(r.out) < cap(r.out) {
		r.out = r.out[:len(r.out)+1]
		r.out[len(r.out)-1] = *b
	}
}
