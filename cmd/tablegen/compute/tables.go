// Package compute contains pure functions that compute every FEC lookup table
// from its mathematical definition. No I/O, no global state.
package compute

import "sync"

// Tables holds every computed FEC lookup table.
type Tables struct {
	DM                        [256]uint8   // BPTC(196,96) deinterleave index, (i*181) mod 196
	Hamming15_11Syndrome      [16]int      // Hamming(15,11,3) bit position per syndrome, -1 for none
	Hamming13_9Syndrome       [16]int      // Hamming(13,9,3) bit position per syndrome, -1 for none
	Hamming743Syndrome        [8]int       // Hamming(7,4,3) bit position per syndrome, -1 for none
	Hamming16_11Syndrome      [32]int      // Hamming(16,11,4) bit position per syndrome, -1 for none
	Hamming17_12Syndrome      [32]int      // Hamming(17,12,3) bit position per syndrome, -1 for none
	CRC32                     [256]uint32  // CRC-32 byte table, polynomial 0x04C11DB7
	CRC7                      [256]uint8   // CRC-7 byte table, polynomial 0xA7, register MSB-aligned
	CRC8                      [256]uint8   // CRC-8 byte table, polynomial 0x107
	CRC9                      [256]uint16  // CRC-9 byte table, polynomial 0x259, register MSB-aligned
	SingleBurstBPTCInterleave [32]uint8    // single-burst BPTC encode matrix index per transmit index
	ReverseConstellation      [][]int8     // trellis 3/4 dibit pair per symbol index
	GaloisExp                 [256]uint8   // GF(2^8) exponent table, polynomial 0x11D
	GaloisLog                 [256]uint8   // GF(2^8) logarithm table, polynomial 0x11D
	AMBEScrambleTable         [4096]uint32 // AMBE 24-bit scramble mask per 12-bit A value

	Golay23_12_7Encoding   []uint32     // Golay(23,12,7) codeword shifted left by 1, per 12-bit data value
	Golay23_12_7Syndrome   [4096]uint32 // Golay(23,12,7) error pattern per syndrome, 0xFFFFFFFF if uncorrectable
	Golay24_12_8Encoding   []uint32     // Golay(24,12,8) codeword per 12-bit data value
	Golay24_12_8Syndrome   [4096]uint32 // Golay(24,12,8) error pattern per syndrome, 0xFFFFFFFF if uncorrectable
	EncoderStateTransition []byte       // trellis 3/4 encoder symbol at index state*8+tribit
	InterleaveMatrix       []byte       // trellis 3/4 dibit interleave order

	Trellis34Transition   [][]byte   // trellis 3/4 tribit per [state][symbol], 0xFF if invalid
	Trellis34BranchDibits []byte     // trellis 3/4 dibit pair per branch state*8+tribit
	Trellis34BranchMetric [][]uint64 // trellis 3/4 Viterbi cost per [received dibit pair][state], tribit t in byte t

	PRFillBits             [96]uint8 // PR FILL information bits of an idle burst
	EmbeddedLCPatterns     []uint16  // embedded LC row error patterns of weight up to 4, by syndrome and weight
	EmbeddedLCPatternStart [161]int  // start of the patterns of syndrome s and weight w at index s*5+w
	EmbeddedLCMinWeight    [32]int   // least pattern weight per syndrome, -1 for none
}

// All computes every table and returns the complete set.
func All() *Tables {
	t := &Tables{}

	wg := sync.WaitGroup{}

	wg.Add(1)
	go func() {
		t.DM = DM()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Hamming15_11Syndrome = Hamming15_11Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Hamming13_9Syndrome = Hamming13_9Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Hamming743Syndrome = Hamming743Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Hamming16_11Syndrome = Hamming16_11Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Hamming17_12Syndrome = Hamming17_12Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.CRC32 = CRC32Table()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.CRC7 = CRC7Table()
		t.CRC8 = CRC8Table()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.CRC9 = CRC9Table()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.SingleBurstBPTCInterleave = SingleBurstBPTCInterleave()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.ReverseConstellation = ReverseConstellation()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.GaloisExp, t.GaloisLog = GaloisTables()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.AMBEScrambleTable = AMBEScrambleTable()
		wg.Done()
	}()
	wg.Wait()

	wg.Add(1)
	go func() {
		t.Golay23_12_7Encoding = Golay23_12_7Encoding()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Golay24_12_8Encoding = Golay24_12_8Encoding()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.EncoderStateTransition = EncoderStateTransition()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.InterleaveMatrix = InterleaveMatrix()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Golay23_12_7Syndrome = Golay23_12_7Syndrome()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.Golay24_12_8Syndrome = Golay24_12_8Syndrome()
		wg.Done()
	}()
	wg.Wait()

	wg.Add(1)
	go func() {
		t.Trellis34Transition = Trellis34Transition(t.EncoderStateTransition)
		t.Trellis34BranchDibits = Trellis34BranchDibits(t.EncoderStateTransition, t.ReverseConstellation)
		t.Trellis34BranchMetric = Trellis34BranchMetric(t.Trellis34BranchDibits)
		wg.Done()
	}()

	t.PRFillBits = PRFillBits()
	t.EmbeddedLCPatterns, t.EmbeddedLCPatternStart, t.EmbeddedLCMinWeight = EmbeddedLCPatterns()

	wg.Wait()
	return t
}
