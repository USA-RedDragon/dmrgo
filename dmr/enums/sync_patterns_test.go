package enums

import "testing"

func TestSyncPatternFromBytes_AllKnownPatterns(t *testing.T) {
	tests := []struct {
		name     string
		pattern  SyncPattern
		expected SyncPattern
	}{
		{"BsSourcedVoice", BsSourcedVoice, BsSourcedVoice},
		{"BsSourcedData", BsSourcedData, BsSourcedData},
		{"MsSourcedVoice", MsSourcedVoice, MsSourcedVoice},
		{"MsSourcedData", MsSourcedData, MsSourcedData},
		{"MsSourcedRcSync", MsSourcedRcSync, MsSourcedRcSync},
		{"Tdma1Voice", Tdma1Voice, Tdma1Voice},
		{"Tdma1Data", Tdma1Data, Tdma1Data},
		{"Tdma2Voice", Tdma2Voice, Tdma2Voice},
		{"Tdma2Data", Tdma2Data, Tdma2Data},
		{"Reserved", Reserved, Reserved},
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
	// An unrecognized byte pattern should return EmbeddedSignallingPattern
	var syncBytes [6]byte // all zeros, not a valid sync pattern
	got := SyncPatternFromBytes(syncBytes)
	if got != EmbeddedSignallingPattern {
		t.Errorf("SyncPatternFromBytes(all zeros) = %v, want EmbeddedSignallingPattern", got)
	}
}

func TestSyncPatternToName_AllPatterns(t *testing.T) {
	tests := []struct {
		pattern  SyncPattern
		expected string
	}{
		{BsSourcedVoice, "BS-sourced voice"},
		{BsSourcedData, "BS-sourced data"},
		{MsSourcedVoice, "MS-sourced voice"},
		{MsSourcedData, "MS-sourced data"},
		{MsSourcedRcSync, "MS-sourced RC sync"},
		{Tdma1Voice, "TDMA1 voice"},
		{Tdma1Data, "TDMA1 data"},
		{Tdma2Voice, "TDMA2 voice"},
		{Tdma2Data, "TDMA2 data"},
		{EmbeddedSignallingPattern, "Embedded Signalling"},
		{Reserved, "Reserved"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := SyncPatternToName(tt.pattern)
			if got != tt.expected {
				t.Errorf("SyncPatternToName(%v) = %q, want %q", tt.pattern, got, tt.expected)
			}
		})
	}
}

func TestSyncPatternToName_UnknownReturnsEmbeddedSignalling(t *testing.T) {
	got := SyncPatternToName(SyncPattern(0x123456789ABC))
	if got != "Embedded Signalling" {
		t.Errorf("SyncPatternToName(unknown) = %q, want %q", got, "Embedded Signalling")
	}
}

func TestSyncPatternFromBytes_RoundTrip(t *testing.T) {
	patterns := []SyncPattern{
		BsSourcedVoice, BsSourcedData, MsSourcedVoice, MsSourcedData,
		MsSourcedRcSync, Tdma1Voice, Tdma1Data, Tdma2Voice, Tdma2Data, Reserved,
	}
	for _, p := range patterns {
		var syncBytes [6]byte
		val := int64(p)
		for i := 0; i < 6; i++ {
			syncBytes[i] = byte(val >> (8 * (5 - i)))
		}
		got := SyncPatternFromBytes(syncBytes)
		if got != p {
			t.Errorf("round-trip failed for %s: got %v", SyncPatternToName(p), got)
		}
	}
}
