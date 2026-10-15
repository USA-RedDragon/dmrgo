package enums

import (
	"math/bits"
	"testing"
)

func TestSyncPatternFromBytes_AllKnownPatterns(t *testing.T) {
	tests := []struct {
		name     string
		pattern  SyncPattern
		expected SyncPattern
	}{
		{"BsSourcedVoice", SyncBSSourcedVoice, SyncBSSourcedVoice},
		{"BsSourcedData", SyncBSSourcedData, SyncBSSourcedData},
		{"MsSourcedVoice", SyncMSSourcedVoice, SyncMSSourcedVoice},
		{"MsSourcedData", SyncMSSourcedData, SyncMSSourcedData},
		{"MsSourcedRcSync", SyncMSSourcedRC, SyncMSSourcedRC},
		{"Tdma1Voice", SyncTDMA1Voice, SyncTDMA1Voice},
		{"Tdma1Data", SyncTDMA1Data, SyncTDMA1Data},
		{"Tdma2Voice", SyncTDMA2Voice, SyncTDMA2Voice},
		{"Tdma2Data", SyncTDMA2Data, SyncTDMA2Data},
		{"Reserved", SyncReserved, SyncReserved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert SyncPattern int64 to 6 bytes
			var syncBytes [6]byte
			val := int64(tt.pattern)
			for i := 0; i < 6; i++ {
				syncBytes[i] = byte(val >> (8 * (5 - i)))
			}
			got := SyncPatternFromBytes(syncBytes)
			if got != tt.expected {
				t.Errorf("SyncPatternFromBytes(%x) = %v, want %v", syncBytes, got, tt.expected)
			}
		})
	}
}

func TestSyncPatternFromBytes_UnknownReturnsEmbeddedSignalling(t *testing.T) {
	// An unrecognized byte pattern should return SyncEmbeddedSignalling
	var syncBytes [6]byte // all zeros, not a valid sync pattern
	got := SyncPatternFromBytes(syncBytes)
	if got != SyncEmbeddedSignalling {
		t.Errorf("SyncPatternFromBytes(all zeros) = %v, want SyncEmbeddedSignalling", got)
	}
}

func TestSyncPatternString_AllPatterns(t *testing.T) {
	tests := []struct {
		pattern  SyncPattern
		expected string
	}{
		{SyncBSSourcedVoice, "BS-sourced voice"},
		{SyncBSSourcedData, "BS-sourced data"},
		{SyncMSSourcedVoice, "MS-sourced voice"},
		{SyncMSSourcedData, "MS-sourced data"},
		{SyncMSSourcedRC, "MS-sourced RC sync"},
		{SyncTDMA1Voice, "TDMA1 voice"},
		{SyncTDMA1Data, "TDMA1 data"},
		{SyncTDMA2Voice, "TDMA2 voice"},
		{SyncTDMA2Data, "TDMA2 data"},
		{SyncEmbeddedSignalling, "Embedded Signalling"},
		{SyncReserved, "Reserved"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.pattern.String()
			if got != tt.expected {
				t.Errorf("(%v).String() = %q, want %q", tt.pattern, got, tt.expected)
			}
		})
	}
}

func TestSyncPatternString_UnknownReturnsEmbeddedSignalling(t *testing.T) {
	got := (SyncPattern(0x123456789ABC)).String()
	if got != "Embedded Signalling" {
		t.Errorf("unknown.String() = %q, want %q", got, "Embedded Signalling")
	}
}

func TestSyncPatternFromBytes_RoundTrip(t *testing.T) {
	patterns := []SyncPattern{
		SyncBSSourcedVoice, SyncBSSourcedData, SyncMSSourcedVoice, SyncMSSourcedData,
		SyncMSSourcedRC, SyncTDMA1Voice, SyncTDMA1Data, SyncTDMA2Voice, SyncTDMA2Data, SyncReserved,
	}
	for _, p := range patterns {
		var syncBytes [6]byte
		val := int64(p)
		for i := 0; i < 6; i++ {
			syncBytes[i] = byte(val >> (8 * (5 - i)))
		}
		got := SyncPatternFromBytes(syncBytes)
		if got != p {
			t.Errorf("round-trip failed for %s: got %v", p.String(), got)
		}
	}
}

func syncPatternBytes(p SyncPattern) [6]byte {
	var b [6]byte
	for i := 0; i < 6; i++ {
		b[i] = byte(int64(p) >> (8 * (5 - i)))
	}
	return b
}

func flipSyncBits(b [6]byte, positions ...int) [6]byte {
	for _, pos := range positions {
		b[pos/8] ^= 0x80 >> (pos % 8)
	}
	return b
}

func TestSyncPatternFromBytesWithDistance_Tolerant(t *testing.T) {
	flips := [][]int{
		{},
		{0},
		{47},
		{0, 47},
		{3, 17, 29},
		{1, 12, 30, 46},
		{44, 45, 46, 47},
	}
	for _, p := range syncPatterns {
		for _, f := range flips {
			got, dist := SyncPatternFromBytesWithDistance(flipSyncBits(syncPatternBytes(p), f...))
			if got != p || dist != len(f) {
				t.Errorf("%s with %d flipped bits: got %s distance %d", p.String(), len(f), got.String(), dist)
			}
		}
	}
}

func TestSyncPatternFromBytesWithDistance_BeyondThreshold(t *testing.T) {
	for _, p := range syncPatterns {
		in := flipSyncBits(syncPatternBytes(p), 0, 9, 18, 27, 36)
		got, dist := SyncPatternFromBytesWithDistance(in)
		if got != SyncEmbeddedSignalling {
			t.Errorf("%s with 5 flipped bits: got %s, want embedded signalling", p.String(), got.String())
		}
		if dist != SyncMatchThreshold+1 {
			t.Errorf("%s with 5 flipped bits: distance %d, want %d", p.String(), dist, SyncMatchThreshold+1)
		}
	}
}

func TestSyncPatterns_MinimumDistance(t *testing.T) {
	for i, a := range syncPatterns {
		for _, b := range syncPatterns[i+1:] {
			n := bits.OnesCount64(uint64(a ^ b))
			if n <= 2*SyncMatchThreshold {
				t.Errorf("%s and %s are %d bits apart; threshold %d is ambiguous", a.String(), b.String(), n, SyncMatchThreshold)
			}
		}
	}
}
