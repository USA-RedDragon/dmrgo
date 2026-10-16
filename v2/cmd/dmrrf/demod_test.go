package main

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

// demodulate is Demodulate that fails t on an error.
func demodulate(t testing.TB, x []complex128, rate float64) Demodulated {
	t.Helper()
	d, err := Demodulate(x, rate)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// demodulateBytes is DemodulateBytes that fails t on an error.
func demodulateBytes(t testing.TB, iq []byte, rate float64, signed bool, tier rf.Tier) Demodulated {
	t.Helper()
	d, err := DemodulateBytes(iq, rate, signed, tier)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
