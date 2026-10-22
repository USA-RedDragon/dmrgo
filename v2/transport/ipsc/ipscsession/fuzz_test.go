//go:build !tinygo

package ipscsession_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

func FuzzSession(f *testing.F) {
	for _, s := range fuzzSeeds() {
		f.Add(s)
	}
	f.Fuzz(checkSession)
}

func tooLong(p []byte) bool {
	return len(p) > ipscsession.MaxPacket
}

func controlTooLong(p []byte) bool {
	return len(p) > ipscsession.MaxControl
}
