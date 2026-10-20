package main

import "testing"

func demodulate(t testing.TB, x []complex128, rate float64) Demodulated {
	t.Helper()
	d, err := Demodulate(x, rate)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
