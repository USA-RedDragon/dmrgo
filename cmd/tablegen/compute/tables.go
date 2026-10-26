// Package compute contains pure functions that compute every FEC lookup table
// from its mathematical definition. No I/O, no global state.
package compute

import "sync"

// Tables holds every computed FEC lookup table.
type Tables struct {
	CRC32                [256]uint32 // CRC-32 byte table, polynomial 0x04C11DB7
	CRC7                 [256]uint8  // CRC-7 byte table, polynomial 0xA7, register MSB-aligned
	CRC8                 [256]uint8  // CRC-8 byte table, polynomial 0x107
	CRC9                 [256]uint16 // CRC-9 byte table, polynomial 0x259, register MSB-aligned
	ReverseConstellation [][]int8    // trellis 3/4 dibit pair per symbol index
	GaloisExp            [256]uint8  // GF(2^8) exponent table, polynomial 0x11D
	GaloisLog            [256]uint8  // GF(2^8) logarithm table, polynomial 0x11D

	EncoderStateTransition []byte // trellis 3/4 encoder symbol at index state*8+tribit
	InterleaveMatrix       []byte // trellis 3/4 dibit interleave order

	Trellis34BranchDibits []byte     // trellis 3/4 dibit pair per branch state*8+tribit
	Trellis34BranchMetric [][]uint64 // trellis 3/4 Viterbi cost per [received dibit pair][state], tribit t in byte t

	PRFillBits             [96]uint8 // PR FILL information bits of an idle burst
	EmbeddedLCPatterns     []uint16  // embedded LC row error patterns of weight up to 4, by syndrome and weight
	EmbeddedLCPatternStart [161]int  // start of the patterns of syndrome s and weight w at index s*5+w
	EmbeddedLCMinWeight    [32]int   // least pattern weight per syndrome, -1 for none

	ISO8859High [][]uint16 // code point of octets 0xA0 to 0xFF per ISO/IEC 8859 part 2 to 16, 0 if undefined
}

// All computes every table and returns the complete set.
func All() *Tables {
	t := &Tables{}

	wg := sync.WaitGroup{}

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
		t.ReverseConstellation = ReverseConstellation()
		wg.Done()
	}()
	wg.Add(1)
	go func() {
		t.GaloisExp, t.GaloisLog = GaloisTables()
		wg.Done()
	}()
	wg.Wait()

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
	wg.Wait()

	wg.Add(1)
	go func() {
		t.Trellis34BranchDibits = Trellis34BranchDibits(t.EncoderStateTransition, t.ReverseConstellation)
		t.Trellis34BranchMetric = Trellis34BranchMetric(t.Trellis34BranchDibits)
		wg.Done()
	}()

	t.PRFillBits = PRFillBits()
	t.ISO8859High = ISO8859High()
	t.EmbeddedLCPatterns, t.EmbeddedLCPatternStart, t.EmbeddedLCMinWeight = EmbeddedLCPatterns()

	wg.Wait()
	return t
}
