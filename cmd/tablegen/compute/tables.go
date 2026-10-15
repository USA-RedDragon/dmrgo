// Package compute contains pure functions that compute every FEC lookup table
// from its mathematical definition. No I/O, no global state.
package compute

import "sync"

// Tables holds every computed FEC lookup table.
type Tables struct {
	DM                        [256]uint8       // BPTC(196,96) deinterleave index, (i*181) mod 196
	Hamming15_11Syndrome      [16]int          // Hamming(15,11,3) bit position per syndrome, -1 for none
	Hamming13_9Syndrome       [16]int          // Hamming(13,9,3) bit position per syndrome, -1 for none
	Hamming743Syndrome        [8]int           // Hamming(7,4,3) bit position per syndrome, -1 for none
	Hamming16_11Syndrome      [32]int          // Hamming(16,11,4) bit position per syndrome, -1 for none
	Hamming17_12Syndrome      [32]int          // Hamming(17,12,3) bit position per syndrome, -1 for none
	CRCCCITT                  [256]uint16      // CRC-CCITT byte table, polynomial 0x1021
	CRC32                     [256]uint32      // CRC-32 byte table, polynomial 0x04C11DB7
	SingleBurstBPTCInterleave [32]uint8        // single-burst BPTC encode matrix index per transmit index
	ConstellationPoints       map[[2]int8]byte // trellis 3/4 symbol index per dibit pair
	ReverseConstellation      [][]int8         // trellis 3/4 dibit pair per symbol index
	ATable                    []int            // AMBE frame position of each bit of the A codeword
	BTable                    []int            // AMBE frame position of each bit of the B codeword
	CTable                    []int            // AMBE frame position of each bit of the C codeword
	GaloisExp                 [256]uint8       // GF(2^8) exponent table, polynomial 0x11D
	GaloisLog                 [256]uint8       // GF(2^8) logarithm table, polynomial 0x11D
	AMBEScrambleTable         [4096]uint32     // AMBE 24-bit scramble mask per 12-bit A value

	Golay20_8_7Encoding    []uint32     // Golay(20,8,7) codeword per 8-bit data value
	Golay20_8_7Syndrome    [4096]uint32 // Golay(20,8,7) error pattern per syndrome, 0xFFFFFFFF if uncorrectable
	Golay23_12_7Encoding   []uint32     // Golay(23,12,7) codeword shifted left by 1, per 12-bit data value
	Golay23_12_7Syndrome   [4096]uint32 // Golay(23,12,7) error pattern per syndrome, 0xFFFFFFFF if uncorrectable
	Golay24_12_8Encoding   []uint32     // Golay(24,12,8) codeword per 12-bit data value
	Golay24_12_8Syndrome   [4096]uint32 // Golay(24,12,8) error pattern per syndrome, 0xFFFFFFFF if uncorrectable
	QR16_7_6Encoding       []uint16     // QR(16,7,6) codeword per 7-bit data value
	QR16_7_6Syndrome       [512]uint16  // QR(16,7,6) error pattern per syndrome, 0xFFFF if uncorrectable
	EncoderStateTransition []byte       // trellis 3/4 encoder symbol at index state*8+tribit
	InterleaveMatrix       []byte       // trellis 3/4 dibit interleave order

	Trellis34Transition [][]byte // trellis 3/4 tribit per [state][symbol], 0xFF if invalid
}

// All computes every table and returns the complete set.
func All() *Tables {
	t := &Tables{}

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		t.DM = ComputeDM()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Hamming15_11Syndrome = ComputeHamming15_11Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Hamming13_9Syndrome = ComputeHamming13_9Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Hamming743Syndrome = ComputeHamming743Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Hamming16_11Syndrome = ComputeHamming16_11Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Hamming17_12Syndrome = ComputeHamming17_12Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.CRCCCITT = ComputeCRCCCITT()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.CRC32 = ComputeCRC32Table()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.SingleBurstBPTCInterleave = ComputeSingleBurstBPTCInterleave()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.ConstellationPoints = ComputeConstellationPoints()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.ReverseConstellation = ComputeReverseConstellation()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.ATable, t.BTable, t.CTable = ComputeVocoderTables()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.GaloisExp, t.GaloisLog = ComputeGaloisTables()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.AMBEScrambleTable = ComputeAMBEScrambleTable()
		wg.Done()
	}()
	wg.Wait()

	wg.Add(1)
	go func() {
		t.Golay23_12_7Encoding = ComputeGolay23_12_7Encoding()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Golay24_12_8Encoding = ComputeGolay24_12_8Encoding()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.QR16_7_6Encoding = ComputeQR16_7_6Encoding()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.QR16_7_6Syndrome = ComputeQR16_7_6Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.EncoderStateTransition = ComputeEncoderStateTransition()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.InterleaveMatrix = ComputeInterleaveMatrix()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Golay20_8_7Encoding = ComputeGolay20_8_7Encoding()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Golay20_8_7Syndrome = ComputeGolay20_8_7Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Golay23_12_7Syndrome = ComputeGolay23_12_7Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Golay24_12_8Syndrome = ComputeGolay24_12_8Syndrome()
		wg.Done()
	}()
	wg.Wait()

	wg.Add(1)
	go func() {
		t.Trellis34Transition = ComputeTrellis34Transition(t.EncoderStateTransition)
		wg.Done()
	}()

	wg.Wait()
	return t
}
