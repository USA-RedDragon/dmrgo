package enums

import (
	"math/bits"
	"testing"
)

func syncPatternBytes(p SyncPattern) [6]byte {
	var b [6]byte
	for i := 0; i < 6; i++ {
		b[i] = byte(uint64(p) >> (8 * (5 - i)))
	}
	return b
}

func flipSyncBits(b [6]byte, positions ...int) [6]byte {
	for _, pos := range positions {
		b[pos/8] ^= 0x80 >> (pos % 8)
	}
	return b
}

func TestNearestSyncPattern_AllKnownPatterns(t *testing.T) {
	t.Parallel()

	for _, p := range syncPatterns {
		got, dist := NearestSyncPattern(syncPatternBytes(p))
		if got != p || dist != 0 {
			t.Errorf("NearestSyncPattern(%s) = %s distance %d", p.String(), got.String(), dist)
		}
	}
}

func TestNearestSyncPattern_Tolerant(t *testing.T) {
	t.Parallel()

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
			got, dist := NearestSyncPattern(flipSyncBits(syncPatternBytes(p), f...))
			if got != p || dist != len(f) {
				t.Errorf("%s with %d flipped bits: got %s distance %d", p.String(), len(f), got.String(), dist)
			}
		}
	}
}

func TestSyncPatternString_AllPatterns(t *testing.T) {
	t.Parallel()

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
		{SyncNone, "Embedded Signalling"},
		{SyncReserved, "Reserved"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			got := tt.pattern.String()
			if got != tt.expected {
				t.Errorf("(%v).String() = %q, want %q", tt.pattern, got, tt.expected)
			}
		})
	}
}

func TestSyncPatternString_Unknown(t *testing.T) {
	t.Parallel()

	got := (SyncPattern(0x123456789ABC)).String()
	if got != "SyncPattern(20015998343868)" {
		t.Errorf("unknown.String() = %q", got)
	}
}

func TestSyncPatterns_MinimumDistance(t *testing.T) {
	t.Parallel()

	for i, a := range syncPatterns {
		for _, b := range syncPatterns[i+1:] {
			if n := bits.OnesCount64(uint64(a ^ b)); n < 10 {
				t.Errorf("%s and %s are %d bits apart, want at least 10", a.String(), b.String(), n)
			}
		}
	}
}
