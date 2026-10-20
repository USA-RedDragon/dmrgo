package fec_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// fuzzResult reads one Result from the first 5 bytes of data: 2 bytes of
// BitsChecked, 2 bytes of ErrorsCorrected and 1 byte whose low bit is
// Uncorrectable. Bytes past the end of data read as 0.
func fuzzResult(data []byte) (fec.Result, []byte) {
	var b [5]byte
	n := copy(b[:], data)
	return fec.Result{
		BitsChecked:     int(b[0])<<8 | int(b[1]),
		ErrorsCorrected: int(b[2])<<8 | int(b[3]),
		Uncorrectable:   b[4]&1 != 0,
	}, data[n:]
}

// checkCombine checks Err, HasErrors and Combine on the Results r.
func checkCombine(t *testing.T, r [5]fec.Result) {
	t.Helper()
	for _, x := range r {
		if err := x.Err(); errors.Is(err, fec.ErrUncorrectable) != x.Uncorrectable || (err == nil) == x.Uncorrectable {
			t.Fatalf("%+v: Err() = %v", x, err)
		}
		if x.HasErrors() != (x.ErrorsCorrected > 0 || x.Uncorrectable) {
			t.Fatalf("%+v: HasErrors() = %v", x, x.HasErrors())
		}
		if got := x.Combine(fec.Result{}); got != x {
			t.Fatalf("%+v.Combine(zero) = %+v", x, got)
		}
	}
	a, b, c := r[0], r[1], r[2]
	if a.Combine(b) != b.Combine(a) {
		t.Fatalf("Combine of %+v and %+v is not commutative", a, b)
	}
	if a.Combine(b).Combine(c) != a.Combine(b.Combine(c)) {
		t.Fatalf("Combine of %+v, %+v and %+v is not associative", a, b, c)
	}
	sum := a.Combine(b)
	want := fec.Result{
		BitsChecked:     a.BitsChecked + b.BitsChecked,
		ErrorsCorrected: a.ErrorsCorrected + b.ErrorsCorrected,
		Uncorrectable:   a.Uncorrectable || b.Uncorrectable,
	}
	if sum != want {
		t.Fatalf("%+v.Combine(%+v) = %+v, want %+v", a, b, sum, want)
	}
}

// checkBER checks BurstFECStats.Aggregate and BERCalculator on the Results
// r, which are the layers of one burst in BurstFECStats field order.
func checkBER(t *testing.T, r [5]fec.Result) {
	t.Helper()
	stats := fec.BurstFECStats{SlotType: r[0], EMB: r[1], Payload: r[2], PDU: r[3], RC: r[4]}
	agg := stats.Aggregate()
	wantAgg := fec.Result{
		BitsChecked:     r[0].BitsChecked + r[1].BitsChecked + r[2].BitsChecked + r[4].BitsChecked,
		ErrorsCorrected: r[0].ErrorsCorrected + r[1].ErrorsCorrected + r[2].ErrorsCorrected + r[3].ErrorsCorrected + r[4].ErrorsCorrected,
		Uncorrectable:   r[0].Uncorrectable || r[1].Uncorrectable || r[2].Uncorrectable || r[3].Uncorrectable || r[4].Uncorrectable,
	}
	if agg != wantAgg {
		t.Fatalf("%+v.Aggregate() = %+v, want %+v", stats, agg, wantAgg)
	}

	var each, burst fec.BERCalculator
	unc := 0
	for _, x := range r {
		each.Add(x)
		if x.Uncorrectable {
			unc++
		}
	}
	burst.AddBurst(stats)
	burst.AddBurst(stats)
	if burst.TotalBitsChecked() != 2*agg.BitsChecked || burst.TotalErrors() != 2*agg.ErrorsCorrected {
		t.Fatalf("AddBurst twice: %d errors / %d bits, Aggregate %+v", burst.TotalErrors(), burst.TotalBitsChecked(), agg)
	}
	wantUnc := 0
	if agg.Uncorrectable {
		wantUnc = 2
	}
	if burst.Uncorrectable() != wantUnc {
		t.Fatalf("AddBurst twice: Uncorrectable() = %d, want %d", burst.Uncorrectable(), wantUnc)
	}
	totalBits := 0
	totalErrs := 0
	for _, x := range r {
		totalBits += x.BitsChecked
		totalErrs += x.ErrorsCorrected
	}
	if each.TotalBitsChecked() != totalBits || each.TotalErrors() != totalErrs || each.Uncorrectable() != unc {
		t.Fatalf("Add: %d errors / %d bits / %d uncorrectable, want %d / %d / %d",
			each.TotalErrors(), each.TotalBitsChecked(), each.Uncorrectable(), totalErrs, totalBits, unc)
	}
	wantBER := 0.0
	if totalBits != 0 {
		wantBER = float64(totalErrs) * 100 / float64(totalBits)
	}
	if got := each.BER(); got != wantBER {
		t.Fatalf("BER() = %v, want %v", got, wantBER)
	}
	if totalErrs <= totalBits && (each.BER() < 0 || each.BER() > 100) {
		t.Fatalf("BER() = %v out of range for %d errors / %d bits", each.BER(), totalErrs, totalBits)
	}
	each.Reset()
	if each != (fec.BERCalculator{}) || each.BER() != 0 {
		t.Fatalf("Reset left %+v", each)
	}
}

// FuzzResult builds five Results from fuzzed bytes and checks Combine,
// Err, HasErrors, BurstFECStats.Aggregate and BERCalculator against their
// definitions: Combine adds counts and ORs Uncorrectable, is commutative
// and associative with the zero Result as identity, Aggregate leaves out
// the PDU bit count only, and BER is the sum of corrected errors over the
// sum of checked bits as a percentage.
func FuzzResult(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 20, 0, 1, 0, 0, 16, 0, 2, 0, 0, 196, 0, 3, 0, 0, 96, 0, 5, 0, 0, 32, 0, 6, 0})
	f.Add([]byte{0, 24, 0, 1, 0, 0, 24, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})
	f.Add([]byte{0, 200, 0, 5, 0})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 1, 0, 0, 0, 1, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		var r [5]fec.Result
		for i := range r {
			r[i], data = fuzzResult(data)
		}
		checkCombine(t, r)
		checkBER(t, r)
	})
}
