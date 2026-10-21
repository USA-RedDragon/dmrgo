package rf

import (
	"math"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	leanChase      = 4
	leanSmallHard  = true
	leanBPTCGather = true
	leanR34Pair    = false
	leanBPTCBits   = 196
	leanBPTCHalf   = leanBPTCBits / 2
	leanNibbles    = softSmallMaxN / 4
	leanCandSize   = 9
	leanShared     = max(leanNibbles*16, softTrellisSteps*softTrellisStates/4, softListMax*leanCandSize, (leanAcqSamples+1)/2)
)

type leanBest struct {
	set  bool
	cost int16
	info uint16
	cw   uint32
}

func (b *leanBest) reset() { b.set = false }

func (b *leanBest) worst() int32 {
	if !b.set {
		return math.MaxInt32
	}
	return int32(b.cost)
}

func (b *leanBest) add(cost int32, info, cw uint32) {
	if b.set && cost >= int32(b.cost) {
		return
	}
	b.set, b.cost, b.info, b.cw = true, int16(cost), uint16(info), cw
}

type leanFEC struct {
	s     [leanShared]int16
	l     [leanLocal]int16
	top   leanBest
	nlist int
}

const leanLocal = max(softSmallMaxN, 2*leanChase)

func (f *leanFEC) setPrev(k, in, v int) {
	q := k*softTrellisStates + in
	sh := 4 * uint(q&3)
	f.s[q>>2] = int16(uint16(f.s[q>>2])&^(0xf<<sh) | uint16(v)<<sh)
}

func (f *leanFEC) prev(k, st int) int {
	q := k*softTrellisStates + st
	return int(uint16(f.s[q>>2]) >> (4 * uint(q&3)) & 0xf)
}

func (f *leanFEC) cand(i int, data *[12]byte) uint8 {
	c := f.s[i*leanCandSize:][:leanCandSize]
	for k := range 6 {
		data[2*k], data[2*k+1] = byte(uint16(c[2+k])>>8), byte(c[2+k])
	}
	return uint8(c[8])
}

func (f *leanFEC) candCost(i int) int32 {
	c := f.s[i*leanCandSize:][:2]
	return int32(uint32(uint16(c[0])) | uint32(uint16(c[1]))<<16)
}

func packCand(c *[leanCandSize]int16, cost int32, data *[12]byte, reserved uint8) {
	c[0], c[1] = int16(uint16(cost)), int16(uint16(uint32(cost)>>16))
	for k := range 6 {
		c[2+k] = int16(uint16(data[2*k])<<8 | uint16(data[2*k+1]))
	}
	c[8] = int16(reserved)
}

