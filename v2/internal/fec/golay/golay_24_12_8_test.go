package golay

import (
	"math/bits"
	"math/rand"
	"testing"
	"time"
)

func TestGolay24128(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for d := 0; d < 4096; d++ {
		data := uint16(d)

		codeword := Encode24128(data)

		decoded, result := Decode24128(codeword)
		if result.Uncorrectable {
			t.Fatalf("Data 0x%03x: Clean codeword reported as uncorrectable", data)
		}
		if result.ErrorsCorrected != 0 {
			t.Fatalf("Data 0x%03x: Clean codeword reported %d errors", data, result.ErrorsCorrected)
		}
		if decoded != data {
			t.Fatalf("Data 0x%03x: Clean codeword decoded as 0x%03x", data, decoded)
		}

		for k := 0; k < 5; k++ {
			bit1 := uint(rng.Intn(24))
			cwErr1 := codeword ^ (1 << bit1)
			decoded, result = Decode24128(cwErr1)
			switch {
			case result.Uncorrectable:
				t.Errorf("Data 0x%03x (1 error): Reported uncorrectable", data)
			case result.ErrorsCorrected != 1:
				t.Errorf("Data 0x%03x (1 error): Reported %d errors", data, result.ErrorsCorrected)
			case decoded != data:
				t.Errorf("Data 0x%03x (1 error): Decoded as 0x%03x", data, decoded)
			}

			bit2 := uint(rng.Intn(24))
			for bit2 == bit1 {
				bit2 = uint(rng.Intn(24))
			}
			cwErr2 := cwErr1 ^ (1 << bit2)
			decoded, result = Decode24128(cwErr2)
			switch {
			case result.Uncorrectable:
				t.Errorf("Data 0x%03x (2 errors): Reported uncorrectable", data)
			case result.ErrorsCorrected != 2:
				t.Errorf("Data 0x%03x (2 errors): Reported %d errors", data, result.ErrorsCorrected)
			case decoded != data:
				t.Errorf("Data 0x%03x (2 errors): Decoded as 0x%03x", data, decoded)
			}

			bit3 := uint(rng.Intn(24))
			for bit3 == bit1 || bit3 == bit2 {
				bit3 = uint(rng.Intn(24))
			}
			cwErr3 := cwErr2 ^ (1 << bit3)
			decoded, result = Decode24128(cwErr3)
			switch {
			case result.Uncorrectable:
				t.Errorf("Data 0x%03x (3 errors): Reported uncorrectable", data)
			case result.ErrorsCorrected != 3:
				t.Errorf("Data 0x%03x (3 errors): Reported %d errors", data, result.ErrorsCorrected)
			case decoded != data:
				t.Errorf("Data 0x%03x (3 errors): Decoded as 0x%03x", data, decoded)
			}

			bit4 := uint(rng.Intn(24))
			for bit4 == bit1 || bit4 == bit2 || bit4 == bit3 {
				bit4 = uint(rng.Intn(24))
			}
			cwErr4 := cwErr3 ^ (1 << bit4)
			_, result = Decode24128(cwErr4)
			if !result.Uncorrectable {
				t.Errorf("Data 0x%03x (4 errors): Expected uncorrectable, got corrected", data)
			}
		}
	}
}

func TestDecodeGolay24128UncorrectableReturnsNearest23127(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(5))
	checked := 0
	for range 20000 {
		var err uint32
		for bits.OnesCount32(err) < 4 {
			err |= 1 << rng.Intn(24)
		}
		received := uint32(rng.Intn(1<<24)) ^ err
		data, res := Decode24128(received)
		if !res.Uncorrectable {
			continue
		}
		checked++
		want, _ := Decode23127(received >> 1)
		if data != want {
			t.Fatalf("received %06x: data %03x, want nearest Golay(23,12) data %03x", received, data, want)
		}
	}
	if checked == 0 {
		t.Fatal("no uncorrectable words generated")
	}
}

func TestEncodeGolay24128ExtendsGolay23127(t *testing.T) {
	t.Parallel()

	if got := Encode23127(1); got != golay23GenPoly {
		t.Fatalf("Encode23127(1) = %#x, want g(x) %#x", got, golay23GenPoly)
	}
	for d := range uint16(4096) {
		cw23 := Encode23127(d)
		if cw23>>11 != uint32(d) || golay23ComputeSyndrome(cw23) != 0 {
			t.Fatalf("Encode23127(%#x) = %#x is not a systematic codeword", d, cw23)
		}
		cw24 := Encode24128(d)
		if cw24>>1 != cw23 || bits.OnesCount32(cw24)%2 != 0 {
			t.Fatalf("Encode24128(%#x) = %#x, want %#x with even parity", d, cw24, cw23)
		}
		if Encode24128(d|0xF000) != cw24 || Encode23127(d|0xF000) != cw23 {
			t.Fatalf("encoders must ignore bits above 12 for %#x", d)
		}
	}
}

func TestDecodeGolayWideInput(t *testing.T) {
	t.Parallel()

	cw24 := Encode24128(0x5A5)
	for _, high := range []uint32{1 << 24, 1 << 28, 1 << 30, 1 << 31} {
		data, res := Decode24128(cw24 | high)
		if data != 0x5A5 || res.ErrorsCorrected != 0 || !res.Uncorrectable || res.BitsChecked != 24 {
			t.Errorf("Golay(24,12) %08x: data %03x result %+v, want data 5a5 with only Uncorrectable", cw24|high, data, res)
		}
	}
	cw23 := cw24 >> 1
	for _, high := range []uint32{1 << 23, 1 << 30} {
		data, res := Decode23127(cw23 | high)
		if data != 0x5A5 || res.ErrorsCorrected != 0 || !res.Uncorrectable || res.BitsChecked != 23 {
			t.Errorf("Golay(23,12) %08x: data %03x result %+v, want data 5a5 with only Uncorrectable", cw23|high, data, res)
		}
	}
}
