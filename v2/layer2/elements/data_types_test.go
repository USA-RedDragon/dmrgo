package elements

import "testing"

func TestDataTypeToName_AllTypes(t *testing.T) {
	tests := []struct {
		dt       DataType
		expected string
	}{
		{DataTypePIHeader, "PI Header"},
		{DataTypeVoiceLCHeader, "Voice LC Header"},
		{DataTypeTerminatorWithLC, "Terminator With LC"},
		{DataTypeCSBK, "CSBK"},
		{DataTypeMBCHeader, "MBC Header"},
		{DataTypeMBCContinuation, "MBC Continuation"},
		{DataTypeDataHeader, "Data Header"},
		{DataTypeRate12, "Rate 1/2"},
		{DataTypeRate34, "Rate 3/4"},
		{DataTypeIdle, "Idle"},
		{DataTypeRate1, "Rate 1"},
		{DataTypeUnifiedSingleBlock, "Unified Single Block"},
		{DataTypeReserved, "Reserved"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := DataTypeToName(tt.dt)
			if got != tt.expected {
				t.Errorf("DataTypeToName(%d) = %q, want %q", tt.dt, got, tt.expected)
			}
		})
	}
}

func TestDataTypeToName_Unknown(t *testing.T) {
	got := DataTypeToName(DataType(99))
	if got != "Unknown" {
		t.Errorf("DataTypeToName(99) = %q, want %q", got, "Unknown")
	}
}

func TestDataType_Constants(t *testing.T) {
	// Verify iota-based ordering matches DMR spec nibble values
	if DataTypePIHeader != 0 {
		t.Errorf("DataTypePIHeader = %d, want 0", DataTypePIHeader)
	}
	if DataTypeVoiceLCHeader != 1 {
		t.Errorf("DataTypeVoiceLCHeader = %d, want 1", DataTypeVoiceLCHeader)
	}
	if DataTypeTerminatorWithLC != 2 {
		t.Errorf("DataTypeTerminatorWithLC = %d, want 2", DataTypeTerminatorWithLC)
	}
	if DataTypeCSBK != 3 {
		t.Errorf("DataTypeCSBK = %d, want 3", DataTypeCSBK)
	}
	if DataTypeIdle != 9 {
		t.Errorf("DataTypeIdle = %d, want 9", DataTypeIdle)
	}
	if DataTypeRate1 != 10 {
		t.Errorf("DataTypeRate1 = %d, want 10", DataTypeRate1)
	}
}
