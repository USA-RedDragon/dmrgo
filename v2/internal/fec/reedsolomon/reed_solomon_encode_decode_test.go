package reedsolomon

import (
	"math/bits"
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	testData := [][]byte{
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09},
		{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9, 0xF8, 0xF7},
		{0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA},
		{0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}

	for i, data := range testData {
		encoded, err := encode(data)
		if err != nil {
			t.Fatalf("test %d: Encode failed: %v", i, err)
		}

		if len(encoded) != dataSize+checksumSize {
			t.Fatalf("test %d: Encode returned %d bytes, want %d", i, len(encoded), dataSize+checksumSize)
		}

		for j := 0; j < dataSize; j++ {
			if encoded[j] != data[j] {
				t.Errorf("test %d: encoded[%d] = 0x%02X, want 0x%02X", i, j, encoded[j], data[j])
			}
		}

		result := decode(encoded)
		if result.Uncorrectable {
			t.Errorf("test %d: Decode reported uncorrectable", i)
		}
		if result.ErrorsFound != 0 {
			t.Errorf("test %d: Decode found %d errors, want 0", i, result.ErrorsFound)
		}
		for j := 0; j < dataSize; j++ {
			if result.Data[j] != data[j] {
				t.Errorf("test %d: decoded[%d] = 0x%02X, want 0x%02X", i, j, result.Data[j], data[j])
			}
		}
	}
}

func TestSyndromeDetectsCorruption(t *testing.T) {
	data := make([]byte, dataSize+checksumSize)

	corrupted := make([]byte, len(data))
	copy(corrupted, data)
	corrupted[3] = 0xFF

	var syndrome poly
	err := calcSyndrome(corrupted, &syndrome)
	if err != nil {
		t.Fatalf("CalcSyndrome failed: %v", err)
	}

	if !syndromeHasErrors(&syndrome) {
		t.Error("Corrupted all-zero codeword should have non-zero syndrome")
	}
}

func TestDecodeTableB18Codewords(t *testing.T) {
	table := tableB18Parity()
	for row := 0; row < dataSize; row++ {
		cw := make([]byte, dataSize+checksumSize)
		cw[row] = 0x01
		copy(cw[dataSize:], table[row][:])
		result := decode(cw)
		if result.Uncorrectable || result.ErrorsFound != 0 {
			t.Errorf("row %d: Uncorrectable=%v ErrorsFound=%d, want clean", row, result.Uncorrectable, result.ErrorsFound)
		}
		if string(result.Data) != string(cw[:dataSize]) {
			t.Errorf("row %d: data % X, want % X", row, result.Data, cw[:dataSize])
		}
	}
}

func TestDecodeSingleSymbolErrorAllPositions(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	for n := 0; n < 8; n++ {
		data := make([]byte, dataSize)
		r.Read(data)
		cw, err := encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		for pos := 0; pos < len(cw); pos++ {
			for e := 1; e < 256; e++ {
				rx := append([]byte(nil), cw...)
				rx[pos] ^= byte(e)
				for _, result := range []decodeResult{decode(rx)} {
					if result.Uncorrectable || result.ErrorsFound != 1 || result.BitsCorrected != bits.OnesCount8(uint8(e)) || string(result.Data) != string(data) {
						t.Fatalf("pos %d err 0x%02X: Uncorrectable=%v ErrorsFound=%d BitsCorrected=%d data % X, want % X",
							pos, e, result.Uncorrectable, result.ErrorsFound, result.BitsCorrected, result.Data, data)
					}
				}
			}
		}
	}
}

func TestDecodeDoubleSymbolErrorsDetected(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for n := 0; n < 50000; n++ {
		data := make([]byte, dataSize)
		r.Read(data)
		cw, err := encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		rx := append([]byte(nil), cw...)
		for _, p := range r.Perm(len(cw))[:2] {
			rx[p] ^= byte(r.Intn(255) + 1)
		}
		if result := decode(rx); !result.Uncorrectable {
			t.Fatalf("rx % X (sent % X): two symbol errors not flagged, ErrorsFound=%d", rx, cw, result.ErrorsFound)
		}
	}
}

