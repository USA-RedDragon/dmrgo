//go:build !tinygo

package data_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/data"
)

func corpusFiles(t *testing.T, name string, n int) [][]string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join("testdata", "fuzz", name, "*"))
	out := make([][]string, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		if len(lines) != n+1 || lines[0] != "go test fuzz v1" {
			t.Fatalf("%s: not a %s input", f, name)
		}
		out = append(out, lines[1:])
	}
	return out
}

func corpusUint8(t *testing.T, v string) uint8 {
	t.Helper()
	switch {
	case strings.HasPrefix(v, "byte('") && strings.HasSuffix(v, "')"):
		r, _, tail, err := strconv.UnquoteChar(v[len("byte('"):len(v)-2], '\'')
		if err == nil && tail == "" && r < 256 {
			return uint8(r)
		}
	case strings.HasPrefix(v, "byte(") && strings.HasSuffix(v, ")"):
		n, err := strconv.ParseUint(v[len("byte("):len(v)-1], 0, 8)
		if err == nil {
			return uint8(n)
		}
	case strings.HasPrefix(v, "uint8(") && strings.HasSuffix(v, ")"):
		n, err := strconv.ParseUint(v[len("uint8("):len(v)-1], 0, 8)
		if err == nil {
			return uint8(n)
		}
	}
	t.Fatalf("%q: not a uint8", v)
	return 0
}

func corpusBytes(t *testing.T, v string) []byte {
	t.Helper()
	if strings.HasPrefix(v, "[]byte(") && strings.HasSuffix(v, ")") {
		s, err := strconv.Unquote(v[len("[]byte(") : len(v)-1])
		if err == nil {
			return []byte(s)
		}
	}
	t.Fatalf("%q: not a []byte", v)
	return nil
}

func rxCorpus(t *testing.T) []rxInput {
	t.Helper()
	files := corpusFiles(t, "FuzzReceiver", 4)
	out := make([]rxInput, 0, len(files))
	for _, v := range files {
		out = append(out, rxInput{c0: corpusUint8(t, v[0]), c1: corpusUint8(t, v[1]),
			step: corpusUint8(t, v[2]), ops: corpusBytes(t, v[3])})
	}
	return out
}

func txCorpus(t *testing.T) []txInput {
	t.Helper()
	files := corpusFiles(t, "FuzzSender", 5)
	out := make([]txInput, 0, len(files))
	for _, v := range files {
		out = append(out, txInput{c0: corpusUint8(t, v[0]), c1: corpusUint8(t, v[1]),
			c2: corpusUint8(t, v[2]), step: corpusUint8(t, v[3]), ops: corpusBytes(t, v[4])})
	}
	return out
}

func TestAllocsReceiverCorpus(t *testing.T) {
	p := pool(t)
	ev := make([]data.Event, 0, data.MaxEvents)
	for i, in := range append(rxSeeds(), rxCorpus(t)...) {
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
