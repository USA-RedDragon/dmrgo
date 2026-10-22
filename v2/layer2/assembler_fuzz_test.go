package layer2_test

import (
	"errors"
	"math/bits"
	"reflect"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func distance(a, b []byte) int {
	n := 0
	for i := range a {
		n += bits.OnesCount8(a[i] ^ b[i])
	}
	return n
}

var cachSeeds = [][]byte{{0x00, 0x00, 0x00}, {0xff, 0xff, 0xff}, {0x5c, 0x3a, 0x81}}

func checkCACH(t *testing.T, v []byte) {
	if len(v) != 3 {
		return
	}
	x := [3]byte(v)
	if got := layer2.CACHDeinterleave(layer2.CACHInterleave(x)); got != x {
		t.Fatalf("CACHDeinterleave(CACHInterleave(%x)) = %x", x, got)
	}
	if got := layer2.CACHInterleave(layer2.CACHDeinterleave(x)); got != x {
		t.Fatalf("CACHInterleave(CACHDeinterleave(%x)) = %x", x, got)
	}
	if a, b := layer2.DecodeCACHField(x), layer2.DecodeCACH(layer2.CACHDeinterleave(x)); a != b {
		t.Fatalf("DecodeCACHField(%x) = %+v, want %+v", x, a, b)
	}
	c := layer2.DecodeCACH(x)
	if c.Payload[2]&0x7F != 0 {
		t.Fatalf("DecodeCACH(%x) payload %x has padding bits set", x, c.Payload)
	}
	enc, err := c.Encode()
	if err != nil {
		t.Fatalf("Encode of DecodeCACH(%x) = %+v: %v", x, c, err)
	}
	d := layer2.DecodeCACH(enc)
	if d.Payload != c.Payload || d.TACT.AccessType != c.TACT.AccessType || d.TACT.TDMAChannel != c.TACT.TDMAChannel ||
		d.TACT.LCSS != c.TACT.LCSS || !clean(d.TACT.FEC) {
		t.Fatalf("CACH %+v encodes to %x, which decodes to %+v", c, enc, d)
	}
	if clean(c.TACT.FEC) && enc != x {
		t.Fatalf("clean CACH %x encodes to %x", x, enc)
	}
	if !c.TACT.FEC.Uncorrectable && distance(enc[:1], x[:1]) != c.TACT.FEC.ErrorsCorrected {
		t.Fatalf("TACT of %x corrected to %x reports %d corrections", x, enc, c.TACT.FEC.ErrorsCorrected)
	}
}

func FuzzCACH(f *testing.F) {
	for _, s := range cachSeeds {
		f.Add(s)
	}
	f.Fuzz(checkCACH)
}

type shortLCModel struct {
	count int
}

func (m *shortLCModel) add(c layer2.CACH) (done, discard bool) {
	if c.TACT.FEC.Uncorrectable {
		discard, m.count = m.count > 0, 0
		return false, discard
	}
	switch c.TACT.LCSS {
	case enums.LCSSFirstFragment:
		m.count = 1
		return false, false
	case enums.LCSSContinuationFragment:
		if m.count == 1 || m.count == 2 {
			m.count++
			return false, false
		}
	case enums.LCSSLastFragment:
		if m.count == 3 {
			m.count = 0
			return true, false
		}
	case enums.LCSSSingleFragment:
		discard, m.count = m.count > 0, 0
		return false, discard
	}
	m.count = 0
	return false, true
}

type shortLCSeed struct {
	seq, slc []byte
	at, tact byte
}

var shortLCSeeds = []shortLCSeed{
	{[]byte{0x00, 0x34, 0x56, 0x78, 0x9a, 0xbc}, []byte{0x12, 0x34, 0x56, 0x78, 0x90}, 0, 0},
	{[]byte{}, []byte{0x2a, 0x00, 0x00, 0x01, 0x20}, 1, 3},
	{[]byte{0xff, 0xff, 0xff, 0x10, 0x00, 0x00, 0x20, 0x00, 0x00}, []byte{0x00, 0x00, 0x00, 0x00, 0x00}, 2, 1},
}

func checkShortLCAssembler(t *testing.T, seq, slcv []byte, at, tactv byte) {
	var info [5]byte
	copy(info[:], slcv)
	want := pdu.DecodeShortLC(info)
	tact := pdu.TACT{AccessType: tactv&1 != 0, TDMAChannel: tactv&2 != 0}
	fields := make([][3]byte, 0, len(seq)/3+4)
	for i := 0; i+3 <= len(seq) && i < 3*64; i += 3 {
		fields = append(fields, [3]byte(seq[i:i+3]))
	}
	valid, err := layer2.EncodeShortLCToCACH(&want, tact)
	first := -1
	if err == nil {
		first = int(at) % (len(fields) + 1)
		fields = slices.Insert(fields, first, valid[:]...)
	}

	var a layer2.ShortLCAssembler
	var m shortLCModel
	for i, field := range fields {
		c := layer2.DecodeCACHField(field)
		wantDone, wantDiscard := m.add(c)
		slc, done, err := a.Add(c)
		if done != wantDone || errors.Is(err, layer2.ErrShortLCSequence) != wantDiscard || (err != nil) != wantDiscard {
			t.Fatalf("field %d (%+v): done %v, %v, want %v, discard %v", i, c.TACT, done, err, wantDone, wantDiscard)
		}
		if a.Count() != m.count {
			t.Fatalf("field %d: count %d, want %d", i, a.Count(), m.count)
		}
		if !done {
			continue
		}
		if i == first+3 {
			if !clean(slc.FEC) || !reflect.DeepEqual(withoutFEC(&slc), withoutFEC(&want)) {
				t.Fatalf("Short LC %s comes back as %s, FEC %+v", want.String(), slc.String(), slc.FEC)
			}
		}
		if slc.FEC.Uncorrectable {
			continue
		}
		coded, err := layer2.EncodeShortLCToCACH(&slc, tact)
		if err != nil {
			continue
		}
		n := 0
		for k := range 4 {
			got, want := layer2.DecodeCACHField(fields[i-3+k]).Payload, layer2.DecodeCACHField(coded[k]).Payload
			n += distance(got[:], want[:])
		}
		if n != slc.FEC.ErrorsCorrected {
			t.Fatalf("field %d: Short LC %s is %d bits from its fields, FEC reports %+v", i, slc.String(), n, slc.FEC)
		}
	}
}

func FuzzShortLCAssembler(f *testing.F) {
	for _, s := range shortLCSeeds {
		f.Add(s.seq, s.slc, s.at, s.tact)
	}
	f.Fuzz(checkShortLCAssembler)
}

type embeddedLCModel struct {
	count int
}

func (m *embeddedLCModel) add(lcss enums.LCSS) (done, discard bool) {
	switch lcss {
	case enums.LCSSFirstFragment:
		discard, m.count = m.count > 0, 1
		return false, discard
	case enums.LCSSContinuationFragment:
		if m.count == 1 || m.count == 2 {
			m.count++
			return false, false
		}
	case enums.LCSSLastFragment:
		if m.count == 3 {
			m.count = 0
			return true, false
		}
	case enums.LCSSSingleFragment:
	}
	discard, m.count = m.count > 0, 0
	return false, discard
}

type embeddedLCSeed struct {
	seq, lc []byte
	at      byte
}

func embeddedLCSeeds() []embeddedLCSeed {
	lc := groupLC(enums.FIDStandardized)
	octets := must(lc.Bytes())
	return []embeddedLCSeed{
		{[]byte{1, 2, 3, 4, 0, 5, 6, 7, 8, 2}, octets[:], 1},
		{[]byte{}, octets[:], 0},
		{[]byte{0xff, 0xff, 0xff, 0xff, 1, 0, 0, 0, 0, 3, 0, 0, 0, 0, 2}, []byte{}, 3},
	}
}

func checkEmbeddedLCAssembler(t *testing.T, seq, lcv []byte, at byte) {
	var lc [9]byte
	copy(lc[:], lcv)
	type frag struct {
		data [4]byte
		lcss enums.LCSS
	}
	frags := make([]frag, 0, len(seq)/5+4)
	for i := 0; i+5 <= len(seq) && i < 5*64; i += 5 {
		frags = append(frags, frag{[4]byte(seq[i : i+4]), enums.LCSS(seq[i+4] & 3)})
	}
	valid := layer2.EncodeEmbeddedLC(lc)
	lcss := [4]enums.LCSS{enums.LCSSFirstFragment, enums.LCSSContinuationFragment, enums.LCSSContinuationFragment, enums.LCSSLastFragment}
	first := int(at) % (len(frags) + 1)
	for k := 3; k >= 0; k-- {
		frags = slices.Insert(frags, first, frag{valid[k], lcss[k]})
	}

	var a layer2.EmbeddedLCAssembler
	var m embeddedLCModel
	for i, fr := range frags {
		wantDone, wantDiscard := m.add(fr.lcss)
		got, res, done, err := a.Add(fr.data, fr.lcss)
		if done != wantDone || errors.Is(err, layer2.ErrEmbeddedLCSequence) != wantDiscard {
			t.Fatalf("fragment %d (%v): done %v, %v, want %v, discard %v", i, fr.lcss, done, err, wantDone, wantDiscard)
		}
		if !done {
			if err != nil && !wantDiscard {
				t.Fatalf("fragment %d: %v", i, err)
			}
			continue
		}
		if errors.Is(err, layer2.ErrEmbeddedLCUncorrectable) != res.Uncorrectable || err != nil && !res.Uncorrectable {
			t.Fatalf("fragment %d: %v with result %+v", i, err, res)
		}
		var in [4][4]byte
		for k := range in {
			in[k] = frags[i-3+k].data
		}
		if lc2, res2 := layer2.DecodeEmbeddedLC(in); lc2 != got || res2 != res {
			t.Fatalf("fragment %d: assembler gives %x, %+v, DecodeEmbeddedLC %x, %+v", i, got, res, lc2, res2)
		}
		if i == first+3 && (got != lc || !clean(res)) {
			t.Fatalf("LC %x comes back as %x, %+v", lc, got, res)
		}
		if res.Uncorrectable {
			continue
		}
		coded := layer2.EncodeEmbeddedLC(got)
		n := 0
		for k := range coded {
			n += distance(coded[k][:], in[k][:])
		}
		if n != res.ErrorsCorrected {
			t.Fatalf("fragment %d: LC %x is %d bits from its fragments, FEC reports %+v", i, got, n, res)
		}
	}
}

func FuzzEmbeddedLCAssembler(f *testing.F) {
	for _, s := range embeddedLCSeeds() {
		f.Add(s.seq, s.lc, s.at)
	}
	f.Fuzz(checkEmbeddedLCAssembler)
}

type embeddedLCCodeSeed struct {
	lc, flc, flips []byte
}

func embeddedLCCodeSeeds() []embeddedLCCodeSeed {
	lc := groupLC(enums.FIDStandardized)
	lc.BurstDataType = enums.DataTypeVoiceLCHeader
	cw := must(lc.Encode())
	octets := must(lc.Bytes())
	return []embeddedLCCodeSeed{
		{octets[:], cw[:], []byte{}},
		{octets[:], cw[:], []byte{17}},
		{octets[:], cw[:], []byte{1, 40, 77, 127}},
	}
}

func checkEmbeddedLCCode(t *testing.T, lcv, flcv, flips []byte) {
	var lc [9]byte
	copy(lc[:], lcv)
	frags := layer2.EncodeEmbeddedLC(lc)
	if got, res := layer2.DecodeEmbeddedLC(frags); got != lc || !clean(res) {
		t.Fatalf("LC %x comes back as %x, %+v", lc, got, res)
	}
	rx := frags
	seen := map[int]bool{}
	for _, fl := range flips[:min(len(flips), 16)] {
		pos := int(fl) % 128
		if seen[pos] {
			continue
		}
		seen[pos] = true
		rx[pos/32][pos%32/8] ^= 0x80 >> (pos % 8)
	}
	got, res := layer2.DecodeEmbeddedLC(rx)
	if len(seen) == 1 && (got != lc || res.Uncorrectable || res.ErrorsCorrected != 1) {
		t.Fatalf("LC %x with bit error %v comes back as %x, %+v", lc, seen, got, res)
	}
	if !res.Uncorrectable {
		coded := layer2.EncodeEmbeddedLC(got)
		n := 0
		for k := range coded {
			n += distance(coded[k][:], rx[k][:])
		}
		if n != res.ErrorsCorrected {
			t.Fatalf("LC %x with bit errors %v: %x is %d bits away, FEC reports %+v", lc, seen, got, n, res)
		}
	}

	var cw [12]byte
	copy(cw[:], flcv)
	flc := pdu.DecodeFullLinkControl(cw, enums.DataTypeVoiceLCHeader)
	ef, err := layer2.EncodeEmbeddedFullLinkControl(&flc)
	if _, bErr := flc.Bytes(); (err != nil) != (bErr != nil) {
		t.Fatalf("EncodeEmbeddedFullLinkControl(%s): %v, Bytes: %v", flc.String(), err, bErr)
	}
	if err != nil {
		return
	}
	back := layer2.DecodeEmbeddedFullLinkControl(ef)
	if back.FEC.HasErrors() || !reflect.DeepEqual(withoutFEC(&back), withoutFEC(&flc)) {
		t.Fatalf("Full LC %s comes back as %s, FEC %+v", flc.String(), back.String(), back.FEC)
	}
}

func FuzzEmbeddedLC(f *testing.F) {
	for _, s := range embeddedLCCodeSeeds() {
		f.Add(s.lc, s.flc, s.flips)
	}
	f.Fuzz(checkEmbeddedLCCode)
}

type rcSeed struct {
	field []byte
	cmd   byte
}

var rcSeeds = []rcSeed{{[]byte{0, 0, 0, 0}, 0}, {[]byte{0x12, 0x34, 0x56, 0x78}, 7}, {[]byte{0xff, 0xff, 0xff, 0xff}, 16}}

func checkEmbeddedRC(t *testing.T, v []byte, cmd byte) {
	rc := pdu.ReverseChannel{RCCommand: enums.RCCommand(cmd & 0x1F)}
	data, err := layer2.EncodeEmbeddedRC(&rc)
	if (err != nil) != (rc.RCCommand > 15) {
		t.Fatalf("EncodeEmbeddedRC(%v): %v", rc.RCCommand, err)
	}
	if err == nil {
		if back := layer2.DecodeEmbeddedRC(data); back.RCCommand != rc.RCCommand || !clean(back.FEC) {
			t.Fatalf("RC %v comes back as %+v", rc.RCCommand, back)
		}
	}
	var field [4]byte
	copy(field[:], v)
	got := layer2.DecodeEmbeddedRC(field)
	enc, err := layer2.EncodeEmbeddedRC(&got)
	if err != nil {
		t.Fatalf("decoded RC %+v does not encode: %v", got, err)
	}
	if clean(got.FEC) && enc != field {
		t.Fatalf("clean RC field %x encodes to %x", field, enc)
	}
	if !got.FEC.Uncorrectable && distance(enc[:], field[:]) != got.FEC.ErrorsCorrected {
		t.Fatalf("RC field %x is %d bits from %x, FEC reports %+v", field, distance(enc[:], field[:]), enc, got.FEC)
	}
}

func FuzzEmbeddedRC(f *testing.F) {
	for _, s := range rcSeeds {
		f.Add(s.field, s.cmd)
	}
	f.Fuzz(checkEmbeddedRC)
}
