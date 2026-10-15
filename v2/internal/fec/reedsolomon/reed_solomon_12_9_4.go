// Package reedsolomon implements the shortened Reed-Solomon (12,9) code of
// ETSI TS 102 361-1 B.3.6, which protects a 9-byte Full Link Control.
package reedsolomon

import (
	"fmt"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

const (
	dataSize     = 9
	checksumSize = 3
	// Maximum degree of various polynomials
	polyMaxDeg = checksumSize * 2
)

type poly [polyMaxDeg]uint8

// decodeResult contains the result of a Reed-Solomon decode operation
type decodeResult struct {
	Data          []byte // Corrected data (9 bytes)
	ErrorsFound   int    // Number of symbol (byte) errors found and corrected
	BitsCorrected int    // Number of bits the correction flipped
	Uncorrectable bool   // True if errors could not be corrected
}

func bitDistance(a, b []byte) int {
	n := 0
	for i := range a {
		n += bits.OnesCount8(a[i] ^ b[i])
	}
	return n
}

// workspace holds reusable scratch buffers for zero-allocation decode paths.
// A workspace must not be shared between goroutines.
type workspace struct {
	locator   poly
	evaluator poly
	product   [polyMaxDeg * 2]uint8
	tmp       [polyMaxDeg * 2]uint8
	psi2      [polyMaxDeg]uint8
	roots     [dataSize + checksumSize]uint8
}

// decode performs syndrome calculation, error correction, and returns comprehensive results.
// It takes a 12-byte codeword (9 data + 3 checksum) and returns the corrected data along with error metrics.
func decode(data []byte) decodeResult {
	result := decodeResult{
		Data:          make([]byte, dataSize),
		ErrorsFound:   0,
		Uncorrectable: false,
	}

	if len(data) != dataSize+checksumSize {
		result.Uncorrectable = true
		return result
	}

	// Make a copy to work with
	workData := make([]byte, len(data))
	copy(workData, data)

	// Calculate syndrome
	var syndrome poly
	err := calcSyndrome(workData, &syndrome)
	if err != nil {
		result.Uncorrectable = true
		return result
	}

	// Check if there are any errors
	if !syndromeHasErrors(&syndrome) {
		// No errors
		copy(result.Data, workData[:dataSize])
		return result
	}

	// Attempt correction
	errorsFound, err := correct(workData, &syndrome)
	if err != nil {
		result.Uncorrectable = true
		result.ErrorsFound = errorsFound
		copy(result.Data, workData[:dataSize])
		return result
	}

	result.ErrorsFound = errorsFound
	result.BitsCorrected = bitDistance(data, workData)
	copy(result.Data, workData[:dataSize])
	return result
}

// Encode returns the 12-byte codeword of 9 data bytes: the data followed by
// 3 parity bytes (ETSI TS 102 361-1 B.3.6).
func Encode(data [9]byte) [12]byte {
	cw, _ := encode(data[:])
	return [12]byte(cw)
}

// Decode corrects up to one symbol error in a 12-byte codeword and returns
// the 9 data bytes. ErrorsCorrected counts corrected bits. When the codeword
// is uncorrectable, Decode returns the received data bytes and sets
// Uncorrectable.
func Decode(cw [12]byte) ([9]byte, fec.Result) {
	r := decode(cw[:])
	return [9]byte(r.Data), fec.Result{
		BitsChecked:     96,
		ErrorsCorrected: r.BitsCorrected,
		Uncorrectable:   r.Uncorrectable,
	}
}

// Check reports whether cw is a codeword.
func Check(cw [12]byte) bool {
	var syndrome poly
	_ = calcSyndrome(cw[:], &syndrome)
	return !syndromeHasErrors(&syndrome)
}

func encode(data []byte) ([]byte, error) {
	if len(data) != dataSize {
		return nil, fmt.Errorf("fec/rs_12_9: data must be %d bytes, got %d", dataSize, len(data))
	}

	// Create codeword with space for checksum
	codeword := make([]byte, dataSize+checksumSize)
	copy(codeword, data)

	// Systematic encoding by division by g(x) = x^3 + 0x0E x^2 + 0x38 x + 0x40
	// (ETSI TS 102 361-1 equation B.10).
	var r0, r1, r2 uint8
	for i := 0; i < dataSize; i++ {
		fb := codeword[i] ^ r0
		r0 = r1 ^ galoisMul(fb, 0x0E)
		r1 = r2 ^ galoisMul(fb, 0x38)
		r2 = galoisMul(fb, 0x40)
	}
	codeword[dataSize] = r0
	codeword[dataSize+1] = r1
	codeword[dataSize+2] = r2

	return codeword, nil
}

// This finds the coefficients of the error locator polynomial, and then calculates
// the error evaluator polynomial using the Berlekamp-Massey algorithm.
// From  Cain, Clark, "Error-Correction Coding For Digital Communications", pp. 216.
func calcSyndrome(data []byte, syndrome *poly) error {
	if len(data) != dataSize+checksumSize {
		return fmt.Errorf("fec/rs_12_9: unexpected size %d, expected %d bytes",
			len(data), dataSize+checksumSize)
	}

	var i, j uint8
	for i = 0; i < 3; i++ {
		syndrome[i] = 0
	}

	for j = 0; j < 3; j++ {
		for i = 0; i < uint8(len(data)); i++ { //nolint:gosec // len(data) is always dataSize+checksumSize (12), fits in uint8
			syndrome[j] = data[i] ^ galoisMul(galois_exp_table[j+1], syndrome[j])
		}
	}

	return nil
}

// syndromeHasErrors reports whether any syndrome value is nonzero.
func syndromeHasErrors(syndrome *poly) bool {
	for _, v := range syndrome {
		if v != 0 {
			return true
		}
	}
	return false
}

func calcErrorEvaluatorPolyWithScratch(locator, syndrome, evaluator *poly, product []uint8, tmp []uint8) {
	mulPolysWithTmp(locator, syndrome, product, tmp)

	var i uint8
	for i = 0; i < checksumSize; i++ {
		evaluator[i] = product[i]
	}
	for ; i < polyMaxDeg; i++ {
		evaluator[i] = 0
	}
}

func mulPolysWithTmp(p1, p2 *poly, dst, tmp []uint8) {
	var i, j uint8

	for i = 0; i < polyMaxDeg*2; i++ {
		dst[i] = 0
	}

	for i = 0; i < polyMaxDeg; i++ {
		for j := polyMaxDeg; j < (polyMaxDeg * 2); j++ {
			tmp[j] = 0
		}

		// Scale tmp by p1[i]
		for j = 0; j < polyMaxDeg; j++ {
			tmp[j] = galoisMul(p2[j], p1[i])
		}

		// Shift (multiply) tmp right by i
		for j = (polyMaxDeg * 2) - 1; j >= i && j < (polyMaxDeg*2)-1; j-- {
			tmp[j] = tmp[j-i]
		}
		for j = 0; j < i; j++ {
			tmp[j] = 0
		}

		// Add into partial product
		for j = 0; j < (polyMaxDeg * 2); j++ {
			dst[j] ^= tmp[j]
		}
	}
}

// galoisInv returns the multiplicative inverse of elt in GF(2^8).
// The second result is false when elt is zero, which has no inverse.
func galoisInv(elt uint8) (uint8, bool) {
	if elt == 0 {
		return 0, false
	}
	return galois_exp_table[255-int(galois_log_table[elt])], true
}

func galoisMul(a, b uint8) uint8 {
	if a == 0 || b == 0 {
		return 0
	}
	return galois_exp_table[(int(galois_log_table[a])+int(galois_log_table[b]))%255]
}

// Multiply by z (shift right by 1).
func mulPolyZ(poly *poly) {
	for i := polyMaxDeg - 1; i > 0; i-- {
		poly[i] = poly[i-1]
	}
	poly[0] = 0
}

func calcDiscrepancy(locator, syndrome *poly, lVal, n uint8) uint8 {
	var i, sum uint8

	for i = 0; i <= lVal && i <= n; i++ {
		sum ^= galoisMul(locator[i], syndrome[n-i])
	}

	return sum
}

func correct(data []byte, syndrome *poly) (int, error) {
	if len(data) != dataSize+checksumSize {
		return -1, fmt.Errorf("fec/rs_12_9: unexpected size %d, expected %d bytes",
			len(data), dataSize+checksumSize)
	}

	var ws workspace
	if !syndromeHasErrors(syndrome) {
		return 0, nil
	}

	calcWithScratch(syndrome, &ws.locator, &ws.evaluator, ws.psi2[:], ws.product[:], ws.tmp[:])

	// With d=4 the code corrects one symbol error, so only a degree-1 locator
	// with one root inside the shortened codeword is acceptable.
	if ws.locator[1] == 0 {
		return 0, fec.ErrUncorrectable
	}
	for j := 2; j < polyMaxDeg; j++ {
		if ws.locator[j] != 0 {
			return 0, fec.ErrUncorrectable
		}
	}
	roots := findRootsInto(&ws.locator, ws.roots[:0])
	if len(roots) != 1 || roots[0] >= dataSize+checksumSize {
		return 0, fec.ErrUncorrectable
	}
	i := roots[0]

	// Forney: the error value is the evaluator over the locator derivative,
	// both evaluated at alpha^(-i).
	var num, denom uint8
	for j := 0; j < polyMaxDeg; j++ {
		idx := (255 - int(i)) * j % 255
		num ^= galoisMul(ws.evaluator[j], galois_exp_table[idx])
	}
	for j := 1; j < polyMaxDeg; j += 2 {
		idx := (255 - int(i)) * (j - 1) % 255
		denom ^= galoisMul(ws.locator[j], galois_exp_table[idx])
	}
	inv, ok := galoisInv(denom)
	if !ok || num == 0 {
		return 0, fec.ErrUncorrectable
	}

	pos := len(data) - int(i) - 1
	value := galoisMul(num, inv)
	data[pos] ^= value

	var check poly
	if err := calcSyndrome(data, &check); err != nil || syndromeHasErrors(&check) {
		data[pos] ^= value
		return 0, fec.ErrUncorrectable
	}

	return 1, nil
}

// The error-locator polynomial's roots are found by looking for the values of a^n where
// evaluating the polynomial yields zero (evaluating rs_12_9_error_locator_poly at
// successive values of alpha (Chien's search)).
func findRoots(locator *poly) []uint8 {
	var rootsBuf [dataSize + checksumSize]uint8
	return findRootsInto(locator, rootsBuf[:0])
}

func findRootsInto(locator *poly, buf []uint8) []uint8 {
	var k, r uint16
	roots := buf[:0]
	for r = 1; r < 256; r++ {
		var sum uint8
		// Evaluate locator at r
		for k = 0; k < checksumSize+1; k++ {
			sum ^= galoisMul(galois_exp_table[(k*r)%255], locator[k])
		}

		if sum == 0 {
			roots = append(roots, uint8(255-r)) //nolint:gosec // r is 1..255, so 255-r is 0..254, fits in uint8
		}
	}

	return roots
}

func calcWithScratch(
	syndrome, locator, evaluator *poly,
	psi2 []uint8, product []uint8, tmp []uint8,
) {
	var (
		n, lCur, l2 uint8
		k           int8
		d, i        uint8
		D           = poly{0, 1, 0}
	)

	k = -1
	for i = 0; i < polyMaxDeg; i++ {
		locator[i] = 0
		psi2[i] = 0
	}
	locator[0] = 1

	for n = 0; n < checksumSize; n++ {
		d = calcDiscrepancy(locator, syndrome, lCur, n)
		if d != 0 {
			// psi2 = locator - d*D
			for i = 0; i < polyMaxDeg; i++ {
				psi2[i] = locator[i] ^ galoisMul(d, D[i])
			}

			if int(lCur) < int(n)-int(k) {
				l2 = uint8(int(n) - int(k))  //nolint:gosec // n < checksumSize (3), result fits in uint8
				k = int8(int(n) - int(lCur)) //nolint:gosec // n < checksumSize (3), result fits in int8
				dInv, _ := galoisInv(d)
				for i = 0; i < polyMaxDeg; i++ {
					D[i] = galoisMul(locator[i], dInv)
				}
				lCur = l2
			}

			// locator = psi2
			for i = 0; i < polyMaxDeg; i++ {
				locator[i] = psi2[i]
			}
		}
		mulPolyZ(&D)
	}
	calcErrorEvaluatorPolyWithScratch(locator, syndrome, evaluator, product, tmp)
}
