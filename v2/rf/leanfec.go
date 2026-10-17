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

// Limits of the decoders of the streaming receiver, as the defaults of
// softDecoder.
const (
	// leanChase is the number of least reliable bits that the BPTC(196,96)
	// decode flips, 2^leanChase hard decodes.
	leanChase = 4
	// leanBPTCBits is the number of bits of a BPTC(196,96) block, and
	// leanBPTCHalf the number on each side of the slot type and SYNC.
	leanBPTCBits = 196
	leanBPTCHalf = leanBPTCBits / 2
	// leanNibbles is the number of nibble cost tables of a small code.
	leanNibbles = softSmallMaxN / 4
	// leanCandSize is the number of values of leanFEC.s that hold one
	// candidate of the BPTC(196,96) decode: its cost in two, its 12 data
	// bytes in six and its reserved bits.
	leanCandSize = 9
	// leanShared is the number of values of leanFEC.s: the most of the
	// nibble tables of a small code, the rate 3/4 survivors at four 4-bit
	// states a value, and the BPTC(196,96) candidates.
	leanShared = max(leanNibbles*16, softTrellisSteps*softTrellisStates/4, softListMax*leanCandSize)
)

// leanBest is a candidate list of searchNibble that keeps the best one.
type leanBest struct {
	set  bool
	cost int16
	info uint16
	cw   uint32
}

// reset empties b.
func (b *leanBest) reset() { b.set = false }

// worst returns the cost of the kept candidate, or MaxInt32.
func (b *leanBest) worst() int32 {
	if !b.set {
		return math.MaxInt32
	}
	return int32(b.cost)
}

// add keeps a candidate when it beats the kept one.
func (b *leanBest) add(cost int32, info, cw uint32) {
	if b.set && cost >= int32(b.cost) {
		return
	}
	b.set, b.cost, b.info, b.cw = true, int16(cost), uint16(info), cw
}

// leanFEC is the decode scratch of the streaming receiver. Its decoders
// take the log-likelihood ratios of the bits of a burst from leanRx.llrAt,
// so that it needs no copy of them, and write the corrected codewords into
// the Fixed bits of the burst.
type leanFEC struct {
	// s is shared by the decoders, one at a time: the nibble cost tables of
	// a small code, the Viterbi survivors of the rate 3/4 decode, or the
	// candidates of the BPTC(196,96) decode.
	s [leanShared]int16
	// l holds the LLRs of a small code, or the leanChase least reliable
	// bits of a BPTC(196,96) block and their magnitudes.
	l     [leanLocal]int16
	top   leanBest
	nlist int
}

// leanLocal is the number of values of leanFEC.l.
const leanLocal = max(softSmallMaxN, 2*leanChase)

// chase returns the codeword bits of the leanChase least reliable bits of
// the BPTC(196,96) block, the least reliable first.
func (f *leanFEC) chase() []int16 { return f.l[:leanChase] }

// cmag returns the magnitudes of the bits of chase.
func (f *leanFEC) cmag() []int16 {
	return f.l[leanChase : 2*leanChase]
}

// setPrev sets the survivor of state in at step k of the rate 3/4 decode.
func (f *leanFEC) setPrev(k, in, v int) {
	q := k*softTrellisStates + in
	sh := 4 * uint(q&3)
	f.s[q>>2] = int16(uint16(f.s[q>>2])&^(0xf<<sh) | uint16(v)<<sh)
}

// prev returns the survivor of state st at step k of the rate 3/4 decode.
func (f *leanFEC) prev(k, st int) int {
	q := k*softTrellisStates + st
	return int(uint16(f.s[q>>2]) >> (4 * uint(q&3)) & 0xf)
}

// cand returns candidate i of the BPTC(196,96) decode.
func (f *leanFEC) cand(i int) (cost int32, data [12]byte, reserved uint8) {
	c := f.s[i*leanCandSize:][:leanCandSize]
	cost = int32(uint32(uint16(c[0])) | uint32(uint16(c[1]))<<16)
	for k := range 6 {
		data[2*k], data[2*k+1] = byte(uint16(c[2+k])>>8), byte(c[2+k])
	}
	return cost, data, uint8(c[8])
}

// setCand sets candidate i of the BPTC(196,96) decode.
func (f *leanFEC) setCand(i int, cost int32, data *[12]byte, reserved uint8) {
	c := f.s[i*leanCandSize:][:leanCandSize]
	c[0], c[1] = int16(uint16(cost)), int16(uint16(uint32(cost)>>16))
	for k := range 6 {
		c[2+k] = int16(uint16(data[2*k])<<8 | uint16(data[2*k+1]))
	}
	c[8] = int16(reserved)
}

