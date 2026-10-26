package bptc

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func TestBPTC19696Decode(t *testing.T) {
	t.Parallel()

	for run := 0; run < 100; run++ {
		var data [96]unpacked.Bit
		for i := 0; i < 96; i++ {
			data[i] = unpacked.Bit(rand.Intn(2))
		}

		encoded := encode196Bits(data)

		decoded, result := decode196Bits(encoded)

		if result.Uncorrectable {
			t.Errorf("Failed to decode clean codeword: uncorrectable")
		}
		if result.ErrorsCorrected != 0 {
			t.Errorf("Decoded clean codeword with %d errors", result.ErrorsCorrected)
		}

		if decoded != data {
			t.Errorf("Decoded payload mismatch")
		}

		encodedErr := encoded
		pos := rand.Intn(196)
		for pos == 0 {
			pos = rand.Intn(196)
		}
		encodedErr[pos] ^= 1

		decoded, result = decode196Bits(encodedErr)

		if result.Uncorrectable {
			t.Errorf("Failed to correct 1 error")
		}

		if result.ErrorsCorrected != 1 {
			t.Errorf("Expected 1 error corrected, got %d", result.ErrorsCorrected)
		}

		if decoded != data {
			t.Errorf("Decoded 1-error payload mismatch")
		}
	}
}

func TestBPTC19696Reserved(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(7))
	for run := 0; run < 100; run++ {
		var data [96]unpacked.Bit
		for i := range data {
			data[i] = unpacked.Bit(r.Intn(2))
		}
		res := [3]unpacked.Bit{unpacked.Bit(run & 1), unpacked.Bit(run >> 1 & 1), unpacked.Bit(run >> 2 & 1)}
		encoded := encode196BitsReserved(data, res)
		if res == ([3]unpacked.Bit{}) && encoded != encode196Bits(data) {
			t.Fatalf("encode with zero R differs from encode without R")
		}
		pos := r.Intn(196)
		encoded[pos] ^= 1
		gotData, gotRes, result := decode196BitsReserved(encoded)
		if gotData != data || gotRes != res || result.Uncorrectable {
			t.Errorf("R %v, error at %d: decoded R %v, data ok %t, %+v", res, pos, gotRes, gotData == data, result)
		}
	}
}

func TestBPTC19696ReservedPositions(t *testing.T) {
	t.Parallel()

	for i, txBit := range []int{151, 166, 181} {
		var res [3]unpacked.Bit
		res[i] = 1
		encoded := encode196BitsReserved([96]unpacked.Bit{}, res)
		if encoded[txBit] != 1 {
			t.Errorf("R(%d) is not at transmit bit %d (ETSI TS 102 361-1 Table B.1)", i, txBit)
		}
	}
}
