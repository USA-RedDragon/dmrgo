//go:build !tinygo

package allocgate_test

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/allocgate/apigen"
)

// tinyGoTargets are the targets of the TinyGo check: the pico, an ARM
// Cortex-M0+ board, at its default stack size, and riscv-qemu, a bare-metal
// RISC-V machine. TinyGo puts a local larger than its stack limit on the
// heap, so it finds allocations that gc does not.
var tinyGoTargets = []string{"pico", "riscv-qemu"}

// tinyGoSiteRE matches a heap allocation site that tinygo -print-allocs
// reports, in the form file:line.col,line.col followed by counts.
var tinyGoSiteRE = regexp.MustCompile(`^(.+\.go):(\d+)\.\d+`)

// TestAllAPIUpToDate fails when allapi_gen.go is not what the generator
// writes for the current library.
func TestAllAPIUpToDate(t *testing.T) {
	t.Parallel()

	want, err := apigen.Generate(moduleRoot)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(moduleRoot, apigen.Out))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("allapi/allapi_gen.go is out of date; run go generate ./... in the root module")
	}
}

// TestTinyGoHeapAllocations builds the allapi program, which references
// every exported function and method of the library, with tinygo build
// -print-allocs for each target of tinyGoTargets. It adds each allocation
// site that TinyGo reports in the library to the call graph of the gc
// machine code, as an allocator that the functions with code at the site
// call, and then applies the rules of TestHeapAllocations. It runs only with
// DMRGO_TINYGO_ALLOCS set, as the TinyGo CI job sets it, and then fails when
// tinygo is missing or cannot build the program, so that a toolchain change
// cannot turn the check off.
func TestTinyGoHeapAllocations(t *testing.T) {
	t.Parallel()

	if os.Getenv("DMRGO_TINYGO_ALLOCS") == "" {
		t.Skip("DMRGO_TINYGO_ALLOCS is not set")
	}
	tinygo, err := exec.LookPath("tinygo")
	if err != nil {
		t.Fatal("DMRGO_TINYGO_ALLOCS is set but there is no tinygo command")
	}
	root, err := filepath.Abs(moduleRoot)
	if err != nil {
		t.Fatal(err)
	}
	// TinyGo builds the files of a build without cgo, so the gc build of
	// the gate does too, and every site that TinyGo reports is in it.
	gt := newGate(t, libraryPackages(t), "CGO_ENABLED=0")
	sites := 0
	for _, target := range tinyGoTargets {
		out := filepath.Join(t.TempDir(), target+".elf")
		cmd := exec.CommandContext(t.Context(), tinygo, "build", "-target="+target,
			`-print-allocs=github\.com/USA-RedDragon/dmrgo/v2/`, "-o", out, "./internal/allocgate/allapi")
		cmd.Dir = root
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		if err := cmd.Run(); err != nil {
			t.Fatalf("tinygo build -target=%s: %v\n%s", target, err, buf.String())
		}
		if _, err := os.Stat(out); err != nil {
			t.Fatalf("tinygo build -target=%s wrote no binary: %v", target, err)
		}
		sc := bufio.NewScanner(&buf)
		for sc.Scan() {
			m := tinyGoSiteRE.FindStringSubmatch(sc.Text())
			if m == nil {
				continue
			}
			line, _ := strconv.Atoi(m[2])
			switch {
			case gt.addSite(target, m[1], line):
				sites++
			case strings.HasPrefix(m[1], root+string(filepath.Separator)) && !gt.outsideLibrary(m[1]):
				t.Errorf("tinygo build -target=%s reports an allocation at %s:%d, which no function of the gc build declares", target, m[1], line)
			}
		}
	}
	if sites == 0 {
		t.Fatal("tinygo reports no allocation site in the library: the output format may have changed")
	}
	t.Logf("%d TinyGo allocation sites in the library", sites)
	gt.paths()
	gt.check(t)
}

// addSite adds the TinyGo allocation site at line of file to the call graph,
// as an allocator that the function that declares the line calls, and every
// library function with machine code at the site, which the compiler
// inlined there. It reports whether the site is in the library.
func (gt *gate) addSite(target, file string, line int) bool {
	var decl *declFunc
	for _, d := range gt.p.declaredFuncs() {
		start, end := gt.p.fset.Position(d.decl.Pos()), gt.p.fset.Position(d.decl.End())
		if start.Filename == file && start.Line <= line && line <= end.Line {
			decl = &d
			break
		}
	}
	if decl == nil {
		return false
	}
	site := "tinygo " + target + " allocation at " + file + ":" + strconv.Itoa(line)
	gt.g.sinks[site] = true
	pos := position{filepath.Base(file), line}
	shared := 0
	for _, l := range gt.p.order {
		for _, f := range l.files {
			if filepath.Base(gt.p.fset.Position(f.Pos()).Filename) == pos.file {
				shared++
			}
		}
	}
	for name, bs := range gt.g.bodies {
		pkg := symPackage(name)
		// objdump gives only the base name of a file, so when two library
		// files share it, only code of the package of the site can be at
		// the site. Code that another package inlined from the site is
		// then missed, and the function that declares the site stands for
		// it below.
		if !gt.isLib(pkg) || shared > 1 && pkg != decl.lib.path {
			continue
		}
		for _, b := range bs {
			for _, in := range b.insts {
				if in.pos == pos {
					gt.g.addEdge(name, site)
				}
			}
		}
	}
	if decl.sym != "" {
		gt.g.addEdge(decl.sym, site)
	}
	return true
}

// outsideLibrary reports whether file is in a directory of the module that
// is not library code, such as the allapi program.
func (gt *gate) outsideLibrary(file string) bool {
	root, err := filepath.Abs(moduleRoot)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, filepath.Dir(file))
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return slices.ContainsFunc(skipDirs, func(s string) bool { return rel == s || strings.HasPrefix(rel, s+"/") })
}
