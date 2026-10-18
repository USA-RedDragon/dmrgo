package trellis

import (
	"os"
	"path/filepath"
	"testing"

	packed "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	reftrellis "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// oneStepError reports whether e changes exactly one dibit of sent, and that
// dibit by one deviation step.
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

// FuzzTrellis encodes fuzzed data with the rate 3/4 trellis code, adds the
// error pattern mask and decodes. Encode agrees with the reference encoder.
// A clean block or one 1-step dibit error decodes to the data. Every decode
// counts the bits between the received and the decoded block, has a path
// metric no higher than that of the sent block, and sets Uncorrectable when
// it returns another block with the same metric.
func FuzzTrellis(f *testing.F) {
	files, err := filepath.Glob("../../../layer2/testdata/*.bin")
	if err != nil || len(files) == 0 {
		f.Fatalf("no captures: %v", err)
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			f.Fatal(err)
		}
		for i := 0; i+33 <= len(data); i += 33 {
			var info [25]byte
			packed.Copy(info[:], 0, data[i:i+33], 0, 98)
			packed.Copy(info[:], 98, data[i:i+33], 166, 98)
			if d, res := Decode(info); res.ErrorsCorrected == 0 && !res.Uncorrectable {
				f.Add(d[:], []byte{byte(i), 0, byte(i >> 8)})
			}
		}
	}
	f.Add(make([]byte, 18), []byte{0x40})
	f.Add(make([]byte, 18), []byte{0xC0, 0, 0, 0, 0x03})

	f.Fuzz(func(t *testing.T, data []byte, mask []byte) {
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
	})
}
