//go:build !tinygo

package allocgate_test

import (
	"bytes"
	"debug/elf"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const ramTableLimit = 1024

const mcuCorrSym = "github.com/USA-RedDragon/dmrgo/v2/internal/fsktx.mcuCorr"

var heaplessExamples = []string{"txaudio", "txiq"}

var radioTypes = map[string]bool{"Transmitter": true, "AudioTransmitter": true, "Receiver": true, "AudioReceiver": true}

func TestTinyGoTablesInFlash(t *testing.T) {
	t.Parallel()

	if os.Getenv("DMRGO_TINYGO_ALLOCS") == "" {
		t.Skip("DMRGO_TINYGO_ALLOCS is not set")
	}
	tinygo, err := exec.LookPath("tinygo")
	if err != nil {
		t.Fatal("DMRGO_TINYGO_ALLOCS is set but there is no tinygo command")
	}
	dir := copyModule(t)
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
		radios, err := radioVars(filepath.Join(dir, "examples", ex))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range ramTablesOf(syms.ram, board, radios) {
			t.Errorf("%s: %s takes %d bytes of RAM", ex, name, syms.ram[name])
		}
		if ex == "txaudio" && syms.flash[mcuCorrSym] < ramTableLimit && syms.ram[mcuCorrSym] == 0 {
			t.Errorf("%s: %s is not in flash", ex, mcuCorrSym)
		}
	}
}

// ramTablesOf returns the RAM symbols of a build that the board stub does not
// have, less the radio objects of the example. A radio object is mutable
// state, so only a table can be a fault.
func ramTablesOf(ram, board map[string]uint64, radios map[string]bool) []string {
	var out []string
	for name, size := range ram {
		if board[name] != size && !radios[name] {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// radioVars returns the symbols of the package-level variables of the main
// package in dir whose type is an rf transmitter or receiver.
func radioVars(dir string) (map[string]bool, error) {
	pkgs, err := parser.ParseDir(token.NewFileSet(), dir, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, f := range pkgs["main"].Files {
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.VAR {
				continue
			}
			for _, sp := range g.Specs {
				v, ok := sp.(*ast.ValueSpec)
				if !ok || !isRadioType(v.Type) {
					continue
				}
				for _, n := range v.Names {
					out["main."+n.Name] = true
				}
			}
		}
	}
	return out, nil
}

func isRadioType(e ast.Expr) bool {
	if st, ok := e.(*ast.StarExpr); ok {
		e = st.X
	}
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "rf" && radioTypes[sel.Sel.Name]
}

func TestRAMTablesOfKeepsTables(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := "package main\n\nimport \"github.com/USA-RedDragon/dmrgo/v2/rf\"\n\nvar (\n\ttx rf.Transmitter\n\trx *rf.AudioReceiver\n\tlut [2048]int16\n)\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	radios, err := radioVars(dir)
	if err != nil {
		t.Fatal(err)
	}
	ram := map[string]uint64{"main.tx": 1040, "main.rx": 4096, "main.lut": 4096, "runtime.stack": 2048}
	board := map[string]uint64{"runtime.stack": 2048}
	if got := ramTablesOf(ram, board, radios); !slices.Equal(got, []string{"main.lut"}) {
		t.Errorf("RAM tables %q, want [main.lut]", got)
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
