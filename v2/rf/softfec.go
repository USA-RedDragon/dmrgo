package rf

import (
	"math"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/ambe"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

const (
	softBurstBits     = 264
	softSmallMaxN     = 24
	softSmallMaxK     = 12
	softOSDMaxN       = 196
	softOSDMaxK       = 99
	softOSDWords      = 4
	softOSDNibbles    = 49
	softTabLen        = max(3*256, softOSDNibbles*16)
	softListMax       = 8
	softChaseMax      = 16
	softBPTCData      = 96
	softBPTCInfo      = 99
	softTrellisBits   = 196
	softTrellisSteps  = 49
	softTrellisStates = 8
	softAMBEC0        = 12
	softAMBEC2        = 25
	softShortLCInfo   = 36
	softShortLCData   = 28
	softEmbLCInfo     = 77
	softEmbLCBits     = 72
	softEmbLCChecksum = 5
	softInf           = math.MaxInt32 / 4
)

type softSmallID uint8

const (
	softSlotTypeCode softSmallID = iota
	softEMBCode
	softTACTCode
	softAMBECode
	softSmallCount = softAMBECode + 2*layer2.VoiceSlotsPerBurst
)

type softOSDID uint8

const (
	softBPTCCode softOSDID = iota
	softShortLCCode
	softEmbLCCode
	softOSDCount
)

type softTop struct {
	n, max int
	cost   [softListMax]int32
	info   [softListMax]uint32
	cw     [softListMax]uint32
}

func (t *softTop) reset(size int) {
	t.n, t.max = 0, max(1, min(size, softListMax))
}

func (t *softTop) worst() int32 {
	if t.n < t.max {
		return math.MaxInt32
	}
	return t.cost[t.n-1]
}

func (t *softTop) add(cost int32, info, cw uint32) {
	if t.n == t.max && cost >= t.cost[t.n-1] {
		return
	}
	i := min(t.n, t.max-1)
	for i > 0 && t.cost[i-1] > cost {
		t.cost[i], t.info[i], t.cw[i] = t.cost[i-1], t.info[i-1], t.cw[i-1]
		i--
	}
	t.cost[i], t.info[i], t.cw[i] = cost, info, cw
	t.n = min(t.n+1, t.max)
}

type softSmallCode struct {
	n, k int
	t    int
	pos  [softSmallMaxN]int16
	base uint32
	rows [softSmallMaxK]uint32
}

func softTables(tab *[softTabLen]int32, l []int32) {
	at := func(j int) int32 {
		if j < len(l) {
			return l[j]
		}
		return 0
	}
	for b := 0; b*8 < len(l); b++ {
		var lo, hi [16]int32
		for m := 1; m < 16; m++ {
			z := bits.TrailingZeros(uint(m))
			lo[m] = lo[m&(m-1)] + at(b*8+z)
			hi[m] = hi[m&(m-1)] + at(b*8+4+z)
		}
		t := (*[256]int32)(tab[256*b:])
		for m := range t {
			t[m] = lo[m&15] + hi[m>>4]
		}
	}
}

func softNibbleTables(tab *[softTabLen]int32, l []int32) {
	for b := 0; b*4 < len(l); b++ {
		t := (*[16]int32)(tab[16*b:])
		t[0] = 0
		for m := 1; m < 16; m++ {
			if j := b*4 + bits.TrailingZeros(uint(m)); j < len(l) {
				t[m] = t[m&(m-1)] + l[j]
			} else {
				t[m] = t[m&(m-1)]
			}
		}
	}
}

func softByteTables(tab *[softParWords * 8 * 256]int32, l []int32) {
	for b := 0; b*8 < len(l); b++ {
		t := (*[256]int32)(tab[256*b:])
		t[0] = 0
		for m := 1; m < 256; m++ {
			if j := b*8 + bits.TrailingZeros(uint(m)); j < len(l) {
				t[m] = t[m&(m-1)] + l[j]
			} else {
				t[m] = t[m&(m-1)]
			}
		}
	}
}

func (id softSmallID) gather(llr []int16, l *[softSmallMaxN]int32) uint32 {
	c := smallCode(id)
	var hard uint32
	for j := range c.n {
		v := llr[c.pos[j]]
		l[j] = int32(v)
		if v < 0 {
			hard |= 1 << j
		}
	}
	return hard
}

func (id softSmallID) search(tab *[softTabLen]int32, off, fixed, want uint32, top *softTop) {
	c := smallCode(id)
	var free [16]int
	nf := 0
	info := want & fixed
	cw := c.base ^ off
	for i := range c.k {
		if fixed>>i&1 == 0 {
			free[nf] = i
			nf++
		} else if info>>i&1 != 0 {
			cw ^= c.rows[i]
		}
	}
	top.add(tab[cw&0xff]+tab[256+cw>>8&0xff]+tab[512+cw>>16&0xff], info, cw)
	worst := top.worst()
	for g := uint32(1); g < uint32(1)<<nf; g++ {
		i := free[bits.TrailingZeros32(g)&15]
		info ^= 1 << i
		cw ^= c.rows[i]
		if cost := tab[cw&0xff] + tab[256+cw>>8&0xff] + tab[512+cw>>16&0xff]; cost < worst {
			top.add(cost, info, cw)
			worst = top.worst()
		}
	}
}

type softSmallScratch struct {
	l         [softSmallMaxN]int32
	tab       *[softTabLen]int32
	top, top1 softTop
}

func (s *softSmallScratch) decode(c softSmallID, llr []int16, fixed, want uint32) (info, cw uint32, errs int) {
	hard := c.gather(llr, &s.l)
	softTables(s.tab, s.l[:smallCode(c).n])
	s.top.reset(1)
	c.search(s.tab, 0, fixed, want, &s.top)
	return s.top.info[0], s.top.cw[0], bits.OnesCount32(s.top.cw[0] ^ hard)
}

func softResult(n, errs, t int) fec.Result {
	return fec.Result{BitsChecked: n, ErrorsCorrected: errs, Uncorrectable: errs > t}
}

func softPut[T int | int16](r *layer2.RawBurst, p T, one bool) {
	m := byte(0x80) >> (p % 8)
	if one {
		r[p/8] |= m
	} else {
		r[p/8] &^= m
	}
}

func smallCode(id softSmallID) *softSmallCode {
	if id >= softAMBECode {
		return &softAMBESmallCodes[id-softAMBECode]
	}
	return &softSmallCodes[id]
}

func softPutSmall(r *layer2.RawBurst, id softSmallID, cw uint32) {
	c := smallCode(id)
	for j := range c.n {
		softPut(r, c.pos[j], cw>>j&1 != 0)
	}
}

func softAMBEPN(v uint32) uint32 {
	var f [7]byte
	f[0], f[1] = byte(v>>(softAMBEC0-8)&0xff), byte(v<<(16-softAMBEC0)&0xff)
	slot := ambe.Encode(f)
	c := smallCode(softAMBECode + 1)
	var m uint32
	for j, p := range c.pos[:c.n] {
		m |= uint32(slot[p/8]>>(7-p%8)&1) << j
	}
	return m
}

type softOSDCode struct {
	n, k int
	t    int
	pos  [softOSDMaxN]int16
	rows [softOSDMaxK][softOSDWords]uint64
	sys  [softOSDMaxK]int16
}

type softOSDList struct {
	n, max int
	cost   [softListMax]int32
	cw     [softListMax][softOSDWords]uint64
	scored int
}

func (t *softOSDList) reset(size int) {
	t.n, t.max, t.scored = 0, max(1, min(size, softListMax)), 0
}

func (t *softOSDList) add(cost int32, cw *[softOSDWords]uint64) bool {
	t.scored++
	if t.n == t.max && cost >= t.cost[t.n-1] {
		return false
	}
	for i := range t.n {
		if t.cost[i] == cost && t.cw[i] == *cw {
			return false
		}
	}
	i := min(t.n, t.max-1)
	for i > 0 && t.cost[i-1] > cost {
		t.cost[i], t.cw[i] = t.cost[i-1], t.cw[i-1]
		i--
	}
	t.cost[i], t.cw[i] = cost, *cw
	t.n = min(t.n+1, t.max)
	return true
}

type softOSDScratch struct {
	l, mag      [softOSDMaxN]int32
	order       [softOSDMaxN]int16
	hard        [softOSDWords]uint64
	work        [softOSDMaxK][softOSDWords]uint64
	pivot       [softOSDMaxK]int16
	tab         *[softTabLen]int32
	list        softOSDList
	accept      func(cw *[softOSDWords]uint64) bool
	cand        [2][softOSDWords]uint64
	pmag        [softOSDMaxK]int32
	pass, floor int32
	nw          int
	chase       [softChaseMax]int16
	bits        [softTrellisBits]unpacked.Bit
	btab        *[softParWords * 8 * 256]int32
	par         [softOSDMaxN]int16
	pm          [softOSDMaxN]int32
	prow        [softOSDMaxK][softParWords]uint64
	phard       [softParWords]uint64
	pcand       [4][softParWords]uint64
}

const softParWords = 2

func (s *softOSDScratch) first() (int, bool) {
	for i := 0; s.accept != nil && i < s.list.n; i++ {
		if s.accept(&s.list.cw[i]) {
			return i, true
		}
	}
	return 0, false
}

func (s *softOSDScratch) prepare(n int) {
	s.hard = [softOSDWords]uint64{}
	for j := range n {
		v := s.l[j]
		if v < 0 {
			s.hard[j>>6] |= 1 << (j & 63)
			v = -v
		}
		s.mag[j] = v
	}
}

func (id softOSDID) osd(s *softOSDScratch, order, window, listMax int) {
	n := softOSDCodes[id].n
	s.prepare(n)
	id.basis(s)
	id.reprocess(s, order, window, listMax)
}

func (id softOSDID) reprocess(s *softOSDScratch, order, window, listMax int) {
	c := &softOSDCodes[id]
	k, np := c.k, c.n-c.k
	s.nw = (c.n + 63) / 64
	var basis [softOSDWords]uint64
	for i := range k {
		j := s.pivot[i]
		basis[j>>6] |= 1 << (j & 63)
	}
	p := 0
	for _, j := range s.order[:c.n] {
		if basis[j>>6]>>(j&63)&1 == 0 {
			s.par[p], s.pm[p] = j, s.mag[j]
			p++
		}
	}
	if s.btab != nil {
		softByteTables(s.btab, s.pm[:np])
	} else {
		softNibbleTables(s.tab, s.pm[:np])
	}
	s.phard = [softParWords]uint64{}
	for q, j := range s.par[:np] {
		if softBit(&s.hard, j) {
			s.phard[q>>6] |= 1 << (q & 63)
		}
	}
	for i := range k {
		w := &s.work[i]
		var r [softParWords]uint64
		for q, j := range s.par[:np] {
			r[q>>6] |= (w[j>>6] >> (j & 63) & 1) << (q & 63)
		}
		s.prow[i] = r
	}
	c0 := &s.cand[0]
	*c0 = [softOSDWords]uint64{}
	p0 := &s.pcand[0]
	*p0 = [softParWords]uint64{}
	for i := range k {
		if j := s.pivot[i]; s.hard[j>>6]>>(j&63)&1 != 0 {
			s.flip(c0, c0, i)
			p0[0] ^= s.prow[i][0]
			p0[1] ^= s.prow[i][1]
		}
	}
	s.list.reset(listMax)
	s.pass, s.floor = math.MaxInt32, 0
	for j := range c.n {
		s.floor += min(0, s.l[j])
	}
	s.try(p0, 0, -1, -1, -1)
	m := &s.pmag
	for i := range k {
		m[i] = s.mag[s.pivot[i]]
	}
	pi, pj, px := &s.pcand[1], &s.pcand[2], &s.pcand[3]
	for i := k - 1; order >= 1 && i >= 0 && m[i] < s.thr(); i-- {
		s.pflip(pi, p0, i)
		s.try(pi, m[i], i, -1, -1)
	}
	for j := k - 1; order >= 2 && j >= 1 && m[j]+m[j-1] < s.thr(); j-- {
		s.pflip(pj, p0, j)
		for i := j - 1; i >= 0 && m[i]+m[j] < s.thr(); i-- {
			s.pflip(pi, pj, i)
			s.try(pi, m[i]+m[j], j, i, -1)
		}
	}
	lo := max(0, k-window)
	for a := k - 1; order >= 3 && a >= lo+2 && m[a]+m[a-1]+m[a-2] < s.thr(); a-- {
		s.pflip(pj, p0, a)
		for b := a - 1; b >= lo+1 && m[a]+m[b]+m[b-1] < s.thr(); b-- {
			s.pflip(pi, pj, b)
			for x := b - 1; x >= lo && m[a]+m[b]+m[x] < s.thr(); x-- {
				s.pflip(px, pi, x)
				s.try(px, m[a]+m[b]+m[x], a, b, x)
			}
		}
	}
}

func (s *softOSDScratch) pflip(dst, src *[softParWords]uint64, i int) {
	dst[0], dst[1] = src[0]^s.prow[i][0], src[1]^s.prow[i][1]
}

func (s *softOSDScratch) try(pc *[softParWords]uint64, flips int32, a, b, x int) {
	bound := int32(math.MaxInt32)
	if s.list.n == s.list.max {
		bound = s.list.cost[s.list.n-1]
	}
	sum := s.floor + flips
	if bt := s.btab; bt != nil {
		for w := 0; w < softParWords && sum < bound; w++ {
			t := bt[2048*w:]
			for i, e := 0, pc[w]^s.phard[w]; e != 0; i, e = i+256, e>>8 {
				sum += t[i+int(e&0xff)]
			}
		}
	}
	for w := 0; s.btab == nil && w < softParWords && sum < bound; w++ {
		t := s.tab[256*w:]
		for i, e := 0, pc[w]^s.phard[w]; e != 0; i, e = i+16, e>>4 {
			sum += t[i+int(e&0xf)]
		}
	}
	if s.list.n == s.list.max && sum >= bound {
		s.list.scored++
		return
	}
	cw := &s.cand[1]
	*cw = s.cand[0]
	for _, r := range [3]int{a, b, x} {
		if r >= 0 {
			for y := range s.nw {
				cw[y] ^= s.work[r][y]
			}
		}
	}
	if s.list.add(sum, cw) && s.accept != nil && sum < s.pass && s.accept(cw) {
		s.pass = sum
	}
}

func (id softOSDID) basis(s *softOSDScratch) {
	c := &softOSDCodes[id]
	n, k, nw := c.n, c.k, (c.n+63)/64
	for j := range n {
		s.order[j] = int16(j)
		for i := j; i > 0 && s.mag[s.order[i-1]] < s.mag[s.order[i]]; i-- {
			s.order[i-1], s.order[i] = s.order[i], s.order[i-1]
		}
	}
	copy(s.work[:k], c.rows[:k])
	rank := 0
	for _, j := range s.order[:n] {
		if rank == k {
			break
		}
		w, b := j>>6, uint64(1)<<(j&63)
		r := rank
		for r < k && s.work[r][w]&b == 0 {
			r++
		}
		if r == k {
			continue
		}
		s.work[r], s.work[rank] = s.work[rank], s.work[r]
		for i := range k {
			if i != rank && s.work[i][w]&b != 0 {
				for x := range nw {
					s.work[i][x] ^= s.work[rank][x]
				}
			}
		}
		s.pivot[rank] = j
		rank++
	}
}

func (s *softOSDScratch) flip(dst, src *[softOSDWords]uint64, i int) {
	for x := range s.nw {
		dst[x] = src[x] ^ s.work[i][x]
	}
}

func (s *softOSDScratch) thr() int32 {
	t := s.pass
	if s.list.n == s.list.max {
		t = min(t, s.list.cost[s.list.n-1])
	}
	if t == math.MaxInt32 {
		return t
	}
	return t - s.floor
}

func (s *softOSDScratch) errors(cw *[softOSDWords]uint64) int {
	e := 0
	for x := range cw {
		e += bits.OnesCount64(cw[x] ^ s.hard[x])
	}
	return e
}

func softBit[T int | int16](cw *[softOSDWords]uint64, j T) bool {
	return cw[j>>6]>>(j&63)&1 != 0
}

type softBPTCResult struct {
	Payload  [12]byte
	Reserved uint8
}

func softBPTCOf(cw *[softOSDWords]uint64) softBPTCResult {
	c := &softOSDCodes[softBPTCCode]
	var r softBPTCResult
	for i := range softBPTCData {
		if softBit(cw, c.sys[i]) {
			r.Payload[i/8] |= 0x80 >> (i % 8)
		}
	}
	for i := range 3 {
		if softBit(cw, c.sys[softBPTCData+i]) {
			r.Reserved |= 1 << i
		}
	}
	return r
}

type softEmbLC struct {
	lc [softEmbLCBits]int16
	cs [softEmbLCChecksum]int16
}

type softTrellis struct {
	pos   [softTrellisBits]int16
	step  [softTrellisSteps][4]uint8
	label [softTrellisStates][softTrellisStates]uint8
}

type softDecoder struct {
	AMBECandidates                   int
	ChaseBits                        int
	OSDOrder, OSDWindow              int
	GridIters                        int
	GridWeight                       float32
	BPTCList, ShortLCList, EmbLCList int

	grid                      softGrid
	small                     softSmallScratch
	osd                       softOSDScratch
	tab                       [softTabLen]int32
	prev                      [softTrellisSteps][softTrellisStates]uint8
	gridPost                  [softBurstBits]int16
	check                     func([12]byte) bool
	acceptBPTC, acceptShortLC func(cw *[softOSDWords]uint64) bool
}

func (d *softDecoder) setBytes(opt *options) {
	if opt.OSDBytes {
		d.osd.btab = new([softParWords * 8 * 256]int32)
	}
}

func newSoftDecoder() *softDecoder {
	d := &softDecoder{AMBECandidates: 4, ChaseBits: 6, OSDOrder: 2, BPTCList: softListMax, ShortLCList: 4, EmbLCList: 2, GridIters: 4, GridWeight: 0.7}
	d.small.tab, d.osd.tab = &d.tab, &d.tab
	d.acceptBPTC = func(cw *[softOSDWords]uint64) bool {
		return d.check(softBPTCOf(cw).Payload)
	}
	d.acceptShortLC = func(cw *[softOSDWords]uint64) bool {
		info := softShortLCOf(cw)
		var data [(softShortLCData + 7) / 8]byte
		for i, v := range info[:softShortLCData] {
			data[i/8] |= byte(v&1) << (7 - i%8)
		}
		return crc.CheckCRC8(data[:], softShortLCData, uint8(unpacked.Uint(info[softShortLCData:])&0xff))
	}
	return d
}

func softTACT(info uint32) pdu.TACT {
	return pdu.TACT{AccessType: info&8 != 0, TDMAChannel: info&4 != 0, LCSS: enums.LCSS(info & 3)}
}

func (d *softDecoder) decodeAMBE(llr *[softBurstBits]int16, out *layer2.RawBurst) ([layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame, fec.Result) {
	var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	var res fec.Result
	s := &d.small
	var hb layer2.RawBurst
	for p, v := range llr {
		if v < 0 {
			hb[p/8] |= 0x80 >> (p % 8)
		}
	}
	slots := hb.VoiceSlots()
	for i := range frames {
		id0, id1 := softAMBECode+2*softSmallID(i), softAMBECode+2*softSmallID(i)+1
		c0, c1 := smallCode(id0), smallCode(id1)
		hard0 := id0.gather(llr[:], &s.l)
		if f, r := vocoder.DecodeAMBE72(slots[i]); r.ErrorsCorrected == 0 && !r.Uncorrectable {
			frames[i] = f
			res = res.Combine(fec.Result{BitsChecked: c0.n + c1.n})
			if out != nil {
				softPutSmall(out, id0, hard0)
				softPutSmall(out, id1, id1.gather(llr[:], &s.l))
				for _, p := range &softAMBEC2Pos[i] {
					softPut(out, p, llr[p] < 0)
				}
			}
			continue
		}
		softTables(s.tab, s.l[:c0.n])
		s.top.reset(d.AMBECandidates)
		id0.search(s.tab, 0, 0, 0, &s.top)
		hard1 := id1.gather(llr[:], &s.l)
		softTables(s.tab, s.l[:c1.n])
		var bound int32
		for j := range c1.n {
			bound += min(0, s.l[j])
		}
		best := int32(softInf)
		var info0, info1, cw0, cw1 uint32
		for c := range s.top.n {
			if s.top.cost[c]+bound >= best {
				break
			}
			s.top1.reset(1)
			id1.search(s.tab, softAMBEPN(s.top.info[c]), 0, 0, &s.top1)
			if total := s.top.cost[c] + s.top1.cost[0]; total < best {
				best = total
				info0, cw0 = s.top.info[c], s.top.cw[c]
				info1, cw1 = s.top1.info[0], s.top1.cw[0]
			}
		}
		v := uint64(info0)<<(softAMBEC0+softAMBEC2) | uint64(info1)<<softAMBEC2
		c2 := &softAMBEC2Pos[i]
		for b, p := range c2 {
			if llr[p] < 0 {
				v |= 1 << (softAMBEC2 - 1 - b)
			}
		}
		for k := range frames[i] {
			frames[i][k] = byte(v << 7 >> (48 - 8*k) & 0xFF)
		}
		e0, e1 := bits.OnesCount32(cw0^hard0), bits.OnesCount32(cw1^hard1)
		res = res.Combine(fec.Result{BitsChecked: c0.n + c1.n, ErrorsCorrected: e0 + e1, Uncorrectable: e0 > c0.t || e1 > c1.t})
		if out != nil {
			softPutSmall(out, id0, cw0)
			softPutSmall(out, id1, cw1)
			for _, p := range c2 {
				softPut(out, p, llr[p] < 0)
			}
		}
	}
	return frames, res
}

func (d *softDecoder) decodeSlotType(llr *[softBurstBits]int16, cc enums.ColorCode, ccKnown bool, out *layer2.RawBurst) (pdu.SlotTypeInfo, fec.Result) {
	var fixed, want uint32
	if ccKnown {
		fixed, want = 0xf0, uint32(cc&0x0f)<<4
	}
	info, cw, errs := d.small.decode(softSlotTypeCode, llr[:], fixed, want)
	if out != nil {
		softPutSmall(out, softSlotTypeCode, cw)
	}
	c := &softSmallCodes[softSlotTypeCode]
	return pdu.SlotTypeInfo(info & 0xff), softResult(c.n, errs, c.t)
}

func (d *softDecoder) decodeEMB(llr *[softBurstBits]int16, cc enums.ColorCode, ccKnown bool, out *layer2.RawBurst) (pdu.EMBInfo, fec.Result) {
	var fixed, want uint32
	if ccKnown {
		fixed, want = 0x78, uint32(cc&0x0f)<<3
	}
	info, cw, errs := d.small.decode(softEMBCode, llr[:], fixed, want)
	if out != nil {
		softPutSmall(out, softEMBCode, cw)
	}
	c := &softSmallCodes[softEMBCode]
	return pdu.EMBInfo(info << 1 & 0xfe), softResult(c.n, errs, c.t)
}

func (d *softDecoder) decodeTACT(llr *[layer2.CACHBits]int16, out *[layer2.CACHBits]unpacked.Bit) (pdu.TACT, fec.Result) {
	info, cw, errs := d.small.decode(softTACTCode, llr[:], 0, 0)
	c := &softSmallCodes[softTACTCode]
	if out != nil {
		for j := range c.n {
			out[c.pos[j]] = unpacked.Bit(cw >> j & 1)
		}
	}
	t := softTACT(info)
	t.FEC = softResult(c.n, errs, c.t)
	return t, t.FEC
}

func softRate34Payload(t *[softTrellisSteps]int) [18]byte {
	var p [18]byte
	for k := range softTrellisSteps - 1 {
		for b := range 3 {
			if t[k]>>(2-b)&1 != 0 {
				i := 3*k + b
				p[i/8] |= 0x80 >> (i % 8)
			}
		}
	}
	return p
}

func (d *softDecoder) decodeRate34(llr *[softBurstBits]int16, out *layer2.RawBurst) ([18]byte, fec.Result) {
	tr := &softTrellisTables
	l := &d.osd.l
	for j, p := range &tr.pos {
		l[j] = int32(llr[p])
	}
	metric := [softTrellisStates]int32{0, softInf, softInf, softInf, softInf, softInf, softInf, softInf}
	var next [softTrellisStates]int32
	for k := range softTrellisSteps {
		var bc [16]int32
		sp := &tr.step[k]
		for lab := 1; lab < 16; lab++ {
			bc[lab] = bc[lab&(lab-1)] + l[sp[3-bits.TrailingZeros(uint(lab))]]
		}
		for in := range softTrellisStates {
			best, arg := int32(softInf), 0
			for s, m := range metric {
				if c := m + bc[tr.label[s][in]]; c < best {
					best, arg = c, s
				}
			}
			next[in] = best
			d.prev[k][in] = uint8(arg)
		}
		metric = next
	}
	res := fec.Result{BitsChecked: softTrellisBits}
	for s := 1; s < softTrellisStates; s++ {
		if metric[s] < metric[0] {
			res.Uncorrectable = true
		}
	}
	var t [softTrellisSteps]int
	state := 0
	for k := softTrellisSteps - 1; k >= 0; k-- {
		t[k] = state
		state = int(d.prev[k][state])
	}
	state = 0
	for k := range softTrellisSteps {
		lab := tr.label[state][t[k]]
		state = t[k]
		for b, j := range &tr.step[k] {
			one := lab>>(3-b)&1 != 0
			if one != (l[j] < 0) {
				res.ErrorsCorrected++
			}
			if out != nil {
				softPut(out, tr.pos[j], one)
			}
		}
	}
	return softRate34Payload(&t), res
}

func (d *softDecoder) bptcLLRs(llr *[softBurstBits]int16) {
	c := &softOSDCodes[softBPTCCode]
	for j := range c.n {
		d.osd.l[j] = int32(llr[c.pos[j]])
	}
}

func (d *softDecoder) bptcPick(check func([12]byte) bool, out *layer2.RawBurst) (softBPTCResult, fec.Result) {
	s, c := &d.osd, &softOSDCodes[softBPTCCode]
	pick, ok := s.first()
	cw := &s.list.cw[pick]
	errs := s.errors(cw)
	res := softResult(c.n, errs, c.t)
	if check != nil {
		res.Uncorrectable = !ok
	}
	if out != nil {
		for j := range c.n {
			softPut(out, c.pos[j], softBit(cw, j))
		}
	}
	return softBPTCOf(cw), res
}

func (d *softDecoder) bptcCheck(check func([12]byte) bool) int {
	d.check = check
	if check == nil {
		d.osd.accept = nil
		return 1
	}
	d.osd.accept = d.acceptBPTC
	return d.BPTCList
}

func (d *softDecoder) decodeBPTCOSD(llr *[softBurstBits]int16, check func([12]byte) bool, out *layer2.RawBurst) (softBPTCResult, fec.Result) {
	if d.bptcHard(llr, check) {
		return d.bptcPick(check, out)
	}
	d.bptcLLRs(llr)
	softBPTCCode.osd(&d.osd, d.OSDOrder, d.OSDWindow, d.bptcCheck(check))
	return d.bptcPick(check, out)
}

func (d *softDecoder) bptcHard(llr *[softBurstBits]int16, check func([12]byte) bool) bool {
	s, c := &d.osd, &softOSDCodes[softBPTCCode]
	var packed [25]byte
	for i, q := range &softInfoPos {
		if llr[q] < 0 {
			packed[i/8] |= 0x80 >> (i % 8)
		}
	}
	data, reserved, res := bptc.Decode196(packed)
	if res.ErrorsCorrected != 0 || res.Uncorrectable || check != nil && !check(data) {
		return false
	}
	var cw [softOSDWords]uint64
	for i := range softBPTCData + 3 {
		var b byte
		if i < softBPTCData {
			b = data[i/8] >> (7 - i%8) & 1
		} else {
			b = reserved >> (i - softBPTCData) & 1
		}
		if b != 0 {
			for x := range cw {
				cw[x] ^= c.rows[i][x]
			}
		}
	}
	d.bptcLLRs(llr)
	s.prepare(c.n)
	if cw != s.hard {
		return false
	}
	s.list.reset(d.bptcCheck(check))
	s.list.add(0, &cw)
	return true
}

func (d *softDecoder) decodeBPTCGrid(llr *[softBurstBits]int16, check func([12]byte) bool, out *layer2.RawBurst) (softBPTCResult, fec.Result) {
	d.grid.decode(llr, d.GridIters, d.GridWeight)
	d.gridPost = [softBurstBits]int16{}
	d.grid.posterior(llr, &d.gridPost)
	return d.decodeBPTCOSD(&d.gridPost, check, out)
}

func (d *softDecoder) bptcExtrinsic(llr *[softBurstBits]int16, ext *[softBurstBits]float32) {
	d.grid.decode(llr, d.GridIters, d.GridWeight)
	d.grid.extrinsic(ext)
}

func (d *softDecoder) decodeBPTCChase(llr *[softBurstBits]int16, check func([12]byte) bool, out *layer2.RawBurst) (softBPTCResult, fec.Result) {
	if d.bptcHard(llr, check) {
		return d.bptcPick(check, out)
	}
	s, c := &d.osd, &softOSDCodes[softBPTCCode]
	d.bptcLLRs(llr)
	s.prepare(c.n)
	p := min(max(d.ChaseBits, 0), softChaseMax)
	for i := range p {
		s.chase[i] = -1
	}
	for i, q := range &softInfoPos {
		s.bits[i] = 0
		if llr[q] < 0 {
			s.bits[i] = 1
		}
	}
	for j := range c.n {
		for i := range p {
			if s.chase[i] < 0 || s.mag[j] < s.mag[s.chase[i]] {
				copy(s.chase[i+1:p], s.chase[i:p-1])
				s.chase[i] = int16(j)
				break
			}
		}
	}
	s.list.reset(d.bptcCheck(check))
	var cw [softOSDWords]uint64
	for g := 0; g < 1<<p; g++ {
		if g > 0 {
			s.bits[softBPTCInfoBit[s.chase[bits.TrailingZeros(uint(g))]]] ^= 1
		}
		var packed [25]byte
		for i, v := range &s.bits {
			packed[i/8] |= byte(v&1) << (7 - i%8)
		}
		data, reserved, _ := bptc.Decode196(packed)
		cw = [softOSDWords]uint64{}
		for i := range softBPTCData + 3 {
			var b byte
			if i < softBPTCData {
				b = data[i/8] >> (7 - i%8) & 1
			} else {
				b = reserved >> (i - softBPTCData) & 1
			}
			if b != 0 {
				for x := range cw {
					cw[x] ^= c.rows[i][x]
				}
			}
		}
		var cost int32
		for x := range cw {
			for e := cw[x] ^ s.hard[x]; e != 0; e &= e - 1 {
				cost += s.mag[x<<6|bits.TrailingZeros64(e)]
			}
		}
		s.list.add(cost, &cw)
	}
	return d.bptcPick(check, out)
}

func softBPTCCheck(dt enums.DataType) func([12]byte) bool {
	switch {
	case dt.CarriesFullLC():
		mask, _, _ := pdu.CRCMask(dt)
		return func(p [12]byte) bool {
			p[9] ^= byte(mask >> 16 & 0xff)
			p[10] ^= byte(mask >> 8 & 0xff)
			p[11] ^= byte(mask & 0xff)
			return reedsolomon.Check(p)
		}
	case dt == enums.DataTypePIHeader || dt == enums.DataTypeCSBK || dt == enums.DataTypeMBCHeader ||
		dt == enums.DataTypeDataHeader || dt == enums.DataTypeUnifiedSingleBlock:
		return func(p [12]byte) bool { return pdu.BlockCRCOK(p, dt) }
	default:
		return nil
	}
}

func (d *softDecoder) decodeShortLC(llr *[4][layer2.CACHBits]int16, out *[4][layer2.CACHBits]unpacked.Bit) ([softShortLCInfo]unpacked.Bit, fec.Result) {
	c, s := &softOSDCodes[softShortLCCode], &d.osd
	for j := range c.n {
		p := c.pos[j]
		s.l[j] = int32(llr[p/layer2.CACHBits][p%layer2.CACHBits])
	}
	s.accept = d.acceptShortLC
	softShortLCCode.osd(s, d.OSDOrder, d.OSDWindow, d.ShortLCList)
	pick, ok := s.first()
	cw := &s.list.cw[pick]
	info := softShortLCOf(cw)
	if out != nil {
		for j := range c.n {
			p := c.pos[j]
			out[p/layer2.CACHBits][p%layer2.CACHBits] = 0
			if softBit(cw, j) {
				out[p/layer2.CACHBits][p%layer2.CACHBits] = 1
			}
		}
	}
	return info, fec.Result{BitsChecked: c.n, ErrorsCorrected: s.errors(cw), Uncorrectable: !ok}
}

func softShortLCOf(cw *[softOSDWords]uint64) [softShortLCInfo]unpacked.Bit {
	c := &softOSDCodes[softShortLCCode]
	var info [softShortLCInfo]unpacked.Bit
	for b := range info {
		if softBit(cw, c.sys[b]) {
			info[b] = 1
		}
	}
	return info
}

func (d *softDecoder) decodeEmbeddedFrags(llr *[4][embBits]int16, out *[4]layer2.RawBurst) ([9]byte, fec.Result) {
	c, s := &softOSDCodes[softEmbLCCode], &d.osd
	for j := range c.n {
		p := c.pos[j]
		s.l[j] = int32(llr[p/softBurstBits][p%softBurstBits-embFirstBit])
	}
	s.accept = softEmbLCAccept
	softEmbLCCode.osd(s, d.OSDOrder, d.OSDWindow, d.EmbLCList)
	pick, ok := s.first()
	cw := &s.list.cw[pick]
	if out != nil {
		for j := range c.n {
			p := c.pos[j]
			softPut(&out[p/softBurstBits], p%softBurstBits, softBit(cw, j))
		}
	}
	return softEmbLCOf(cw), fec.Result{BitsChecked: c.n, ErrorsCorrected: s.errors(cw), Uncorrectable: !ok}
}

// embMiss is the share of right embedded LC words that elcStat.bound lets a
// check reject.
const embMiss = 0.001

// embLCDistance is the minimum distance of the embedded LC block of ETSI TS
// 102 361-1 B.2.1: the product of the distance 4 of its Hamming (16,11,4) rows
// and the distance 2 of its parity column.
const embLCDistance = 8

// The superframe-to-superframe spread of the fragment errors, as the
// intraclass correlation of a beta-binomial: the variance of the count of
// disagreeing bits among 128, measured on 24 streams of one LC at 0 and 1 dB,
// is 1.4 times binomial after the lean demodulator and 2.7 times after the
// coherent stage, from error events that span symbols.
const (
	elcSpreadLean = (1.4 - 1) / (4*embBits - 1)
	elcSpreadCoh  = (2.7 - 1) / (4*embBits - 1)
)

// embLCRadius is the number of bit errors that the embedded LC block corrects
// without doubt.
const embLCRadius = (embLCDistance - 1) / 2

// elcStat holds the fragment bits of the last embedded LC word of one channel
// and counts the hard fragment bits of each later superframe that disagree
// with them, over the last maxCombine superframes. The word is fixed before
// the superframe arrives, so the count is not biased toward the noise of the
// superframe as the count against its own decode would be.
type elcStat struct {
	k, m   float64
	rho    float64
	prev   [4]uint32
	prevOK bool
	miss   bool
}

func (e *elcStat) note(d int) {
	if e.m >= maxCombine*4*embBits {
		e.k, e.m = e.k*(maxCombine-1)/maxCombine, e.m*(maxCombine-1)/maxCombine
	}
	e.k, e.m = e.k+float64(d), e.m+4*embBits
}

func (e *elcStat) disagree(cur *[4][embBits]int16) int {
	n := 0
	for i := range cur {
		for j, v := range cur[i] {
			if v != 0 && (v < 0) != (e.prev[i]>>(embBits-1-j)&1 != 0) {
				n++
			}
		}
	}
	return n
}

func (e *elcStat) keep(out *[4]layer2.RawBurst) {
	for i := range out {
		var w uint32
		for j := range embBits {
			q := embFirstBit + j
			w = w<<1 | uint32(out[i][q/8]>>(7-q%8)&1)
		}
		e.prev[i] = w
	}
	e.prevOK = true
}

// bound returns the smallest t with P(X > t) <= embMiss for the number X of
// disagreeing bits among n. X is beta-binomial: its rate has the mean of the
// counts and the spread of whichever is wider, the Jeffreys posterior of the
// counts or the superframe-to-superframe spread rho of the demodulator.
func (e *elcStat) bound(n int) int {
	if e.m == 0 {
		return n
	}
	p := (e.k + 0.5) / (e.m + 1)
	c := e.m + 1
	if e.rho > 0 {
		c = min(c, 1/e.rho-1)
	}
	a, b := p*c, (1-p)*c
	lb, _ := math.Lgamma(a + b)
	la, _ := math.Lgamma(a)
	lbb, _ := math.Lgamma(b)
	ln, _ := math.Lgamma(float64(n) + a + b)
	lc, _ := math.Lgamma(float64(n + 1))
	cdf := 0.0
	for t := range n {
		x, _ := math.Lgamma(float64(t) + a)
		y, _ := math.Lgamma(float64(n-t) + b)
		c2, _ := math.Lgamma(float64(t + 1))
		c3, _ := math.Lgamma(float64(n - t + 1))
		cdf += math.Exp(lc - c2 - c3 + x + y - ln + lb - la - lbb)
		if cdf >= 1-embMiss {
			return t
		}
	}
	return n
}

// decodeELC decodes the embedded LC of one superframe from cur and the
// combined fragments of the earlier superframes in acc. A decode of cur alone
// within embLCRadius wins over the combined decode, so a talker who changes the
// LC within a call gets the new LC at once. A combined decode that gives the
// last LC again needs the hard bits of cur within the bound of the error rate
// measured against that LC, or a changed LC would pass as the old one. The
// first superframe out of the bound yields no LC and leaves acc as it is, so
// one superframe of bad luck costs one LC; a second one in a row resets acc.
// keep false leaves acc and st unchanged.
func (d *softDecoder) decodeELC(cur, acc, sum *[4][embBits]int16, accN *int, accLC *[9]byte, out *[4]layer2.RawBurst, keep bool, st *elcStat) ([9]byte, bool) {
	fit := true
	if keep && st.prevOK {
		np := st.disagree(cur)
		if fit = np <= st.bound(4*embBits); fit {
			st.note(np)
		}
	}
	l, res := d.decodeEmbeddedFrags(cur, out)
	if !res.Uncorrectable && res.ErrorsCorrected <= embLCRadius {
		if keep {
			if *accN > 0 && l == *accLC {
				for i := range acc {
					softCombine(acc[i][:], cur[i][:])
				}
				*accN = min(*accN+1, maxCombine)
			} else {
				*acc, *accN = *cur, 1
			}
			*accLC = l
			st.keep(out)
			st.miss = false
		}
		return l, true
	}
	if *accN > 0 && keep {
		for i := range sum {
			sum[i] = acc[i]
			softCombine(sum[i][:], cur[i][:])
		}
		c, cres := d.decodeEmbeddedFrags(sum, out)
		switch {
		case cres.Uncorrectable:
		case fit || c != *accLC:
			*acc, *accN, *accLC = *sum, min(*accN+1, maxCombine), c
			st.keep(out)
			st.miss = false
			return c, true
		case !st.miss:
			st.miss = true
			return c, false
		}
		st.miss = false
		l, res = d.decodeEmbeddedFrags(cur, out)
	}
	if keep {
		*acc, *accN, *accLC = *cur, 1, l
		if !res.Uncorrectable {
			st.keep(out)
		}
	}
	return l, !res.Uncorrectable
}

func softEmbLCAccept(cw *[softOSDWords]uint64) bool {
	c := &softOSDCodes[softEmbLCCode]
	var cs uint8
	for b, x := range &softEmbLCLayout.cs {
		if softBit(cw, c.sys[x]) {
			cs |= 1 << b
		}
	}
	return crc.CheckChecksum5(softEmbLCOf(cw), cs)
}

func softEmbLCOf(cw *[softOSDWords]uint64) [9]byte {
	c := &softOSDCodes[softEmbLCCode]
	var lc [9]byte
	for j, i := range &softEmbLCLayout.lc {
		if softBit(cw, c.sys[i]) {
			lc[j/8] |= 0x80 >> (j % 8)
		}
	}
	return lc
}

func softCombine(dst, src []int16) {
	for i := range min(len(dst), len(src)) {
		dst[i] = int16(max(math.MinInt16, min(math.MaxInt16, int32(dst[i])+int32(src[i]))))
	}
}

const (
	embFirstBit = 116
	embBits     = 32
	maxCombine  = 4
)
