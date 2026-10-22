package layer3

import (
	"strings"
	"testing"
)

func TestPositionError_Constants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    PositionAccuracy
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
			t.Parallel()

			if uint8(tt.value) != tt.expected {
				t.Errorf("%s = %d, want %d", tt.name, tt.value, tt.expected)
			}
		})
	}
}

func TestPositionError_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    PositionAccuracy
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
			t.Parallel()

			str := tt.value.String()
			if !strings.Contains(str, tt.contains) {
				t.Errorf("String() = %q, want to contain %q", str, tt.contains)
			}
		})
	}
}

func TestPositionErrorString(t *testing.T) {
	t.Parallel()

	for i := uint8(0); i <= 7; i++ {
		pe := PositionAccuracy(i)
		name := pe.String()
		if name == "Unknown" {
			t.Errorf("(%d).String() returned Unknown", i)
		}
	}

	name := (PositionAccuracy(99)).String()
	if name != "Unknown" {
		t.Errorf("(99).String() = %q, want Unknown", name)
	}
}
