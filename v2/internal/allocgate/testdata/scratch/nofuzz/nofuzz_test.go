package nofuzz_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/allocgate/testdata/scratch/nofuzz"
)

func TestRun(t *testing.T) {
	var m nofuzz.Machine
	if out := m.Run(nil); len(out) > nofuzz.MaxOut {
		t.Fatal(len(out))
	}
}
