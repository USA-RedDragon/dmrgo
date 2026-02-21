package reedsolomon

import (
	"errors"
	"fmt"
)

const (
	RS_12_9_DATASIZE     = 9
	RS_12_9_CHECKSUMSIZE = 3
	// Maximum degree of various polynomials
	RS_12_9_POLY_MAXDEG = RS_12_9_CHECKSUMSIZE * 2
)

type ReedSolomon1294 [RS_12_9_POLY_MAXDEG]uint8

// DecodeResult contains the result of a Reed-Solomon decode operation
type DecodeResult struct {
	Data          []byte // Corrected data (9 bytes)
	ErrorsFound   int    // Number of errors found and corrected
	Uncorrectable bool   // True if errors could not be corrected
}

// DecodeWorkspace holds reusable scratch buffers for zero-allocation decode paths.
// A workspace must not be shared between goroutines.
type DecodeWorkspace struct {
	workData  [RS_12_9_DATASIZE + RS_12_9_CHECKSUMSIZE]byte
	syndrome  ReedSolomon1294
	locator   ReedSolomon1294
	evaluator ReedSolomon1294
	product   [RS_12_9_POLY_MAXDEG * 2]uint8
	tmp       [RS_12_9_POLY_MAXDEG * 2]uint8
	psi2      [RS_12_9_POLY_MAXDEG]uint8
	roots     [RS_12_9_DATASIZE + RS_12_9_CHECKSUMSIZE]uint8
}

// Decode performs syndrome calculation, error correction, and returns comprehensive results.
// It takes a 12-byte codeword (9 data + 3 checksum) and returns the corrected data along with error metrics.
func Decode(data []byte) DecodeResult {
	result := DecodeResult{
		Data:          make([]byte, RS_12_9_DATASIZE),
		ErrorsFound:   0,
		Uncorrectable: false,
	}

	if len(data) != RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE {
		result.Uncorrectable = true
		return result
	}

	// Make a copy to work with
	workData := make([]byte, len(data))
	copy(workData, data)

	// Calculate syndrome
	var syndrome ReedSolomon1294
	err := ReedSolomon1294CalcSyndrome(workData, &syndrome)
	if err != nil {
		result.Uncorrectable = true
		return result
	}

	// Check if there are any errors
	if !ReedSolomon1294CheckSyndrome(&syndrome) {
		// No errors
		copy(result.Data, workData[:RS_12_9_DATASIZE])
		return result
	}

	// Attempt correction
	errorsFound, err := ReedSolomon1294Correct(workData, &syndrome)
	if err != nil {
		result.Uncorrectable = true
		result.ErrorsFound = errorsFound
		copy(result.Data, workData[:RS_12_9_DATASIZE])
		return result
	}

	result.ErrorsFound = errorsFound
	copy(result.Data, workData[:RS_12_9_DATASIZE])
	return result
}

// DecodeInto performs the same operation as Decode but writes the corrected data into dst
// and reuses the provided workspace to avoid heap allocations. The dst slice must be at
// least RS_12_9_DATASIZE bytes long. If workspace is nil a short-lived local workspace is
// used.
func DecodeInto(data []byte, dst []byte, workspace *DecodeWorkspace) DecodeResult {
	result := DecodeResult{
		Data:          dst,
		ErrorsFound:   0,
		Uncorrectable: false,
	}

	if len(dst) < RS_12_9_DATASIZE {
		result.Uncorrectable = true
		return result
	}
	result.Data = dst[:RS_12_9_DATASIZE]

	if len(data) != RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE {
		result.Uncorrectable = true
		return result
	}

	ws := workspace
	if ws == nil {
		ws = &DecodeWorkspace{}
	}

	copy(ws.workData[:], data)

	if err := ReedSolomon1294CalcSyndrome(ws.workData[:], &ws.syndrome); err != nil {
		result.Uncorrectable = true
		return result
	}

	if !ReedSolomon1294CheckSyndrome(&ws.syndrome) {
		copy(result.Data, ws.workData[:RS_12_9_DATASIZE])
		return result
	}

	errorsFound, err := ReedSolomon1294CorrectWithWorkspace(ws.workData[:], &ws.syndrome, ws)
	if err != nil {
		result.Uncorrectable = true
	}
	result.ErrorsFound = errorsFound
	copy(result.Data, ws.workData[:RS_12_9_DATASIZE])
	return result
}

