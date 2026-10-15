package layer2_test

import (
	"strconv"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestTransmitOrder_BurstBitsConstant(t *testing.T) {
	t.Parallel()
	if layer2.BurstBits != 264 {
		t.Fatalf("BurstBits = %d, want 264", layer2.BurstBits)
	}
}

func TestTransmitOrder_FieldBoundaries(t *testing.T) {
	t.Parallel()
	// Verify the field boundaries sum to 264 and don't overlap
	fields := []struct {
		name  string
		start int
		end   int
	}{
		{"InfoFirst", layer2.InfoFirstStart, layer2.InfoFirstEnd},
		{"SlotTypeFirst", layer2.SlotTypeFirstStart, layer2.SlotTypeFirstEnd},
		{"Sync", layer2.SyncStart, layer2.SyncEnd},
		{"SlotTypeSecond", layer2.SlotTypeSecondStart, layer2.SlotTypeSecondEnd},
		{"InfoSecond", layer2.InfoSecondStart, layer2.InfoSecondEnd},
	}

	totalBits := 0
	for i, f := range fields {
		width := f.end - f.start
		if width <= 0 {
			t.Fatalf("field %s has non-positive width: %d", f.name, width)
		}
		totalBits += width

		// Check contiguous (each field starts where the previous ended)
		if i > 0 && f.start != fields[i-1].end {
			t.Fatalf("field %s starts at %d, but previous field %s ends at %d",
				f.name, f.start, fields[i-1].name, fields[i-1].end)
		}
	}

	if totalBits != 264 {
		t.Fatalf("total field bits = %d, want 264", totalBits)
	}
}

func TestTransmitOrder_FieldWidths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		start    int
		end      int
		expected int
	}{
		{"InfoFirst", layer2.InfoFirstStart, layer2.InfoFirstEnd, 98},
		{"SlotTypeFirst", layer2.SlotTypeFirstStart, layer2.SlotTypeFirstEnd, 10},
		{"Sync/EMB", layer2.SyncStart, layer2.SyncEnd, 48},
		{"SlotTypeSecond", layer2.SlotTypeSecondStart, layer2.SlotTypeSecondEnd, 10},
		{"InfoSecond", layer2.InfoSecondStart, layer2.InfoSecondEnd, 98},
	}
	for _, tt := range tests {
		if tt.end-tt.start != tt.expected {
			t.Errorf("%s width = %d, want %d", tt.name, tt.end-tt.start, tt.expected)
		}
	}
}

func TestTransmitOrder_EMBFieldWidths(t *testing.T) {
	t.Parallel()
	if layer2.EMBFirstEnd-layer2.EMBFirstStart != 8 {
		t.Error("EMB first half should be 8 bits")
	}
	if layer2.EmbeddedDataEnd-layer2.EmbeddedDataStart != 32 {
		t.Error("embedded data should be 32 bits")
	}
	if layer2.EMBSecondEnd-layer2.EMBSecondStart != 8 {
		t.Error("EMB second half should be 8 bits")
	}
	// EMB first + embedded data + EMB second = sync field
	total := (layer2.EMBFirstEnd - layer2.EMBFirstStart) +
		(layer2.EmbeddedDataEnd - layer2.EmbeddedDataStart) +
		(layer2.EMBSecondEnd - layer2.EMBSecondStart)
	if total != 48 {
		t.Fatalf("EMB+ES total = %d, want 48", total)
	}
}

// TestTransmitOrder_StandaloneRCFields checks the standalone RC burst field
// bounds against the symbol labels in Table E.12.
func TestTransmitOrder_StandaloneRCFields(t *testing.T) {
	tests := []struct {
		bitIndex int
		label    string
	}{
		{layer2.RCEMBFirstStart, "L24"},
		{layer2.RCEMBFirstEnd - 2, "L21"},
		{layer2.RCDataFirstStart, "L20"},
		{layer2.RCDataFirstEnd - 2, "L13"},
		{layer2.SyncStart, "L12"},
		{layer2.SyncEnd - 2, "R12"},
		{layer2.RCDataSecondStart, "R13"},
		{layer2.RCDataSecondEnd - 2, "R20"},
		{layer2.RCEMBSecondStart, "R21"},
		{layer2.RCEMBSecondEnd - 2, "R24"},
	}
	for _, tt := range tests {
		if got := dibitLabel(tt.bitIndex / 2); got != tt.label {
			t.Errorf("bit %d: symbol %s, want %s", tt.bitIndex, got, tt.label)
		}
	}
}

// dibitLabel returns the Annex E label of dibit symbol idx: L66 to L1, then
// R1 to R66.
func dibitLabel(idx int) string {
	if idx < 66 {
		return "L" + strconv.Itoa(66-idx)
	}
	return "R" + strconv.Itoa(idx-65)
}
