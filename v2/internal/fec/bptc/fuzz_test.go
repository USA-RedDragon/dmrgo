package bptc_test

import (
	"encoding/binary"
	"math/bits"
	"os"
	"path/filepath"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	packed "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// captureBlocks returns the BPTC(196,96) data of the bursts in the captures
// of layer2/testdata whose information bits decode without an uncorrectable
// error: the 98 bits before and the 98 bits after the center 68 bits.
func captureBlocks(tb testing.TB) [][12]byte {
	tb.Helper()
	files, err := filepath.Glob("../../../layer2/testdata/*.bin")
	if err != nil || len(files) == 0 {
		tb.Fatalf("no captures: %v", err)
	}
	var out [][12]byte
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			tb.Fatal(err)
		}
		for i := 0; i+33 <= len(data); i += 33 {
			var info [25]byte
			packed.Copy(info[:], 0, data[i:i+33], 0, 98)
			packed.Copy(info[:], 98, data[i:i+33], 166, 98)
			if d, _, res := bptc.Decode196(info); !res.Uncorrectable {
				out = append(out, d)
			}
		}
	}
	return out
}

// gridDistance returns the number of grid bits, all but R(3), in which the
// BPTC(196,96) blocks a and b differ.
func gridDistance(a, b [25]byte) int {
	ra, rb := bptc.Rows(a), bptc.Rows(b)
	n := 0
	for i := range ra {
		n += bits.OnesCount16(ra[i] ^ rb[i])
	}
	return n
}

// bptc196Seed is one seed of FuzzBPTC196.
type bptc196Seed struct {
	data     []byte
	reserved uint8
	mask     []byte
}

// bptc196Seeds returns the seeds of FuzzBPTC196.
func bptc196Seeds(tb testing.TB) []bptc196Seed {
	blocks := captureBlocks(tb)
	seeds := make([]bptc196Seed, 0, len(blocks)+1)
	for i, d := range blocks {
		var mask [25]byte
		for k := range i % 4 {
			p := (37*i + 53*k) % 196
			mask[p/8] |= 0x80 >> (p % 8)
		}
		seeds = append(seeds, bptc196Seed{d[:], uint8(i), mask[:]})
	}
	seeds = append(seeds, bptc196Seed{make([]byte, 12), uint8(0), []byte{0xFF, 0xFF, 0xFF, 0xFF}})
	return seeds
}

// checkBPTC196 checks the properties of FuzzBPTC196 for one input.
func checkBPTC196(t testing.TB, data []byte, reserved uint8, mask []byte) {
	t.Helper()
	var d [12]byte
	copy(d[:], data)
	reserved &= 7
	var e [25]byte
	copy(e[:], mask)
	e[24] &= 0xF0

	cw := bptc.Encode196(d, reserved)
	refRes := [3]unpacked.Bit{unpacked.Bit(reserved & 1), unpacked.Bit(reserved >> 1 & 1), unpacked.Bit(reserved >> 2 & 1)}
	refCw := ref.Encode196WithReserved(unpacked.Unpack96(d), refRes)
	var want [25]byte
	unpacked.PackAt(want[:], 0, refCw[:])
	if cw != want {
		t.Fatalf("Encode196(%x, %d) = %x, reference %x", d, reserved, cw, want)
	}

	var rx [25]byte
	for i := range rx {
		rx[i] = cw[i] ^ e[i]
	}
	got, gotRes, res := bptc.Decode196(rx)
	n := gridDistance(cw, rx)
	if n <= 2 && (got != d || gotRes != reserved || res != (fec.Result{BitsChecked: 196, ErrorsCorrected: n})) {
		t.Fatalf("%d errors %x: got %x %d %+v, want %x %d", n, e, got, gotRes, res, d, reserved)
	}
	if !res.Uncorrectable {
		if dist := gridDistance(bptc.Encode196(got, gotRes), rx); dist != res.ErrorsCorrected {
			t.Fatalf("errors %x: decoded block is %d bits from the received block, ErrorsCorrected %d", e, dist, res.ErrorsCorrected)
		}
	}

	var refIn [196]unpacked.Bit
	unpacked.UnpackAt(refIn[:], rx[:], 0)
	wantData, wantReserved, wantResult := ref.Decode196WithReserved(refIn)
	if got != unpacked.Pack96(wantData) || gotRes != uint8(wantReserved[0]|wantReserved[1]<<1|wantReserved[2]<<2) || res != wantResult {
		t.Fatalf("Decode196(%x) = %x %d %+v, reference %x %v %+v", rx, got, gotRes, res, unpacked.Pack96(wantData), wantReserved, wantResult)
	}
}

