package enums

import "testing"

func TestLCSSString(t *testing.T) {
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
			got := tt.lcss.String()
			if got != tt.expected {
				t.Errorf("(%d).String() = %q, want %q", tt.lcss, got, tt.expected)
			}
		})
	}
}

func TestLCSSString_Unknown(t *testing.T) {
	got := (LCSS(99)).String()
	if got != unknownName {
		t.Errorf("(99).String() = %q, want %q", got, unknownName)
	}
}

func TestLCSS_Cast(t *testing.T) {
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
		got := LCSS(tt.input)
		if got != tt.expected {
			t.Errorf("LCSS(%d) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}
