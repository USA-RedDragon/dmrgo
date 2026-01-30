package reedsolomon

import (
	"testing"
)

func TestReedSolomon1294SyndromeNoError(t *testing.T) {
	// Test that a valid codeword has zero syndrome
	// Use a known good codeword - all zeros is a valid codeword
	data := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)
	var syndrome ReedSolomon1294

	err := ReedSolomon1294CalcSyndrome(data, &syndrome)
	if err != nil {
		t.Fatalf("CalcSyndrome failed: %v", err)
	}

	if ReedSolomon1294CheckSyndrome(&syndrome) {
		t.Error("Zero codeword should have zero syndrome")
	}
}

func TestReedSolomon1294DecodeClean(t *testing.T) {
	// Test decoding a clean codeword (no errors)
	// All-zero codeword is valid
	data := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)

	result := Decode(data)

	if result.Uncorrectable {
		t.Error("Clean codeword reported as uncorrectable")
	}
	if result.ErrorsFound != 0 {
		t.Errorf("Clean codeword should have 0 errors, got %d", result.ErrorsFound)
	}
	for i, b := range result.Data {
		if b != 0 {
			t.Errorf("Data[%d] = %d, expected 0", i, b)
		}
	}
}

func TestReedSolomon1294SyndromeWithError(t *testing.T) {
	// Test that syndrome is non-zero when data has errors
	data := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)
	data[0] = 0x55 // Introduce an error

	var syndrome ReedSolomon1294
	err := ReedSolomon1294CalcSyndrome(data, &syndrome)
	if err != nil {
		t.Fatalf("CalcSyndrome failed: %v", err)
	}

	if !ReedSolomon1294CheckSyndrome(&syndrome) {
		t.Error("Corrupted codeword should have non-zero syndrome")
	}
}

func TestReedSolomon1294GaloisMul(t *testing.T) {
	// Test Galois field multiplication properties
	// 1. Multiplication by 0
	if ReedSolomon1294GaloisMul(0, 0x5A) != 0 {
		t.Error("0 * x should be 0")
	}
	if ReedSolomon1294GaloisMul(0x5A, 0) != 0 {
		t.Error("x * 0 should be 0")
	}

	// 2. Multiplication by 1 (α^0 = 1)
	if ReedSolomon1294GaloisMul(1, 0x5A) != 0x5A {
		t.Error("1 * x should be x")
	}
	if ReedSolomon1294GaloisMul(0x5A, 1) != 0x5A {
		t.Error("x * 1 should be x")
	}

	// 3. Commutativity
	a, b := byte(0x12), byte(0x34)
	if ReedSolomon1294GaloisMul(a, b) != ReedSolomon1294GaloisMul(b, a) {
		t.Error("Galois multiplication should be commutative")
	}
}

func TestReedSolomon1294GaloisMulAssociativity(t *testing.T) {
	// Test (a*b)*c = a*(b*c)
	a, b, c := byte(0x12), byte(0x34), byte(0x56)
	left := ReedSolomon1294GaloisMul(ReedSolomon1294GaloisMul(a, b), c)
	right := ReedSolomon1294GaloisMul(a, ReedSolomon1294GaloisMul(b, c))
	if left != right {
		t.Errorf("Galois multiplication should be associative: (%d*%d)*%d=%d vs %d*(%d*%d)=%d",
			a, b, c, left, a, b, c, right)
	}
}

func TestReedSolomon1294FindRootsEmpty(t *testing.T) {
	// Locator polynomial of 1 (no errors) should have no roots
	locator := ReedSolomon1294{1, 0, 0, 0, 0, 0}
	roots := ReedSolomon1294FindRoots(&locator)
	if len(roots) != 0 {
		t.Errorf("Locator=1 should have no roots, got %d", len(roots))
	}
}

func TestReedSolomon1294DecodeInvalidLength(t *testing.T) {
	// Test that invalid input length is handled gracefully
	shortData := make([]byte, RS_12_9_DATASIZE) // Too short
	result := Decode(shortData)
	if !result.Uncorrectable {
		t.Error("Short data should be uncorrectable")
	}

	longData := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE+5) // Too long
	result = Decode(longData)
	if !result.Uncorrectable {
		t.Error("Long data should be uncorrectable")
	}
}

func TestReedSolomon1294CalcSyndromeError(t *testing.T) {
	// Test syndrome calculation with wrong size input
	shortData := make([]byte, 5)
	var syndrome ReedSolomon1294

	err := ReedSolomon1294CalcSyndrome(shortData, &syndrome)
	if err == nil {
		t.Error("CalcSyndrome should return error for wrong input size")
	}
}

func TestReedSolomon1294CorrectError(t *testing.T) {
	// Test ReedSolomon1294Correct with wrong size input
	shortData := make([]byte, 5)
	var syndrome ReedSolomon1294

	_, err := ReedSolomon1294Correct(shortData, &syndrome)
	if err == nil {
		t.Error("Correct should return error for wrong input size")
	}
}

func TestReedSolomon1294DecodeResultFields(t *testing.T) {
	// Test that DecodeResult fields are properly initialized
	data := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)

	result := Decode(data)

	// Verify Data slice is correct size
	if len(result.Data) != RS_12_9_DATASIZE {
		t.Errorf("DecodeResult.Data length = %d, expected %d", len(result.Data), RS_12_9_DATASIZE)
	}
}

// BenchmarkReedSolomon1294CalcSyndrome benchmarks syndrome calculation
func BenchmarkReedSolomon1294CalcSyndrome(b *testing.B) {
	data := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)
	for i := range data {
		data[i] = byte(i)
	}
	var syndrome ReedSolomon1294

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ReedSolomon1294CalcSyndrome(data, &syndrome)
	}
}

// BenchmarkReedSolomon1294Decode benchmarks the full decode operation
func BenchmarkReedSolomon1294Decode(b *testing.B) {
	data := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)
	for i := range data {
		data[i] = byte(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Decode(data)
	}
}

// BenchmarkReedSolomon1294GaloisMul benchmarks Galois field multiplication
func BenchmarkReedSolomon1294GaloisMul(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ReedSolomon1294GaloisMul(byte(i), byte(i+1))
	}
}

func TestReedSolomon1294CheckSyndromeWithErrors(t *testing.T) {
	// Create a codeword with errors and verify syndrome is non-zero
	data := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)
	data[0] = 0x42 // Introduce an error

	var syndrome ReedSolomon1294
	err := ReedSolomon1294CalcSyndrome(data, &syndrome)
	if err != nil {
		t.Fatalf("CalcSyndrome failed: %v", err)
	}

	if !ReedSolomon1294CheckSyndrome(&syndrome) {
		t.Error("Corrupted codeword should have non-zero syndrome")
	}
}
