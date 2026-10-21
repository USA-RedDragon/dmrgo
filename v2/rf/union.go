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
	unionHeads   = 16
	unionBlock   = 2 * slotSeconds
	unionTol     = 0.005
)

const (
	unionUnknown = iota
	unionWrong
	unionRight
)

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
	extra    []Burst
	hist     [unionHist]float64
	hn       int
	heads    [unionHeads]unionHead
	headN    int
	late     int
	maxLag   float64
	dropped  int
	filled   int
	replaced int
}

func (r *DiscReceiver) addUnion(opt *options) {
	hz, out, user := r.hz(), r.out, r.onBurst
	u := &discUnion{r: r, user: user, hz: float64(hz), limit: math.Inf(-1)}
	u.main = make([]Burst, 0, unionCap)
	u.extra = make([]Burst, 0, unionCap)
	for i := range u.hist {
		u.hist[i] = math.Inf(-1)
	}
	for i := range u.heads {
		u.heads[i].at = math.Inf(-1)
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
	if b.Seconds < u.limit-2*unionSlot {
		u.remember(b.Seconds)
		u.emit(b)
		return
	}
	if len(u.main) == cap(u.main) {
		u.release(u.main[0].Seconds + 2*unionSlot + 1e-9)
	}
	u.main = append(u.main, *b)
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
	u.heads[u.headN] = unionHead{at: b.Seconds, bf: int(p[8] & 0x7F)}
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
		if clash {
			if j >= 0 && u.status(&u.main[j]) == unionWrong && u.status(x) == unionRight {
				m := &u.main[j]
				_, c := m.Decoded()
				m.Fixed, _ = x.Decoded()
				m.FixedCACH, m.HasFixed = c, true
				u.replaced++
			}
			continue
		}
		for len(u.main) > 0 && u.main[0].Seconds < limit-2*unionSlot && u.main[0].Seconds <= x.Seconds {
			u.popMain()
		}
		u.filled++
		u.emit(x)
	}
	u.extra = u.extra[:k]
	for len(u.main) > 0 && u.main[0].Seconds < limit-2*unionSlot {
		u.popMain()
	}
}

func (u *discUnion) popMain() {
	b := u.main[0]
	copy(u.main, u.main[1:])
	u.main = u.main[:len(u.main)-1]
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