// nibbleTables fills tab[16b+m] with the sum of l[4b+i] over the set bits i
// of m, for the tables that cover l.
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

// nibbleCost returns the cost of codeword cw of n bits from the nibble
// tables tab.
func nibbleCost(tab []int16, cw uint32, n int) int32 {
	var c int32
	for b := 0; b*4 < n; b++ {
		c += int32(tab[16*b+int(cw>>(4*b)&15)])
	}
	return c
}

// searchNibble is softSmallID.search with nibble tables.
func searchNibble(id softSmallID, tab []int16, off, fixed, want uint32, top *leanBest) {
	c := &softSmallCodes[id]
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
	top.add(nibbleCost(tab, cw, c.n), info, cw)
	worst := top.worst()
	for g := uint32(1); g < uint32(1)<<nf; g++ {
		i := free[bits.TrailingZeros32(g)&15]
		info ^= 1 << i
		cw ^= c.rows[i]
		if cost := nibbleCost(tab, cw, c.n); cost < worst {
			top.add(cost, info, cw)
			worst = top.worst()
		}
	}
}

// gather loads the LLRs of small code id into f.l from the burst or, when
// cach, from the CACH of the slot of r, and returns the hard decisions.
func (r *leanCore) gather(f *leanFEC, id softSmallID, cach bool) uint32 {
	c := &softSmallCodes[id]
	var hard uint32
	for j := range c.n {
		var v int16
		if cach {
			v = r.cachAt(int(c.pos[j]))
		} else {
			v = r.llrAt(int(c.pos[j]))
		}
		f.l[j] = v
		if v < 0 {
			hard |= 1 << j
		}
	}
	return hard
}

// decodeSmall returns the lowest-cost codeword of small code id among those
// whose info bits under fixed equal want, its info word and the number of
// bits where it differs from the hard decisions.
func (r *leanCore) decodeSmall(f *leanFEC, id softSmallID, cach bool, fixed, want uint32) (info, cw uint32, errs int) {
	hard := r.gather(f, id, cach)
	nibbleTables(f.s[:leanNibbles*16], f.l[:softSmallCodes[id].n])
	f.top.reset()
	searchNibble(id, f.s[:leanNibbles*16], 0, fixed, want, &f.top)
	return uint32(f.top.info), f.top.cw, bits.OnesCount32(f.top.cw ^ hard)
}