// Encode generates a 12-byte codeword from 9 bytes of data.
// Returns the original data with 3 checksum bytes appended.
func Encode(data []byte) ([]byte, error) {
	if len(data) != RS_12_9_DATASIZE {
		return nil, fmt.Errorf("fec/rs_12_9: data must be %d bytes, got %d", RS_12_9_DATASIZE, len(data))
	}

	// Create codeword with space for checksum
	codeword := make([]byte, RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)
	copy(codeword, data)

	// Calculate checksum bytes using the generator polynomial
	// RS(12,9) systematic encoding: divide by generator polynomial
	for i := 0; i < RS_12_9_DATASIZE; i++ {
		feedback := codeword[i] ^ codeword[RS_12_9_DATASIZE]
		if feedback != 0 {
			// Generator polynomial coefficients for RS(12,9,4) over GF(256)
			// g(x) = (x - α)(x - α²)(x - α³) = x³ + g2·x² + g1·x + g0
			// For DMR: α = 2, so roots are 2, 4, 8
			codeword[RS_12_9_DATASIZE] ^= ReedSolomon1294GaloisMul(feedback, 0x40)   // g2
			codeword[RS_12_9_DATASIZE+1] ^= ReedSolomon1294GaloisMul(feedback, 0x78) // g1
			codeword[RS_12_9_DATASIZE+2] ^= ReedSolomon1294GaloisMul(feedback, 0x40) // g0 (shifted in next iteration)
		}
		// Shift
		codeword[RS_12_9_DATASIZE] = codeword[RS_12_9_DATASIZE+1]
		codeword[RS_12_9_DATASIZE+1] = codeword[RS_12_9_DATASIZE+2]
		codeword[RS_12_9_DATASIZE+2] = feedback
	}

	return codeword, nil
}

// This finds the coefficients of the error locator polynomial, and then calculates
// the error evaluator polynomial using the Berlekamp-Massey algorithm.
// From  Cain, Clark, "Error-Correction Coding For Digital Communications", pp. 216.
func ReedSolomon1294CalcSyndrome(data []byte, syndrome *ReedSolomon1294) error {
	if len(data) != RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE {
		return fmt.Errorf("fec/rs_12_9: unexpected size %d, expected %d bytes",
			len(data), RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)
	}

	var i, j uint8
	for i = 0; i < 3; i++ {
		syndrome[i] = 0
	}

	for j = 0; j < 3; j++ {
		for i = 0; i < uint8(len(data)); i++ { //nolint:gosec // len(data) is always RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE (12), fits in uint8
			syndrome[j] = data[i] ^ ReedSolomon1294GaloisMul(galois_exp_table[j+1], syndrome[j])
		}
	}

	return nil
}

func ReedSolomon1294CheckSyndrome(syndrome *ReedSolomon1294) bool {
	for _, v := range syndrome {
		if v != 0 {
			return true
		}
	}
	return false
}

func ReedSolomon1294CalcErrorEvaluatorPoly(locator, syndrome, evaluator *ReedSolomon1294) {
	var (
		product [RS_12_9_POLY_MAXDEG * 2]uint8
		tmp     [RS_12_9_POLY_MAXDEG * 2]uint8
	)

	ReedSolomon1294CalcErrorEvaluatorPolyWithScratch(locator, syndrome, evaluator, product[:], tmp[:])
}

func ReedSolomon1294CalcErrorEvaluatorPolyWithScratch(locator, syndrome, evaluator *ReedSolomon1294, product []uint8, tmp []uint8) {
	ReedSolomon1294MulPolysWithTmp(locator, syndrome, product, tmp)

	var i uint8
	for i = 0; i < RS_12_9_CHECKSUMSIZE; i++ {
		evaluator[i] = product[i]
	}
	for ; i < RS_12_9_POLY_MAXDEG; i++ {
		evaluator[i] = 0
	}
}

func ReedSolomon1294MulPolys(p1, p2 *ReedSolomon1294, dst []uint8) {
	var tmp [RS_12_9_POLY_MAXDEG * 2]uint8
	ReedSolomon1294MulPolysWithTmp(p1, p2, dst, tmp[:])
}

