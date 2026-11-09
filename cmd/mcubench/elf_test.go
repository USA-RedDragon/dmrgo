package main

import (
	"strings"
	"testing"
)

func TestCycleModel(t *testing.T) {
	if err := checkCycleModel(); err != nil {
		t.Fatal(err)
	}
}

func TestCountsWithoutDisassembly(t *testing.T) {
	for name, in := range map[string]string{
		"empty disassembly": "1000 2 3 0 \n1002 4 1 1 \n",
		"no disassembly":    "1000 2 3 0\n",
		"mixed":             "1000 2 3 0 ldr r0, [r1]\n1002 2 1 0 \n",
		"hex bytes":         "1000 2 3 0 0x6808\n",
		"no instructions":   "",
	} {
		t.Run(name, func(t *testing.T) {
			var total int64
			err := scanCounts(strings.NewReader(in), func(_ uint64, v int64) { total += v })
			if err == nil {
				t.Fatalf("no error, %d cycles", total)
			}
		})
	}
}

func TestCountsWithDisassembly(t *testing.T) {
	var total int64
	err := scanCounts(strings.NewReader("1000 2 3 0 ldr r0, [r1]\n1002 4 1 1 b.w #0x1000\n"), func(_ uint64, v int64) { total += v })
	if err != nil {
		t.Fatal(err)
	}
	if total != 9 {
		t.Fatalf("%d cycles, want 9", total)
	}
}
