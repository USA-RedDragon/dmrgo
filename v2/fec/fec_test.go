package fec_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

func TestBERCalculator_AddBurstMatchesAggregate(t *testing.T) {
	stats := fec.BurstFECStats{
		SlotType: fec.FECResult{BitsChecked: 20, ErrorsCorrected: 1},
		EMB:      fec.FECResult{BitsChecked: 16, ErrorsCorrected: 2},
		Payload:  fec.FECResult{BitsChecked: 196, ErrorsCorrected: 3},
		Voice:    fec.FECResult{BitsChecked: 144, ErrorsCorrected: 4},
		PDU:      fec.FECResult{BitsChecked: 96, ErrorsCorrected: 5},
		RC:       fec.FECResult{BitsChecked: 32, ErrorsCorrected: 6},
	}
	var b fec.BERCalculator
	b.AddBurst(stats)

	agg := stats.Aggregate()
	if b.TotalBitsChecked() != agg.BitsChecked || b.TotalErrors() != agg.ErrorsCorrected {
		t.Errorf("AddBurst totals %d/%d, Aggregate %d/%d",
			b.TotalErrors(), b.TotalBitsChecked(), agg.ErrorsCorrected, agg.BitsChecked)
	}
	if b.TotalBitsChecked() != 504 || b.TotalErrors() != 21 {
		t.Errorf("totals %d errors / %d bits, want 21 / 504", b.TotalErrors(), b.TotalBitsChecked())
	}
}

func TestBERCalculator_BER(t *testing.T) {
	var b fec.BERCalculator
	if b.BER() != 0 {
		t.Errorf("empty BER = %v, want 0", b.BER())
	}
	b.Add(fec.FECResult{BitsChecked: 200, ErrorsCorrected: 5})
	if got := b.BER(); got != 2.5 {
		t.Errorf("BER = %v, want 2.5", got)
	}
	b.Reset()
	if b.TotalBitsChecked() != 0 || b.TotalErrors() != 0 || b.BER() != 0 {
		t.Error("Reset did not clear totals")
	}
}

func TestFECResult_HasErrors(t *testing.T) {
	if (fec.FECResult{BitsChecked: 10}).HasErrors() {
		t.Error("clean result reports errors")
	}
	if !(fec.FECResult{ErrorsCorrected: 1}).HasErrors() {
		t.Error("corrected result reports no errors")
	}
	if !(fec.FECResult{Uncorrectable: true}).HasErrors() {
		t.Error("uncorrectable result reports no errors")
	}
}
