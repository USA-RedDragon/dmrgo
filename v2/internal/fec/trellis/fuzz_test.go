package trellis

import (
	"os"
	"path/filepath"
	"testing"

	packed "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	reftrellis "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func oneStepError(sent, e *[25]byte) bool {
	changed := -1
	for i := range dibits {
		if dibitAt(e, i) == 0 {
			continue
		}
		if changed >= 0 {
			return false
		}
		changed = i
	}
	if changed < 0 {
		return false
	}
	rx := *sent
	for i := range rx {
		rx[i] ^= e[i]
	}
	d := level(dibitAt(&rx, changed)) - level(dibitAt(sent, changed))
	return d == 2 || d == -2
}

type trellisSeed struct {
	data []byte
	mask []byte
}

func trellisSeeds(tb testing.TB) []trellisSeed {
	var seeds []trellisSeed
	files, err := filepath.Glob("../../../layer2/testdata/*.bin")
	if err != nil || len(files) == 0 {
		tb.Fatalf("no captures: %v", err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			tb.Fatal(err)
		}
		for i := 0; i+33 <= len(data); i += 33 {
			var info [25]byte
			packed.Copy(info[:], 0, data[i:i+33], 0, 98)
			packed.Copy(info[:], 98, data[i:i+33], 166, 98)
			if d, res := Decode(info); res.ErrorsCorrected == 0 && !res.Uncorrectable {
				seeds = append(seeds, trellisSeed{d[:], []byte{byte(i), 0, byte(i >> 8)}})
			}
		}
	}
	seeds = append(seeds, trellisSeed{make([]byte, 18), []byte{0x40}})
	seeds = append(seeds, trellisSeed{make([]byte, 18), []byte{0xC0, 0, 0, 0, 0x03}})
	return seeds
}

func checkTrellis(t testing.TB, data []byte, mask []byte) {
	t.Helper()
	var d [18]byte
	copy(d[:], data)
	var e [25]byte
	copy(e[:], mask)
	e[24] &= 0xF0

	enc := Encode(d)
	var in [144]unpacked.Bit
	unpacked.UnpackAt(in[:], d[:], 0)
	want := reftrellis.Encode(in)
	var refEnc [25]byte
	unpacked.PackAt(refEnc[:], 0, want[:])
	if enc != refEnc {
		t.Fatalf("Encode(%x) = %x, reference %x", d, enc, refEnc)
	}

	rx := enc
	for i := range rx {
		rx[i] ^= e[i]
	}
	got, res := Decode(rx)
	if res.BitsChecked != 196 {
		t.Fatalf("BitsChecked %d, want 196", res.BitsChecked)
	}
	if (e == [25]byte{} || oneStepError(&enc, &e)) && (got != d || res.ErrorsCorrected != bitErrors(&rx, &enc) || res.Uncorrectable) {
		t.Fatalf("errors %x: got %x %+v, want %x", e, got, res, d)
	}
	dec := Encode(got)
	if res.ErrorsCorrected != bitErrors(&rx, &dec) {
		t.Fatalf("errors %x: ErrorsCorrected %d, want %d bits between received and decoded block", e, res.ErrorsCorrected, bitErrors(&rx, &dec))
	}
	mDec, mSent := cost(&rx, &dec), cost(&rx, &enc)
	switch {
	case mDec > mSent:
		t.Fatalf("errors %x: decoded metric %d is above the sent block's %d", e, mDec, mSent)
	case got != d && mDec == mSent && !res.Uncorrectable:
		t.Fatalf("errors %x: decoded block %x has the sent block's metric %d but no Uncorrectable", e, got, mDec)
	}
}

func FuzzTrellis(f *testing.F) {
	for _, s := range trellisSeeds(f) {
		f.Add(s.data, s.mask)
	}
	f.Fuzz(func(t *testing.T, data []byte, mask []byte) {
		checkTrellis(t, data, mask)
	})
}
