package rf

import (
	"encoding/binary"
	"errors"
	"fmt"
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

// errSoftDerive is returned when a code table cannot be derived from the
// library encoders.
var errSoftDerive = errors.New("soft FEC: cannot derive code tables")

// softEncoder returns the field image of info word info: the bits of the
// burst, or of the fields, that carry its codeword.
type softEncoder func(info uint32) ([]unpacked.Bit, error)

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

// newSoftSmallCode derives a code from enc by encoding the zero info word and
// each unit vector. Image positions where skip is set are not code bits.
func newSoftSmallCode(k, t int, enc softEncoder, skip []bool) (softSmallCode, error) {
	c := softSmallCode{k: k, t: t}
	zero, err := enc(0)
	if err != nil {
		return c, err
	}
	imgs := make([][]unpacked.Bit, k)
	for i := range k {
		if imgs[i], err = enc(uint32(1) << i); err != nil {
			return c, err
		}
	}
	for p := range zero {
		if skip != nil && skip[p] {
			continue
		}
		for i := range k {
			if imgs[i][p] == zero[p] {
				continue
			}
			if c.n == softSmallMaxN {
				return c, fmt.Errorf("%w: more than 24 code bits", errSoftDerive)
			}
			c.pos[c.n] = int16(p)
			c.n++
			break
		}
	}
	c.base = softMask(zero, c.pos[:c.n])
	for i := range k {
		c.rows[i] = softMask(imgs[i], c.pos[:c.n]) ^ c.base
	}
	return c, nil
}

// softMask returns the bits of img at pos, pos[j] at 1<<j.
func softMask[T int | int16](img []unpacked.Bit, pos []T) uint32 {
	var m uint32
	for j, p := range pos {
		if img[p]&1 != 0 {
			m |= 1 << j
		}
	}
	return m
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

// gather copies the LLRs of the code bits into l and returns the hard
// decision codeword.
func (c *softSmallCode) gather(llr []int16, l *[softSmallMaxN]int32) uint32 {
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
func (c *softSmallCode) search(tab *[softTabLen]int32, off, fixed, want uint32, top *softTop) {
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
func (s *softSmallScratch) decode(c *softSmallCode, llr []int16, fixed, want uint32) (info, cw uint32, errs int) {
	hard := c.gather(llr, &s.l)
	softTables(s.tab, s.l[:c.n])
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
func softPutSmall(r *layer2.RawBurst, c *softSmallCode, cw uint32) {
	for j := range c.n {
		softPut(r, c.pos[j], cw>>j&1 != 0)
	}
}

// softBurstImage returns the bits of r as a field image.
func softBurstImage(r *layer2.RawBurst) []unpacked.Bit {
	var b [264]unpacked.Bit
	unpacked.UnpackAt(b[:], r[:], 0)
	return b[:]
}

// softAMBESlot holds the codes of one of the three voice slots of a burst.
type softAMBESlot struct {
	// c0 is the Golay(24,12) code C0 and c1 the Golay(23,12) code C1 without
	// its scrambling. Info bit 11-i is frame bit i, and frame bit 12+i for C1.
	c0, c1 softSmallCode
	// c2 holds the burst position of uncoded frame bit 24+i.
	c2 [softAMBEC2]int16
}

// softAMBE holds the tables of the AMBE+2 frame decoder.
type softAMBE struct {
	// slot holds the codes of voice slot i.
	slot [layer2.VoiceSlotsPerBurst]softAMBESlot
}

// softAMBEEncoder returns the encoder of a frame in voice slot s whose bits
// at from to from+11 hold info word info, with frame bit from at bit 11.
func softAMBEEncoder(s, from int) softEncoder {
	return func(info uint32) ([]unpacked.Bit, error) {
		var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
		for i := range softAMBEC0 {
			if info>>(softAMBEC0-1-i)&1 != 0 {
				frames[s][(from+i)/8] |= 0x80 >> ((from + i) % 8)
			}
		}
		var r layer2.RawBurst
		if err := r.SetAMBEFrames(frames); err != nil {
			return nil, err
		}
		return softBurstImage(&r), nil
	}
}

// newSoftAMBE derives the AMBE+2 tables from layer2.RawBurst.SetAMBEFrames.
func newSoftAMBE() (*softAMBE, error) {
	a := &softAMBE{}
	for s := range a.slot {
		sl := &a.slot[s]
		var err error
		if sl.c1, err = newSoftSmallCode(softAMBEC0, 3, softAMBEEncoder(s, softAMBEC0), nil); err != nil {
			return nil, err
		}
		sl.c1.base = 0
		skip := make([]bool, softBurstBits)
		for j := range sl.c1.n {
			skip[sl.c1.pos[j]] = true
		}
		var zero layer2.RawBurst
		if err := zero.SetAMBEFrames([layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame{}); err != nil {
			return nil, err
		}
		zimg := softBurstImage(&zero)
		for i := range softAMBEC2 {
			var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
			frames[s][(2*softAMBEC0+i)/8] |= 0x80 >> ((2*softAMBEC0 + i) % 8)
			var r layer2.RawBurst
			if err := r.SetAMBEFrames(frames); err != nil {
				return nil, err
			}
			img := softBurstImage(&r)
			n := 0
			for p := range img {
				if img[p] != zimg[p] {
					sl.c2[i], skip[p] = int16(p), true
					n++
				}
			}
			if n != 1 {
				return nil, fmt.Errorf("%w: AMBE+2 bit %d is not one burst bit", errSoftDerive, 2*softAMBEC0+i)
			}
		}
		if sl.c0, err = newSoftSmallCode(softAMBEC0, 3, softAMBEEncoder(s, 0), skip); err != nil {
			return nil, err
		}
		if sl.c0.n != 24 || sl.c1.n != 23 {
			return nil, fmt.Errorf("%w: AMBE+2 C0 has %d bits and C1 %d", errSoftDerive, sl.c0.n, sl.c1.n)
		}
	}
	for _, p := range a.slot[0].c1.pos[:a.slot[0].c1.n] {
		if int(p) >= 8*len(ambe.Encode([7]byte{})) {
			return nil, fmt.Errorf("%w: AMBE+2 C1 bit %d is not in voice slot 0", errSoftDerive, p)
		}
	}
	return a, nil
}

// pn returns the C1 scrambling mask, on the C1 bits, of C0 info word v: the
// C1 bits of the slot that ambe.Encode makes of a frame whose C0 is v and
// whose other bits are 0.
func (a *softAMBE) pn(v uint32) uint32 {
	var f [7]byte
	f[0], f[1] = byte(v>>(softAMBEC0-8)&0xff), byte(v<<(16-softAMBEC0)&0xff)
	slot := ambe.Encode(f)
	c := &a.slot[0].c1
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
	rows [][softOSDWords]uint64
	// sys holds a codeword bit that equals info bit i.
	sys [softOSDMaxK]int16
}

// newSoftOSDCode derives a linear code from enc, where enc(0) encodes the
// zero info word, which must give the zero codeword, and enc(i+1) encodes
// unit vector i.
func newSoftOSDCode(k, t int, enc softEncoder) (*softOSDCode, error) {
	c := &softOSDCode{k: k, t: t, rows: make([][softOSDWords]uint64, k)}
	zero, err := enc(0)
	if err != nil {
		return nil, err
	}
	imgs := make([][]unpacked.Bit, k)
	for i := range k {
		if imgs[i], err = enc(uint32(i) + 1); err != nil {
			return nil, err
		}
	}
	for p := range zero {
		if zero[p] != 0 {
			return nil, fmt.Errorf("%w: the zero word does not encode to zero", errSoftDerive)
		}
		for i := range k {
			if imgs[i][p] == 0 {
				continue
			}
			if c.n == softOSDMaxN {
				return nil, fmt.Errorf("%w: more than 196 code bits", errSoftDerive)
			}
			c.pos[c.n] = int16(p)
			c.n++
			break
		}
	}
	var weight [softOSDMaxN]int
	for i := range k {
		for j := range c.n {
			if imgs[i][c.pos[j]] != 0 {
				c.rows[i][j>>6] |= 1 << (j & 63)
				weight[j]++
			}
		}
	}
	for i := range k {
		c.sys[i] = -1
		for j := range c.n {
			if weight[j] == 1 && c.rows[i][j>>6]>>(j&63)&1 != 0 {
				c.sys[i] = int16(j)
				break
			}
		}
		if c.sys[i] < 0 {
			return nil, fmt.Errorf("%w: info bit %d is not systematic", errSoftDerive, i)
		}
	}
	return c, nil
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
}

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
func (c *softOSDCode) osd(s *softOSDScratch, order, window, listMax int) {
	s.prepare(c.n)
	softNibbleTables(s.tab, s.mag[:c.n])
	c.basis(s)
	c.reprocess(s, order, window, listMax)
}

// basis sorts the code bits by reliability and reduces the generator rows
// into s.work, row i with its only basis bit at s.pivot[i], taking the most
// reliable independent bits first.
func (c *softOSDCode) basis(s *softOSDScratch) {
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
func (c *softOSDCode) reprocess(s *softOSDScratch, order, window, listMax int) {
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

// softBPTCEncoder encodes info bit i of BPTC(196,96) into a burst: data bit
// i for i below 96 and R(i-96) above. The info word is the bit number plus
// one, and 0 encodes the zero block.
func softBPTCEncoder(info uint32) ([]unpacked.Bit, error) {
	var r layer2.RawBurst
	if info == 0 {
		r.SetBPTCPayload([12]byte{})
		return softBurstImage(&r), nil
	}
	i := int(info) - 1
	if i < softBPTCData {
		var p [12]byte
		p[i/8] = 0x80 >> (i % 8)
		r.SetBPTCPayload(p)
		return softBurstImage(&r), nil
	}
	r.SetBPTCPayload([12]byte{})
	if err := r.SetBPTCReserved(1 << (i - softBPTCData)); err != nil {
		return nil, err
	}
	return softBurstImage(&r), nil
}

// bptcResult returns the BPTC data and reserved bits of codeword cw.
func (c *softOSDCode) bptcResult(cw *[softOSDWords]uint64) softBPTCResult {
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

// softShortLCEncoder encodes Short LC info word info (0 or unit vector
// info-1) into the 96 bits of four CACH fields in transmit order.
func softShortLCEncoder(info uint32) ([]unpacked.Bit, error) {
	var in [softShortLCInfo]unpacked.Bit
	if i := int(info) - 1; i >= 0 && i < len(in) {
		in[i] = 1
	}
	frags := bptc.EncodeCACH(unpacked.Uint(in[:]))
	img := make([]unpacked.Bit, 0, 4*layer2.CACHBits)
	for _, f := range frags {
		var c layer2.CACH
		c.Payload = cachPayload(f)
		logical, err := c.Encode()
		if err != nil {
			return nil, err
		}
		onAir := cachBits(layer2.CACHInterleave(logical))
		img = append(img, onAir[:]...)
	}
	return img, nil
}

// softEmbFragments returns the image of four embedded signalling fields in
// four bursts, burst i at 264i.
func softEmbFragments(frags [4][4]byte) []unpacked.Bit {
	img := make([]unpacked.Bit, 0, 4*softBurstBits)
	for _, f := range frags {
		var r layer2.RawBurst
		r.SetEmbeddedData(f)
		img = append(img, softBurstImage(&r)...)
	}
	return img
}

// softEmbLCEncoder encodes embedded LC info word info (0 or unit vector
// info-1 of the 77 BPTC info bits) into the image of four bursts.
func softEmbLCEncoder(info uint32) ([]unpacked.Bit, error) {
	var in [softEmbLCInfo]unpacked.Bit
	if i := int(info) - 1; i >= 0 && i < len(in) {
		in[i] = 1
	}
	var packed [10]byte
	copy(packed[:], unpacked.Pack(in[:]))
	var frags [4][4]byte
	for i, f := range bptc.EncodeEmbedded(packed) {
		binary.BigEndian.PutUint32(frags[i][:], f)
	}
	return softEmbFragments(frags), nil
}

// softEmbLC holds the embedded LC code and the place of the LC and
// checksum bits among its info bits.
type softEmbLC struct {
	// code is the BPTC(128,77) code over four bursts, burst i at 264i.
	code *softOSDCode
	// lc holds the info bit of LC bit j, most significant bit first.
	lc [softEmbLCBits]int16
	// cs holds the info bit of checksum bit c, bit 0 least significant.
	cs [softEmbLCChecksum]int16
}

// newSoftEmbLC derives the embedded LC code from bptc.EncodeEmbedded and the
// LC and checksum layout from layer2.EncodeEmbeddedLC.
func newSoftEmbLC() (*softEmbLC, error) {
	code, err := newSoftOSDCode(softEmbLCInfo, 3, softEmbLCEncoder)
	if err != nil {
		return nil, err
	}
	for _, p := range code.pos[:code.n] {
		if b := int(p) % softBurstBits; b < embFirstBit || b >= embFirstBit+embBits {
			return nil, fmt.Errorf("%w: embedded LC bit at burst bit %d", errSoftDerive, b)
		}
	}
	e := &softEmbLC{code: code}
	infoOf := func(lc [9]byte) []int {
		img := softEmbFragments(layer2.EncodeEmbeddedLC(lc))
		var set []int
		for i := range code.k {
			if img[code.pos[code.sys[i]]] != 0 {
				set = append(set, i)
			}
		}
		return set
	}
	var count [softEmbLCInfo]int
	sets := make([][]int, softEmbLCBits)
	for j := range softEmbLCBits {
		var lc [9]byte
		lc[j/8] = 0x80 >> (j % 8)
		sets[j] = infoOf(lc)
		for _, i := range sets[j] {
			count[i]++
		}
	}
	for j, set := range sets {
		e.lc[j] = -1
		for _, i := range set {
			if count[i] == 1 {
				e.lc[j] = int16(i & 0x7f)
			}
		}
		if e.lc[j] < 0 {
			return nil, fmt.Errorf("%w: LC bit %d has no info bit", errSoftDerive, j)
		}
	}
	for c := range softEmbLCChecksum {
		j := softEmbLCBits - 1 - c
		e.cs[c] = -1
		for _, i := range sets[j] {
			if i != int(e.lc[j]) {
				e.cs[c] = int16(i & 0x7f)
			}
		}
		if e.cs[c] < 0 || len(sets[j]) != 2 {
			return nil, fmt.Errorf("%w: checksum bit %d has no info bit", errSoftDerive, c)
		}
	}
	return e, nil
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

// softTrellisImage returns the burst image of the 48 tribits t, tribit k
// holding data bits 3k to 3k+2 most significant bit first.
func softTrellisImage(t *[softTrellisSteps - 1]int) []unpacked.Bit {
	var p [18]byte
	for k, v := range t {
		for b := range 3 {
			if v>>(2-b)&1 != 0 {
				i := 3*k + b
				p[i/8] |= 0x80 >> (i % 8)
			}
		}
	}
	var r layer2.RawBurst
	r.SetRate34Payload(p)
	return softBurstImage(&r)
}

// newSoftTrellis derives the trellis tables from
// layer2.RawBurst.SetRate34Payload. pos holds the burst positions of the 196
// information bits, as the BPTC(196,96) code finds them: one trellis bit is 1
// in every codeword, so the trellis images alone do not find it.
func newSoftTrellis(pos *[softTrellisBits]int16) (*softTrellis, error) {
	tr := &softTrellis{pos: *pos}
	pairs, err := tr.pairs()
	if err != nil {
		return nil, err
	}
	for s := range softTrellisStates {
		for in := range softTrellisStates {
			tr.label[s][in] = uint8(tr.read(pairs, 1, false, s, in) & 0xf)
		}
	}
	for k := range softTrellisSteps {
		if !tr.order(pairs, k) {
			return nil, fmt.Errorf("%w: trellis step %d", errSoftDerive, k)
		}
	}
	return tr, nil
}

// pairs returns the two dibits, as indexes of codeword bit pairs, of each
// trellis step. Tribit k changes the dibits of steps k and k+1.
func (tr *softTrellis) pairs() (*[softTrellisSteps][2]int, error) {
	var t [softTrellisSteps - 1]int
	zero := softTrellisImage(&t)
	var changed [softTrellisSteps - 1][softTrellisBits / 2]bool
	for k := range t {
		for v := 1; v < softTrellisStates; v++ {
			t[k] = v
			img := softTrellisImage(&t)
			t[k] = 0
			for j, p := range tr.pos {
				if img[p] != zero[p] {
					changed[k][j/2] = true
				}
			}
		}
	}
	var none [softTrellisBits / 2]bool
	at := func(k int) *[softTrellisBits / 2]bool {
		if k >= 0 && k < len(changed) {
			return &changed[k]
		}
		return &none
	}
	var pairs [softTrellisSteps][2]int
	for k := range softTrellisSteps {
		m := 0
		for q := range softTrellisBits / 2 {
			var in bool
			switch k {
			case 0:
				in = at(0)[q] && !at(1)[q]
			case softTrellisSteps - 1:
				in = at(k - 1)[q] && !at(k - 2)[q]
			default:
				in = at(k)[q] && at(k - 1)[q]
			}
			if !in {
				continue
			}
			if m == 2 {
				return nil, fmt.Errorf("%w: trellis step %d has more than 2 dibits", errSoftDerive, k)
			}
			pairs[k][m] = q
			m++
		}
		if m != 2 {
			return nil, fmt.Errorf("%w: trellis step %d has %d dibits", errSoftDerive, k, m)
		}
	}
	return &pairs, nil
}

// stepBits returns the codeword bits of step k, the dibits of pairs in
// order, or swapped.
func stepBits(pairs *[softTrellisSteps][2]int, k int, swap bool) [4]uint8 {
	a, b := uint8(pairs[k][0]&0x7f), uint8(pairs[k][1]&0x7f)
	if swap {
		a, b = b, a
	}
	return [4]uint8{2 * a, 2*a + 1, 2 * b, 2*b + 1}
}

// read encodes tribit in after tribit s at step k, every other tribit 0,
// and returns the bits of the step as a 4-bit label.
func (tr *softTrellis) read(pairs *[softTrellisSteps][2]int, k int, swap bool, s, in int) int {
	var t [softTrellisSteps - 1]int
	for i := range t {
		switch i {
		case k - 1:
			t[i] = s
		case k:
			t[i] = in
		}
	}
	img := softTrellisImage(&t)
	l := 0
	for _, j := range stepBits(pairs, k, swap) {
		l <<= 1
		if img[tr.pos[j]] != 0 {
			l |= 1
		}
	}
	return l
}

// order sets the step bits of step k to the dibit order whose labels match
// tr.label, and reports whether one does. Step 0 starts in state 0 and the
// tail step 48 has input 0.
func (tr *softTrellis) order(pairs *[softTrellisSteps][2]int, k int) bool {
	for _, swap := range [2]bool{false, true} {
		ok := true
		for s := range softTrellisStates {
			for in := range softTrellisStates {
				if (k == 0 && s != 0) || (k == softTrellisSteps-1 && in != 0) {
					continue
				}
				ok = ok && tr.read(pairs, k, swap, s, in) == int(tr.label[s][in])
			}
		}
		if ok {
			tr.step[k] = stepBits(pairs, k, swap)
			return true
		}
	}
	return false
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

	// ambe to trellis hold the code tables, built once by newSoftDecoder.
	ambe     *softAMBE
	slotType softSmallCode
	emb      softSmallCode
	tact     softSmallCode
	bptc     *softOSDCode
	shortLC  *softOSDCode
	embLC    *softEmbLC
	trellis  *softTrellis
	grid     *softGrid
	// info holds the burst positions of the 196 information bits, and
	// bptcInfo the information bit of BPTC(196,96) codeword bit j, which has
	// no R(3).
	info     [softTrellisBits]int16
	bptcInfo [softOSDMaxN]uint8
	// small and osd hold the scratch of the decodes, tab their cost tables,
	// and prev the Viterbi survivors.
	small softSmallScratch
	osd   softOSDScratch
	tab   [softTabLen]int32
	prev  [softTrellisSteps][softTrellisStates]uint8
	// gridPost holds the a posteriori ratios of decodeBPTCGrid.
	gridPost [softBurstBits]int16
	// check is the BPTC check of the current decode, and acceptBPTC,
	// acceptShortLC and acceptEmbLC are the checks of the OSD codewords.
	check                                  func([12]byte) bool
	acceptBPTC, acceptShortLC, acceptEmbLC func(cw *[softOSDWords]uint64) bool
}

// newSoftDecoder derives every code table from the library encoders. It
// returns an error only when a library encoder changes its layout.
func newSoftDecoder() (*softDecoder, error) {
	d := &softDecoder{AMBECandidates: 4, ChaseBits: 6, OSDOrder: 2, BPTCList: softListMax, ShortLCList: 4, EmbLCList: 2, GridIters: 4, GridWeight: 0.7}
	d.small.tab, d.osd.tab = &d.tab, &d.tab
	var err error
	if d.ambe, err = newSoftAMBE(); err != nil {
		return nil, err
	}
	if d.embLC, err = newSoftEmbLC(); err != nil {
		return nil, err
	}
	if d.bptc, err = newSoftOSDCode(softBPTCInfo, 4, softBPTCEncoder); err != nil {
		return nil, err
	}
	if err = d.infoBits(); err != nil {
		return nil, err
	}
	if d.trellis, err = newSoftTrellis(&d.info); err != nil {
		return nil, err
	}
	if d.grid, err = newSoftGrid(&d.info, d.bptc); err != nil {
		return nil, err
	}
	if d.shortLC, err = newSoftOSDCode(softShortLCInfo, 2, softShortLCEncoder); err != nil {
		return nil, err
	}
	if d.slotType, err = newSoftSmallCode(8, 3, func(info uint32) ([]unpacked.Bit, error) {
		var r layer2.RawBurst
		if err := r.SetSlotType(pdu.SlotTypeInfo(info & 0xff).SlotType()); err != nil {
			return nil, err
		}
		return softBurstImage(&r), nil
	}, nil); err != nil {
		return nil, err
	}
	if d.emb, err = newSoftSmallCode(7, 2, func(info uint32) ([]unpacked.Bit, error) {
		var r layer2.RawBurst
		if err := r.SetEMB(pdu.EMBInfo(info << 1 & 0xfe).EmbeddedSignalling()); err != nil {
			return nil, err
		}
		return softBurstImage(&r), nil
	}, nil); err != nil {
		return nil, err
	}
	if d.tact, err = newSoftSmallCode(4, 1, func(info uint32) ([]unpacked.Bit, error) {
		c := layer2.CACH{TACT: softTACT(info)}
		logical, err := c.Encode()
		if err != nil {
			return nil, err
		}
		onAir := cachBits(layer2.CACHInterleave(logical))
		return onAir[:], nil
	}, nil); err != nil {
		return nil, err
	}
	if d.bptc.n != softOSDMaxN-1 || d.shortLC.n != 68 || d.embLC.code.n != 128 ||
		d.slotType.n != 20 || d.emb.n != 16 || d.tact.n != 7 {
		return nil, fmt.Errorf("%w: unexpected code lengths", errSoftDerive)
	}
	d.acceptBPTC = func(cw *[softOSDWords]uint64) bool {
		return d.check(d.bptc.bptcResult(cw).Payload)
	}
	d.acceptShortLC = func(cw *[softOSDWords]uint64) bool {
		info := d.shortLC.shortLCInfo(cw)
		var data [(softShortLCData + 7) / 8]byte
		for i, v := range info[:softShortLCData] {
			data[i/8] |= byte(v&1) << (7 - i%8)
		}
		return crc.CheckCRC8(data[:], softShortLCData, uint8(unpacked.Uint(info[softShortLCData:])&0xff))
	}
	d.acceptEmbLC = d.embLC.accept
	return d, nil
}

// infoBits sets d.info to the burst bits that BPTC(196,96) or a Rate 1 block
// use, and d.bptcInfo to the place of each BPTC bit among them.
func (d *softDecoder) infoBits() error {
	var used [softBurstBits]bool
	for j := range d.bptc.n {
		used[d.bptc.pos[j]] = true
	}
	for i := range 24 * 8 {
		var p [24]byte
		p[i/8] = 0x80 >> (i % 8)
		var r layer2.RawBurst
		r.SetRate1Payload(p)
		for q, b := range softBurstImage(&r) {
			used[q] = used[q] || b != 0
		}
	}
	n := 0
	for q, u := range used {
		if !u {
			continue
		}
		if n == softTrellisBits {
			return fmt.Errorf("%w: more than 196 information bits", errSoftDerive)
		}
		d.info[n] = int16(q)
		for j := range d.bptc.n {
			if d.bptc.pos[j] == int16(q) {
				d.bptcInfo[j] = uint8(n)
			}
		}
		n++
	}
	if n != softTrellisBits {
		return fmt.Errorf("%w: %d information bits", errSoftDerive, n)
	}
	return nil
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
	for i := range frames {
		sl := &d.ambe.slot[i]
		hard0 := sl.c0.gather(llr[:], &s.l)
		softTables(s.tab, s.l[:sl.c0.n])
		s.top.reset(d.AMBECandidates)
		sl.c0.search(s.tab, 0, 0, 0, &s.top)
		hard1 := sl.c1.gather(llr[:], &s.l)
		softTables(s.tab, s.l[:sl.c1.n])
		var bound int32
		for j := range sl.c1.n {
			bound += min(0, s.l[j])
		}
		best := int32(softInf)
		var info0, info1, cw0, cw1 uint32
		for c := range s.top.n {
			if s.top.cost[c]+bound >= best {
				break
			}
			s.top1.reset(1)
			sl.c1.search(s.tab, d.ambe.pn(s.top.info[c]), 0, 0, &s.top1)
			if total := s.top.cost[c] + s.top1.cost[0]; total < best {
				best = total
				info0, cw0 = s.top.info[c], s.top.cw[c]
				info1, cw1 = s.top1.info[0], s.top1.cw[0]
			}
		}
		v := uint64(info0)<<(softAMBEC0+softAMBEC2) | uint64(info1)<<softAMBEC2
		for b, p := range sl.c2 {
			if llr[p] < 0 {
				v |= 1 << (softAMBEC2 - 1 - b)
			}
		}
		for k := range frames[i] {
			frames[i][k] = byte(v << 7 >> (48 - 8*k) & 0xFF)
		}
		e0, e1 := bits.OnesCount32(cw0^hard0), bits.OnesCount32(cw1^hard1)
		res = res.Combine(fec.Result{BitsChecked: sl.c0.n + sl.c1.n, ErrorsCorrected: e0 + e1, Uncorrectable: e0 > sl.c0.t || e1 > sl.c1.t})
		if out != nil {
			softPutSmall(out, &sl.c0, cw0)
			softPutSmall(out, &sl.c1, cw1)
			for _, p := range sl.c2 {
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
	info, cw, errs := d.small.decode(&d.slotType, llr[:], fixed, want)
	if out != nil {
		softPutSmall(out, &d.slotType, cw)
	}
	return pdu.SlotTypeInfo(info & 0xff), softResult(d.slotType.n, errs, d.slotType.t)
}

// decodeEMB decodes the QR(16,7) EMB of a burst from its 264 LLRs by scoring
// all 128 codewords, or the 8 of color code cc when ccKnown. When out is not
// nil, the corrected codeword is written into it.
func (d *softDecoder) decodeEMB(llr *[softBurstBits]int16, cc enums.ColorCode, ccKnown bool, out *layer2.RawBurst) (pdu.EMBInfo, fec.Result) {
	var fixed, want uint32
	if ccKnown {
		fixed, want = 0x78, uint32(cc&0x0f)<<3
	}
	info, cw, errs := d.small.decode(&d.emb, llr[:], fixed, want)
	if out != nil {
		softPutSmall(out, &d.emb, cw)
	}
	return pdu.EMBInfo(info << 1 & 0xfe), softResult(d.emb.n, errs, d.emb.t)
}

// decodeTACT decodes the Hamming(7,4) TACT of a CACH from its 24 LLRs in
// transmit order by scoring all 16 codewords. When out is not nil, the
// corrected TACT bits are written into it.
func (d *softDecoder) decodeTACT(llr *[layer2.CACHBits]int16, out *[layer2.CACHBits]unpacked.Bit) (pdu.TACT, fec.Result) {
	info, cw, errs := d.small.decode(&d.tact, llr[:], 0, 0)
	if out != nil {
		for j := range d.tact.n {
			out[d.tact.pos[j]] = unpacked.Bit(cw >> j & 1)
		}
	}
	t := softTACT(info)
	t.FEC = softResult(d.tact.n, errs, d.tact.t)
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
	tr := d.trellis
	l := &d.osd.l
	for j, p := range tr.pos {
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
		for b, j := range tr.step[k] {
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
	for j := range d.bptc.n {
		d.osd.l[j] = int32(llr[d.bptc.pos[j]])
	}
}

// bptcPick returns the first candidate in the scratch list that check
// accepts, or the first candidate when check is nil or accepts none, and
// writes its codeword into out when out is not nil.
func (d *softDecoder) bptcPick(check func([12]byte) bool, out *layer2.RawBurst) (softBPTCResult, fec.Result) {
	s := &d.osd
	pick, ok := s.first()
	cw := &s.list.cw[pick]
	errs := s.errors(cw)
	res := softResult(d.bptc.n, errs, d.bptc.t)
	if check != nil {
		res.Uncorrectable = !ok
	}
	if out != nil {
		for j := range d.bptc.n {
			softPut(out, d.bptc.pos[j], softBit(cw, j))
		}
	}
	return d.bptc.bptcResult(cw), res
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
	d.bptcLLRs(llr)
	d.bptc.osd(&d.osd, d.OSDOrder, d.OSDWindow, d.bptcCheck(check))
	return d.bptcPick(check, out)
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
	s, c := &d.osd, d.bptc
	d.bptcLLRs(llr)
	s.prepare(c.n)
	p := min(max(d.ChaseBits, 0), softChaseMax)
	for i := range p {
		s.chase[i] = -1
	}
	for i, q := range d.info {
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
			s.bits[d.bptcInfo[s.chase[bits.TrailingZeros(uint(g))]]] ^= 1
		}
		var packed [25]byte
		for i, v := range s.bits {
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
	c, s := d.shortLC, &d.osd
	for j := range c.n {
		p := c.pos[j]
		s.l[j] = int32(llr[p/layer2.CACHBits][p%layer2.CACHBits])
	}
	s.accept = d.acceptShortLC
	c.osd(s, d.OSDOrder, d.OSDWindow, d.ShortLCList)
	pick, ok := s.first()
	cw := &s.list.cw[pick]
	info := c.shortLCInfo(cw)
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

// shortLCInfo returns the 36 Short LC info bits of codeword cw.
func (c *softOSDCode) shortLCInfo(cw *[softOSDWords]uint64) [softShortLCInfo]unpacked.Bit {
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
	e, s := d.embLC, &d.osd
	c := e.code
	for j := range c.n {
		p := c.pos[j]
		s.l[j] = int32(llr[p/softBurstBits][p%softBurstBits-embFirstBit])
	}
	s.accept = d.acceptEmbLC
	c.osd(s, d.OSDOrder, d.OSDWindow, d.EmbLCList)
	pick, ok := s.first()
	cw := &s.list.cw[pick]
	if out != nil {
		for j := range c.n {
			p := c.pos[j]
			softPut(&out[p/softBurstBits], p%softBurstBits, softBit(cw, j))
		}
	}
	return e.lcOf(cw), fec.Result{BitsChecked: c.n, ErrorsCorrected: s.errors(cw), Uncorrectable: !ok}
}

// accept reports whether the 5-bit checksum of codeword cw matches its LC.
func (e *softEmbLC) accept(cw *[softOSDWords]uint64) bool {
	var cs uint8
	for b, x := range e.cs {
		if softBit(cw, e.code.sys[x]) {
			cs |= 1 << b
		}
	}
	return crc.CheckChecksum5(e.lcOf(cw), cs)
}

// lcOf returns the 9 LC octets of codeword cw.
func (e *softEmbLC) lcOf(cw *[softOSDWords]uint64) [9]byte {
	var lc [9]byte
	for j, i := range e.lc {
		if softBit(cw, e.code.sys[i]) {
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
