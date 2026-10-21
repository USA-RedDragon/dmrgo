//go:build !tinygo

package mmdvm_test

import "testing"

func FuzzStream(f *testing.F) {
	for _, s := range fuzzSeeds() {
		f.Add(s)
	}
	f.Fuzz(checkStream)
}
