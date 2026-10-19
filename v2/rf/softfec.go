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
	// softBurstBits is the number of bits in a burst.
	softBurstBits = 264
	// softSmallMaxN is the largest codeword length of a softSmallCode.
	softSmallMaxN = 24
	// softSmallMaxK is the largest number of info bits of a softSmallCode.
	softSmallMaxK = 12
	// softOSDMaxN is the largest codeword length of a softOSDCode.
	softOSDMaxN = 196
	// softOSDMaxK is the largest number of info bits of a softOSDCode.
	softOSDMaxK = 99
	// softOSDWords is the number of 64-bit words of a softOSDCode codeword.
	softOSDWords = 4
	// softOSDNibbles is the number of cost tables of a softOSDCode codeword,
	// one per 4 bits.
	softOSDNibbles = 49
	// softTabLen is the length of the cost tables of one decode: three
	// tables of 8 bits for a softSmallCode or the tables of 4 bits for a
	// softOSDCode. The decodes run one at a time and share the tables.
	softTabLen = max(3*256, softOSDNibbles*16)
	// softListMax is the largest candidate list of a decode.
	softListMax = 8
	// softChaseMax is the largest number of bits that the Chase decode flips.
	softChaseMax = 16
	// softBPTCData is the number of data bits of BPTC(196,96).
	softBPTCData = 96
	// softBPTCInfo is the number of BPTC(196,96) data and reserved bits.
	softBPTCInfo = 99
	// softTrellisBits is the number of coded bits of a rate 3/4 block.
	softTrellisBits = 196
	// softTrellisSteps is the number of trellis steps, 48 tribits and the tail.
	softTrellisSteps = 49
	// softTrellisStates is the number of trellis states.
	softTrellisStates = 8
	// softAMBEC0 is the number of info bits of AMBE+2 C0 and of C1.
	softAMBEC0 = 12
	// softAMBEC2 is the number of uncoded bits of an AMBE+2 frame.
	softAMBEC2 = 25
	// softShortLCInfo is the number of Short LC info bits, 28 data and a CRC-8.
	softShortLCInfo = 36
	// softShortLCData is the number of Short LC data bits before the CRC-8.
	softShortLCData = 28
	// softEmbLCInfo is the number of embedded LC info bits, 72 LC and a checksum.
	softEmbLCInfo = 77
	// softEmbLCBits is the number of LC bits of an embedded LC.
	softEmbLCBits = 72
	// softEmbLCChecksum is the number of checksum bits of an embedded LC.
	softEmbLCChecksum = 5
	// softInf is the path metric of an unreachable trellis state.
	softInf = math.MaxInt32 / 4
)

// softSmallID names a code of softSmallCodes.
type softSmallID uint8

// Codes of softSmallCodes.
const (
	softSlotTypeCode softSmallID = iota
	softEMBCode
	softTACTCode
	// softAMBECode is the code C0 of voice slot 0. C0 of voice slot s is
	// softAMBECode+2s, and its C1 without the scrambling the code after it.
	softAMBECode
	softSmallCount = softAMBECode + 2*layer2.VoiceSlotsPerBurst
)

// softOSDID names a code of softOSDCodes.
type softOSDID uint8

// Codes of softOSDCodes: BPTC(196,96), the BPTC(68,36) of a Short LC, and
// the BPTC(128,77) of an embedded LC over four bursts, burst i at 264i.
const (
	softBPTCCode softOSDID = iota
	softShortLCCode
	softEmbLCCode
	softOSDCount
)

// softTop is a list of the lowest-cost candidates of a softSmallCode search,
// in increasing cost order.
type softTop struct {
	// n is the number of candidates and max the list size.
	n, max int
	// cost, info and cw hold candidate i at index i.
	cost [softListMax]int32
	info [softListMax]uint32
	cw   [softListMax]uint32
}

// reset empties the list and sets its size to size, from 1 to softListMax.
func (t *softTop) reset(size int) {
	t.n, t.max = 0, max(1, min(size, softListMax))
}

