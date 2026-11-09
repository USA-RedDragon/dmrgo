package main

import (
	"bufio"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type image struct {
	flash, ram, mainData uint64
	funcs                []funcSym
	sizes                map[string]uint64
}

type funcSym struct {
	lo, hi uint64
	name   string
}

func readELF(path string) (*image, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img := &image{sizes: map[string]uint64{}}
	for _, s := range f.Sections {
		switch s.Name {
		case ".text", ".rodata":
			img.flash += s.Size
		case ".data":
			img.flash += s.Size
			img.ram += s.Size
		case ".bss":
			img.ram += s.Size
		}
	}
	syms, err := f.Symbols()
	if err != nil {
		return nil, err
	}
	for _, s := range syms {
		img.sizes[s.Name] = s.Size
		switch elf.ST_TYPE(s.Info) {
		case elf.STT_FUNC:
			lo := s.Value &^ 1
			img.funcs = append(img.funcs, funcSym{lo, lo + s.Size, s.Name})
		case elf.STT_OBJECT:
			if strings.HasPrefix(s.Name, "main.") && s.Name != "main.rx" && s.Name != "main.pe" && s.Name != "main.mod" {
				img.mainData += s.Size
			}
		default:
		}
	}
	sort.Slice(img.funcs, func(i, j int) bool { return img.funcs[i].lo < img.funcs[j].lo })
	return img, nil
}

func (img *image) symSize(name string) uint64 { return img.sizes[name] }

func (img *image) owner(a uint64) string {
	i := sort.Search(len(img.funcs), func(i int) bool { return img.funcs[i].lo > a }) - 1
	if i >= 0 && a < img.funcs[i].hi {
		return img.funcs[i].name
	}
	return ""
}

var (
	reMulti = regexp.MustCompile(`^(ldm|stm)(ia|db|ea|fd)?$|^push$|^pop$|^v(ldm|stm)(ia|db)?$|^vpush$|^vpop$`)
	reReg   = regexp.MustCompile(`^([rsd])(\d+)$`)
	reCond  = regexp.MustCompile(`(eq|ne|cs|hs|cc|lo|mi|pl|vs|vc|hi|ls|ge|lt|gt|le|al)?(\.w|\.n)?$`)
)

var coreRegAlias = map[string]int{"sb": 9, "sl": 10, "fp": 11, "ip": 12, "sp": 13, "lr": 14, "pc": 15}

// cyclesOf returns the Cortex-M4 cycles of one instruction with mnemonic mn
// (condition and width suffixes removed) and operands ops, at zero wait
// states, by ARM DDI 0439C (Cortex-M4 TRM r0p1) Table 3-1 and Table 7-1.
// LDM, STM, PUSH and POP take 1+N, where N counts every register of the
// list, PC and LR included. VLDM, VSTM, VPUSH and VPOP take 1+N for single
// registers and 1+2*N for double registers. LDRD and STRD take 3 (1+N),
// other loads 2, SDIV and UDIV the worst case 12, and every other
// instruction 1. The pipeline refill P of a branch or of a load of PC is
// not part of the result: the caller adds 2 cycles for each time the next
// executed instruction is not the next in memory.
func cyclesOf(mn, ops string) (int64, error) {
	m, _, _ := strings.Cut(mn, ".")
	switch {
	case reMulti.MatchString(m):
		n, err := regListCycles(ops)
		if err != nil {
			return 0, fmt.Errorf("%s %s: %w", mn, ops, err)
		}
		return 1 + n, nil
	case m == "ldrd" || m == "strd":
		return 3, nil
	case strings.HasPrefix(m, "ldr"):
		return 2, nil
	case m == "sdiv" || m == "udiv":
		return 12, nil
	}
	return 1, nil
}

func regListCycles(ops string) (int64, error) {
	_, list, ok := strings.Cut(ops, "{")
	if ok {
		list, _, ok = strings.Cut(list, "}")
	}
	if !ok {
		return 0, errors.New("no register list")
	}
	var n int64
	for _, item := range strings.Split(list, ",") {
		lo, hi, isRange := strings.Cut(strings.TrimSpace(item), "-")
		if !isRange {
			hi = lo
		}
		kl, il, err := parseReg(strings.TrimSpace(lo))
		if err != nil {
			return 0, err
		}
		kh, ih, err := parseReg(strings.TrimSpace(hi))
		if err != nil {
			return 0, err
		}
		if kl != kh || ih < il {
			return 0, fmt.Errorf("bad register range %q", item)
		}
		w := int64(1)
		if kl == 'd' {
			w = 2
		}
		n += w * int64(ih-il+1)
	}
	return n, nil
}

func parseReg(r string) (byte, int, error) {
	if i, ok := coreRegAlias[r]; ok {
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

type cycleCount struct{ total, matched int64 }

var reMnemonic = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z0-9]+)*$`)

func (img *image) cycles(counts string, match func(string) bool) (cycleCount, error) {
	f, err := os.Open(counts)
	if err != nil {
		return cycleCount{}, err
	}
	defer func() { _ = f.Close() }()
	var c cycleCount
	err = scanCounts(f, func(a uint64, v int64) {
		c.total += v
		if match(img.owner(a)) {
			c.matched += v
		}
	})
	if err != nil {
		return c, fmt.Errorf("%s: %w", counts, err)
	}
	return c, nil
}

func scanCounts(r io.Reader, add func(addr uint64, cycles int64)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	var lines, undecoded int64
	var firstBad string
	for sc.Scan() {
		fs := strings.SplitN(sc.Text(), " ", 5)
		if len(fs) < 4 {
			return fmt.Errorf("%q", sc.Text())
		}
		a, err1 := strconv.ParseUint(fs[0], 16, 64)
		n, err2 := strconv.ParseInt(fs[2], 10, 64)
		taken, err3 := strconv.ParseInt(fs[3], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return fmt.Errorf("%q", sc.Text())
		}
		lines++
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
			continue
		}
		switch mn {
		case "bls", "bhs", "bvs", "bcs":
			mn = "b"
		default:
			mn = reCond.ReplaceAllString(mn, "")
		}
		v, err := cyclesOf(mn, ops)
		if err != nil {
			return fmt.Errorf("%q: %w", sc.Text(), err)
		}
		add(a, n*v+2*taken)
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if lines == 0 {
		return errors.New("the plugin counted no instructions")
	}
	if undecoded > 0 {
		return fmt.Errorf("%d of %d instructions have no ARM mnemonic, first %q: QEMU must be built with capstone (--enable-capstone) for the cycle model", undecoded, lines, firstBad)
	}
	return nil
}

const cycleSample = `1000 2 3 0 ldr r0, [r1, #4]
1002 4 2 0 ldr.w r2, [r3, r4, lsl #2]
1006 2 1 0 ldrne r0, [r1]
100a 4 1 0 ldrd r2, r3, [r0]
100e 4 1 0 strd r2, r3, [r0, #8]
1012 2 2 0 push {r4, r5, r6, lr}
1014 4 1 0 stmdb sp!, {r4, r5, r6, r7, r8}
1018 2 1 0 ldm r0!, {r1, r2}
101a 4 1 0 sdiv r0, r1, r2
101e 4 1 0 udiv r0, r1, r2
1022 2 5 0 adds r0, r0, r1
1024 4 2 0 vmla.f32 s0, s1, s2
1028 2 4 3 bls #0x1000
102a 2 1 1 pop {r4, r5, r6, pc}
102c 2 1 0 push {r8, sb, sl, fp, ip, lr}
102e 4 1 1 pop.w {r4-r7, pc}
1032 4 1 0 vpush {d8, d9, d10, d11}
1036 4 1 0 vpop {s16-s31}
103a 4 1 0 vldmia r0!, {s0, s1, s2, s3}
103e 4 1 0 vstmdb sp!, {d8-d15}
1042 4 1 0 ldmdb r0, {r1, r2, r3}
1046 2 1 0 popne {r4, lr}
1048 4 1 1 ldm.w sp!, {r4, r5, r6, r7, r8, sb, sl, fp, pc}
`

const cycleSampleTotal = 167

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
