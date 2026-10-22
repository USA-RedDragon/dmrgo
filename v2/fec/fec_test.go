package fec_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

func TestBERCalculator_AddBurstMatchesAggregate(t *testing.T) {
	t.Parallel()

	stats := fec.BurstFECStats{
		SlotType: fec.Result{BitsChecked: 20, ErrorsCorrected: 1},
		EMB:      fec.Result{BitsChecked: 16, ErrorsCorrected: 2},
		Payload:  fec.Result{BitsChecked: 196, ErrorsCorrected: 3},
		PDU:      fec.Result{BitsChecked: 96, ErrorsCorrected: 5},
		RC:       fec.Result{BitsChecked: 32, ErrorsCorrected: 6},
	}
	var b fec.BERCalculator
	b.AddBurst(stats)

	agg := stats.Aggregate()
	if b.TotalBitsChecked() != uint64(agg.BitsChecked) || b.TotalErrors() != uint64(agg.ErrorsCorrected) {
		t.Errorf("AddBurst totals %d/%d, Aggregate %d/%d",
			b.TotalErrors(), b.TotalBitsChecked(), agg.ErrorsCorrected, agg.BitsChecked)
	}
	if b.TotalBitsChecked() != 264 || b.TotalErrors() != 17 {
		t.Errorf("totals %d errors / %d bits, want 17 / 264", b.TotalErrors(), b.TotalBitsChecked())
	}
}

func TestBERCalculator_BER(t *testing.T) {
	t.Parallel()

	var b fec.BERCalculator
	if b.BER() != 0 {
		t.Errorf("empty BER = %v, want 0", b.BER())
	}
	b.Add(fec.Result{BitsChecked: 200, ErrorsCorrected: 5})
	if got := b.BER(); got != 2.5 {
		t.Errorf("BER = %v, want 2.5", got)
	}
	b.Reset()
	if b.TotalBitsChecked() != 0 || b.TotalErrors() != 0 || b.BER() != 0 {
		t.Error("Reset did not clear totals")
	}
}

func TestResult_HasErrors(t *testing.T) {
	t.Parallel()

	if (fec.Result{BitsChecked: 10}).HasErrors() {
		t.Error("clean result reports errors")
	}
	if !(fec.Result{ErrorsCorrected: 1}).HasErrors() {
		t.Error("corrected result reports no errors")
	}
	if !(fec.Result{Uncorrectable: true}).HasErrors() {
		t.Error("uncorrectable result reports no errors")
	}
}

func TestBERCalculator_Uncorrectable(t *testing.T) {
	t.Parallel()

	var b fec.BERCalculator
	b.Add(fec.Result{BitsChecked: 24, ErrorsCorrected: 1})
	b.Add(fec.Result{BitsChecked: 24, Uncorrectable: true})
	b.AddBurst(fec.BurstFECStats{PDU: fec.Result{Uncorrectable: true}})
	if got := b.Uncorrectable(); got != 2 {
		t.Errorf("Uncorrectable() = %d, want 2", got)
	}
	b.Add(fec.Result{BitsChecked: -1, ErrorsCorrected: 1, Uncorrectable: true})
	b.Add(fec.Result{BitsChecked: 1, ErrorsCorrected: -1})
	if b.TotalBitsChecked() != 48 || b.TotalErrors() != 1 || b.Uncorrectable() != 2 {
		t.Errorf("negative counts were added: %d / %d / %d", b.TotalErrors(), b.TotalBitsChecked(), b.Uncorrectable())
	}
	b.Reset()
	if b.Uncorrectable() != 0 {
		t.Error("Reset did not clear Uncorrectable")
	}
}

func TestResult_Combine(t *testing.T) {
	t.Parallel()

	got := fec.Result{BitsChecked: 32, ErrorsCorrected: 1}.Combine(fec.Result{Uncorrectable: true})
	want := fec.Result{BitsChecked: 32, ErrorsCorrected: 1, Uncorrectable: true}
	if got != want {
		t.Errorf("Combine = %+v, want %+v", got, want)
	}
}

func TestResult_Err(t *testing.T) {
	t.Parallel()

	if err := (fec.Result{BitsChecked: 96, ErrorsCorrected: 3}).Err(); err != nil {
		t.Errorf("corrected result: Err() = %v, want nil", err)
	}
	if err := (fec.Result{Uncorrectable: true}).Err(); !errors.Is(err, fec.ErrUncorrectable) {
		t.Errorf("uncorrectable result: Err() = %v, want ErrUncorrectable", err)
	}
}
