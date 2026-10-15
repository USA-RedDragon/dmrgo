package enums

import "testing"

func TestDataTypeString_AllTypes(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			got := tt.dt.String()
			if got != tt.expected {
				t.Errorf("(%d).String() = %q, want %q", tt.dt, got, tt.expected)
			}
		})
	}
}

func TestDataTypeString_Unknown(t *testing.T) {
	t.Parallel()

	got := (DataType(99)).String()
	if got != "Unknown" {
		t.Errorf("(99).String() = %q, want %q", got, "Unknown")
	}
}

func TestDataType_Constants(t *testing.T) {
	t.Parallel()

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

func TestDataType_IsBPTC(t *testing.T) {
	t.Parallel()

	for dt := DataType(0); dt < 0x20; dt++ {
		want := dt <= DataTypeUnifiedSingleBlock && dt != DataTypeRate34 && dt != DataTypeRate1
		if got := dt.IsBPTC(); got != want {
			t.Errorf("DataType(%d).IsBPTC() = %v, want %v", dt, got, want)
		}
		wantLC := dt == DataTypeVoiceLCHeader || dt == DataTypeTerminatorWithLC
		if got := dt.CarriesFullLC(); got != wantLC {
			t.Errorf("DataType(%d).CarriesFullLC() = %v, want %v", dt, got, wantLC)
		}
	}
}

func TestDataType_IsReserved(t *testing.T) {
	t.Parallel()

	for dt := DataType(0); dt < 0x20; dt++ {
		want := dt >= 12 && dt <= 15
		if got := dt.IsReserved(); got != want {
			t.Errorf("DataType(%d).IsReserved() = %v, want %v", dt, got, want)
		}
		if want && dt.String() != "Reserved" {
			t.Errorf("(%d).String() = %q, want Reserved", dt, dt.String())
		}
	}
}
