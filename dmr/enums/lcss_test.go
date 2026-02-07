package enums

import "testing"

func TestLCSSToName(t *testing.T) {
	tests := []struct {
		lcss     LCSS
		expected string
	}{
		{SingleFragmentLCorCSBK, "Single fragment LC or CSBK"},
		{FirstFragmentLC, "First fragment LC"},
		{LastFragmentLCorCSBK, "Last fragment LC or CSBK"},
		{ContinuationFragmentLCorCSBK, "Continuation fragment LC or CSBK"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := LCSSToName(tt.lcss)
			if got != tt.expected {
				t.Errorf("LCSSToName(%d) = %q, want %q", tt.lcss, got, tt.expected)
			}
		})
	}
}

func TestLCSSToName_Unknown(t *testing.T) {
	got := LCSSToName(LCSS(99))
	if got != voiceBurstUnknownName {
		t.Errorf("LCSSToName(99) = %q, want %q", got, voiceBurstUnknownName)
	}
}

func TestLCSSFromInt(t *testing.T) {
	tests := []struct {
		input    int
		expected LCSS
	}{
		{0, SingleFragmentLCorCSBK},
		{1, FirstFragmentLC},
		{2, LastFragmentLCorCSBK},
		{3, ContinuationFragmentLCorCSBK},
	}
	for _, tt := range tests {
		got := LCSSFromInt(tt.input)
		if got != tt.expected {
			t.Errorf("LCSSFromInt(%d) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func TestLCSSFromInt_InvalidDefaultsToSingle(t *testing.T) {
	got := LCSSFromInt(99)
	if got != SingleFragmentLCorCSBK {
		t.Errorf("LCSSFromInt(99) = %d, want %d (SingleFragmentLCorCSBK)", got, SingleFragmentLCorCSBK)
	}
}
