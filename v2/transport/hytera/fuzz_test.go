//go:build !tinygo

package hytera_test

import "testing"

func FuzzPacket(f *testing.F) {
	for _, s := range fuzzSeeds() {
		f.Add(s)
	}
	f.Fuzz(checkPacket)
}
