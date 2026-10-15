package reedsolomon

import (
	"math/rand"
	"testing"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	// Test that Encode -> Decode produces the original data
	testData := [][]byte{
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09},
		{0xFF, 0xFE, 0xFD, 0xFC, 0xFB, 0xFA, 0xF9, 0xF8, 0xF7},
		{0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA, 0x55, 0xAA},
		{0x42, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	}

	for i, data := range testData {
		encoded, err := Encode(data)
		if err != nil {
			t.Fatalf("test %d: Encode failed: %v", i, err)
		}

		if len(encoded) != RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE {
			t.Fatalf("test %d: Encode returned %d bytes, want %d", i, len(encoded), RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)
		}

		// Verify data portion is preserved
		for j := 0; j < RS_12_9_DATASIZE; j++ {
			if encoded[j] != data[j] {
				t.Errorf("test %d: encoded[%d] = 0x%02X, want 0x%02X", i, j, encoded[j], data[j])
			}
		}

		// Decode and verify
		result := Decode(encoded)
		if result.Uncorrectable {
			t.Errorf("test %d: Decode reported uncorrectable", i)
		}
		if result.ErrorsFound != 0 {
			t.Errorf("test %d: Decode found %d errors, want 0", i, result.ErrorsFound)
		}
		for j := 0; j < RS_12_9_DATASIZE; j++ {
			if result.Data[j] != data[j] {
				t.Errorf("test %d: decoded[%d] = 0x%02X, want 0x%02X", i, j, result.Data[j], data[j])
			}
		}
	}
}

func TestSyndromeDetectsCorruption(t *testing.T) {
	// All-zero codeword is valid
	data := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)

	// Corrupt one byte
	corrupted := make([]byte, len(data))
	copy(corrupted, data)
	corrupted[3] = 0xFF

	var syndrome ReedSolomon1294
	err := ReedSolomon1294CalcSyndrome(corrupted, &syndrome)
	if err != nil {
		t.Fatalf("CalcSyndrome failed: %v", err)
	}

	if !ReedSolomon1294CheckSyndrome(&syndrome) {
		t.Error("Corrupted all-zero codeword should have non-zero syndrome")
	}
}

func TestDecodeTableB18Codewords(t *testing.T) {
	table := tableB18Parity()
	for row := 0; row < RS_12_9_DATASIZE; row++ {
		cw := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)
		cw[row] = 0x01
		copy(cw[RS_12_9_DATASIZE:], table[row][:])
		result := Decode(cw)
		if result.Uncorrectable || result.ErrorsFound != 0 {
			t.Errorf("row %d: Uncorrectable=%v ErrorsFound=%d, want clean", row, result.Uncorrectable, result.ErrorsFound)
		}
		if string(result.Data) != string(cw[:RS_12_9_DATASIZE]) {
			t.Errorf("row %d: data % X, want % X", row, result.Data, cw[:RS_12_9_DATASIZE])
		}
	}
}

func TestDecodeSingleSymbolErrorAllPositions(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	var ws DecodeWorkspace
	var dst [RS_12_9_DATASIZE]byte
	for n := 0; n < 8; n++ {
		data := make([]byte, RS_12_9_DATASIZE)
		r.Read(data)
		cw, err := Encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		for pos := 0; pos < len(cw); pos++ {
			for e := 1; e < 256; e++ {
				rx := append([]byte(nil), cw...)
				rx[pos] ^= byte(e)
				for _, result := range []DecodeResult{Decode(rx), DecodeInto(rx, dst[:], &ws)} {
					if result.Uncorrectable || result.ErrorsFound != 1 || string(result.Data) != string(data) {
						t.Fatalf("pos %d err 0x%02X: Uncorrectable=%v ErrorsFound=%d data % X, want % X",
							pos, e, result.Uncorrectable, result.ErrorsFound, result.Data, data)
					}
				}
			}
		}
	}
}

func TestDecodeDoubleSymbolErrorsDetected(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for n := 0; n < 50000; n++ {
		data := make([]byte, RS_12_9_DATASIZE)
		r.Read(data)
		cw, err := Encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		rx := append([]byte(nil), cw...)
		for _, p := range r.Perm(len(cw))[:2] {
			rx[p] ^= byte(r.Intn(255) + 1)
		}
		if result := Decode(rx); !result.Uncorrectable {
			t.Fatalf("rx % X (sent % X): two symbol errors not flagged, ErrorsFound=%d", rx, cw, result.ErrorsFound)
		}
	}
}

