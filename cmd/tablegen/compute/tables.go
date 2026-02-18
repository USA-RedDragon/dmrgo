// Package compute contains pure functions that compute every FEC lookup table
// from its mathematical definition. No I/O, no global state.
package compute

import "sync"

// Tables holds every computed FEC lookup table.
type Tables struct {
	// Tier 0
	DM                   [256]uint8
	Hamming15_11Syndrome [16]int
	Hamming13_9Syndrome  [16]int
	CRCCCITT             [256]uint16
	ConstellationPoints  map[[2]int8]byte
	ReverseConstellation [][]int8
	ATable               []int
	BTable               []int
	CTable               []int
	GaloisExp            [256]uint8
	GaloisLog            [256]uint8

	// Tier 1
	Golay20_8_7Encoding    []uint32
	Golay20_8_7Syndrome    [4096]uint32
	Golay23_12_7Encoding   []uint32
	Golay23_12_7Syndrome   [4096]uint32
	Golay24_12_8Encoding   []uint32
	Golay24_12_8Syndrome   [4096]uint32
	QR16_7_6Encoding       []uint16
	QR16_7_6Syndrome       [512]uint16
	EncoderStateTransition []byte
	InterleaveMatrix       []byte
	PRNGTable              []uint32

	// Tier 2
	Trellis34Transition [][]byte
}

// All computes every table and returns the complete set.
func All() *Tables {
	t := &Tables{}

	wg := sync.WaitGroup{}

	// Tier 0
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
		t.CRCCCITT = ComputeCRCCCITT()
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
	wg.Wait()

	// Tier 1
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
	wg.Add(1)
	go func() {
		t.PRNGTable = ComputePRNGTable()
		wg.Done()
	}()
	wg.Wait()

	// Tier 2
	wg.Add(1)
	go func() {
		t.Trellis34Transition = ComputeTrellis34Transition(t.EncoderStateTransition)
		wg.Done()
	}()

	wg.Wait()
	return t
}
