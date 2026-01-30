package prng

import "testing"

// TestPRNGTableLength verifies the PRNG table has the expected size (4096 entries)
func TestPRNGTableLength(t *testing.T) {
	expectedLength := 4096
	if len(PRNG_TABLE) != expectedLength {
		t.Errorf("PRNG_TABLE length = %d, expected %d", len(PRNG_TABLE), expectedLength)
	}
}

// TestPRNGTableNotEmpty verifies the PRNG table is populated with non-zero values
func TestPRNGTableNotEmpty(t *testing.T) {
	nonZeroCount := 0
	for _, v := range PRNG_TABLE {
		if v != 0 {
			nonZeroCount++
		}
	}

	// Expect vast majority of entries to be non-zero (statistically, nearly all should be)
	if nonZeroCount < len(PRNG_TABLE)*95/100 {
		t.Errorf("Too many zero values in PRNG_TABLE: %d non-zero out of %d", nonZeroCount, len(PRNG_TABLE))
	}
}

// TestPRNGTableValues verifies specific known values in the PRNG table
func TestPRNGTableValues(t *testing.T) {
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
		if tc.index >= len(PRNG_TABLE) {
			t.Errorf("Test index %d out of range", tc.index)
			continue
		}
		if PRNG_TABLE[tc.index] != tc.expected {
			t.Errorf("PRNG_TABLE[%d] = 0x%06X, expected 0x%06X", tc.index, PRNG_TABLE[tc.index], tc.expected)
		}
	}
}

// TestPRNGTableBitWidth verifies all values fit within 24 bits (DMR constraint)
func TestPRNGTableBitWidth(t *testing.T) {
	maxValue := uint32(0xFFFFFF) // 24-bit max value
	for i, v := range PRNG_TABLE {
		if v > maxValue {
			t.Errorf("PRNG_TABLE[%d] = 0x%08X exceeds 24-bit max (0x%06X)", i, v, maxValue)
		}
	}
}

// TestPRNGTableDistribution verifies statistical properties of the table
func TestPRNGTableDistribution(t *testing.T) {
	// Check that bit distribution is roughly uniform (each bit set ~50% of the time)
	bitCounts := make([]int, 24)

	for _, v := range PRNG_TABLE {
		for bit := 0; bit < 24; bit++ {
			if v&(1<<bit) != 0 {
				bitCounts[bit]++
			}
		}
	}

	tableLen := len(PRNG_TABLE)
	expectedCount := tableLen / 2
	tolerance := tableLen / 10 // Allow 10% deviation

	for bit, count := range bitCounts {
		if count < expectedCount-tolerance || count > expectedCount+tolerance {
			t.Errorf("Bit %d set %d times out of %d (expected ~%d±%d)",
				bit, count, tableLen, expectedCount, tolerance)
		}
	}
}

// TestPRNGTableUniqueness verifies a high degree of unique values
func TestPRNGTableUniqueness(t *testing.T) {
	seen := make(map[uint32]int)
	duplicates := 0

	for i, v := range PRNG_TABLE {
		if prev, exists := seen[v]; exists {
			duplicates++
			if duplicates <= 5 {
				t.Logf("Duplicate value 0x%06X at indices %d and %d", v, prev, i)
			}
		}
		seen[v] = i
	}

	// Allow some duplicates but not too many (PRNG tables may have rare collisions)
	maxDuplicates := len(PRNG_TABLE) / 100 // Allow up to 1% duplicates
	if duplicates > maxDuplicates {
		t.Errorf("Too many duplicate values in PRNG_TABLE: %d (max allowed %d)", duplicates, maxDuplicates)
	}
}

// BenchmarkPRNGTableAccess measures random access performance
func BenchmarkPRNGTableAccess(b *testing.B) {
	tableLen := len(PRNG_TABLE)
	for i := 0; i < b.N; i++ {
		_ = PRNG_TABLE[i%tableLen]
	}
}

// BenchmarkPRNGXOR simulates typical DMR whitening operation
func BenchmarkPRNGXOR(b *testing.B) {
	data := make([]byte, 196) // Typical DMR burst size
	for i := range data {
		data[i] = byte(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := 0; j < len(data); j++ {
			prngVal := PRNG_TABLE[j%len(PRNG_TABLE)]
			data[j] ^= byte(prngVal)
		}
	}
}