// FuzzBPTC196 encodes fuzzed data and reserved bits, adds the error pattern
// mask and decodes. Up to 2 grid errors decode to the data with
// ErrorsCorrected equal to their count. A block that is not uncorrectable is
// a codeword ErrorsCorrected grid bits from the received block. Encode196
// and Decode196 agree with the reference codec on every block.
func FuzzBPTC196(f *testing.F) {
	for _, s := range bptc196Seeds(f) {
		f.Add(s.data, s.reserved, s.mask)
	}
	f.Fuzz(func(t *testing.T, data []byte, reserved uint8, mask []byte) {
		checkBPTC196(t, data, reserved, mask)
	})
}

// embeddedDistance returns the number of bits in which the fragments a and
// b differ.
func embeddedDistance(a, b [4]uint32) int {
	n := 0
	for i := range a {
		n += bits.OnesCount32(a[i] ^ b[i])
	}
	return n
}

// unpackFragments returns the fragments with one unpacked.Bit per bit.
func unpackFragments(w [4]uint32) [4][32]unpacked.Bit {
	var out [4][32]unpacked.Bit
	for i := range w {
		unpacked.PutUint(out[i][:], uint64(w[i]))
	}
	return out
}

// packEmbeddedInfo packs 77 reference information bits.
func packEmbeddedInfo(info [77]unpacked.Bit) [10]byte {
	var p [10]byte
	unpacked.PackAt(p[:], 0, info[:])
	return p
}

// embeddedLCSeed is one seed of FuzzEmbeddedLC.
type embeddedLCSeed struct {
	data   []byte
	mask   []byte
	parity bool
}

// embeddedLCSeeds returns the seeds of FuzzEmbeddedLC.
func embeddedLCSeeds(tb testing.TB) []embeddedLCSeed {
	var seeds []embeddedLCSeed
	for i, d := range captureBlocks(tb) {
		mask := make([]byte, 16)
		for k := range i % 6 {
			p := (29*i + 41*k) % 128
			mask[p/8] |= 0x80 >> (p % 8)
		}
		seeds = append(seeds, embeddedLCSeed{d[:10], mask, i%2 == 0})
	}
	// Testdata 11fdda4b86eb35a8: 6 errors, 5 of them in row 4, at the same
	// distance from the sent LC and another one.
	return append(seeds, embeddedLCSeed{[]byte{0x30}, []byte{0x08, 0x08, 0x08, 0x08, 0x08, 0x20}, false})
}

// checkEmbeddedLC checks the properties of FuzzEmbeddedLC for one input.
func checkEmbeddedLC(t testing.TB, data []byte, mask []byte, parity bool) {
	t.Helper()
	var info [10]byte
	copy(info[:], data)
	info[9] &= 0xF8
	var m [16]byte
	copy(m[:], mask)
	var e [4]uint32
	for i := range e {
		e[i] = binary.BigEndian.Uint32(m[4*i:])
	}

	cw := bptc.EncodeEmbedded(info)
	var rx [4]uint32
	for i := range rx {
		rx[i] = cw[i] ^ e[i]
	}
	n := embeddedDistance(cw, rx)
	got, res := bptc.DecodeEmbedded(rx)
	if n <= 3 && (got != info || res != (fec.Result{BitsChecked: 128, ErrorsCorrected: n})) {
		t.Fatalf("%d errors %x: got %x %+v, want %x", n, e, got, res, info)
	}
	if !res.Uncorrectable {
		if dist := embeddedDistance(bptc.EncodeEmbedded(got), rx); dist != res.ErrorsCorrected || dist > bptc.EmbeddedMaxErrors {
			t.Fatalf("errors %x: decoded LC is %d bits from the received one, ErrorsCorrected %d", e, dist, res.ErrorsCorrected)
		}
		if n <= bptc.EmbeddedMaxErrors && (res.ErrorsCorrected > n || res.ErrorsCorrected == n && got != info) {
			t.Fatalf("%d errors %x: decoded %x at distance %d, sent %x", n, e, got, res.ErrorsCorrected, info)
		}
	}

	refRx := unpackFragments(rx)
	if wantInfo, wantRes := ref.DecodeEmbedded(refRx); got != packEmbeddedInfo(wantInfo) || res != wantRes {
		t.Fatalf("DecodeEmbedded(%x) = %x %+v, reference %x %+v", rx, got, res, packEmbeddedInfo(wantInfo), wantRes)
	}

	check := func(p [10]byte) bool {
		ones := 0
		for _, b := range p {
			ones += bits.OnesCount8(b)
		}
		return (ones%2 == 1) == parity
	}
	got, res = bptc.DecodeEmbeddedWithCheck(rx, check)
	if !res.Uncorrectable && !check(got) {
		t.Fatalf("DecodeEmbeddedWithCheck(%x) returned %x, which check rejects", rx, got)
	}
	wantInfo, wantRes := ref.DecodeEmbeddedWithCheck(refRx, func(i [77]unpacked.Bit) bool { return check(packEmbeddedInfo(i)) })
	if got != packEmbeddedInfo(wantInfo) || res != wantRes {
		t.Fatalf("DecodeEmbeddedWithCheck(%x) = %x %+v, reference %x %+v", rx, got, res, packEmbeddedInfo(wantInfo), wantRes)
	}

	got, res = bptc.DecodeEmbeddedStrict(rx)
	if n == 0 && (got != info || res != (fec.Result{BitsChecked: 128})) {
		t.Fatalf("DecodeEmbeddedStrict of a clean LC: got %x %+v, want %x", got, res, info)
	}
	if !res.Uncorrectable && embeddedDistance(bptc.EncodeEmbedded(got), rx) != res.ErrorsCorrected {
		t.Fatalf("DecodeEmbeddedStrict(%x) = %x %+v is not a codeword at that distance", rx, got, res)
	}
	if wantInfo, wantRes := ref.DecodeEmbeddedStrict(refRx); got != packEmbeddedInfo(wantInfo) || res != wantRes {
		t.Fatalf("DecodeEmbeddedStrict(%x) = %x %+v, reference %x %+v", rx, got, res, packEmbeddedInfo(wantInfo), wantRes)
	}
}

