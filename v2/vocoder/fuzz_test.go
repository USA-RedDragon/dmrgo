package vocoder_test

import (
	"errors"
	"math/bits"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	refambe "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/ambe"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// ambeSections returns the slot bits of the Golay(24,12) section A, the
// Golay(23,12) section B and the unprotected section C. Slot bit c+4j is
// bit j of interleave column c, and the sections fill the columns in order:
// A the 18 bits of column 0 and 6 of column 1, B the other 12 of column 1
// and 11 of column 2, and C the rest.
func ambeSections() (a, b, c [9]byte) {
	for k := range 72 {
		i := 18*(k%4) + k/4
		switch {
		case i < 24:
			a[k/8] |= 0x80 >> (k % 8)
		case i < 47:
			b[k/8] |= 0x80 >> (k % 8)
		default:
			c[k/8] |= 0x80 >> (k % 8)
		}
	}
	return a, b, c
}

// ones returns the number of bits set in x and m.
func ones(x, m [9]byte) int {
	n := 0
	for i := range x {
		n += bits.OnesCount8(x[i] & m[i])
	}
	return n
}

// ambe72Seed is one seed of FuzzAMBE72.
type ambe72Seed struct {
	data []byte
	mask []byte
}

// ambe72Seeds returns the seeds of FuzzAMBE72.
func ambe72Seeds(tb testing.TB) []ambe72Seed {
	var seeds []ambe72Seed
	for i, slot := range voiceSlots(tb) {
		if i%9 != 0 {
			continue
		}
		frame, res := vocoder.DecodeAMBE72(slot)
		if res.HasErrors() {
			continue
		}
		mask := make([]byte, 9)
		for k := range i % 7 {
			p := (13*i + 31*k) % 72
			mask[p/8] |= 0x80 >> (p % 8)
		}
		seeds = append(seeds, ambe72Seed{frame[:], mask})
	}
	silence := vocoder.AMBESilence()
	seeds = append(seeds, ambe72Seed{silence[:], []byte{0xFF, 0, 0, 0, 0, 0, 0, 0, 0x01}})
	return seeds
}

// checkAMBE72 checks the properties of FuzzAMBE72 for one input.
func checkAMBE72(t testing.TB, data []byte, mask []byte) {
	t.Helper()
	secA, secB, secC := ambeSections()
	var frame vocoder.AMBEFrame
	copy(frame[:], data)
	var rangeErr *dmrgo.FieldRangeError
	if _, err := vocoder.EncodeAMBE72(frame); (frame[6]&0x7F != 0) != errors.As(err, &rangeErr) {
		t.Fatalf("EncodeAMBE72(%x) error %v", frame, err)
	}
	frame[6] &= 0x80
	var e [9]byte
	copy(e[:], mask)

	cw, err := vocoder.EncodeAMBE72(frame)
	if err != nil {
		t.Fatalf("EncodeAMBE72(%x): %v", frame, err)
	}
	var in [49]unpacked.Bit
	unpacked.UnpackAt(in[:], frame[:], 0)
	refCw := refambe.Encode(in)
	var want [9]byte
	unpacked.PackAt(want[:], 0, refCw[:])
	if cw != want {
		t.Fatalf("EncodeAMBE72(%x) = %x, reference %x", frame, cw, want)
	}

	var rx [9]byte
	for i := range rx {
		rx[i] = cw[i] ^ e[i]
	}
	got, res := vocoder.DecodeAMBE72(rx)
	dec, err := vocoder.EncodeAMBE72(got)
	if err != nil {
		t.Fatalf("DecodeAMBE72(%x) = %x, which does not encode: %v", rx, got, err)
	}
	var diff, coded [9]byte
	for i := range diff {
		diff[i] = dec[i] ^ rx[i]
		coded[i] = e[i] &^ secC[i]
	}
	nA, nB := ones(e, secA), ones(e, secB)
	switch {
	case nA <= 3 && nB <= 3 && (diff != coded || res != (fec.Result{BitsChecked: 47, ErrorsCorrected: nA + nB})):
		t.Fatalf("errors %x: got %x %+v, slot %x away", e, got, res, diff)
	case nA == 4 && !res.Uncorrectable:
		t.Fatalf("4 errors in section A %x: accepted %x %+v", e, got, res)
	case nA <= 3 && (got[0] != frame[0] || got[1]>>4 != frame[1]>>4):
		t.Fatalf("errors %x: section A decoded to %x, want %x", e, got[:2], frame[:2])
	}
	if ones(diff, secC) != 0 {
		t.Fatalf("DecodeAMBE72(%x) = %x changed section C", rx, got)
	}
	if !res.Uncorrectable && ones(diff, secA)+ones(diff, secB) != res.ErrorsCorrected {
		t.Fatalf("DecodeAMBE72(%x) = %x %+v re-encodes %x away", rx, got, res, diff)
	}

	var refRx [72]unpacked.Bit
	unpacked.UnpackAt(refRx[:], rx[:], 0)
	wantInfo, wantRes := refambe.Decode(refRx)
	var wantFrame vocoder.AMBEFrame
	unpacked.PackAt(wantFrame[:], 0, wantInfo[:])
	if got != wantFrame || res != wantRes {
		t.Fatalf("DecodeAMBE72(%x) = %x %+v, reference %x %+v", rx, got, res, wantFrame, wantRes)
	}
}

// FuzzAMBE72 encodes a fuzzed AMBE+2 frame, adds the error pattern mask to
// the slot and decodes. With up to 3 errors in each of sections A and B the
// frame comes back with the errors of C in its C bits, and ErrorsCorrected
// counts the errors of A and B. 4 errors in A set Uncorrectable, and up to 3
// in A decode the bits of A whatever B holds. Section C always passes
// through, a decode without Uncorrectable re-encodes ErrorsCorrected bits
// from the slot, and both functions agree with the reference codec.
func FuzzAMBE72(f *testing.F) {
	for _, s := range ambe72Seeds(f) {
		f.Add(s.data, s.mask)
	}
	f.Fuzz(func(t *testing.T, data []byte, mask []byte) {
		checkAMBE72(t, data, mask)
	})
}

// ambeVoiceSeed is one seed of FuzzAMBEVoice.
type ambeVoiceSeed struct {
	f1      []byte
	f2      []byte
	f3      []byte
	flags   uint8
	payload []byte
}

// ambeVoiceSeeds returns the seeds of FuzzAMBEVoice.
func ambeVoiceSeeds() []ambeVoiceSeed {
	silence := vocoder.AMBESilence()
	ones := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	return []ambeVoiceSeed{
		{silence[:], silence[:], silence[:], 0, make([]byte, 19)},
		{[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x80}, []byte{1, 2, 3, 4, 5, 6, 0}, []byte{0x80}, 6, ones},
	}
}

// checkAMBEVoice checks the properties of FuzzAMBEVoice for one input.
func checkAMBEVoice(t testing.TB, f1, f2, f3 []byte, flags uint8, payload []byte) {
	t.Helper()
	var frames [3]vocoder.AMBEFrame
	copy(frames[0][:], f1)
	copy(frames[1][:], f2)
	copy(frames[2][:], f3)
	bad := [3]bool{flags&1 != 0, flags&2 != 0, flags&4 != 0}
	padded := frames[0][6]&0x7F != 0 || frames[1][6]&0x7F != 0 || frames[2][6]&0x7F != 0

	data, err := vocoder.PackAMBEVoice(frames)
	dataFlags, errFlags := vocoder.PackAMBEVoiceFlags(frames, bad)
	var rangeErr *dmrgo.FieldRangeError
	if padded != errors.As(err, &rangeErr) || padded != errors.As(errFlags, &rangeErr) {
		t.Fatalf("PackAMBEVoice(%x) errors %v, %v", frames, err, errFlags)
	}
	if !padded {
		if got := vocoder.UnpackAMBEVoice(data); got != frames {
			t.Fatalf("UnpackAMBEVoice(PackAMBEVoice(%x)) = %x", frames, got)
		}
		got, gotBad := vocoder.UnpackAMBEVoiceFlags(dataFlags)
		if got != frames || gotBad != [3]bool{false, bad[1], bad[2]} {
			t.Fatalf("UnpackAMBEVoiceFlags(PackAMBEVoiceFlags(%x, %v)) = %x %v", frames, bad, got, gotBad)
		}
		if got, _ := vocoder.UnpackAMBEVoiceFlags(data); got != frames {
			t.Fatalf("UnpackAMBEVoiceFlags(PackAMBEVoice(%x)) = %x", frames, got)
		}
	}

	var p [19]byte
	copy(p[:], payload)
	var frameBits, flagBits [19]byte
	for i := range 152 {
		switch {
		case i == 49 || i == 99:
			flagBits[i/8] |= 0x80 >> (i % 8)
		case i < 149 && i%50 < 49:
			frameBits[i/8] |= 0x80 >> (i % 8)
		}
	}
	again, err := vocoder.PackAMBEVoice(vocoder.UnpackAMBEVoice(p))
	if err != nil {
		t.Fatalf("PackAMBEVoice(UnpackAMBEVoice(%x)): %v", p, err)
	}
	gotFrames, gotBad := vocoder.UnpackAMBEVoiceFlags(p)
	againFlags, err := vocoder.PackAMBEVoiceFlags(gotFrames, gotBad)
	if err != nil || gotBad[0] {
		t.Fatalf("PackAMBEVoiceFlags(UnpackAMBEVoiceFlags(%x)): %v %v", p, gotBad, err)
	}
	for i := range p {
		if again[i] != p[i]&frameBits[i] || againFlags[i] != p[i]&(frameBits[i]|flagBits[i]) {
			t.Fatalf("payload %x packs again to %x and %x", p, again, againFlags)
		}
	}
}

// FuzzAMBEVoice packs three fuzzed frames and bad flags into the IPSC AMBE
// payload and unpacks them again, and unpacks a fuzzed payload and packs it
// again. The frames and the flags of frames 2 and 3 round-trip, a frame
// with a padding bit set is an error, and a payload keeps only its frame
// and flag bits.
func FuzzAMBEVoice(f *testing.F) {
	for _, s := range ambeVoiceSeeds() {
		f.Add(s.f1, s.f2, s.f3, s.flags, s.payload)
	}
	f.Fuzz(func(t *testing.T, f1, f2, f3 []byte, flags uint8, payload []byte) {
		checkAMBEVoice(t, f1, f2, f3, flags, payload)
	})
}
