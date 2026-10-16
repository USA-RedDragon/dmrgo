//go:build !tinygo

package allocgate_test

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// tinyGoSiteRE matches a heap allocation site that tinygo -print-allocs
// reports, in the form file:line.col,line.col followed by counts.
var tinyGoSiteRE = regexp.MustCompile(`^(.+\.go):(\d+)\.\d+`)

// TestTinyGoHeapAllocations builds examples/tinygo for the pico at its
// default stack size and fails on each heap allocation in the library that
// the rules of exemptReason do not allow. TinyGo puts a local larger than
// 256 bytes on the heap at that stack size, so it finds allocations that gc
// does not. It runs only with DMRGO_TINYGO_ALLOCS set, as the TinyGo CI job
// sets it, and then fails when tinygo is missing or cannot build the
// example, so that a toolchain change cannot turn the check off.
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
	out := filepath.Join(t.TempDir(), "tinygo.elf")
	cmd := exec.CommandContext(t.Context(), tinygo, "build", "-target=pico",
		`-print-allocs=github\.com/USA-RedDragon/dmrgo/v2/`, "-o", out, "./examples/tinygo")
	cmd.Dir = root
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("tinygo build: %v\n%s", err, buf.String())
	}

	pkgs := map[string]*pkgFuncs{}
	var bad []string
	sc := bufio.NewScanner(&buf)
	for sc.Scan() {
		m := tinyGoSiteRE.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		rel, err := filepath.Rel(root, m[1])
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		rel = filepath.ToSlash(rel)
		dir := filepath.ToSlash(filepath.Dir(rel))
		if slices.ContainsFunc(skipDirs, func(s string) bool { return dir == s || strings.HasPrefix(dir, s+"/") }) {
			continue
		}
		p, ok := pkgs[dir]
		if !ok {
			p = parsePackage(t, dir)
			pkgs[dir] = p
		}
		line, _ := strconv.Atoi(m[2])
		fd := p.enclosing(filepath.Base(rel), line)
		if exemptReason(p, fd) != "" {
			continue
		}
		name := "package level"
		if fd != nil {
			name = funcKey(fd)
		}
		bad = append(bad, fmt.Sprintf("%s:%d: %s", rel, line, name))
	}
	slices.Sort(bad)
	bad = slices.Compact(bad)
	for _, b := range bad {
		t.Error(b)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("tinygo wrote no binary: %v", err)
	}
}