func nibbleTables(tab []int16, l []int16) {
	for b := 0; b*4 < len(l); b++ {
		t := tab[16*b : 16*b+16]
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

func nibbleCost[_ any](tab []int16, cw uint32, n int) int32 {
	var c int32
	for b := 0; b*4 < n; b++ {
		c += int32(tab[16*b+int(cw>>(4*b)&15)])
	}
	return c
}

func searchNibble(id softSmallID, tab []int16, off, fixed, want uint32, top *leanBest) {
	c := &softSmallCodes[id]
	var free [16]uint8
	nf := 0
	info := want & fixed
	cw := c.base ^ off
	for i := range c.k {
		if fixed>>i&1 == 0 {
			free[nf] = uint8(i)
			nf++
		} else if info>>i&1 != 0 {
			cw ^= c.rows[i]
		}
	}
	top.add(nibbleCost[site1](tab, cw, c.n), info, cw)
	worst := top.worst()
	for g := uint32(1); g < uint32(1)<<nf; g++ {
		i := free[bits.TrailingZeros32(g)&15]
		info ^= 1 << i
		cw ^= c.rows[i]
		if cost := nibbleCost[site0](tab, cw, c.n); cost < worst {
			top.add(cost, info, cw)
			worst = top.worst()
		}
	}
}

func (r *leanCore) gather(f *leanFEC, id softSmallID, cach bool) uint32 {
	c := &softSmallCodes[id]
	base := r.base + cachSymbols
	if cach {
		base = r.base
	}
	var hard uint32
	last := int64(-1)
	var l0, l1 int16
	for j := range c.n {
		p := int(c.pos[j])
		if k := base + int64(p>>1); k != last {
			l0, l1 = r.symLLR(k)
			last = k
		}
		v := l0
		if p&1 != 0 {
			v = l1
		}
		f.l[j] = v
		if v < 0 {
			hard |= 1 << j
		}
	}
	return hard
}

func (r *leanCore) decodeSmall(f *leanFEC, id softSmallID, cach bool, fixed, want uint32) (info, cw uint32, errs int) {
	hard := r.gather(f, id, cach)
	if leanSmallHard {
		c := &softSmallCodes[id]
		var hi uint32
		cw := c.base
		for i := range c.k {
			if hard>>(c.k-1-i)&1 != 0 {
				hi |= 1 << i
				cw ^= c.rows[i]
			}
		}
		if cw == hard && (hi^want)&fixed == 0 {
			return hi, cw, 0
		}
	}
	nibbleTables(f.s[:leanNibbles*16], f.l[:softSmallCodes[id].n])
	f.top.reset()
	searchNibble(id, f.s[:leanNibbles*16], 0, fixed, want, &f.top)
	return uint32(f.top.info), f.top.cw, bits.OnesCount32(f.top.cw ^ hard)
}

func bptcCheck(dt enums.DataType, data *[12]byte) (ok, checked bool) {
	p := *data
	switch {
	case dt.CarriesFullLC():
		mask, _, _ := pdu.CRCMask(dt)
		p[9] ^= byte(mask >> 16 & 0xff)
		p[10] ^= byte(mask >> 8 & 0xff)
		p[11] ^= byte(mask & 0xff)
		return reedsolomon.Check(p), true
	case dt == enums.DataTypePIHeader || dt == enums.DataTypeCSBK || dt == enums.DataTypeMBCHeader ||
		dt == enums.DataTypeDataHeader || dt == enums.DataTypeUnifiedSingleBlock:
		return pdu.BlockCRCOK(p, dt), true
	}
	return false, false
}

func bptcPos(p int) int {
	if p < leanBPTCHalf {
		return p
	}
	return p + softBurstBits - 2*leanBPTCHalf
}

//go:noinline
func (f *leanFEC) addCand(cost int32, data *[12]byte, reserved uint8, limit int) {
	var e [leanCandSize]int16
	packCand(&e, cost, data, reserved)
	for i := range f.nlist {
		if [leanCandSize]int16(f.s[i*leanCandSize:]) == e {
			return
		}
	}
	if f.nlist == limit && cost >= f.candCost(f.nlist-1) {
		return
	}
	i := min(f.nlist, limit-1)
	for i > 0 && f.candCost(i-1) > cost {
		copy(f.s[i*leanCandSize:(i+1)*leanCandSize], f.s[(i-1)*leanCandSize:])
		i--
	}
	copy(f.s[i*leanCandSize:], e[:])
	f.nlist = min(f.nlist+1, limit)
}

//go:noinline
func (r *leanCore) fixBPTC(f *leanFEC, dt enums.DataType, out *layer2.RawBurst) bool {
	chase, cmag := f.l[:leanChase], f.l[leanChase:2*leanChase]
	for i := range chase {
		chase[i], cmag[i] = -1, 0
	}
	var rows [13]uint16
	var base int32
	if !leanBPTCGather {
		if v := r.llrAt(bptcPos(0)); v < 0 {
			base = -int32(v)
		}
	}
	var bit [leanChase]int16
	at, c := r.base+cachSymbols, 0
	var hi int16
	for cell := range 13 * 15 {
		var p int
		var v int16
		row, col := cell/15, cell%15
		if leanBPTCGather {
			p = cell + 1
			v = hi
			if p == leanBPTCHalf {
				at += (softBurstBits - 2*leanBPTCHalf) / 2
			}
			if p&1 == 0 {
				v, hi = r.symLLR(at + int64(p>>1))
			}
			if p == 1 {
				l0, l1 := r.symLLR(at)
				if l0 < 0 {
					base = -int32(l0)
				}
				v = l1
			}
			c += 13
			if c >= leanBPTCBits {
				c -= leanBPTCBits
			}
			row, col = (c-1)/15, (c-1)%15
		} else {
			p = bptc.GridBit(row, col)
			v = llrAtS[site0](r, bptcPos(p))
		}
		if v < 0 {
			rows[row] |= 1 << col
			v = -v
		}
		if l := leanChase - 1; chase[l] >= 0 && (v > cmag[l] || v == cmag[l] && int16(p) > bit[l]) {
			continue
		}
		for i := range chase {
			if chase[i] < 0 || v < cmag[i] || v == cmag[i] && int16(p) < bit[i] {
				copy(chase[i+1:], chase[i:leanChase-1])
				copy(cmag[i+1:], cmag[i:leanChase-1])
				copy(bit[i+1:], bit[i:leanChase-1])
				chase[i], cmag[i], bit[i] = int16(row*15+col), v, int16(p)
				break
			}
		}
	}
	var data [12]byte
	_, checked := bptcCheck(dt, &data)
	size := 1
	if checked {
		size = softListMax
	}
	f.nlist = 0
	chase = chase[:r.chase]
	for g := 0; g < 1<<len(chase); g++ {
		if g > 0 {
			cell := chase[bits.TrailingZeros(uint(g))]
			rows[cell/15] ^= 1 << (cell % 15)
		}
		reserved, cost := r.bptcPattern(&rows, chase, g^g>>1, &data)
		cost += base
		if g == 0 && cost == 0 {
			if ok, _ := bptcCheck(dt, &data); ok || !checked {
				return false
			}
		}
		f.addCand(cost, &data, reserved, size)
	}
	pick, failed := 0, checked
	for i := 0; checked && i < f.nlist; i++ {
		f.cand(i, &data)
		if ok, _ := bptcCheck(dt, &data); ok {
			pick, failed = i, false
			break
		}
	}
	putBPTC(out, &data, f.cand(pick, &data))
	return failed
}

//go:noinline
func (r *leanCore) bptcPattern(rows *[13]uint16, chase []int16, g int, data *[12]byte) (uint8, int32) {
	cw := *rows
	reserved := bptc.Recode(&cw, data)
	var cost int32
	for row := range cw {
		for e := cw[row] ^ rows[row]; e != 0; e &= e - 1 {
			l := r.llrAt(bptcPos(bptc.GridBit(row, bits.TrailingZeros16(e))))
			cost += int32(max(l, -l))
		}
	}
	for j, cell := range chase {
		if g>>j&1 == 0 {
			continue
		}
		row, col := int(cell)/15, int(cell)%15
		l := r.llrAt(bptcPos(bptc.GridBit(row, col)))
		if (cw[row]^rows[row])>>col&1 != 0 {
			cost -= int32(max(l, -l))
		} else {
			cost += int32(max(l, -l))
		}
	}
	return reserved, cost
}

//go:noinline
func putBPTC(out *layer2.RawBurst, data *[12]byte, reserved uint8) {
	cw := bptc.EncodeRows(data, reserved)
	softPut(out, bptcPos(0), false)
	for row, w := range &cw {
		for col := range 15 {
			softPut(out, bptcPos(bptc.GridBit(row, col)), w>>col&1 != 0)
		}
	}
}

//go:noinline
func (r *leanCore) fixRate34(f *leanFEC, out *layer2.RawBurst) {
	tr := &softTrellisTables
	metric := [softTrellisStates]int32{0, softInf, softInf, softInf, softInf, softInf, softInf, softInf}
	var next [softTrellisStates]int32
	var bc [16]int32
	for k := range softTrellisSteps {
		var l [4]int16
		if leanR34Pair {
			at := r.base + cachSymbols
			l[0], l[1] = r.symLLR(at + int64(tr.pos[tr.step[k][0]]>>1))
			l[2], l[3] = r.symLLR(at + int64(tr.pos[tr.step[k][2]]>>1))
		} else {
			for b, j := range &tr.step[k] {
				l[b] = r.llrAt(int(tr.pos[j]))
			}
		}
		for lab := 1; lab < 16; lab++ {
			bc[lab] = bc[lab&(lab-1)] + int32(l[3-bits.TrailingZeros(uint(lab))])
		}
		for in := range softTrellisStates {
			best, arg := int32(softInf), 0
			for s, m := range &metric {
				if c := m + bc[tr.label[s][in]]; c < best {
					best, arg = c, s
				}
			}
			next[in] = best
			f.setPrev(k, in, arg)
		}
		metric = next
	}
	state := 0
	for k := softTrellisSteps - 1; k >= 0; k-- {
		p := f.prev(k, state)
		f.setPrev(k, 0, state)
		state = p
	}
	state = 0
	for k := range softTrellisSteps {
		tk := f.prev(k, 0)
		lab := tr.label[state][tk]
		state = tk
		for b, j := range &tr.step[k] {
			softPut(out, tr.pos[j], lab>>(3-b)&1 != 0)
		}
	}
}

func (r *leanCore) fixTACT(f *leanFEC, b *Burst) {
	_, cw, _ := r.decodeSmall(f, softTACTCode, true, 0, 0)
	c := &softSmallCodes[softTACTCode]
	for j := range c.n {
		p := c.pos[j]
		m := byte(0x80) >> (p % 8)
		if cw>>j&1 != 0 {
			b.FixedCACH[p/8] |= m
		} else {
			b.FixedCACH[p/8] &^= m
		}
	}
}

//go:noinline
func (r *leanCore) repair(f *leanFEC, b *Burst, cc enums.ColorCode, ccOK bool) {
	b.Fixed, b.FixedCACH, b.HasFixed = b.Data, b.CACH, true
	out := &b.Fixed
	_, kind, known := layer2.SyncSourceOf(b.Sync)
	switch {
	case b.Voice != enums.VoiceBurstUnknown || known && kind == layer2.KindVoice:
		if b.Voice != enums.VoiceBurstUnknown && b.Voice != enums.VoiceBurstA {
			var fixed, want uint32
			if ccOK {
				fixed, want = 0x78, uint32(cc&0x0f)<<3
			}
			_, cw, _ := r.decodeSmall(f, softEMBCode, false, fixed, want)
			leanPutSmall(out, softEMBCode, cw)
		}
	case known && kind == layer2.KindData:
		var fixed, want uint32
		if ccOK {
			fixed, want = 0xf0, uint32(cc&0x0f)<<4
		}
		info, cw, errs := r.decodeSmall(f, softSlotTypeCode, false, fixed, want)
		leanPutSmall(out, softSlotTypeCode, cw)
		if errs > softSmallCodes[softSlotTypeCode].t {
			break
		}
		switch dt := pdu.SlotTypeInfo(info & 0xff).DataType(); dt {
		case enums.DataTypeRate34:
			r.fixRate34(f, out)
		case enums.DataTypeRate1, enums.DataTypeReserved, enums.DataTypeIdle:
		default:
			r.fixBPTC(f, dt, out)
		}
	}
	if known && r.anchor == b.Sync {
		_ = out.SetSync(b.Sync)
	}
	if b.HasCACH {
		r.fixTACT(f, b)
	}
}

func leanPutSmall(r *layer2.RawBurst, id softSmallID, cw uint32) {
	c := &softSmallCodes[id]
	for j := range c.n {
		softPut(r, c.pos[j], cw>>j&1 != 0)
	}
}