// FuzzEmbeddedLC encodes fuzzed embedded LC information bits, adds the
// error pattern mask and decodes. Up to 3 errors decode to the information
// bits. A decode that is not uncorrectable returns a codeword
// ErrorsCorrected bits from the received fragments, no farther than the
// sent one when at most EmbeddedMaxErrors errors were added, and the sent
// one at the same distance. DecodeEmbedded, DecodeEmbeddedWithCheck and
// DecodeEmbeddedStrict agree with the reference codec.
func FuzzEmbeddedLC(f *testing.F) {
	for _, s := range embeddedLCSeeds(f) {
		f.Add(s.data, s.mask, s.parity)
	}
	f.Fuzz(func(t *testing.T, data []byte, mask []byte, parity bool) {
		checkEmbeddedLC(t, data, mask, parity)
	})
}

// cachSeed is one seed of FuzzCACH.
type cachSeed struct {
	info uint64
	mask []byte
}

// cachSeeds returns the seeds of FuzzCACH.
func cachSeeds(tb testing.TB) []cachSeed {
	blocks := captureBlocks(tb)
	seeds := make([]cachSeed, 0, len(blocks)+1)
	for i, d := range blocks {
		mask := make([]byte, 16)
		for k := range i % 4 {
			p := (19*i + 23*k) % 68
			mask[p/17*4+3-p%17/8] |= 1 << (p % 17 % 8)
		}
		seeds = append(seeds, cachSeed{binary.BigEndian.Uint64(d[:8]) >> 28, mask})
	}
	return seeds
}

// checkCACH checks the properties of FuzzCACH for one input.
func checkCACH(t testing.TB, info uint64, mask []byte) {
	t.Helper()
	info &= 1<<36 - 1
	var m [16]byte
	copy(m[:], mask)
	cw := bptc.EncodeCACH(info)
	var rx, junk [4]uint32
	n := 0
	for i := range rx {
		w := binary.BigEndian.Uint32(m[4*i:])
		rx[i] = cw[i] ^ w&0x1FFFF
		junk[i] = rx[i] | w&^0x1FFFF
		n += bits.OnesCount32(w & 0x1FFFF)
	}
	got, res := bptc.DecodeCACH(rx)
	switch {
	case n <= 2 && (got != info || res != (fec.Result{BitsChecked: 68, ErrorsCorrected: n})):
		t.Fatalf("%d errors %x: got %x %+v, want %x", n, rx, got, res, info)
	case n == 3 && !res.Uncorrectable && (got != info || res.ErrorsCorrected != 3):
		t.Fatalf("3 errors %x: accepted %x %+v, want %x", rx, got, res, info)
	}
	if !res.Uncorrectable && embeddedDistance(bptc.EncodeCACH(got), rx) != res.ErrorsCorrected {
		t.Fatalf("DecodeCACH(%x) = %x %+v is not a codeword at that distance", rx, got, res)
	}
	if g, r := bptc.DecodeCACH(junk); g != got || r != res {
		t.Fatalf("DecodeCACH(%x) = %x %+v, without the high bits %x %+v", junk, g, r, got, res)
	}

	var refRx [4][17]unpacked.Bit
	for i := range rx {
		unpacked.PutUint(refRx[i][:], uint64(rx[i]))
	}
	wantInfo, wantRes := ref.DecodeCACH(refRx)
	if got != unpacked.Uint(wantInfo[:]) || res != wantRes {
		t.Fatalf("DecodeCACH(%x) = %x %+v, reference %x %+v", rx, got, res, unpacked.Uint(wantInfo[:]), wantRes)
	}
	var refInfo [36]unpacked.Bit
	unpacked.PutUint(refInfo[:], info)
	for i, frag := range ref.EncodeCACH(refInfo) {
		if uint32(unpacked.Uint(frag[:])) != cw[i] {
			t.Fatalf("EncodeCACH(%x) = %x, reference fragment %d %x", info, cw, i, unpacked.Uint(frag[:]))
		}
	}
}

