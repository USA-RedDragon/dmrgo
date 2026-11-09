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
	reLoadMulti  = regexp.MustCompile(`^(ldm|ldmia|ldmdb|pop)(ia|db)?$`)
	reStoreMulti = regexp.MustCompile(`^(stm|stmia|stmdb|push)(ia|db)?$`)
	reRegs       = regexp.MustCompile(`\b(r\d+|lr|sb|sl|fp|ip)\b`)
	reCond       = regexp.MustCompile(`(eq|ne|cs|hs|cc|lo|mi|pl|vs|vc|hi|ls|ge|lt|gt|le|al)?(\.w|\.n)?$`)
)

func cyclesOf(mn, ops string) int64 {
	m, _, _ := strings.Cut(mn, ".")
	switch {
	case reLoadMulti.MatchString(m):
		n := 1 + strings.Count(ops, "r") - strings.Count(ops, "pc")
		if strings.Contains(ops, "pc") {
			n++
		}
		return int64(n)
	case reStoreMulti.MatchString(m):
		return int64(1 + len(reRegs.FindAllString(ops, -1)))
	case m == "ldrd" || m == "strd":
		return 3
	case strings.HasPrefix(m, "ldr"):
		return 2
	case m == "sdiv" || m == "udiv":
		return 12
	}
	return 1
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
		add(a, n*cyclesOf(mn, ops)+2*taken)
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
`

const cycleSampleTotal = 85

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
