package reedsolomon

import "testing"

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

func TestDecodeChecksumByteErrorCorrection(t *testing.T) {
	// All-zero codeword is valid
	data := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)

	// Corrupt one checksum byte
	corrupted := make([]byte, len(data))
	copy(corrupted, data)
	corrupted[RS_12_9_DATASIZE] = 0xFF

	result := Decode(corrupted)
	if result.Uncorrectable {
		t.Error("Single checksum byte error should be correctable")
	}

	// Verify data is still correct
	for j := 0; j < RS_12_9_DATASIZE; j++ {
		if result.Data[j] != 0 {
			t.Errorf("corrected[%d] = 0x%02X, want 0x00", j, result.Data[j])
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

func BenchmarkEncode(b *testing.B) {
	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Encode(data)
	}
}
