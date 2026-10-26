package reedsolomon

import (
	"testing"
)

func TestReedSolomon1294SyndromeNoError(t *testing.T) {
	t.Parallel()

	var data [dataSize + checksumSize]byte
	var syndrome syndromes

	calcSyndrome(&data, &syndrome)

	if syndromeHasErrors(&syndrome) {
		t.Error("Zero codeword should have zero syndrome")
	}
}

func TestReedSolomon1294DecodeClean(t *testing.T) {
	t.Parallel()

	var data [dataSize + checksumSize]byte

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
	t.Parallel()

	var data [dataSize + checksumSize]byte
	data[0] = 0x55

	var syndrome syndromes
	calcSyndrome(&data, &syndrome)

	if !syndromeHasErrors(&syndrome) {
		t.Error("Corrupted codeword should have non-zero syndrome")
	}
}

func TestReedSolomon1294GaloisMul(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	a, b, c := byte(0x12), byte(0x34), byte(0x56)
	left := galoisMul(galoisMul(a, b), c)
	right := galoisMul(a, galoisMul(b, c))
	if left != right {
		t.Errorf("Galois multiplication should be associative: (%d*%d)*%d=%d vs %d*(%d*%d)=%d",
			a, b, c, left, a, b, c, right)
	}
}

func TestReedSolomon1294DecodeResultFields(t *testing.T) {
	t.Parallel()

	var data [dataSize + checksumSize]byte

	result := decode(data)

	if result != (decodeResult{}) {
		t.Errorf("decode of the zero codeword = %+v, want the zero decodeResult", result)
	}
}

func BenchmarkReedSolomon1294CalcSyndrome(b *testing.B) {
	var data [dataSize + checksumSize]byte
	for i := range data {
		data[i] = byte(i)
	}
	var syndrome syndromes

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		calcSyndrome(&data, &syndrome)
	}
}

func BenchmarkReedSolomon1294Decode(b *testing.B) {
	var data [dataSize + checksumSize]byte
	for i := range data {
		data[i] = byte(i)
	}

	b.ReportAllocs()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decode(data)
	}
}

func BenchmarkReedSolomon1294GaloisMul(b *testing.B) {
	for i := 0; i < b.N; i++ {
		galoisMul(byte(i), byte(i+1))
	}
}

func TestReedSolomon1294SyndromeHasErrors(t *testing.T) {
	t.Parallel()

	var data [dataSize + checksumSize]byte
	data[0] = 0x42

	var syndrome syndromes
	calcSyndrome(&data, &syndrome)

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
	t.Parallel()

	for a := 0; a < 256; a++ {
		for b := 0; b < 256; b++ {
			got := galoisMul(byte(a), byte(b))
			if want := refGaloisMul(byte(a), byte(b)); got != want {
				t.Fatalf("GaloisMul(0x%02X, 0x%02X) = 0x%02X, want 0x%02X", a, b, got, want)
			}
		}
	}
}

func galoisMul(a, b uint8) uint8 {
	var p uint8
	for range 8 {
		p ^= a & -(b & 1)
		a, b = xtime(a), b>>1
	}
	return p
}
