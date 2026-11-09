package main

import (
	"strings"
	"testing"
)

func TestCycleModel(t *testing.T) {
	t.Parallel()
	if err := checkCycleModel(); err != nil {
		t.Fatal(err)
	}
}

func TestBadCounts(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
			var total int64
			err := scanCounts(strings.NewReader(in), func(_ uint64, v int64) { total += v })
			if err == nil {
				t.Fatalf("no error, %d cycles", total)
			}
		})
	}
}

func TestCountsWithDisassembly(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestTableCosts(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		mn, ops string
		want    int64
	}{
		{"ldrb", "r0, [r1, #1]", 2},
		{"ldrsh.w", "r0, [r1, r2, lsl #1]", 2},
		{"strh", "r0, [r1]", 2},
		{"ldrex", "r0, [r1]", 2},
		{"strex", "r2, r0, [r1]", 2},
		{"tbb", "[pc, r0]", 2},
		{"mls", "r0, r1, r2, r3", 1},
		{"umlal", "r0, r1, r2, r3", 1},
		{"vldr", "s0, [r0]", 2},
		{"vldr", "d0, [r0, #8]", 3},
		{"vstr", "s1, [sp, #4]", 2},
		{"vstr", "d8, [sp]", 3},
		{"vdiv.f32", "s0, s1, s2", 14},
		{"vsqrt.f32", "s0, s1", 14},
		{"vmla.f32", "s0, s1, s2", 3},
		{"vmls.f32", "s0, s1, s2", 3},
		{"vmlsne.f32", "s0, s1, s2", 3},
		{"vnmla.f32", "s0, s1, s2", 3},
		{"vnmls.f32", "s0, s1, s2", 3},
		{"vfma.f32", "s0, s1, s2", 3},
		{"vfms.f32", "s0, s1, s2", 3},
		{"vfnma.f32", "s0, s1, s2", 3},
		{"vfnms.f32", "s0, s1, s2", 3},
		{"vadd.f32", "s0, s1, s2", 1},
		{"vmul.f32", "s0, s1, s2", 1},
		{"vcvt.s32.f32", "s0, s0", 1},
		{"vmov", "r0, s0", 2},
		{"vmov", "s0, r0", 2},
		{"vmov", "r0, r1, d0", 2},
		{"vmov", "s0, s1, r0, r1", 2},
		{"vmov.32", "d0[1], r0", 1},
		{"vmov.f32", "s0, #1.000000e+00", 1},
		{"vmov", "s0, s1", 1},
		{"vmrs", "apsr_nzcv, fpscr", 1},
	} {
		got, err := cyclesOf(c.mn, c.ops)
		if err != nil || got != c.want {
			t.Errorf("%s %s: %d, %v, want %d", c.mn, c.ops, got, err, c.want)
		}
	}
}

func TestNeighbors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, counts string
		want         int64
	}{
		{"TRM ldr then ldr", "0 2 1 0 ldr r0, [r1]\n2 2 1 0 ldr r1, [r2]\n", 3},
		{"TRM ldr str data", "0 4 1 0 ldr.w r0, [r1, r2]\n4 4 1 0 str.w r0, [r3, #20]\n", 3},
		{"TRM ldr str offset", "0 4 1 0 ldr.w r0, [r1, r2]\n4 4 1 0 str.w r1, [r3, r2]\n", 3},
		{"TRM three loads", "0 4 1 0 ldr.w r0, [r1, r5]\n4 2 1 0 ldr r1, [r2]\n6 2 1 0 ldr r2, [r3, #4]\n", 4},
		{"load address uses load", "0 2 1 0 ldr r0, [r1]\n2 2 1 0 ldr r2, [r0]\n", 4},
		{"nothing after a store", "0 2 1 0 str r0, [r1]\n2 2 1 0 ldr r2, [r3]\n", 4},
		{"no pair across a branch", "0 2 4 4 ldr r0, [r1]\n2 2 1 0 ldr r2, [r3]\n", 18},
		{"pair counts", "0 2 4 1 ldr r0, [r1]\n2 2 3 0 ldr r2, [r3]\n", 8 + 2 + 6 - 3},
		{"ldr pc", "0 2 1 1 ldr pc, [r1]\n2 2 1 0 ldr r2, [r3]\n", 6},
		{"ldrd", "0 2 1 0 ldr r0, [r1]\n2 4 1 0 ldrd r2, r3, [r4]\n", 5},
		{"address stall", "0 2 1 0 adds r1, #4\n2 2 1 0 ldr r0, [r1]\n", 4},
		{"address stall push", "0 2 1 0 sub sp, #8\n2 2 1 0 push {r4, lr}\n", 5},
		{"no address stall on data", "0 2 1 0 adds r0, #4\n2 2 1 0 str r0, [r1]\n", 3},
		{"no stall after compare", "0 2 1 0 cmp r1, #4\n2 2 1 0 ldr r0, [r1]\n", 3},
		{"fp result used", "0 4 1 0 vadd.f32 s0, s1, s2\n4 4 1 0 vmul.f32 s3, s0, s0\n", 3},
		{"fp result unused", "0 4 1 0 vadd.f32 s0, s1, s2\n4 4 1 0 vmul.f32 s3, s1, s2\n", 2},
		{"fp addend", "0 4 1 0 vmul.f32 s0, s1, s2\n4 4 1 0 vmla.f32 s0, s3, s4\n", 4},
		{"fp multiplicand", "0 4 1 0 vmul.f32 s0, s1, s2\n4 4 1 0 vmla.f32 s3, s0, s4\n", 5},
		{"fp stored", "0 4 1 0 vadd.f32 s0, s1, s2\n4 4 1 0 vstr s0, [r0]\n", 4},
		{"fp double alias", "0 4 1 0 vadd.f32 s1, s2, s1\n4 4 1 0 vmov r0, r1, d0\n", 4},
		{"fp loaded over", "0 4 1 0 vadd.f32 s0, s1, s2\n4 4 1 0 vldr s0, [r0]\n", 3},
		{"fp compared", "0 4 1 0 vsub.f32 s0, s1, s2\n4 4 1 0 vcmpe.f32 s0, #0\n", 3},
		{"vmov is not arithmetic", "0 4 1 0 vmov s0, r0\n4 4 1 0 vadd.f32 s1, s0, s0\n", 3},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var got int64
			if err := scanCounts(strings.NewReader(c.counts), func(_ uint64, v int64) { got += v }); err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("%d cycles, want %d", got, c.want)
			}
		})
	}
}
