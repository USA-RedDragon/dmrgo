package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFront(t *testing.T) {
	t.Parallel()
	xs := []float64{1, 2, 3, 4, 2}
	ys := []float64{5, 4, 4.5, 1, 6}
	if got, want := front(xs, ys), []int{0, 1, 3}; !slices.Equal(got, want) {
		t.Fatalf("front = %v, want %v", got, want)
	}
}

func TestAt90(t *testing.T) {
	t.Parallel()
	x := []float64{0, 1, 2}
	if c := at90(x, []float64{0.5, 0.8, 1}); !c.OK || c.Below || c.X != 1.5 {
		t.Fatalf("at90 = %+v, want 1.5", c)
	}
	if c := at90(x, []float64{0.95, 1, 1}); !c.OK || !c.Below {
		t.Fatalf("at90 = %+v, want below", c)
	}
	if c := at90(x, []float64{0.1, 0.2, 0.3}); c.OK {
		t.Fatalf("at90 = %+v, want none", c)
	}
}

func TestExpand(t *testing.T) {
	t.Parallel()
	f := family{Name: "f", Base: variant{Tier: 1}, Mode: "product", Vary: map[string][]json.RawMessage{
		"opt.Window":                  {json.RawMessage("1"), json.RawMessage("2")},
		"consts.leanfec.go.leanChase": {json.RawMessage(`"4"`)},
		"hz":                          {json.RawMessage("24000"), json.RawMessage("48000")},
	}}
	vs, err := f.expand()
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 4 {
		t.Fatalf("product gave %d variants, want 4", len(vs))
	}
	if vs[0].Name != "f-leanChase=4-hz=24000-Window=1" || vs[0].Consts["leanfec.go"]["leanChase"] != "4" || string(vs[0].Opt["Window"]) != "1" {
		t.Fatalf("first variant = %+v", vs[0])
	}
	f.Mode = "each"
	vs, err = f.expand()
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 6 || vs[0].Name != "f-base" {
		t.Fatalf("each gave %d variants, first %q", len(vs), vs[0].Name)
	}
}

func TestApply(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "rf"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package rf\n\nconst (\n\t// a is a.\n\tleanChase = 6\n\tleanTop int = 3\n)\n\nfunc f() int { return leanChase }\n"
	path := filepath.Join(dir, "rf", "x.go")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	v := variant{Consts: map[string]map[string]string{"x.go": {"leanChase": "2", "leanTop": "1"}}, Edits: map[string][][2]string{"x.go": {{"return leanChase", "return leanChase + 1"}}}}
	if err := v.apply(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	want := "package rf\n\nconst (\n\t// a is a.\n\tleanChase = 2\n\tleanTop int = 1\n)\n\nfunc f() int { return leanChase + 1 }\n"
	if string(b) != want {
		t.Fatalf("patched source:\n%s\nwant:\n%s", b, want)
	}
	bad := variant{Consts: map[string]map[string]string{"x.go": {"missing": "1"}}}
	if err := bad.apply(dir); err == nil {
		t.Fatal("apply of a missing constant did not fail")
	}
}