func TestDecodeTripleSymbolErrorsNeverInvalid(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	for n := 0; n < 50000; n++ {
		data := make([]byte, RS_12_9_DATASIZE)
		r.Read(data)
		cw, err := Encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		rx := append([]byte(nil), cw...)
		for _, p := range r.Perm(len(cw))[:3] {
			rx[p] ^= byte(r.Intn(255) + 1)
		}
		result := Decode(rx)
		if result.Uncorrectable {
			continue
		}
		if result.ErrorsFound != 1 {
			t.Fatalf("rx % X: accepted with ErrorsFound=%d", rx, result.ErrorsFound)
		}
		reencoded, err := Encode(result.Data)
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
	_, err := Encode(short)
	if err == nil {
		t.Error("Encode should return error for wrong size input")
	}

	long := make([]byte, 15)
	_, err = Encode(long)
	if err == nil {
		t.Error("Encode should return error for too-long input")
	}
}

func TestDecodeIntoRoundTrip(t *testing.T) {
	data := []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99}
	encoded, err := Encode(data)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	var dst [RS_12_9_DATASIZE]byte
	var ws DecodeWorkspace

	result := DecodeInto(encoded, dst[:], &ws)
	if result.Uncorrectable {
		t.Error("DecodeInto reported uncorrectable for valid codeword")
	}
	if result.ErrorsFound != 0 {
		t.Errorf("DecodeInto found %d errors, want 0", result.ErrorsFound)
	}

	for j := 0; j < RS_12_9_DATASIZE; j++ {
		if dst[j] != data[j] {
			t.Errorf("dst[%d] = 0x%02X, want 0x%02X", j, dst[j], data[j])
		}
	}
}

func TestSyndromeCleanForValidCodeword(t *testing.T) {
	// All-zero codeword should have zero syndrome
	data := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)

	var syndrome ReedSolomon1294
	err := ReedSolomon1294CalcSyndrome(data, &syndrome)
	if err != nil {
		t.Fatalf("CalcSyndrome failed: %v", err)
	}

	if ReedSolomon1294CheckSyndrome(&syndrome) {
		t.Error("Valid all-zero codeword should have zero syndrome")
	}

	// Verify DecodeInto also reports clean
	var dst [RS_12_9_DATASIZE]byte
	var ws DecodeWorkspace
	result := DecodeInto(data, dst[:], &ws)
	if result.Uncorrectable {
		t.Error("Valid codeword should not be uncorrectable")
	}
	if result.ErrorsFound != 0 {
		t.Errorf("Valid codeword should have 0 errors, got %d", result.ErrorsFound)
	}
}

func TestEncodePreservesData(t *testing.T) {
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE, 0x00}
	encoded, err := Encode(data)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	if len(encoded) != RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE {
		t.Fatalf("Encode returned %d bytes, want %d", len(encoded), RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)
	}

	// Verify data portion is unchanged
	for i := 0; i < RS_12_9_DATASIZE; i++ {
		if encoded[i] != data[i] {
			t.Errorf("encoded[%d] = 0x%02X, want 0x%02X", i, encoded[i], data[i])
		}
	}

	// Checksum bytes should not all be zero for non-zero data
	allZero := true
	for i := RS_12_9_DATASIZE; i < len(encoded); i++ {
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
func tableB18Parity() [RS_12_9_DATASIZE][RS_12_9_CHECKSUMSIZE]byte {
	return [RS_12_9_DATASIZE][RS_12_9_CHECKSUMSIZE]byte{
		{0x1C, 0xBC, 0xFD}, {0x89, 0x31, 0x08}, {0xAD, 0x41, 0x36},
		{0x7D, 0x71, 0x16}, {0xF3, 0xA6, 0x3A}, {0x08, 0x83, 0x7B},
		{0x3F, 0x6F, 0x02}, {0x6C, 0x0D, 0xA7}, {0x0E, 0x38, 0x40},
	}
}

func TestEncodeTableB18Rows(t *testing.T) {
	table := tableB18Parity()
	for row := 0; row < RS_12_9_DATASIZE; row++ {
		data := make([]byte, RS_12_9_DATASIZE)
		data[row] = 0x01
		encoded, err := Encode(data)
		if err != nil {
			t.Fatalf("row %d: Encode failed: %v", row, err)
		}
		got := [RS_12_9_CHECKSUMSIZE]byte(encoded[RS_12_9_DATASIZE:])
		if got != table[row] {
			t.Errorf("row %d: parity % X, want % X", row, got, table[row])
		}
	}
}

func specParity(data []byte) [RS_12_9_CHECKSUMSIZE]byte {
	table := tableB18Parity()
	var p [RS_12_9_CHECKSUMSIZE]byte
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
		data := make([]byte, RS_12_9_DATASIZE)
		r.Read(data)
		encoded, err := Encode(data)
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
		if got, want := [RS_12_9_CHECKSUMSIZE]byte(encoded[RS_12_9_DATASIZE:]), specParity(data); got != want {
			t.Fatalf("data % X: parity % X, want % X", data, got, want)
		}
	}
}

func TestEncodeKnownVector(t *testing.T) {
	data := []byte{0x03, 0x00, 0x00, 0x00, 0x27, 0x06, 0x30, 0xB4, 0x3C}
	encoded, err := Encode(data)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}
	want := [RS_12_9_CHECKSUMSIZE]byte{0xCB, 0x59, 0x57}
	if got := [RS_12_9_CHECKSUMSIZE]byte(encoded[RS_12_9_DATASIZE:]); got != want {
		t.Errorf("parity % X, want % X", got, want)
	}
}

func BenchmarkEncode(b *testing.B) {
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Encode(data)
	}
}
