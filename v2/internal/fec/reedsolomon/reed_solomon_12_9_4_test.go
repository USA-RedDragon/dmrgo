package reedsolomon

import (
	"testing"
)

func TestReedSolomon1294SyndromeNoError(t *testing.T) {
	data := make([]byte, dataSize+checksumSize)
	var syndrome poly

	err := calcSyndrome(data, &syndrome)
	if err != nil {
		t.Fatalf("CalcSyndrome failed: %v", err)
	}

	if syndromeHasErrors(&syndrome) {
		t.Error("Zero codeword should have zero syndrome")
	}
}

func TestReedSolomon1294DecodeClean(t *testing.T) {
	data := make([]byte, dataSize+checksumSize)

	result := decode(data)

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
	data := make([]byte, dataSize+checksumSize)
	data[0] = 0x55

	var syndrome poly
	err := calcSyndrome(data, &syndrome)
	if err != nil {
		t.Fatalf("CalcSyndrome failed: %v", err)
	}

	if !syndromeHasErrors(&syndrome) {
		t.Error("Corrupted codeword should have non-zero syndrome")
	}
}

func TestReedSolomon1294GaloisMul(t *testing.T) {
	if galoisMul(0, 0x5A) != 0 {
		t.Error("0 * x should be 0")
	}
	if galoisMul(0x5A, 0) != 0 {
		t.Error("x * 0 should be 0")
	}

	if galoisMul(1, 0x5A) != 0x5A {
		t.Error("1 * x should be x")
	}
	if galoisMul(0x5A, 1) != 0x5A {
		t.Error("x * 1 should be x")
	}

	a, b := byte(0x12), byte(0x34)
	if galoisMul(a, b) != galoisMul(b, a) {
		t.Error("Galois multiplication should be commutative")
	}
}

func TestReedSolomon1294GaloisMulAssociativity(t *testing.T) {
	a, b, c := byte(0x12), byte(0x34), byte(0x56)
	left := galoisMul(galoisMul(a, b), c)
	right := galoisMul(a, galoisMul(b, c))
	if left != right {
		t.Errorf("Galois multiplication should be associative: (%d*%d)*%d=%d vs %d*(%d*%d)=%d",
			a, b, c, left, a, b, c, right)
	}
}

func TestReedSolomon1294FindRootsEmpty(t *testing.T) {
	locator := poly{1, 0, 0, 0, 0, 0}
	roots := findRoots(&locator)
	if len(roots) != 0 {
		t.Errorf("Locator=1 should have no roots, got %d", len(roots))
	}
}

func TestReedSolomon1294DecodeInvalidLength(t *testing.T) {
	shortData := make([]byte, dataSize)
	result := decode(shortData)
	if !result.Uncorrectable {
		t.Error("Short data should be uncorrectable")
	}

	longData := make([]byte, dataSize+checksumSize+5)
	result = decode(longData)
	if !result.Uncorrectable {
		t.Error("Long data should be uncorrectable")
	}
}

func TestReedSolomon1294CalcSyndromeError(t *testing.T) {
	shortData := make([]byte, 5)
	var syndrome poly

	err := calcSyndrome(shortData, &syndrome)
	if err == nil {
		t.Error("CalcSyndrome should return error for wrong input size")
	}
}

func TestReedSolomon1294CorrectError(t *testing.T) {
	shortData := make([]byte, 5)
	var syndrome poly

	_, err := correct(shortData, &syndrome)
	if err == nil {
		t.Error("Correct should return error for wrong input size")
	}
}

func TestReedSolomon1294DecodeResultFields(t *testing.T) {
	data := make([]byte, dataSize+checksumSize)

	result := decode(data)

	if len(result.Data) != dataSize {
		t.Errorf("decodeResult.Data length = %d, expected %d", len(result.Data), dataSize)
	}
}

// BenchmarkReedSolomon1294CalcSyndrome benchmarks syndrome calculation
func BenchmarkReedSolomon1294CalcSyndrome(b *testing.B) {
	data := make([]byte, dataSize+checksumSize)
	for i := range data {
		data[i] = byte(i)
	}
	var syndrome poly

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = calcSyndrome(data, &syndrome)
	}
}

// BenchmarkReedSolomon1294Decode benchmarks the full decode operation
func BenchmarkReedSolomon1294Decode(b *testing.B) {
	data := make([]byte, dataSize+checksumSize)
	for i := range data {
		data[i] = byte(i)
	}

	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decode(data)
	}
}

// BenchmarkReedSolomon1294GaloisMul benchmarks Galois field multiplication
func BenchmarkReedSolomon1294GaloisMul(b *testing.B) {
	for i := 0; i < b.N; i++ {
		galoisMul(byte(i), byte(i+1))
	}
}

func TestReedSolomon1294SyndromeHasErrors(t *testing.T) {
	data := make([]byte, dataSize+checksumSize)
	data[0] = 0x42

	var syndrome poly
	err := calcSyndrome(data, &syndrome)
	if err != nil {
		t.Fatalf("CalcSyndrome failed: %v", err)
	}

	if !syndromeHasErrors(&syndrome) {
		t.Error("Corrupted codeword should have non-zero syndrome")
	}
}

func refGaloisMul(a, b byte) byte {
	var p uint16
	x, y := uint16(a), uint16(b)
	for y != 0 {
		if y&1 != 0 {
			p ^= x
		}
		y >>= 1
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11D
		}
	}
	return byte(p)
}

func TestReedSolomon1294GaloisMulExhaustive(t *testing.T) {
	for a := 0; a < 256; a++ {
		for b := 0; b < 256; b++ {
			got := galoisMul(byte(a), byte(b))
			if want := refGaloisMul(byte(a), byte(b)); got != want {
				t.Fatalf("GaloisMul(0x%02X, 0x%02X) = 0x%02X, want 0x%02X", a, b, got, want)
			}
		}
	}
}

func TestReedSolomon1294GaloisInv(t *testing.T) {
	if _, ok := galoisInv(0); ok {
		t.Error("GaloisInv(0) should report failure")
	}
	for a := 1; a < 256; a++ {
		inv, ok := galoisInv(byte(a))
		if !ok {
			t.Fatalf("GaloisInv(0x%02X) reported failure", a)
		}
		if p := refGaloisMul(byte(a), inv); p != 1 {
			t.Fatalf("0x%02X * GaloisInv = 0x%02X, want 1", a, p)
		}
	}
}
