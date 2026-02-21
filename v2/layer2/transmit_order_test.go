package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestTransmitOrder_BurstBitsConstant(t *testing.T) {
	t.Parallel()
	if layer2.BurstBits != 264 {
		t.Fatalf("BurstBits = %d, want 264", layer2.BurstBits)
	}
}

func TestTransmitOrder_BurstSymbolsConstant(t *testing.T) {
	t.Parallel()
	if layer2.BurstSymbols != 132 {
		t.Fatalf("BurstSymbols = %d, want 132", layer2.BurstSymbols)
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

func TestTransmitOrder_BurstToDibits(t *testing.T) {
	t.Parallel()
	var bits [264]bit.Bit
	// Set first dibit to 10 (bit1=1, bit0=0) — symbol L66
	bits[0] = 1
	bits[1] = 0
	// Set last dibit to 01 (bit1=0, bit0=1) — symbol R66
	bits[262] = 0
	bits[263] = 1

	symbols := layer2.BurstToDibits(bits)

	if len(symbols) != 132 {
		t.Fatalf("BurstToDibits returned %d symbols, want 132", len(symbols))
	}

	// First symbol is L66
	if symbols[0].Label != "L66" {
		t.Errorf("first symbol label = %q, want L66", symbols[0].Label)
	}
	if symbols[0].Bit1 != 1 || symbols[0].Bit0 != 0 {
		t.Errorf("first symbol bits = (%d,%d), want (1,0)", symbols[0].Bit1, symbols[0].Bit0)
	}

	// Last symbol is R66
	if symbols[131].Label != "R66" {
		t.Errorf("last symbol label = %q, want R66", symbols[131].Label)
	}
	if symbols[131].Bit1 != 0 || symbols[131].Bit0 != 1 {
		t.Errorf("last symbol bits = (%d,%d), want (0,1)", symbols[131].Bit1, symbols[131].Bit0)
	}

	// Symbol L1 is at index 65
	if symbols[65].Label != "L1" {
		t.Errorf("symbols[65].Label = %q, want L1", symbols[65].Label)
	}

	// Symbol R1 is at index 66
	if symbols[66].Label != "R1" {
		t.Errorf("symbols[66].Label = %q, want R1", symbols[66].Label)
	}
}

func TestTransmitOrder_DibitsToBurst_Roundtrip(t *testing.T) {
	t.Parallel()
	var original [264]bit.Bit
	// Fill with a pattern
	for i := range original {
		original[i] = bit.Bit(i % 2)
	}

	symbols := layer2.BurstToDibits(original)
	result := layer2.DibitsToBurst(symbols)

	if result != original {
		t.Fatal("DibitsToBurst(BurstToDibits(bits)) should return original bits")
	}
}

func TestTransmitOrder_ValidateBurstStructure_Valid(t *testing.T) {
	t.Parallel()
	var bits [264]bit.Bit
	for i := range bits {
		bits[i] = bit.Bit(i % 2)
	}
	if idx := layer2.ValidateBurstStructure(bits); idx != -1 {
		t.Fatalf("ValidateBurstStructure returned %d for valid burst", idx)
	}
}

func TestTransmitOrder_ValidateBurstStructure_Invalid(t *testing.T) {
	t.Parallel()
	var bits [264]bit.Bit
	bits[42] = 2 // invalid bit value
	if idx := layer2.ValidateBurstStructure(bits); idx != 42 {
		t.Fatalf("ValidateBurstStructure returned %d, want 42", idx)
	}
}

func TestTransmitOrder_DibitLabels_Symmetry(t *testing.T) {
	t.Parallel()
	var bits [264]bit.Bit
	symbols := layer2.BurstToDibits(bits)

	// L-symbols: indices 0..65 should all start with "L"
	for i := 0; i < 66; i++ {
		if symbols[i].Label[0] != 'L' {
			t.Errorf("symbols[%d].Label = %q, want L-prefix", i, symbols[i].Label)
		}
	}

	// R-symbols: indices 66..131 should all start with "R"
	for i := 66; i < 132; i++ {
		if symbols[i].Label[0] != 'R' {
			t.Errorf("symbols[%d].Label = %q, want R-prefix", i, symbols[i].Label)
		}
	}
}
