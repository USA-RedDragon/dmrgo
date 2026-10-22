package enums

import "testing"

func TestSlotString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		slot Slot
		want string
	}{
		{SlotUnknown, unknownName},
		{Slot1, "TS1"},
		{Slot2, "TS2"},
		{Slot(3), "Slot(3)"},
	}
	for _, tt := range tests {
		if got := tt.slot.String(); got != tt.want {
			t.Errorf("Slot(%d).String() = %q, want %q", tt.slot, got, tt.want)
		}
	}
}

func TestSlotOther(t *testing.T) {
	t.Parallel()

	tests := []struct{ slot, want Slot }{
		{SlotUnknown, SlotUnknown},
		{Slot1, Slot2},
		{Slot2, Slot1},
		{Slot(3), SlotUnknown},
	}
	for _, tt := range tests {
		if got := tt.slot.Other(); got != tt.want {
			t.Errorf("Slot(%d).Other() = %v, want %v", tt.slot, got, tt.want)
		}
	}
}

func TestAccessString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		access Access
		want   string
	}{
		{AccessPoliteOwnCC, "Polite to own Color Code"},
		{AccessPoliteAll, "Polite to all"},
		{AccessImpolite, "Impolite"},
		{Access(9), "Access(9)"},
	}
	for _, tt := range tests {
		if got := tt.access.String(); got != tt.want {
			t.Errorf("Access(%d).String() = %q, want %q", tt.access, got, tt.want)
		}
	}
}
