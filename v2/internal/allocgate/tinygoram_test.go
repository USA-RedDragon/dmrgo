//go:build !tinygo

package allocgate_test

import (
	"bytes"
	"debug/elf"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const ramTableLimit = 1024

const mcuCorrSym = "github.com/USA-RedDragon/dmrgo/v2/internal/fsktx.mcuCorr"

var tableKnobs = map[string][][2]string{
	"internal/fsktx/mcu.go": {
		{"\ttier0Exact = false\n", "\ttier0Exact = true\n"},
		{"\ttier0Corr = false\n", "\ttier0Corr = true\n"},
		{"\ttier0Corr24 = false\n", "\ttier0Corr24 = true\n"},
	},
}

var heaplessExamples = []string{"txaudio", "txiq"}

func TestTinyGoTablesInFlash(t *testing.T) {
	t.Parallel()

	if os.Getenv("DMRGO_TINYGO_ALLOCS") == "" {
		t.Skip("DMRGO_TINYGO_ALLOCS is not set")
	}
	tinygo, err := exec.LookPath("tinygo")
	if err != nil {
		t.Fatal("DMRGO_TINYGO_ALLOCS is set but there is no tinygo command")
	}
	gotool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := copyModule(t)
	for f, edits := range tableKnobs {
		p := filepath.Join(dir, f)
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		for _, e := range edits {
			if strings.Count(s, e[0]) != 1 {
				t.Fatalf("%s: %q does not occur once", f, e[0])
			}
			s = strings.Replace(s, e[0], e[1], 1)
		}
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(t, dir, gotool, "test", "-count=1", "-run", "^TestTablesGenerated$", "./internal/fsktx", "-update")
	stub := filepath.Join(dir, "examples", "ramstub")
	if err := os.MkdirAll(stub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stub, "main.go"), []byte("package main\n\nfunc main() { println(\"x\") }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	build := func(ex string) tableSyms {
		out := filepath.Join(t.TempDir(), ex+".elf")
		run(t, dir, tinygo, "build", "-target=feather-m4", "-serial=uart", "-gc=none", "-scheduler=none", "-o", out, "./examples/"+ex)
		return ramTables(t, out)
	}
	board := build("ramstub").ram
	for _, ex := range heaplessExamples {
		syms := build(ex)
		for name, size := range syms.ram {
			if board[name] != size {
				t.Errorf("%s: %s takes %d bytes of RAM", ex, name, size)
			}
		}
		if ex == "txaudio" && syms.flash[mcuCorrSym] < ramTableLimit && syms.ram[mcuCorrSym] == 0 {
			t.Errorf("%s: the knobs no longer link %s", ex, mcuCorrSym)
		}
	}
}

type tableSyms struct {
	ram, flash map[string]uint64
}

func ramTables(t *testing.T, path string) tableSyms {
	t.Helper()
	f, err := elf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	syms, err := f.Symbols()
	if err != nil {
		t.Fatal(err)
	}
	ts := tableSyms{map[string]uint64{}, map[string]uint64{}}
	for _, s := range syms {
		if s.Size < ramTableLimit || s.Section < elf.SHN_UNDEF+1 || int(s.Section) >= len(f.Sections) {
			continue
		}
		sec := f.Sections[s.Section]
		if sec.Flags&elf.SHF_ALLOC == 0 {
			continue
		}
		if sec.Flags&elf.SHF_WRITE != 0 {
			ts.ram[ramKey(s)] = s.Size
		} else {
			ts.flash[s.Name] = s.Size
		}
	}
	return ts
}

func ramKey(s elf.Symbol) string {
	if strings.HasPrefix(s.Name, "runtime$alloc.") {
		return "runtime$alloc of " + strconv.FormatUint(s.Size, 10) + " bytes"
	}
	return s.Name
}

func copyModule(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(moduleRoot)
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if d.Name() == "testdata" && rel != filepath.Join("internal", "fsktx", "testdata") {
				return fs.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !strings.HasSuffix(path, ".go") && !strings.HasPrefix(d.Name(), "go.") && !strings.HasPrefix(rel, filepath.Join("internal", "fsktx", "testdata")) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func run(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), name, args...)
	cmd.Dir = dir
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s: %v\n%s", filepath.Base(name), strings.Join(args, " "), err, buf.String())
	}
}
