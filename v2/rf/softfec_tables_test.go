package rf

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/ambe"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// softFECTables holds the soft FEC tables of tables_gen.go as
// deriveSoftFEC derives them from the library encoders.
type softFECTables struct {
	small    [softSmallCount]softSmallCode
	c2       [layer2.VoiceSlotsPerBurst][softAMBEC2]int16
	osd      [softOSDCount]softOSDCode
	embLC    softEmbLC
	trellis  softTrellis
	info     [softTrellisBits]int16
	bptcInfo [softOSDMaxN]uint8
	gridPos  [gridRows][gridCols]int16
	rowH     [gridCols]uint8
	colH     [gridRows]uint8
}

// deriveSoftFEC derives every soft FEC table from the library encoders. It
// returns an error when a library encoder has a layout that the decoders
// do not expect.
func deriveSoftFEC() (*softFECTables, error) {
	t := &softFECTables{}
	if err := t.deriveAMBE(); err != nil {
		return nil, err
	}
	bptcCode, err := newSoftOSDCode(softBPTCInfo, 4, softBPTCEncoder)
	if err != nil {
		return nil, err
	}
	shortLC, err := newSoftOSDCode(softShortLCInfo, 2, softShortLCEncoder)
	if err != nil {
		return nil, err
	}
	embLC, err := newSoftOSDCode(softEmbLCInfo, 3, softEmbLCEncoder)
	if err != nil {
		return nil, err
	}
	t.osd[softBPTCCode], t.osd[softShortLCCode], t.osd[softEmbLCCode] = *bptcCode, *shortLC, *embLC
	if t.embLC, err = newSoftEmbLC(embLC); err != nil {
		return nil, err
	}
	if err := t.infoBits(); err != nil {
		return nil, err
	}
	tr, err := newSoftTrellis(&t.info)
	if err != nil {
		return nil, err
	}
	t.trellis = *tr
	if err := t.gridLayout(); err != nil {
		return nil, err
	}
	if t.small[softSlotTypeCode], err = newSoftSmallCode(8, 3, func(info uint32) ([]unpacked.Bit, error) {
		var r layer2.RawBurst
		if err := r.SetSlotType(pdu.SlotTypeInfo(info & 0xff).SlotType()); err != nil {
			return nil, err
		}
		return softBurstImage(&r), nil
	}, nil); err != nil {
		return nil, err
	}
	if t.small[softEMBCode], err = newSoftSmallCode(7, 2, func(info uint32) ([]unpacked.Bit, error) {
		var r layer2.RawBurst
		if err := r.SetEMB(pdu.EMBInfo(info << 1 & 0xfe).EmbeddedSignalling()); err != nil {
			return nil, err
		}
		return softBurstImage(&r), nil
	}, nil); err != nil {
		return nil, err
	}
	if t.small[softTACTCode], err = newSoftSmallCode(4, 1, func(info uint32) ([]unpacked.Bit, error) {
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
	if t.osd[softBPTCCode].n != softOSDMaxN-1 || t.osd[softShortLCCode].n != 68 || t.osd[softEmbLCCode].n != 128 ||
		t.small[softSlotTypeCode].n != 20 || t.small[softEMBCode].n != 16 || t.small[softTACTCode].n != 7 {
		return nil, fmt.Errorf("%w: unexpected code lengths", errSoftDerive)
	}
	return t, nil
}

// errSoftDerive is returned when a code table cannot be derived from the
// library encoders.
var errSoftDerive = errors.New("soft FEC: cannot derive code tables")

// softEncoder returns the field image of info word info: the bits of the
// burst, or of the fields, that carry its codeword.
type softEncoder func(info uint32) ([]unpacked.Bit, error)

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

// softBurstImage returns the bits of r as a field image.
func softBurstImage(r *layer2.RawBurst) []unpacked.Bit {
	var b [264]unpacked.Bit
	unpacked.UnpackAt(b[:], r[:], 0)
	return b[:]
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

// deriveAMBE derives the AMBE+2 tables from layer2.RawBurst.SetAMBEFrames:
// the Golay(24,12) code C0 and the Golay(23,12) code C1 without its
// scrambling of each voice slot, where info bit 11-i is frame bit i, and
// frame bit 12+i for C1, and the burst position of uncoded frame bit 24+i.
func (t *softFECTables) deriveAMBE() error {
	for s := range layer2.VoiceSlotsPerBurst {
		c0, c1 := &t.small[softAMBECode+2*softSmallID(s)], &t.small[softAMBECode+2*softSmallID(s)+1]
		var err error
		if *c1, err = newSoftSmallCode(softAMBEC0, 3, softAMBEEncoder(s, softAMBEC0), nil); err != nil {
			return err
		}
		c1.base = 0
		skip := make([]bool, softBurstBits)
		for j := range c1.n {
			skip[c1.pos[j]] = true
		}
		var zero layer2.RawBurst
		if err := zero.SetAMBEFrames([layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame{}); err != nil {
			return err
		}
		zimg := softBurstImage(&zero)
		for i := range softAMBEC2 {
			var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
			frames[s][(2*softAMBEC0+i)/8] |= 0x80 >> ((2*softAMBEC0 + i) % 8)
			var r layer2.RawBurst
			if err := r.SetAMBEFrames(frames); err != nil {
				return err
			}
			img := softBurstImage(&r)
			n := 0
			for p := range img {
				if img[p] != zimg[p] {
					t.c2[s][i], skip[p] = int16(p), true
					n++
				}
			}
			if n != 1 {
				return fmt.Errorf("%w: AMBE+2 bit %d is not one burst bit", errSoftDerive, 2*softAMBEC0+i)
			}
		}
		if *c0, err = newSoftSmallCode(softAMBEC0, 3, softAMBEEncoder(s, 0), skip); err != nil {
			return err
		}
		if c0.n != 24 || c1.n != 23 {
			return fmt.Errorf("%w: AMBE+2 C0 has %d bits and C1 %d", errSoftDerive, c0.n, c1.n)
		}
	}
	c1 := &t.small[softAMBECode+1]
	for _, p := range c1.pos[:c1.n] {
		if int(p) >= 8*len(ambe.Encode([7]byte{})) {
			return fmt.Errorf("%w: AMBE+2 C1 bit %d is not in voice slot 0", errSoftDerive, p)
		}
	}
	return nil
}

// newSoftOSDCode derives a linear code from enc, where enc(0) encodes the
// zero info word, which must give the zero codeword, and enc(i+1) encodes
// unit vector i.
func newSoftOSDCode(k, t int, enc softEncoder) (*softOSDCode, error) {
	c := &softOSDCode{k: k, t: t}
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

// newSoftEmbLC derives the LC and checksum layout of the embedded LC code
// from layer2.EncodeEmbeddedLC.
func newSoftEmbLC(code *softOSDCode) (softEmbLC, error) {
	var e softEmbLC
	for _, p := range code.pos[:code.n] {
		if b := int(p) % softBurstBits; b < embFirstBit || b >= embFirstBit+embBits {
			return e, fmt.Errorf("%w: embedded LC bit at burst bit %d", errSoftDerive, b)
		}
	}
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
			return e, fmt.Errorf("%w: LC bit %d has no info bit", errSoftDerive, j)
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
			return e, fmt.Errorf("%w: checksum bit %d has no info bit", errSoftDerive, c)
		}
	}
	return e, nil
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

// infoBits sets t.info to the burst bits that BPTC(196,96) or a Rate 1
// block use, and t.bptcInfo to the place of each BPTC bit among them, the
// information bit of BPTC(196,96) codeword bit j, which has no R(3).
func (t *softFECTables) infoBits() error {
	c := &t.osd[softBPTCCode]
	var used [softBurstBits]bool
	for j := range c.n {
		used[c.pos[j]] = true
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
		t.info[n] = int16(q)
		for j := range c.n {
			if c.pos[j] == int16(q) {
				t.bptcInfo[j] = uint8(n)
			}
		}
		n++
	}
	if n != softTrellisBits {
		return fmt.Errorf("%w: %d information bits", errSoftDerive, n)
	}
	return nil
}

// cachBits is the inverse of cachBytes.
func cachBits(p [3]byte) [layer2.CACHBits]unpacked.Bit {
	var b [layer2.CACHBits]unpacked.Bit
	for i := range b {
		b[i] = unpacked.Bit(p[i>>3] >> (7 - i&7) & 1)
	}
	return b
}

// cachPayload returns the 17-bit CACH payload word f packed as
// layer2.CACH.Payload holds it.
func cachPayload(f uint32) [3]byte {
	p := f << 7
	return [3]byte{byte(p >> 16 & 0xFF), byte(p >> 8 & 0xFF), byte(p & 0x80)}
}

// cachPayloadWord is the inverse of cachPayload.
func cachPayloadWord(p [3]byte) uint32 {
	return uint32(p[0])<<9 | uint32(p[1])<<1 | uint32(p[2])>>7
}

// gridChecks returns the columns that each parity check of the row code
// covers, and the rows that each check of the column code covers (ETSI TS
// 102 361-1 clause B.1.1).
func gridChecks() (row, col [4]uint16) {
	row = [4]uint16{
		1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<5 | 1<<7 | 1<<8 | 1<<11,
		1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<6 | 1<<8 | 1<<9 | 1<<12,
		1<<2 | 1<<3 | 1<<4 | 1<<5 | 1<<7 | 1<<9 | 1<<10 | 1<<13,
		1<<0 | 1<<1 | 1<<2 | 1<<4 | 1<<6 | 1<<7 | 1<<10 | 1<<14,
	}
	col = [4]uint16{
		1<<0 | 1<<1 | 1<<3 | 1<<5 | 1<<6 | 1<<9,
		1<<0 | 1<<1 | 1<<2 | 1<<4 | 1<<6 | 1<<7 | 1<<10,
		1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<5 | 1<<7 | 1<<8 | 1<<11,
		1<<0 | 1<<2 | 1<<4 | 1<<5 | 1<<8 | 1<<12,
	}
	return row, col
}

// gridLayout lays out the BPTC(196,96) grid on the burst bits t.info of
// the 196 information bits: grid cell i, from 1, is information bit 181i
// mod 196. It checks the layout against the codewords of the code.
func (t *softFECTables) gridLayout() error {
	for r := range gridRows {
		for c := range gridCols {
			t.gridPos[r][c] = t.info[(1+gridCols*r+c)*181%softTrellisBits]
		}
	}
	rowChecks, colChecks := gridChecks()
	for k, m := range rowChecks {
		for c := range gridCols {
			if m>>c&1 != 0 {
				t.rowH[c] |= 1 << k
			}
		}
	}
	for k, m := range colChecks {
		for r := range gridRows {
			if m>>r&1 != 0 {
				t.colH[r] |= 1 << k
			}
		}
	}
	code := &t.osd[softBPTCCode]
	var img [softBurstBits]unpacked.Bit
	for i := range code.k {
		clear(img[:])
		for j := range code.n {
			if softBit(&code.rows[i], j) {
				img[code.pos[j]] = 1
			}
		}
		if !gridIsCodeword(&t.gridPos, &t.rowH, &t.colH, &img) {
			return fmt.Errorf("%w: BPTC(196,96) grid layout", errSoftDerive)
		}
	}
	return nil
}

// bits returns the burst positions of the code bits of code id.
func (id softSmallID) bits() []int16 {
	c := &softSmallCodes[id]
	return c.pos[:c.n]
}

// code returns the tables of code id.
func (id softOSDID) code() *softOSDCode {
	return &softOSDCodes[id]
}

// softAMBEView holds the codes of a voice slot: the ids and tables of C0
// and C1, and the burst positions of the uncoded bits.
type softAMBEView struct {
	id0, id1 softSmallID
	c0, c1   *softSmallCode
	c2       *[softAMBEC2]int16
}

// softAMBESlot returns the codes of voice slot s.
func softAMBESlot(s int) softAMBEView {
	id0 := softAMBECode + 2*softSmallID(s)
	return softAMBEView{id0: id0, id1: id0 + 1, c0: &softSmallCodes[id0], c1: &softSmallCodes[id0+1], c2: &softAMBEC2Pos[s]}
}

// gridIsCodeword reports whether every row and column of the grid in img,
// with the cells at pos and the syndromes rowH and colH, has a zero
// syndrome.
func gridIsCodeword(pos *[gridRows][gridCols]int16, rowH *[gridCols]uint8, colH *[gridRows]uint8, img *[softBurstBits]unpacked.Bit) bool {
	var col [gridCols]uint8
	for r := range gridRows {
		var s uint8
		for c := range gridCols {
			if img[pos[r][c]] != 0 {
				s ^= rowH[c]
				col[c] ^= colH[r]
			}
		}
		if s != 0 {
			return false
		}
	}
	for _, s := range col {
		if s != 0 {
			return false
		}
	}
	return true
}
