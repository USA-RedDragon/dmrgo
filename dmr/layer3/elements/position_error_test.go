package elements

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
)

func TestNewPositionErrorFromBits_AllValues(t *testing.T) {
	tests := []struct {
		name     string
		bits     [3]bit.Bit
		checkFn  func(pe *PositionError) bool
		contains string
	}{
		{"LessThan2M", [3]bit.Bit{0, 0, 0}, func(pe *PositionError) bool { return pe.PositionLessThan2M }, "PositionLessThan2M"},
		{"LessThan20M", [3]bit.Bit{0, 0, 1}, func(pe *PositionError) bool { return pe.PositionLessThan20M }, "PositionLessThan20M"},
		{"LessThan200M", [3]bit.Bit{0, 1, 0}, func(pe *PositionError) bool { return pe.PositionLessThan200M }, "PositionLessThan200M"},
		{"LessThan2KM", [3]bit.Bit{0, 1, 1}, func(pe *PositionError) bool { return pe.PositionLessThan2KM }, "PositionLessThan2KM"},
		{"LessThan20KM", [3]bit.Bit{1, 0, 0}, func(pe *PositionError) bool { return pe.PositionLessThan20KM }, "PositionLessThan20KM"},
		{"LessThan200KM", [3]bit.Bit{1, 0, 1}, func(pe *PositionError) bool { return pe.PositionLessThan200KM }, "PositionLessThan200KM"},
		{"MoreThan200KM", [3]bit.Bit{1, 1, 0}, func(pe *PositionError) bool { return pe.PositionMoreThan200KM }, "PositionMoreThan200KM"},
		{"Unknown", [3]bit.Bit{1, 1, 1}, func(pe *PositionError) bool { return pe.PositionErrorUnknown }, "PositionErrorUnknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pe := NewPositionErrorFromBits(tt.bits)
			if pe == nil {
				t.Fatal("NewPositionErrorFromBits returned nil")
			}
			if !tt.checkFn(pe) {
				t.Errorf("Expected %s to be true", tt.contains)
			}
		})
	}
}

func TestPositionError_ToString(t *testing.T) {
	tests := []struct {
		name     string
		bits     [3]bit.Bit
		contains string
	}{
		{"LessThan2M", [3]bit.Bit{0, 0, 0}, "PositionLessThan2M"},
		{"LessThan20M", [3]bit.Bit{0, 0, 1}, "PositionLessThan20M"},
		{"LessThan200M", [3]bit.Bit{0, 1, 0}, "PositionLessThan200M"},
		{"LessThan2KM", [3]bit.Bit{0, 1, 1}, "PositionLessThan2KM"},
		{"LessThan20KM", [3]bit.Bit{1, 0, 0}, "PositionLessThan20KM"},
		{"LessThan200KM", [3]bit.Bit{1, 0, 1}, "PositionLessThan200KM"},
		{"MoreThan200KM", [3]bit.Bit{1, 1, 0}, "PositionMoreThan200KM"},
		{"Unknown", [3]bit.Bit{1, 1, 1}, "PositionErrorUnknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pe := NewPositionErrorFromBits(tt.bits)
			str := pe.ToString()
			if len(str) == 0 {
				t.Error("ToString returned empty string")
			}
			found := false
			for i := 0; i <= len(str)-len(tt.contains); i++ {
				if str[i:i+len(tt.contains)] == tt.contains {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("ToString() = %q, want to contain %q", str, tt.contains)
			}
		})
	}
}

func TestPositionError_OnlyOneFlagSet(t *testing.T) {
	for i := 0; i < 8; i++ {
		bits := [3]bit.Bit{
			bit.Bit((i >> 2) & 1),
			bit.Bit((i >> 1) & 1),
			bit.Bit(i & 1),
		}

		pe := NewPositionErrorFromBits(bits)
		if pe == nil {
			t.Fatalf("NewPositionErrorFromBits(%v) returned nil", bits)
		}

		count := 0
		if pe.PositionLessThan2M {
			count++
		}
		if pe.PositionLessThan20M {
			count++
		}
		if pe.PositionLessThan200M {
			count++
		}
		if pe.PositionLessThan2KM {
			count++
		}
		if pe.PositionLessThan20KM {
			count++
		}
		if pe.PositionLessThan200KM {
			count++
		}
		if pe.PositionMoreThan200KM {
			count++
		}
		if pe.PositionErrorUnknown {
			count++
		}

		if count != 1 {
			t.Errorf("bits=%v: expected exactly 1 flag set, got %d", bits, count)
		}
	}
}
