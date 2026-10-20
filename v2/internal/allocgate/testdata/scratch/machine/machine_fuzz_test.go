package machine_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/allocgate/testdata/scratch/machine"
)

func FuzzRun(f *testing.F) {
	f.Fuzz(func(t *testing.T, _ []byte) {
		var m machine.Machine
		if out := m.Run(nil); len(out) > machine.MaxOut {
			t.Fatal(len(out))
		}
	})
}
