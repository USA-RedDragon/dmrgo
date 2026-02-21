package elements

import (
	"strings"
	"testing"
)

func TestPositionError_Constants(t *testing.T) {
	tests := []struct {
		name     string
		value    PositionError
		expected uint8
	}{
		{"LessThan2M", PositionLessThan2M, 0},
		{"LessThan20M", PositionLessThan20M, 1},
		{"LessThan200M", PositionLessThan200M, 2},
		{"LessThan2KM", PositionLessThan2KM, 3},
		{"LessThan20KM", PositionLessThan20KM, 4},
		{"LessThan200KM", PositionLessThan200KM, 5},
		{"MoreThan200KM", PositionMoreThan200KM, 6},
		{"Unknown", PositionErrorUnknown, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if uint8(tt.value) != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.value, tt.expected)
			}
		})
	}
}

func TestPositionError_ToString(t *testing.T) {
	tests := []struct {
		value    PositionError
		contains string
	}{
		{PositionLessThan2M, "PositionLessThan2M"},
		{PositionLessThan20M, "PositionLessThan20M"},
		{PositionLessThan200M, "PositionLessThan200M"},
		{PositionLessThan2KM, "PositionLessThan2KM"},
		{PositionLessThan20KM, "PositionLessThan20KM"},
		{PositionLessThan200KM, "PositionLessThan200KM"},
		{PositionMoreThan200KM, "PositionMoreThan200KM"},
		{PositionErrorUnknown, "PositionErrorUnknown"},
	}

	for _, tt := range tests {
		t.Run(tt.contains, func(t *testing.T) {
			str := tt.value.ToString()
			if !strings.Contains(str, tt.contains) {
				t.Errorf("ToString() = %q, want to contain %q", str, tt.contains)
			}
		})
	}
}

func TestPositionErrorToName(t *testing.T) {
	for i := uint8(0); i <= 7; i++ {
		pe := PositionError(i)
		name := PositionErrorToName(pe)
		if name == "Unknown" {
			t.Errorf("PositionErrorToName(%d) returned Unknown", i)
		}
	}

	// Out of range should return Unknown
	name := PositionErrorToName(PositionError(99))
	if name != "Unknown" {
		t.Errorf("PositionErrorToName(99) = %q, want Unknown", name)
	}
}
