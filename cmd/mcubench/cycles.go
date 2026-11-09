package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var (
	reMnemonic   = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z0-9]+)*$`)
	reMulti      = regexp.MustCompile(`^(ldm|stm)(ia|db|ea|fd)?$|^push$|^pop$|^v(ldm|stm)(ia|db)?$|^vpush$|^vpop$`)
	reReg        = regexp.MustCompile(`^([rsd])(\d+)$`)
	reRegToken   = regexp.MustCompile(`\b(r1[0-5]|r[0-9]|sb|sl|fp|ip|sp|lr|pc|s[12]?[0-9]|s3[01]|d[12]?[0-9]|d3[01])\b`)
	reCondSuffix = regexp.MustCompile(`(eq|ne|cs|hs|cc|lo|mi|pl|vs|vc|hi|ls|ge|lt|gt|le|al)$`)
)

const (
	mnLDRD = "ldrd"
	mnSTRD = "strd"
	mnTEQ  = "teq"
)

func coreRegAlias(r string) (int, bool) {
	switch r {
	case "sb":
		return 9, true
	case "sl":
		return 10, true
	case "fp":
		return 11, true
	case "ip":
		return 12, true
	case "sp":
		return 13, true
	case "lr":
		return 14, true
	case "pc":
		return 15, true
	}
	return 0, false
}

const (
	regSP  = 13
	regPC  = 15
	fpBank = 100
)

func noCondStrip(m string) bool {
	switch m {
	case "mls", "smlal", "umlal", "smmls", "vmls", "vnmls", mnTEQ, "svc", "sbcs", "adcs", "lsls":
		return true
	}
	return false
}

func nonDataProc(m string) bool {
	switch m {
	case "b", "bl", "bx", "blx", "cbz", "cbnz", "tbb", "tbh",
		"cmp", "cmn", "tst", mnTEQ, "nop", "isb", "dmb", "dsb",
		"wfe", "wfi", "sev", "yield", "cpsid", "cpsie", "msr", "mrs",
		"svc", "bkpt", "clrex":
		return true
	}
	return false
}

func isFPMAC(m string) bool {
	switch m {
	case "vmla", "vmls", "vnmla", "vnmls", "vfma", "vfms", "vfnma", "vfnms":
		return true
	}
	return false
}

func isFPArith(m string) bool {
	switch m {
	case "vadd", "vsub", "vmul", "vnmul", "vdiv", "vsqrt":
		return true
	}
	return isFPMAC(m) || strings.HasPrefix(m, "vcvt")
}

type insn struct {
	addr, size uint64
	n, taken   int64
	m          string
	ops        []string
	cost       int64
}

func normalize(mn string) string {
	head, _, _ := strings.Cut(mn, ".")
	if !noCondStrip(head) {
		if loc := reCondSuffix.FindStringIndex(head); loc != nil && loc[0] > 0 {
			head = head[:loc[0]]
		}
	}
	return head
}

func splitOps(s string) []string {
	var ops []string
	depth, start := 0, 0
	for i, c := range s {
		switch c {
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		case ',':
			if depth == 0 {
				ops = append(ops, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if t := strings.TrimSpace(s[start:]); t != "" {
		ops = append(ops, t)
	}
	return ops
}

func parseReg(r string) (byte, int, error) {
	if i, ok := coreRegAlias(r); ok {
		return 'r', i, nil
	}
	m := reReg.FindStringSubmatch(r)
	if m == nil {
		return 0, 0, fmt.Errorf("unknown register %q", r)
	}
	i, err := strconv.Atoi(m[2])
	if err != nil || (m[1] == "r" && i > 15) || i > 31 {
		return 0, 0, fmt.Errorf("unknown register %q", r)
	}
	return m[1][0], i, nil
}

func regIDs(kind byte, i int) []int {
	switch kind {
	case 's':
		return []int{fpBank + i}
	case 'd':
		return []int{fpBank + 2*i, fpBank + 2*i + 1}
	}
	return []int{i}
}

func regList(op string) (ids []int, words int64, err error) {
	_, list, ok := strings.Cut(op, "{")
	if ok {
		list, _, ok = strings.Cut(list, "}")
	}
	if !ok {
		return nil, 0, errors.New("no register list")
	}
	for _, item := range strings.Split(list, ",") {
		lo, hi, isRange := strings.Cut(strings.TrimSpace(item), "-")
		if !isRange {
			hi = lo
		}
		kl, il, err := parseReg(strings.TrimSpace(lo))
		if err != nil {
			return nil, 0, err
		}
		kh, ih, err := parseReg(strings.TrimSpace(hi))
		if err != nil {
			return nil, 0, err
		}
		if kl != kh || ih < il {
			return nil, 0, fmt.Errorf("bad register range %q", item)
		}
		for i := il; i <= ih; i++ {
			r := regIDs(kl, i)
			ids = append(ids, r...)
			words += int64(len(r))
		}
	}
	return ids, words, nil
}

func opRegs(op string) []int {
	if strings.Contains(op, "{") {
		ids, _, _ := regList(op)
		return ids
	}
	var ids []int
	for _, t := range reRegToken.FindAllString(op, -1) {
		if k, i, err := parseReg(t); err == nil {
			ids = append(ids, regIDs(k, i)...)
		}
	}
	return ids
}

func regsOf(ops []string) []int {
	ids := make([]int, 0, len(ops))
	for _, op := range ops {
		ids = append(ids, opRegs(op)...)
	}
	return ids
}

func overlaps(a, b []int) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func isSingleLoad(m string) bool  { return strings.HasPrefix(m, "ldr") && m != mnLDRD }
func isSingleStore(m string) bool { return strings.HasPrefix(m, "str") && m != mnSTRD }

func isLoad(m string) bool {
	return isSingleLoad(m) || m == mnLDRD || m == "vldr" || m == "pop" || m == "vpop" ||
		strings.HasPrefix(m, "ldm") || strings.HasPrefix(m, "vldm")
}

func isStore(m string) bool {
	return isSingleStore(m) || m == mnSTRD || m == "vstr" || m == "push" || m == "vpush" ||
		strings.HasPrefix(m, "stm") || strings.HasPrefix(m, "vstm")
}

func addrRegs(m string, ops []string) []int {
	switch {
	case m == "push" || m == "pop" || m == "vpush" || m == "vpop":
		return []int{regSP}
	case reMulti.MatchString(m):
		if len(ops) > 0 {
			return opRegs(ops[0])
		}
	case isLoad(m) || isStore(m):
		for _, op := range ops {
			if strings.Contains(op, "[") {
				return opRegs(op)
			}
		}
	}
	return nil
}

func isDataProc(m string) bool {
	return !strings.HasPrefix(m, "v") && !strings.HasPrefix(m, "it") && !nonDataProc(m) && !isLoad(m) && !isStore(m)
}

func dataProcDest(m string, ops []string) []int {
	n := 1
	for _, p := range []string{"smull", "umull", "smlal", "umlal", "umaal", "smlald", "smlsld"} {
		if strings.HasPrefix(m, p) {
			n = 2
		}
	}
	var ids []int
	for _, r := range regsOf(ops[:min(n, len(ops))]) {
		if r < fpBank {
			ids = append(ids, r)
		}
	}
	return ids
}

func fpSources(m string, ops []string) []int {
	var src []string
	switch {
	case isLoad(m):
	case isStore(m) || m == "cmp" || m == "cmn" || m == "tst" || m == mnTEQ || strings.HasPrefix(m, "vcmp"):
		src = ops
	case len(ops) > 1:
		src = ops[1:]
	}
	var ids []int
	for _, r := range regsOf(src) {
		if r >= fpBank {
			ids = append(ids, r)
		}
	}
	return ids
}

// cyclesOf returns the Cortex-M4F cycles of one instruction with mnemonic
// mn and operands ops at zero wait states, by ARM DDI 0439C (Cortex-M4 TRM
// r0p1) Table 3-1, Table 3-2 and Table 7-1:
//   - LDM, STM, PUSH and POP take 1+N, where N counts every register of the
//     list, PC and LR included; VLDM, VSTM, VPUSH and VPOP take 1+N for
//     single and 1+2*N for double registers.
//   - Single loads and stores, exclusive ones included, take 2, LDRD and
//     STRD 3 (1+N), TBB and TBH 2.
//   - SDIV and UDIV take 12, the end of the TRM range of 2 to 12, as the
//     early termination depends on the operands.
//   - VLDR and VSTR take 2 for single and 3 for double registers, VDIV and
//     VSQRT 14, the multiply-accumulate forms VMLA, VMLS, VNMLA, VNMLS,
//     VFMA, VFMS, VFNMA and VFNMS 3, a VMOV between core and floating-point
//     registers 2, and every other instruction 1.
//
// The pipeline refill P of a branch, of a load of PC and of TBB and TBH is
// not part of the result: scanCounts adds 2, inside the TRM range of 1 to
// 3, each time the next executed instruction is not the next in memory.
// The model takes no cycle off an IT folded onto a 16-bit instruction,
// adds none for the fetch contention of LDR Rx,[PC,#imm], charges an
// instruction that fails its condition in full, charges VDIV and VSQRT
// the full 14 when no later floating-point instruction waits for them,
// and charges CPSID, CPSIE, MRS and MSR 1.
func cyclesOf(mn, ops string) (int64, error) {
	return costOf(normalize(mn), splitOps(ops))
}

func costOf(m string, ops []string) (int64, error) {
	switch {
	case reMulti.MatchString(m):
		if len(ops) == 0 {
			return 0, errors.New("no register list")
		}
		_, words, err := regList(ops[len(ops)-1])
		if err != nil {
			return 0, err
		}
		return 1 + words, nil
	case m == mnLDRD || m == mnSTRD:
		return 3, nil
	case isSingleLoad(m) || isSingleStore(m) || m == "tbb" || m == "tbh":
		return 2, nil
	case m == "sdiv" || m == "udiv":
		return 12, nil
	case m == "vldr" || m == "vstr":
		if len(ops) > 0 && strings.HasPrefix(ops[0], "d") {
			return 3, nil
		}
		return 2, nil
	case m == "vdiv" || m == "vsqrt":
		return 14, nil
	case isFPMAC(m):
		return 3, nil
	case m == "vmov":
		var core, fp, scalar bool
		for _, op := range ops {
			scalar = scalar || strings.Contains(op, "[")
			for _, r := range opRegs(op) {
				core = core || r < fpBank
				fp = fp || r >= fpBank
			}
		}
		if core && fp && !scalar {
			return 2, nil
		}
	}
	return 1, nil
}

// neighborCycles returns the cycles that the TRM adds to or takes from x
// and y when y runs right after x, by ARM DDI 0439C section 3.3.2 and the
// notes of Table 7-1: a single load or store after a single load, other
// than a load of PC, whose address does not use the loaded register takes
// 1 less; a load or store whose address uses the result of the data
// processing instruction before it takes 1 more; a floating-point
// arithmetic instruction whose result the next instruction consumes, other
// than as the addend of a multiply-accumulate, takes 1 more.
func neighborCycles(x, y *insn) (dx, dy int64) {
	if len(x.ops) == 0 {
		return 0, 0
	}
	ya := addrRegs(y.m, y.ops)
	xd := opRegs(x.ops[0])
	if isSingleLoad(x.m) && !overlaps(xd, []int{regPC}) && (isSingleLoad(y.m) || isSingleStore(y.m)) && !overlaps(ya, xd) {
		dy--
	}
	if isDataProc(x.m) && overlaps(ya, dataProcDest(x.m, x.ops)) {
		dy++
	}
	if isFPArith(x.m) && overlaps(fpSources(y.m, y.ops), xd) {
		dx++
	}
	return dx, dy
}

// scanCounts reads the output of the m4count QEMU plugin, one line per
// translated instruction with its address, size, executions, taken
// redirections and disassembly, and calls add with the cycles of each
// instruction address. An instruction runs before the next one in memory
// its executions less its taken redirections, which gives the count of
// each neighbor pair for neighborCycles.
func scanCounts(r io.Reader, add func(addr uint64, cycles int64)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	var ins []insn
	var undecoded int64
	var firstBad string
	for sc.Scan() {
		fs := strings.SplitN(sc.Text(), " ", 5)
		if len(fs) < 4 {
			return fmt.Errorf("%q", sc.Text())
		}
		a, err1 := strconv.ParseUint(fs[0], 16, 64)
		size, err2 := strconv.ParseUint(fs[1], 10, 64)
		n, err3 := strconv.ParseInt(fs[2], 10, 64)
		taken, err4 := strconv.ParseInt(fs[3], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || taken > n {
			return fmt.Errorf("%q", sc.Text())
		}
		var mn, ops string
		if len(fs) > 4 {
			d := strings.Fields(strings.ToLower(fs[4]))
			if len(d) > 0 {
				mn = d[0]
				ops = strings.Join(d[1:], " ")
			}
		}
		if !reMnemonic.MatchString(mn) {
			if undecoded == 0 {
				firstBad = sc.Text()
			}
			undecoded++
			ins = append(ins, insn{})
			continue
		}
		i := insn{addr: a, size: size, n: n, taken: taken, m: normalize(mn), ops: splitOps(ops)}
		c, err := costOf(i.m, i.ops)
		if err != nil {
			return fmt.Errorf("%q: %w", sc.Text(), err)
		}
		i.cost = c
		ins = append(ins, i)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if len(ins) == 0 {
		return errors.New("the plugin counted no instructions")
	}
	if undecoded > 0 {
		return fmt.Errorf("%d of %d instructions have no ARM mnemonic, first %q: QEMU must be built with capstone (--enable-capstone) for the cycle model", undecoded, len(ins), firstBad)
	}
	at := map[uint64]*insn{}
	seq := map[uint64]int64{}
	var addrs []uint64
	for k := range ins {
		i := &ins[k]
		add(i.addr, i.n*i.cost+2*i.taken)
		if at[i.addr] == nil {
			at[i.addr] = i
			addrs = append(addrs, i.addr)
		}
		seq[i.addr] += i.n - i.taken
	}
	for _, a := range addrs {
		x := at[a]
		y := at[a+x.size]
		if y == nil || seq[a] == 0 {
			continue
		}
		dx, dy := neighborCycles(x, y)
		if dx != 0 {
			add(x.addr, seq[a]*dx)
		}
		if dy != 0 {
			add(y.addr, seq[a]*dy)
		}
	}
	return nil
}

const cycleSample = `1000 2 3 0 ldr r0, [r1, #4]
1010 4 2 0 ldr.w r2, [r3, r4, lsl #2]
1020 2 1 0 ldrne r0, [r1]
1030 4 1 0 ldrd r2, r3, [r0]
1040 4 1 0 strd r2, r3, [r0, #8]
1050 2 2 0 push {r4, r5, r6, lr}
1060 4 1 0 stmdb sp!, {r4, r5, r6, r7, r8}
1070 2 1 0 ldm r0!, {r1, r2}
1080 4 1 0 sdiv r0, r1, r2
1090 4 1 0 udiv r0, r1, r2
10a0 2 5 0 adds r0, r1, r0
10b0 4 2 0 vmla.f32 s0, s1, s2
10c0 2 4 3 bls #0x1000
10d0 2 1 1 pop {r4, r5, r6, pc}
10e0 2 1 0 push {r8, sb, sl, fp, ip, lr}
10f0 4 1 1 pop.w {r4-r7, pc}
1100 4 1 0 vpush {d8, d9, d10, d11}
1110 4 1 0 vpop {s16-s31}
1120 4 1 0 vldmia r0!, {s0, s1, s2, s3}
1130 4 1 0 vstmdb sp!, {d8-d15}
1140 4 1 0 ldmdb r0, {r1, r2, r3}
1150 2 1 0 popne {r4, lr}
1160 4 1 1 ldm.w sp!, {r4, r5, r6, r7, r8, sb, sl, fp, pc}
2000 2 10 0 ldr r0, [r1]
2002 2 10 0 ldr r1, [r2]
2004 4 10 0 str r1, [r3, #20]
2008 2 10 0 ldr r2, [r3]
200a 2 10 0 ldr r3, [r2]
200c 2 10 4 adds r4, r3, #1
200e 2 6 0 ldr r5, [r4]
2010 4 6 0 vadd.f32 s0, s1, s2
2014 4 6 0 vmul.f32 s3, s0, s4
2018 4 6 0 vmla.f32 s3, s5, s6
201c 4 6 0 vstr s3, [r0]
2020 4 6 0 vldr d1, [r0, #8]
2024 4 6 0 vdiv.f32 s4, s5, s6
2028 4 6 0 vmov r0, s4
202c 4 6 0 vmov.f32 s7, #1.000000e+00
2030 4 6 0 vsqrt.f32 s8, s7
2034 4 6 0 vmov r1, r2, d4
2038 4 6 0 vcvt.s32.f32 s10, s11
203c 4 6 0 vmov.32 r3, d5[0]
2040 4 6 0 vnmls.f32 s0, s1, s2
2044 4 6 6 vmlsne.f32 s0, s1, s2
2050 4 1 0 umlal r0, r1, r2, r3
2054 2 1 0 ldr r2, [r1]
2056 2 1 1 bx lr
`

const cycleSampleTotal = 642

func checkCycleModel() error {
	var got int64
	if err := scanCounts(strings.NewReader(cycleSample), func(_ uint64, v int64) { got += v }); err != nil {
		return fmt.Errorf("cycle model sample: %w", err)
	}
	if got != cycleSampleTotal {
		return fmt.Errorf("cycle model sample: %d cycles, want %d", got, cycleSampleTotal)
	}
	return nil
}
