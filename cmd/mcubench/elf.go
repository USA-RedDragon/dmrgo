package main

import (
	"debug/elf"
	"fmt"
	"os"
	"sort"
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

type cycleCount struct{ total, matched int64 }

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