func ReedSolomon1294MulPolysWithTmp(p1, p2 *ReedSolomon1294, dst, tmp []uint8) {
	var i, j uint8

	for i = 0; i < RS_12_9_POLY_MAXDEG*2; i++ {
		dst[i] = 0
	}

	for i = 0; i < RS_12_9_POLY_MAXDEG; i++ {
		for j := RS_12_9_POLY_MAXDEG; j < (RS_12_9_POLY_MAXDEG * 2); j++ {
			tmp[j] = 0
		}

		// Scale tmp by p1[i]
		for j = 0; j < RS_12_9_POLY_MAXDEG; j++ {
			tmp[j] = ReedSolomon1294GaloisMul(p2[j], p1[i])
		}

		// Shift (multiply) tmp right by i
		for j = (RS_12_9_POLY_MAXDEG * 2) - 1; j >= i && j < (RS_12_9_POLY_MAXDEG*2)-1; j-- {
			tmp[j] = tmp[j-i]
		}
		for j = 0; j < i; j++ {
			tmp[j] = 0
		}

		// Add into partial product
		for j = 0; j < (RS_12_9_POLY_MAXDEG * 2); j++ {
			dst[j] ^= tmp[j]
		}
	}
}

func ReedSolomon1294GaloisInv(elt uint8) uint8 {
	return galois_exp_table[255-galois_log_table[elt]]
}

func ReedSolomon1294GaloisMul(a, b uint8) uint8 {
	if a == 0 || b == 0 {
		return 0
	}
	return galois_exp_table[(galois_log_table[a]+galois_log_table[b])%255]
}

// Multiply by z (shift right by 1).
func ReedSolomon1294MulPolyZ(poly *ReedSolomon1294) {
	for i := RS_12_9_POLY_MAXDEG - 1; i > 0; i-- {
		poly[i] = poly[i-1]
	}
	poly[0] = 0
}

func ReedSolomon1294CalcDiscrepancy(locator, syndrome *ReedSolomon1294, lVal, n uint8) uint8 {
	var i, sum uint8

	for i = 0; i < lVal; i++ {
		sum ^= ReedSolomon1294GaloisMul(locator[i], syndrome[n-i])
	}

	return sum
}

func ReedSolomon1294Correct(data []byte, syndrome *ReedSolomon1294) (int, error) {
	var workspace DecodeWorkspace
	return ReedSolomon1294CorrectWithWorkspace(data, syndrome, &workspace)
}

func ReedSolomon1294CorrectWithWorkspace(data []byte, syndrome *ReedSolomon1294, workspace *DecodeWorkspace) (int, error) {
	if len(data) != RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE {
		return -1, fmt.Errorf("fec/rs_12_9: unexpected size %d, expected %d bytes",
			len(data), RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE)
	}

	ws := workspace
	if ws == nil {
		ws = &DecodeWorkspace{}
	}

	ReedSolomon1294CalcWithScratch(syndrome, &ws.locator, &ws.evaluator, ws.psi2[:], ws.product[:], ws.tmp[:])
	roots := ReedSolomon1294FindRootsInto(&ws.locator, ws.roots[:0])
	errorsFound := len(roots)

	if errorsFound == 0 {
		return 0, nil
	}

	// Error correction is done using the error-evaluator equation on pp 207.
	if errorsFound > 0 && errorsFound < RS_12_9_CHECKSUMSIZE {
		// First check for illegal error locations.
		for r := 0; r < errorsFound; r++ {
			if roots[r] >= RS_12_9_DATASIZE+RS_12_9_CHECKSUMSIZE {
				return errorsFound, errors.New("fec/rs_12_9: errors can't be corrected")
			}
		}

		// Evaluates rs_12_9_error_evaluator_poly/rs_12_9_error_locator_poly' at the roots
		// alpha^(-i) for error locs i.
		for r := 0; r < errorsFound; r++ {
			i := roots[r]

			var num, denom uint8
			// Evaluate rs_12_9_error_evaluator_poly at alpha^(-i)
			for j := 0; j < RS_12_9_POLY_MAXDEG; j++ {
				idx := (255 - int(i)) * j % 255
				num ^= ReedSolomon1294GaloisMul(ws.evaluator[j], galois_exp_table[idx])
			}

			// Evaluate rs_12_9_error_evaluator_poly' (derivative) at alpha^(-i). All odd powers disappear.
			for j := 1; j < RS_12_9_POLY_MAXDEG; j += 2 {
				idx := (255 - int(i)) * (j - 1) % 255
				denom ^= ReedSolomon1294GaloisMul(ws.locator[j], galois_exp_table[idx])
			}

			data[len(data)-int(i)-1] ^= ReedSolomon1294GaloisMul(num, ReedSolomon1294GaloisInv(denom))
		}

		return errorsFound, nil
	}

	return 0, nil
}