// FuzzCACH encodes a fuzzed Short LC with the CACH BPTC, adds the error
// pattern mask to the 68 coded bits and junk above them, and decodes. Up to
// 2 errors decode to the Short LC, and 3 errors decode to it or set
// Uncorrectable. A decode that is not uncorrectable returns a codeword
// ErrorsCorrected bits from the received payloads. DecodeCACH ignores the
// bits above the 17th and agrees with the reference codec.
func FuzzCACH(f *testing.F) {
	for _, s := range cachSeeds(f) {
		f.Add(s.info, s.mask)
	}
	f.Fuzz(func(t *testing.T, info uint64, mask []byte) {
		checkCACH(t, info, mask)
	})
}

// singleBurstSeed is one seed of FuzzSingleBurst.
type singleBurstSeed struct {
	info uint16
	odd  bool
	mask uint32
}

// singleBurstSeeds returns the seeds of FuzzSingleBurst.
func singleBurstSeeds(tb testing.TB) []singleBurstSeed {
	blocks := captureBlocks(tb)
	seeds := make([]singleBurstSeed, 0, len(blocks)+1)
	for i, d := range blocks {
		mask := uint32(0)
		for k := range i % 6 {
			mask |= 1 << ((7*i + 11*k) % 32)
		}
		seeds = append(seeds, singleBurstSeed{binary.BigEndian.Uint16(d[:2]), i%3 == 0, mask})
	}
	return seeds
}

// checkSingleBurst checks the properties of FuzzSingleBurst for one input.
func checkSingleBurst(t testing.TB, info uint16, odd bool, mask uint32) {
	t.Helper()
	info &= 0x7FF
	cw := bptc.EncodeSingleBurst(info, odd)
	rx := cw ^ mask
	n := bits.OnesCount32(mask)
	got, res := bptc.DecodeSingleBurst(rx, odd)
	switch {
	case n <= 3 && (got != info || res != (fec.Result{BitsChecked: 32, ErrorsCorrected: n})):
		t.Fatalf("%d errors %08x: got %03x %+v, want %03x", n, mask, got, res, info)
	case n == 4 && !res.Uncorrectable:
		t.Fatalf("4 errors %08x: accepted %03x %+v", mask, got, res)
	}
	if got > 0x7FF {
		t.Fatalf("DecodeSingleBurst(%08x) = %#x, above 11 bits", rx, got)
	}
	if !res.Uncorrectable {
		if dist := bits.OnesCount32(bptc.EncodeSingleBurst(got, odd) ^ rx); dist != res.ErrorsCorrected || dist > 3 {
			t.Fatalf("DecodeSingleBurst(%08x) = %03x %+v is %d bits away", rx, got, res, dist)
		}
	}

	var refRx [32]unpacked.Bit
	unpacked.PutUint(refRx[:], uint64(rx))
	wantInfo, wantRes := ref.DecodeSingleBurst(refRx, odd)
	if uint64(got) != unpacked.Uint(wantInfo[:]) || res != wantRes {
		t.Fatalf("DecodeSingleBurst(%08x, %v) = %03x %+v, reference %03x %+v", rx, odd, got, res, unpacked.Uint(wantInfo[:]), wantRes)
	}
	var refInfo [11]unpacked.Bit
	unpacked.PutUint(refInfo[:], uint64(info))
	if want := ref.EncodeSingleBurst(refInfo, odd); uint64(cw) != unpacked.Uint(want[:]) {
		t.Fatalf("EncodeSingleBurst(%03x, %v) = %08x, reference %08x", info, odd, cw, unpacked.Uint(want[:]))
	}
}

// FuzzSingleBurst encodes fuzzed information bits with the single burst
// BPTC, even or odd parity, adds the error pattern mask and decodes. Up to 3
// errors decode to the information bits, and 4 set Uncorrectable. A decode
// that is not uncorrectable returns a codeword ErrorsCorrected bits from the
// received word. Both codecs agree with the reference codec.
func FuzzSingleBurst(f *testing.F) {
	for _, s := range singleBurstSeeds(f) {
		f.Add(s.info, s.odd, s.mask)
	}
	f.Fuzz(func(t *testing.T, info uint16, odd bool, mask uint32) {
		checkSingleBurst(t, info, odd, mask)
	})
}