// worst returns the cost that a candidate must beat to enter the list.
func (t *softTop) worst() int32 {
	if t.n < t.max {
		return math.MaxInt32
	}
	return t.cost[t.n-1]
}

// add inserts a candidate when it is among the max lowest costs.
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

// softSmallCode is an (n, k) binary block code with n at most 24 and k at
// most 12 that search decodes by scoring every codeword.
type softSmallCode struct {
	// n is the codeword length and k the number of info bits.
	n, k int
	// t is the number of bit errors that the code always corrects.
	t int
	// pos holds the LLR index of codeword bit j, in increasing order.
	pos [softSmallMaxN]int16
	// base is the codeword of the zero info word, codeword bit j at 1<<j.
	base uint32
	// rows holds the codeword change of info bit i.
	rows [softSmallMaxK]uint32
}

// softTables fills tab[256b+m] with the sum of l[8b+i] over the set bits i
// of m, for the tables that cover l.
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

// softNibbleTables fills tab[16b+m] with the sum of l[4b+i] over the set
// bits i of m, for the tables that cover l.
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

// softByteTables fills tab[256b+m] with the sum of l[8b+i] over the set
// bits i of m, for the tables that cover l.
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

// gather copies the LLRs of the code bits into l and returns the hard
// decision codeword.
func (id softSmallID) gather(llr []int16, l *[softSmallMaxN]int32) uint32 {
	c := &softSmallCodes[id]
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

// search scores every codeword whose info bits under fixed equal want, with
// off XORed into the codeword, and keeps the lowest costs in top. The cost of
// a codeword is the sum of the LLRs of its 1 bits, from tab.
func (id softSmallID) search(tab *[softTabLen]int32, off, fixed, want uint32, top *softTop) {
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

// softSmallScratch is the per-call state of the softSmallCode decodes.
type softSmallScratch struct {
	// l holds the LLRs of the code bits.
	l [softSmallMaxN]int32
	// tab holds the cost tables of the code bits.
	tab *[softTabLen]int32
	// top and top1 hold candidate lists.
	top, top1 softTop
}

// decode returns the lowest-cost codeword of c, its info word and the number
// of bits where it differs from the hard decisions.
func (s *softSmallScratch) decode(c softSmallID, llr []int16, fixed, want uint32) (info, cw uint32, errs int) {
	hard := c.gather(llr, &s.l)
	softTables(s.tab, s.l[:softSmallCodes[c].n])
	s.top.reset(1)
	c.search(s.tab, 0, fixed, want, &s.top)
	return s.top.info[0], s.top.cw[0], bits.OnesCount32(s.top.cw[0] ^ hard)
}

// softResult returns the fec.Result of a decode without a check: errs bit
// errors in n bits, uncorrectable beyond the t errors the code always
// corrects.
func softResult(n, errs, t int) fec.Result {
	return fec.Result{BitsChecked: n, ErrorsCorrected: errs, Uncorrectable: errs > t}
}

// softPut sets bit p of r, in transmit order, to 1 when one is true and to 0
// when it is false.
func softPut[T int | int16](r *layer2.RawBurst, p T, one bool) {
	m := byte(0x80) >> (p % 8)
	if one {
		r[p/8] |= m
	} else {
		r[p/8] &^= m
	}
}

// softPutSmall writes codeword cw of c into the burst r.
func softPutSmall(r *layer2.RawBurst, id softSmallID, cw uint32) {
	c := &softSmallCodes[id]
	for j := range c.n {
		softPut(r, c.pos[j], cw>>j&1 != 0)
	}
}

// softAMBEPN returns the C1 scrambling mask, on the C1 bits, of C0 info
// word v: the C1 bits of the slot that ambe.Encode makes of a frame whose C0
// is v and whose other bits are 0.
func softAMBEPN(v uint32) uint32 {
	var f [7]byte
	f[0], f[1] = byte(v>>(softAMBEC0-8)&0xff), byte(v<<(16-softAMBEC0)&0xff)
	slot := ambe.Encode(f)
	c := &softSmallCodes[softAMBECode+1]
	var m uint32
	for j, p := range c.pos[:c.n] {
		m |= uint32(slot[p/8]>>(7-p%8)&1) << j
	}
	return m
}

// softOSDCode is a binary linear (n, k) block code with n at most 196 and k
// at most 99 for ordered statistics decoding.
type softOSDCode struct {
	// n is the codeword length and k the number of info bits.
	n, k int
	// t is the number of bit errors that the code always corrects.
	t int
	// pos holds the image index of codeword bit j, in increasing order.
	pos [softOSDMaxN]int16
	// rows holds the codeword of each of the k info bits.
	rows [softOSDMaxK][softOSDWords]uint64
	// sys holds a codeword bit that equals info bit i.
	sys [softOSDMaxK]int16
}

// softOSDList is a list of the lowest-cost codewords of a decode, in
// increasing cost order.
type softOSDList struct {
	// n is the number of candidates and max the list size.
	n, max int
	// cost and cw hold candidate i at index i.
	cost [softListMax]int32
	cw   [softListMax][softOSDWords]uint64
	// scored is the number of codewords scored since reset.
	scored int
}

// reset empties the list and sets its size to size, from 1 to softListMax.
func (t *softOSDList) reset(size int) {
	t.n, t.max, t.scored = 0, max(1, min(size, softListMax)), 0
}

// add inserts cw when it is among the max lowest costs and not in the list,
// and reports whether it did.
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

// softOSDScratch is the per-call state of the OSD and Chase decodes.
type softOSDScratch struct {
	// l holds the LLRs of the code bits and mag their magnitudes.
	l, mag [softOSDMaxN]int32
	// order holds the code bits by decreasing reliability.
	order [softOSDMaxN]int16
	// hard holds the hard decisions of the code bits.
	hard [softOSDWords]uint64
	// work holds the generator rows in systematic form on pivot[i].
	work  [softOSDMaxK][softOSDWords]uint64
	pivot [softOSDMaxK]int16
	// tab holds the tables of the LLR magnitudes of the code bits, 4 bits
	// per table.
	tab *[softTabLen]int32
	// list holds the best candidates.
	list softOSDList
	// accept is the check of the decode, nil for none.
	accept func(cw *[softOSDWords]uint64) bool
	// cand holds the candidate codewords of the reprocessing and pmag the
	// LLR magnitudes of the basis bits.
	cand [4][softOSDWords]uint64
	pmag [softOSDMaxK]int32
	// pass is the cost of the best accepted codeword, floor the cost of the
	// hard decisions, and nw the words of a codeword.
	pass, floor int32
	nw          int
	// chase holds the Chase test pattern bits and bits the hard decisions of
	// the 196 information bits.
	chase [softChaseMax]int16
	bits  [softTrellisBits]unpacked.Bit
	// fast selects reprocessFast, and btab, when not nil, holds byte
	// tables of the non-basis bits for it.
	fast bool
	btab *[softParWords * 8 * 256]int32
	// par holds the non-basis code bits in reliability order, prow the
	// work rows and phard the hard decisions on them, bit p for par[p].
	par   [softOSDMaxN]int16
	prow  [softOSDMaxK][softParWords]uint64
	phard [softParWords]uint64
	pcand [4][softParWords]uint64
}

// softParWords is the number of words that hold the non-basis bits of a
// softOSDCode codeword.
const softParWords = 2

// first returns the index of the first candidate in the list that s.accept
// accepts, and false with index 0 when s.accept is nil or accepts none.
func (s *softOSDScratch) first() (int, bool) {
	for i := 0; s.accept != nil && i < s.list.n; i++ {
		if s.accept(&s.list.cw[i]) {
			return i, true
		}
	}
	return 0, false
}

// prepare takes the LLRs in s.l of the n code bits and sets s.hard and s.mag.
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

// cost returns the sum of the LLRs of the 1 bits of cw: s.floor plus the
// LLR magnitudes of the bits where cw differs from the hard decisions, from
// s.tab. It stops once the sum reaches bound and returns the partial sum.
func (s *softOSDScratch) cost(cw *[softOSDWords]uint64, bound int32) int32 {
	sum := s.floor
	for x := range s.nw {
		t := s.tab[256*x:]
		for i, e := 0, cw[x]^s.hard[x]; e != 0; i, e = i+16, e>>4 {
			sum += t[i+int(e&0xf)]
		}
		if sum >= bound {
			break
		}
	}
	return sum
}

// osd decodes the LLRs in s.l with ordered statistics decoding: it reduces
// the generator to the most reliable basis and scores the hard decision of
// the basis with every pattern of up to order flips, and order 3 within the
// window least reliable basis bits. It keeps the best listMax codewords in
// s.list (Fossorier and Lin, IEEE Trans. Inf. Theory 41(5), 1995). A pattern
// whose flipped basis bits alone cost more than the worst listed codeword,
// or than the best codeword that s.accept accepts, cannot change the first
// accepted codeword of the list, so it is skipped.
func (id softOSDID) osd(s *softOSDScratch, order, window, listMax int) {
	n := softOSDCodes[id].n
	s.prepare(n)
	if s.fast {
		id.basis(s)
		id.reprocessFast(s, order, window, listMax)
		return
	}
	softNibbleTables(s.tab, s.mag[:n])
	id.basis(s)
	id.reprocess(s, order, window, listMax)
}

// reprocessFast scores the same patterns as reprocess and keeps the same
// list. A pattern flips its basis bits and so costs their magnitudes, and
// only the non-basis bits need the tables: it scores those, packed into
// softParWords words with the most reliable first.
func (id softOSDID) reprocessFast(s *softOSDScratch, order, window, listMax int) {
	c := &softOSDCodes[id]
	k, np := c.k, c.n-c.k
	s.nw = (c.n + 63) / 64
	var basis [softOSDWords]uint64
	for i := range k {
		j := s.pivot[i]
		basis[j>>6] |= 1 << (j & 63)
	}
	p := 0
	var pm [softOSDMaxN]int32
	for _, j := range s.order[:c.n] {
		if basis[j>>6]>>(j&63)&1 == 0 {
			s.par[p], pm[p] = j, s.mag[j]
			p++
		}
	}
	if s.btab != nil {
		softByteTables(s.btab, pm[:np])
	} else {
		softNibbleTables(s.tab, pm[:np])
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
	s.tryFast(p0, 0, -1, -1, -1)
	m := &s.pmag
	for i := range k {
		m[i] = s.mag[s.pivot[i]]
	}
	pi, pj, px := &s.pcand[1], &s.pcand[2], &s.pcand[3]
	for i := k - 1; order >= 1 && i >= 0 && m[i] < s.thr(); i-- {
		s.pflip(pi, p0, i)
		s.tryFast(pi, m[i], i, -1, -1)
	}
	for j := k - 1; order >= 2 && j >= 1 && m[j]+m[j-1] < s.thr(); j-- {
		s.pflip(pj, p0, j)
		for i := j - 1; i >= 0 && m[i]+m[j] < s.thr(); i-- {
			s.pflip(pi, pj, i)
			s.tryFast(pi, m[i]+m[j], j, i, -1)
		}
	}
	lo := max(0, k-window)
	for a := k - 1; order >= 3 && a >= lo+2 && m[a]+m[a-1]+m[a-2] < s.thr(); a-- {
		s.pflip(pj, p0, a)
		for b := a - 1; b >= lo+1 && m[a]+m[b]+m[b-1] < s.thr(); b-- {
			s.pflip(pi, pj, b)
			for x := b - 1; x >= lo && m[a]+m[b]+m[x] < s.thr(); x-- {
				s.pflip(px, pi, x)
				s.tryFast(px, m[a]+m[b]+m[x], a, b, x)
			}
		}
	}
}

// pflip sets dst to src with packed row i added.
func (s *softOSDScratch) pflip(dst, src *[softParWords]uint64, i int) {
	dst[0], dst[1] = src[0]^s.prow[i][0], src[1]^s.prow[i][1]
}

// tryFast scores the pattern of basis rows a, b and x (-1 for none), whose
// non-basis bits are pc and whose basis bits cost flips, as try scores its
// codeword.
func (s *softOSDScratch) tryFast(pc *[softParWords]uint64, flips int32, a, b, x int) {
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

// basis sorts the code bits by reliability and reduces the generator rows
// into s.work, row i with its only basis bit at s.pivot[i], taking the most
// reliable independent bits first.
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

// reprocess scores the hard decision of the basis and its flip patterns
// into s.list.
func (id softOSDID) reprocess(s *softOSDScratch, order, window, listMax int) {
	c := &softOSDCodes[id]
	k := c.k
	s.nw = (c.n + 63) / 64
	c0, ci, cj, cw := &s.cand[0], &s.cand[1], &s.cand[2], &s.cand[3]
	*c0 = [softOSDWords]uint64{}
	for i := range k {
		if j := s.pivot[i]; s.hard[j>>6]>>(j&63)&1 != 0 {
			s.flip(c0, c0, i)
		}
	}
	s.list.reset(listMax)
	s.pass, s.floor = math.MaxInt32, 0
	for j := range c.n {
		s.floor += min(0, s.l[j])
	}
	s.try(c0)
	m := &s.pmag
	for i := range k {
		m[i] = s.mag[s.pivot[i]]
	}
	for i := k - 1; order >= 1 && i >= 0 && m[i] < s.thr(); i-- {
		s.flip(ci, c0, i)
		s.try(ci)
	}
	for j := k - 1; order >= 2 && j >= 1 && m[j]+m[j-1] < s.thr(); j-- {
		s.flip(cj, c0, j)
		for i := j - 1; i >= 0 && m[i]+m[j] < s.thr(); i-- {
			s.flip(ci, cj, i)
			s.try(ci)
		}
	}
	lo := max(0, k-window)
	for a := k - 1; order >= 3 && a >= lo+2 && m[a]+m[a-1]+m[a-2] < s.thr(); a-- {
		s.flip(cj, c0, a)
		for b := a - 1; b >= lo+1 && m[a]+m[b]+m[b-1] < s.thr(); b-- {
			s.flip(ci, cj, b)
			for x := b - 1; x >= lo && m[a]+m[b]+m[x] < s.thr(); x-- {
				s.flip(cw, ci, x)
				s.try(cw)
			}
		}
	}
}

// flip sets dst to src with work row i added.
func (s *softOSDScratch) flip(dst, src *[softOSDWords]uint64, i int) {
	for x := range s.nw {
		dst[x] = src[x] ^ s.work[i][x]
	}
}

// try scores cw, adds it to the list and checks it when it beats the best
// accepted codeword.
func (s *softOSDScratch) try(cw *[softOSDWords]uint64) {
	bound := int32(math.MaxInt32)
	if s.list.n == s.list.max {
		bound = s.list.cost[s.list.n-1]
	}
	cost := s.cost(cw, bound)
	if s.list.add(cost, cw) && s.accept != nil && cost < s.pass && s.accept(cw) {
		s.pass = cost
	}
}

// thr returns the discrepancy that a pattern must stay under to change the
// first accepted codeword of the list.
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

// errors returns the number of bits where cw differs from the hard decisions.
func (s *softOSDScratch) errors(cw *[softOSDWords]uint64) int {
	e := 0
	for x := range cw {
		e += bits.OnesCount64(cw[x] ^ s.hard[x])
	}
	return e
}

// softBit reports whether bit j of cw is set.
func softBit[T int | int16](cw *[softOSDWords]uint64, j T) bool {
	return cw[j>>6]>>(j&63)&1 != 0
}

// softBPTCResult is a decoded BPTC(196,96) block.
type softBPTCResult struct {
	// Payload holds the 96 data bits, most significant bit first.
	Payload [12]byte
	// Reserved holds R(0) to R(2), R(i) in bit i.
	Reserved uint8
}

// softBPTCOf returns the BPTC data and reserved bits of BPTC(196,96)
// codeword cw.
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

// softEmbLC holds the place of the LC and checksum bits among the info bits
// of the embedded LC code softEmbLCCode.
type softEmbLC struct {
	// lc holds the info bit of LC bit j, most significant bit first.
	lc [softEmbLCBits]int16
	// cs holds the info bit of checksum bit c, bit 0 least significant.
	cs [softEmbLCChecksum]int16
}

// softTrellis holds the rate 3/4 trellis tables.
type softTrellis struct {
	// pos holds the burst position of codeword bit j, in increasing order.
	pos [softTrellisBits]int16
	// step holds the codeword bits of the 4-bit label of step k, label most
	// significant bit first.
	step [softTrellisSteps][4]uint8
	// label holds the label of input tribit in from state s.
	label [softTrellisStates][softTrellisStates]uint8
}

// softDecoder holds the tables and scratch of the soft-decision FEC
// decoders. An LLR is positive when bit 0 is more likely, with a magnitude
// in proportion to the log-likelihood ratio, and the decoders do not depend
// on the scale. The decodes do not allocate. A softDecoder is not safe for
// concurrent use.
type softDecoder struct {
	// AMBECandidates is the number of best C0 codewords that the AMBE+2
	// decode tries with C1. 1 decodes C0 and C1 one after the other.
	AMBECandidates int
	// ChaseBits is the number of least reliable bits, 0 to 16, that the BPTC
	// Chase decode flips, 2^ChaseBits hard decodes.
	ChaseBits int
	// OSDOrder is the OSD order, 0 to 3, and OSDWindow the number of least
	// reliable basis bits that order 3 uses.
	OSDOrder, OSDWindow int
	// GridIters is the number of row and column passes of decodeBPTCGrid,
	// and GridWeight the weight of the extrinsic information that one
	// component passes to the other.
	GridIters  int
	GridWeight float32
	// BPTCList, ShortLCList and EmbLCList are the numbers of best candidates
	// that the BPTC(196,96) decodes with a check, the Short LC decode and the
	// embedded LC decode try. A longer list corrects more blocks and passes
	// more wrong ones through a weak check such as the 5-bit LC checksum.
	BPTCList, ShortLCList, EmbLCList int

	// grid holds the scratch of decodeBPTCGrid, small and osd the scratch
	// of the other decodes, tab their cost tables, and prev the Viterbi
	// survivors.
	grid  softGrid
	small softSmallScratch
	osd   softOSDScratch
	tab   [softTabLen]int32
	prev  [softTrellisSteps][softTrellisStates]uint8
	// gridPost holds the a posteriori ratios of decodeBPTCGrid.
	gridPost [softBurstBits]int16
	// check is the BPTC check of the current decode, and acceptBPTC and
	// acceptShortLC are the checks of the OSD codewords.
	check                     func([12]byte) bool
	acceptBPTC, acceptShortLC func(cw *[softOSDWords]uint64) bool
}

// setFast sets the OSDFast and OSDBytes settings of opt.
func (d *softDecoder) setFast(opt *options) {
	d.osd.fast = opt.OSDFast
	if opt.OSDFast && opt.OSDBytes {
		d.osd.btab = new([softParWords * 8 * 256]int32)
	}
}

// newSoftDecoder returns a decoder over the code tables that the library
// encoders give (tables_gen.go).
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

// softTACT returns the TACT of info word info: AT in bit 3, TC in bit 2 and
// the LCSS in bits 1 and 0.
func softTACT(info uint32) pdu.TACT {
	return pdu.TACT{AccessType: info&8 != 0, TDMAChannel: info&4 != 0, LCSS: enums.LCSS(info & 3)}
}

// decodeAMBE decodes the three AMBE+2 frames of a voice burst from its 264
// LLRs. C0 and C1 are soft ML; C2 is hard. The AMBECandidates best C0
// codewords are tried with their C1 scrambling, and the pair with the lowest
// total cost wins; a C0 codeword is skipped once no C1 can beat the best
// pair, so the result is the joint ML pair whenever the list is long
// enough. When out is not nil, the corrected bits are written into it.
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
		c0, c1 := &softSmallCodes[id0], &softSmallCodes[id1]
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

// decodeSlotType decodes the Golay(20,8) slot type of a burst from its 264
// LLRs by scoring all 256 codewords, or the 16 of color code cc when
// ccKnown. When out is not nil, the corrected codeword is written into it.
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

// decodeEMB decodes the QR(16,7) EMB of a burst from its 264 LLRs by scoring
// all 128 codewords, or the 8 of color code cc when ccKnown. When out is not
// nil, the corrected codeword is written into it.
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

// decodeTACT decodes the Hamming(7,4) TACT of a CACH from its 24 LLRs in
// transmit order by scoring all 16 codewords. When out is not nil, the
// corrected TACT bits are written into it.
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

// softRate34Payload packs 48 tribits into the 144 data bits.
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

// decodeRate34 decodes a rate 3/4 trellis block from the 264 LLRs of its
// burst with an 8-state soft-decision Viterbi decoder that starts and ends
// in state 0, which is maximum likelihood. Uncorrectable is set when a path
// that ends in another state has a lower cost, as trellis.Decode does. When
// out is not nil, the codeword of the decoded data is written into it.
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

// bptcLLRs copies the LLRs of the BPTC(196,96) bits of a burst into the
// scratch.
func (d *softDecoder) bptcLLRs(llr *[softBurstBits]int16) {
	c := &softOSDCodes[softBPTCCode]
	for j := range c.n {
		d.osd.l[j] = int32(llr[c.pos[j]])
	}
}

// bptcPick returns the first candidate in the scratch list that check
// accepts, or the first candidate when check is nil or accepts none, and
// writes its codeword into out when out is not nil.
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

// bptcCheck sets the check of a BPTC decode and returns its list size.
func (d *softDecoder) bptcCheck(check func([12]byte) bool) int {
	d.check = check
	if check == nil {
		d.osd.accept = nil
		return 1
	}
	d.osd.accept = d.acceptBPTC
	return d.BPTCList
}

// decodeBPTCOSD decodes a BPTC(196,96) block from the 264 LLRs of its burst
// with ordered statistics decoding of order OSDOrder. With a check, it keeps
// the BPTCList best codewords and returns the best one that check accepts,
// with Uncorrectable set when check accepts none. When out is not nil, the
// corrected codeword is written into it.
func (d *softDecoder) decodeBPTCOSD(llr *[softBurstBits]int16, check func([12]byte) bool, out *layer2.RawBurst) (softBPTCResult, fec.Result) {
	if d.bptcHard(llr, check) {
		return d.bptcPick(check, out)
	}
	d.bptcLLRs(llr)
	softBPTCCode.osd(&d.osd, d.OSDOrder, d.OSDWindow, d.bptcCheck(check))
	return d.bptcPick(check, out)
}

// bptcHard reports whether the hard decisions of the BPTC(196,96) bits of
// the 264 LLRs llr form a codeword that check accepts, any codeword when
// check is nil, and then leaves it as the one candidate for bptcPick. No
// other codeword costs as little, so every soft decode picks it too.
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

// decodeBPTCGrid decodes a BPTC(196,96) block from the 264 LLRs of its
// burst by GridIters passes of row and column decoding of the grid, then
// by ordered statistics from the a posteriori ratios. It corrects blocks
// with confidently wrong bits, which ordered statistics alone takes into
// its most reliable basis. check and out work as in decodeBPTCOSD.
func (d *softDecoder) decodeBPTCGrid(llr *[softBurstBits]int16, check func([12]byte) bool, out *layer2.RawBurst) (softBPTCResult, fec.Result) {
	d.grid.decode(llr, d.GridIters, d.GridWeight)
	d.gridPost = [softBurstBits]int16{}
	d.grid.posterior(llr, &d.gridPost)
	return d.decodeBPTCOSD(&d.gridPost, check, out)
}

// bptcExtrinsic sets ext to the extrinsic log-likelihood ratios, in nats,
// of the BPTC(196,96) bits of a burst from its 264 LLRs, by GridIters
// passes of the grid decode, and to 0 elsewhere.
func (d *softDecoder) bptcExtrinsic(llr *[softBurstBits]int16, ext *[softBurstBits]float32) {
	d.grid.decode(llr, d.GridIters, d.GridWeight)
	d.grid.extrinsic(ext)
}

// decodeBPTCChase decodes a BPTC(196,96) block from the 264 LLRs of its
// burst with a Chase-II decoder: it flips every pattern of the ChaseBits
// least reliable bits, decodes each with the library hard decoder and keeps
// the codeword with the lowest soft cost. It needs no cost tables, so it
// suits a small MCU. check works as in decodeBPTCOSD.
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

// softBPTCCheck returns the check of a BPTC(196,96) payload of data type dt
// for decodeBPTCOSD and decodeBPTCChase: the RS(12,9) parity of a Full LC,
// as pdu.FullLCParityOK checks it, or the CRC-CCITT of a single block, each
// with the mask of dt. It returns nil for a data type without either.
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

// decodeShortLC decodes a Short LC from the LLRs of the four CACH fields
// that carry it, in transmit order, with ordered statistics decoding of
// order OSDOrder over BPTC(68,36). It keeps the ShortLCList best codewords and
// returns the 36 info bits (28 data bits and the CRC-8) of the best one
// whose CRC-8 matches, with Uncorrectable set when none does. When out is
// not nil, the corrected payload bits are written into it.
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

// softShortLCOf returns the 36 Short LC info bits of codeword cw.
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

// decodeEmbeddedLC decodes an embedded LC from the 264 LLRs of each of the
// four voice bursts B to E that carry it, with ordered statistics decoding
// of order OSDOrder over BPTC(128,77). It keeps the EmbLCList best codewords
// and returns the LC of the best one whose 5-bit checksum matches, with
// Uncorrectable set when none does. When out is not nil, the corrected
// embedded signalling fields are written into its bursts.
func (d *softDecoder) decodeEmbeddedLC(llr *[4][softBurstBits]int16, out *[4]layer2.RawBurst) ([9]byte, fec.Result) {
	var frags [4][embBits]int16
	for i := range frags {
		copy(frags[i][:], llr[i][embFirstBit:embFirstBit+embBits])
	}
	return d.decodeEmbeddedFrags(&frags, out)
}

// decodeEmbeddedFrags decodes an embedded LC as decodeEmbeddedLC does,
// from the LLRs of the embedded signalling fragment of each burst only.
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

// softEmbLCAccept reports whether the 5-bit checksum of embedded LC
// codeword cw matches its LC.
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

// softEmbLCOf returns the 9 LC octets of embedded LC codeword cw.
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

// softCombine adds the LLRs of src to dst, saturated to the int16 range, to
// combine repeats of one codeword, such as identical voice LC headers or the
// embedded LC of consecutive superframes.
func softCombine(dst, src []int16) {
	for i := range min(len(dst), len(src)) {
		dst[i] = int16(max(math.MinInt16, min(math.MaxInt16, int32(dst[i])+int32(src[i]))))
	}
}