// The error-locator polynomial's roots are found by looking for the values of a^n where
// evaluating the polynomial yields zero (evaluating rs_12_9_error_locator_poly at
// successive values of alpha (Chien's search)).
func ReedSolomon1294FindRoots(locator *ReedSolomon1294) []uint8 {
	var rootsBuf [RS_12_9_DATASIZE + RS_12_9_CHECKSUMSIZE]uint8
	return ReedSolomon1294FindRootsInto(locator, rootsBuf[:0])
}

func ReedSolomon1294FindRootsInto(locator *ReedSolomon1294, buf []uint8) []uint8 {
	var k, r uint16
	roots := buf[:0]
	for r = 1; r < 256; r++ {
		var sum uint8
		// Evaluate locator at r
		for k = 0; k < RS_12_9_CHECKSUMSIZE+1; k++ {
			sum ^= ReedSolomon1294GaloisMul(galois_exp_table[(k*r)%255], locator[k])
		}

		if sum == 0 {
			roots = append(roots, uint8(255-r)) //nolint:gosec // r is 1..255, so 255-r is 0..254, fits in uint8
		}
	}

	return roots
}

func ReedSolomon1294Calc(syndrome, locator, evaluator *ReedSolomon1294) {
	var (
		psi2    [RS_12_9_POLY_MAXDEG]uint8
		product [RS_12_9_POLY_MAXDEG * 2]uint8
		tmp     [RS_12_9_POLY_MAXDEG * 2]uint8
	)

	ReedSolomon1294CalcWithScratch(syndrome, locator, evaluator, psi2[:], product[:], tmp[:])
}

func ReedSolomon1294CalcWithScratch(
	syndrome, locator, evaluator *ReedSolomon1294,
	psi2 []uint8, product []uint8, tmp []uint8,
) {
	var (
		n, lCur, l2 uint8
		k           int8
		d, i        uint8
		D           = ReedSolomon1294{0, 1, 0}
	)

	k = -1
	for i = 0; i < RS_12_9_POLY_MAXDEG; i++ {
		locator[i] = 0
		psi2[i] = 0
	}
	locator[0] = 1

	for n = 0; n < RS_12_9_CHECKSUMSIZE; n++ {
		d = ReedSolomon1294CalcDiscrepancy(locator, syndrome, lCur, n)
		if d != 0 {
			// psi2 = locator - d*D
			for i = 0; i < RS_12_9_POLY_MAXDEG; i++ {
				psi2[i] = locator[i] ^ ReedSolomon1294GaloisMul(d, D[i])
			}

			if int(lCur) < int(n)-int(k) {
				l2 = uint8(int(n) - int(k))  //nolint:gosec // n < RS_12_9_CHECKSUMSIZE (3), result fits in uint8
				k = int8(int(n) - int(lCur)) //nolint:gosec // n < RS_12_9_CHECKSUMSIZE (3), result fits in int8
				for i = 0; i < RS_12_9_POLY_MAXDEG; i++ {
					D[i] = ReedSolomon1294GaloisMul(locator[i], ReedSolomon1294GaloisInv(d))
				}
				lCur = l2
			}

			// locator = psi2
			for i = 0; i < RS_12_9_POLY_MAXDEG; i++ {
				locator[i] = psi2[i]
			}
		}
		ReedSolomon1294MulPolyZ(&D)
	}
	ReedSolomon1294CalcErrorEvaluatorPolyWithScratch(locator, syndrome, evaluator, product, tmp)
}
