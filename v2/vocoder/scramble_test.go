package vocoder

import "testing"

// TestAMBEScrambleTableValues verifies specific known values in the AMBE scramble table
func TestAMBEScrambleTableValues(t *testing.T) {
	// Test first few known values from the table
	expectedValues := []struct {
		index    int
		expected uint32
	}{
		{0, 0x42CC47},
		{1, 0x19D6FE},
		{2, 0x304729},
		{3, 0x6B2CD0},
		{4, 0x60BF47},
		{5, 0x39650E},
		{10, 0xD7B745},
		{100, 0x16C05E},
	}

	for _, tc := range expectedValues {
		if tc.index >= len(ambeScrambleTable) {
			t.Errorf("Test index %d out of range", tc.index)
			continue
		}
		if ambeScrambleTable[tc.index] != tc.expected {
			t.Errorf("ambeScrambleTable[%d] = 0x%06X, expected 0x%06X", tc.index, ambeScrambleTable[tc.index], tc.expected)
		}
	}
}

// TestScrambleTableBitWidth verifies all values fit within 24 bits (DMR constraint)
func TestScrambleTableBitWidth(t *testing.T) {
	maxValue := uint32(0xFFFFFF) // 24-bit max value
	for i, v := range ambeScrambleTable {
		if v > maxValue {
			t.Errorf("ambeScrambleTable[%d] = 0x%08X exceeds 24-bit max (0x%06X)", i, v, maxValue)
		}
	}
}

// TestScrambleTableDistribution verifies statistical properties of the table
func TestScrambleTableDistribution(t *testing.T) {
	// Check that bit distribution is roughly uniform (each bit set ~50% of the time)
	bitCounts := make([]int, 24)

	for _, v := range ambeScrambleTable {
		for bit := 0; bit < 24; bit++ {
			if v&(1<<bit) != 0 {
				bitCounts[bit]++
			}
		}
	}

	tableLen := len(ambeScrambleTable)
	expectedCount := tableLen / 2
	tolerance := tableLen / 10 // Allow 10% deviation

	for bit, count := range bitCounts {
		if count < expectedCount-tolerance || count > expectedCount+tolerance {
			t.Errorf("Bit %d set %d times out of %d (expected ~%d±%d)",
				bit, count, tableLen, expectedCount, tolerance)
		}
	}
}

// BenchmarkScrambleTableAccess measures random access performance
func BenchmarkScrambleTableAccess(b *testing.B) {
	tableLen := len(ambeScrambleTable)
	for i := 0; i < b.N; i++ {
		_ = ambeScrambleTable[i%tableLen]
	}
}

// BenchmarkScrambleXOR simulates typical DMR whitening operation
func BenchmarkScrambleXOR(b *testing.B) {
	data := make([]byte, 196) // Typical DMR burst size
	for i := range data {
		data[i] = byte(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(data); j++ {
			scrambleVal := ambeScrambleTable[j%len(ambeScrambleTable)]
			data[j] ^= byte(scrambleVal)
		}
	}
}
