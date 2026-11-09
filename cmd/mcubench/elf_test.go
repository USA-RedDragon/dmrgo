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

func TestBadCounts(t *testing.T) {
	for name, in := range map[string]string{
		"empty disassembly": "1000 2 3 0 \n1002 4 1 1 \n",
		"no disassembly":    "1000 2 3 0\n",
		"mixed":             "1000 2 3 0 ldr r0, [r1]\n1002 2 1 0 \n",
		"hex bytes":         "1000 2 3 0 0x6808\n",
		"no instructions":   "",
		"no register list":  "1000 2 1 0 push r4\n",
		"unknown register":  "1000 2 1 0 pop {r4, q0}\n",
		"reversed range":    "1000 4 1 0 vpush {s5-s2}\n",
		"mixed range":       "1000 4 1 0 vpush {s0-d2}\n",
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

func TestRegisterLists(t *testing.T) {
	for _, c := range []struct {
		mn, ops string
		want    int64
	}{
		{"ldm", "r0!, {r1, r2}", 3},
		{"ldmia", "r0, {r1-r3}", 4},
		{"stm", "r0!, {r1, r2, r3, r4}", 5},
		{"stmdb", "sp!, {r4, r5, r6, r7, r8, sb, sl, fp, lr}", 10},
		{"push", "{r4, lr}", 3},
		{"pop", "{r4, r5, r6, pc}", 5},
		{"pop", "{r0-r12, lr}", 15},
		{"ldm", "sp!, {ip, sp, lr, pc}", 5},
		{"vpush", "{s16, s17}", 3},
		{"vpush", "{d8-d15}", 17},
		{"vpop", "{d8}", 3},
		{"vldmia", "r1!, {s0-s7}", 9},
		{"vstmia", "r0, {d0, d1}", 5},
		{"vldmdb", "r2!, {s4}", 2},
	} {
		got, err := cyclesOf(c.mn, c.ops)
		if err != nil || got != c.want {
			t.Errorf("%s %s: %d, %v, want %d", c.mn, c.ops, got, err, c.want)
		}
	}
}
