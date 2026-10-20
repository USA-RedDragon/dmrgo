//go:build !tinygo

package data_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/data"
)

func corpusFiles(t *testing.T, name string) [][]byte {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join("testdata", "fuzz", name, "*"))
	out := make([][]byte, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, b)
	}
	return out
}

func TestAllocsReceiverCorpus(t *testing.T) {
	p := pool(t)
	ev := make([]data.Event, 0, data.MaxEvents)
	for i, in := range append(rxSeeds(), corpusFiles(t, "FuzzReceiver")...) {
		s := parseRxScript(in, p)
		r, err := data.NewReceiver(&s.cfg)
		if err != nil {
			continue
		}
		allocs := testing.AllocsPerRun(3, func() {
			r.Reset()
			for j := range s.ops {
				ev = runRxOp(r, &s.ops[j], ev[:0])
			}
		})
		if allocs != 0 {
			t.Errorf("input %d: %v allocations", i, allocs)
		}
	}
}
