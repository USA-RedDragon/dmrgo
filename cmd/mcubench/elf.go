package main

import (
	"bufio"
	"debug/elf"
	"fmt"
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

func (img *image) cycles(counts string, match func(string) bool) (cycleCount, error) {
	f, err := os.Open(counts)
	if err != nil {
		return cycleCount{}, err
	}
	defer func() { _ = f.Close() }()
	var c cycleCount
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	for sc.Scan() {
		fs := strings.SplitN(sc.Text(), " ", 5)
		if len(fs) < 4 {
			return c, fmt.Errorf("%s: %q", counts, sc.Text())
		}
		a, err1 := strconv.ParseUint(fs[0], 16, 64)
		n, err2 := strconv.ParseInt(fs[2], 10, 64)
		taken, err3 := strconv.ParseInt(fs[3], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return c, fmt.Errorf("%s: %q", counts, sc.Text())
		}
		var mn, ops string
		if len(fs) > 4 {
			d := strings.Fields(strings.ToLower(fs[4]))
			if len(d) > 0 {
				mn = d[0]
				ops = strings.Join(d[1:], " ")
			}
		}
		switch mn {
		case "bls", "bhs", "bvs", "bcs":
			mn = "b"
		default:
			mn = reCond.ReplaceAllString(mn, "")
		}
		v := n*cyclesOf(mn, ops) + 2*taken
		c.total += v
		if match(img.owner(a)) {
			c.matched += v
		}
	}
	return c, sc.Err()
}