func TestDecodeTripleSymbolErrorsNeverInvalid(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	for n := 0; n < 50000; n++ {
		data := make([]byte, dataSize)
		r.Read(data)
		cw, err := encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		rx := append([]byte(nil), cw...)
		for _, p := range r.Perm(len(cw))[:3] {
			rx[p] ^= byte(r.Intn(255) + 1)
		}
		result := decode(rx)
		if result.Uncorrectable {
			continue
		}
		if result.ErrorsFound != 1 {
			t.Fatalf("rx % X: accepted with ErrorsFound=%d", rx, result.ErrorsFound)
		}
		reencoded, err := encode(result.Data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		diff := 0
		for i := range rx {
			if rx[i] != reencoded[i] {
				diff++
			}
		}
		if diff != 1 {
			t.Fatalf("rx % X: accepted codeword at distance %d", rx, diff)
		}
	}
}

func TestEncodeInvalidLength(t *testing.T) {
	short := make([]byte, 5)
	_, err := encode(short)
	if err == nil {
		t.Error("Encode should return error for wrong size input")
	}

	long := make([]byte, 15)
	_, err = encode(long)
	if err == nil {
		t.Error("Encode should return error for too-long input")
	}
}

func TestPublicEncodeDecodeRoundTrip(t *testing.T) {
	data := [9]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99}
	cw := Encode(data)
	if !Check(cw) {
		t.Fatal("Check rejects an encoded codeword")
	}
	if got, res := Decode(cw); got != data || res != (fec.Result{BitsChecked: 96}) {
		t.Fatalf("Decode = % X %+v, want % X clean", got, res, data)
	}
	cw[4] ^= 0x81
	if Check(cw) {
		t.Fatal("Check accepts a corrupted codeword")
	}
	if got, res := Decode(cw); got != data || res.Uncorrectable || res.ErrorsCorrected != 2 {
		t.Fatalf("Decode = % X %+v, want % X with 2 corrected bits", got, res, data)
	}
}

func TestSyndromeCleanForValidCodeword(t *testing.T) {
	data := make([]byte, dataSize+checksumSize)

	var syndrome poly
	err := calcSyndrome(data, &syndrome)
	if err != nil {
		t.Fatalf("CalcSyndrome failed: %v", err)
	}

	if syndromeHasErrors(&syndrome) {
		t.Error("Valid all-zero codeword should have zero syndrome")
	}

	result := decode(data)
	if result.Uncorrectable {
		t.Error("Valid codeword should not be uncorrectable")
	}
	if result.ErrorsFound != 0 {
		t.Errorf("Valid codeword should have 0 errors, got %d", result.ErrorsFound)
	}
}

func TestEncodePreservesData(t *testing.T) {
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE, 0x00}
	encoded, err := encode(data)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(encoded) != dataSize+checksumSize {
		t.Fatalf("Encode returned %d bytes, want %d", len(encoded), dataSize+checksumSize)
	}

	for i := 0; i < dataSize; i++ {
		if encoded[i] != data[i] {
			t.Errorf("encoded[%d] = 0x%02X, want 0x%02X", i, encoded[i], data[i])
		}
	}

	allZero := true
	for i := dataSize; i < len(encoded); i++ {
		if encoded[i] != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("Checksum bytes should not all be zero for non-zero data")
	}
}

// tableB18Parity returns the parity columns of each row of ETSI TS 102 361-1 Table B.18.
func tableB18Parity() [dataSize][checksumSize]byte {
	return [dataSize][checksumSize]byte{
		{0x1C, 0xBC, 0xFD}, {0x89, 0x31, 0x08}, {0xAD, 0x41, 0x36},
		{0x7D, 0x71, 0x16}, {0xF3, 0xA6, 0x3A}, {0x08, 0x83, 0x7B},
		{0x3F, 0x6F, 0x02}, {0x6C, 0x0D, 0xA7}, {0x0E, 0x38, 0x40},
	}
}

func TestEncodeTableB18Rows(t *testing.T) {
	table := tableB18Parity()
	for row := 0; row < dataSize; row++ {
		data := make([]byte, dataSize)
		data[row] = 0x01
		encoded, err := encode(data)
		if err != nil {
			t.Fatalf("row %d: Encode failed: %v", row, err)
		}
		got := [checksumSize]byte(encoded[dataSize:])
		if got != table[row] {
			t.Errorf("row %d: parity % X, want % X", row, got, table[row])
		}
	}
}

func specParity(data []byte) [checksumSize]byte {
	table := tableB18Parity()
	var p [checksumSize]byte
	for i := range data {
		for j := range p {
			p[j] ^= refGaloisMul(data[i], table[i][j])
		}
	}
	return p
}

func TestEncodeMatchesGeneratorMatrix(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for n := 0; n < 2000; n++ {
		data := make([]byte, dataSize)
		r.Read(data)
		encoded, err := encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if got, want := [checksumSize]byte(encoded[dataSize:]), specParity(data); got != want {
			t.Fatalf("data % X: parity % X, want % X", data, got, want)
		}
	}
}

func TestEncodeKnownVector(t *testing.T) {
	data := []byte{0x03, 0x00, 0x00, 0x00, 0x27, 0x06, 0x30, 0xB4, 0x3C}
	encoded, err := encode(data)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	want := [checksumSize]byte{0xCB, 0x59, 0x57}
	if got := [checksumSize]byte(encoded[dataSize:]); got != want {
		t.Errorf("parity % X, want % X", got, want)
	}
}

func BenchmarkEncode(b *testing.B) {
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = encode(data)
	}
}