// bptcCheck reports whether the BPTC(196,96) payload p of data type dt
// passes its check, as the function of softBPTCCheck does, and whether dt
// has one.
func bptcCheck(dt enums.DataType, p [12]byte) (ok, checked bool) {
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

// bptcPos returns the burst bit of BPTC(196,96) information bit p: 98
// bits before the slot type and SYNC, and 98 after.
func bptcPos(p int) int {
	if p < leanBPTCHalf {
		return p
	}
	return p + softBurstBits - 2*leanBPTCHalf
}

// addCand adds a candidate to the list of the BPTC decode, which holds the
// lowest costs in increasing order, up to max.
func (f *leanFEC) addCand(cost int32, data [12]byte, reserved uint8, max int) {
	for i := range f.nlist {
		if c, d, r := f.cand(i); c == cost && d == data && r == reserved {
			return
		}
	}
	if f.nlist == max {
		if c, _, _ := f.cand(f.nlist - 1); cost >= c {
			return
		}
	}
	i := min(f.nlist, max-1)
	for i > 0 {
		c, d, r := f.cand(i - 1)
		if c <= cost {
			break
		}
		f.setCand(i, c, &d, r)
		i--
	}
	f.setCand(i, cost, &data, reserved)
	f.nlist = min(f.nlist+1, max)
}

// fixBPTC decodes the BPTC(196,96) block of the burst of data type dt with
// the Chase-II decode of softDecoder.decodeBPTCChase into out, and reports
// whether a block with a check failed it. It scores each candidate on the
// grid against the hard decisions, so that it needs no codeword table.
func (r *leanCore) fixBPTC(f *leanFEC, dt enums.DataType, out *layer2.RawBurst) bool {
	chase, cmag := f.chase(), f.cmag()
	for i := range chase {
		chase[i], cmag[i] = -1, 0
	}
	var packed [25]byte
	var base int32
	for p := range leanBPTCBits {
		v := r.llrAt(bptcPos(p))
		if v < 0 {
			packed[p/8] |= 0x80 >> (p % 8)
			v = -v
		}
		// R(3), bit 0, is outside the grid and 0 in every codeword.
		if p == 0 {
			if packed[0] != 0 {
				base = int32(v)
			}
			continue
		}
		for i := range chase {
			if chase[i] < 0 || v < cmag[i] {
				copy(chase[i+1:], chase[i:leanChase-1])
				copy(cmag[i+1:], cmag[i:leanChase-1])
				chase[i], cmag[i] = int16(p), v
				break
			}
		}
	}
	_, checked := bptcCheck(dt, [12]byte{})
	size := 1
	if checked {
		size = softListMax
	}
	f.nlist = 0
	// The grid is deinterleaved once; each pattern flips one grid bit.
	hard := bptc.Rows(packed)
	rows := hard
	var flipRow [leanChase]int8
	var flipCol [leanChase]uint16
	for i, p := range chase {
		r, c, _ := bptc.GridPos(int(p))
		flipRow[i], flipCol[i] = int8(r), 1<<c
	}
	for g := 0; g < 1<<leanChase; g++ {
		if g > 0 {
			i := bits.TrailingZeros(uint(g))
			rows[flipRow[i]] ^= flipCol[i]
		}
		data, reserved, _ := bptc.DecodeRows(rows)
		cw := bptc.EncodeRows(data, reserved)
		cost := base
		for row := range cw {
			for e := cw[row] ^ hard[row]; e != 0; e &= e - 1 {
				l := r.llrAt(bptcPos(bptc.GridBit(row, bits.TrailingZeros16(e))))
				cost += int32(max(l, -l))
			}
		}
		if g == 0 && cost == 0 {
			if ok, _ := bptcCheck(dt, data); ok || !checked {
				return false
			}
		}
		f.addCand(cost, data, reserved, size)
	}
	pick, failed := 0, checked
	for i := 0; checked && i < f.nlist; i++ {
		_, d, _ := f.cand(i)
		if ok, _ := bptcCheck(dt, d); ok {
			pick, failed = i, false
			break
		}
	}
	_, data, reserved := f.cand(pick)
	cw := bptc.Encode196(data, reserved)
	for p := range leanBPTCBits {
		softPut(out, bptcPos(p), cw[p/8]&(0x80>>(p%8)) != 0)
	}
	return failed
}

// fixRate34 decodes the rate 3/4 trellis of the burst, as
// softDecoder.decodeRate34, into out.
func (r *leanCore) fixRate34(f *leanFEC, out *layer2.RawBurst) {
	tr := &softTrellisTables
	metric := [softTrellisStates]int32{0, softInf, softInf, softInf, softInf, softInf, softInf, softInf}
	var next [softTrellisStates]int32
	for k := range softTrellisSteps {
		var l [4]int16
		for b, j := range tr.step[k] {
			l[b] = r.llrAt(int(tr.pos[j]))
		}
		var bc [16]int32
		for lab := 1; lab < 16; lab++ {
			bc[lab] = bc[lab&(lab-1)] + int32(l[3-bits.TrailingZeros(uint(lab))])
		}
		for in := range softTrellisStates {
			best, arg := int32(softInf), 0
			for s, m := range metric {
				if c := m + bc[tr.label[s][in]]; c < best {
					best, arg = c, s
				}
			}
			next[in] = best
			f.setPrev(k, in, arg)
		}
		metric = next
	}
	var t [softTrellisSteps]uint8
	state := 0
	for k := softTrellisSteps - 1; k >= 0; k-- {
		t[k] = uint8(state)
		state = f.prev(k, state)
	}
	state = 0
	for k := range softTrellisSteps {
		lab := tr.label[state][t[k]]
		state = int(t[k])
		for b, j := range tr.step[k] {
			softPut(out, tr.pos[j], lab>>(3-b)&1 != 0)
		}
	}
}

// fixTACT decodes the Hamming(7,4) TACT of the CACH of burst b into its
// FixedCACH bits.
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

// repair rewrites the FEC fields of burst b with the codewords that
// soft-decision decoding picks, with the scratch f, as repairer.repair does
// for TierDisc, with the color code cc of the stream when ccOK. The payload
// of an idle burst carries nothing, so only its slot type is decoded.
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
			softPutSmall(out, softEMBCode, cw)
		}
	case known && kind == layer2.KindData:
		var fixed, want uint32
		if ccOK {
			fixed, want = 0xf0, uint32(cc&0x0f)<<4
		}
		info, cw, errs := r.decodeSmall(f, softSlotTypeCode, false, fixed, want)
		softPutSmall(out, softSlotTypeCode, cw)
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
