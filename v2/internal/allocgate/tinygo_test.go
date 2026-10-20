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

var tinyGoTargets = []string{"pico", "riscv-qemu"}

var tinyGoSiteRE = regexp.MustCompile(`^(.+\.go):(\d+)\.\d+`)

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
