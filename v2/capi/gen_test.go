package main

import (
	"bytes"
	"os"
	"regexp"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/capi/internal/capigen"
)

func TestGenerated(t *testing.T) {
	t.Parallel()

	lock, err := os.ReadFile("abi.txt")
	if err != nil {
		t.Fatal(err)
	}
	out, err := capigen.Generate(".", lock, capigen.Options{})
	if out != nil {
		for _, b := range out.Broken {
			t.Log("changed:", b)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range out.Files() {
		got, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s is out of date; run go generate ./... in the root module", name)
		}
	}
	for _, re := range []*regexp.Regexp{
		regexp.MustCompile(`\b[a-z][a-z0-9_]*__[a-z0-9_]*\b`),
		regexp.MustCompile(`[(,] *(?:const )?[a-z0-9_]+ \**(?:p|out)[0-9]+[,)\[]`),
		regexp.MustCompile(`\b(?:type|user|self|range|func)_\b`),
	} {
		if m := re.FindAll(out.H, 5); m != nil {
			t.Errorf("dmrgo_gen.h has names that %s matches: %q", re, m)
		}
	}
	protoDoc := regexp.MustCompile(`(?m)^/\* Go: .*\*/\nint dmrgo_`)
	goErr := regexp.MustCompile(`\bErr[A-Z]\w*`)
	for _, d := range protoDoc.FindAll(out.H, -1) {
		if m := goErr.Find(d); m != nil {
			t.Errorf("a function comment names the Go error %s: %s", m, d)
		}
		if bytes.Contains(d, []byte("without a copy")) {
			t.Errorf("a function comment says without a copy for a call that copies: %s", d)
		}
	}
	if !bytes.Contains(out.H, []byte(" may be NULL.")) {
		t.Error("no function comment says which pointer may be NULL")
	}
	mapped := 0
	for _, e := range out.Exports {
		if e.Err != "" {
			t.Errorf("%s %s has no C counterpart: %s", e.Kind, e.Go, e.Err)
			continue
		}
		mapped++
	}
	t.Logf("%d of %d exports mapped", mapped, len(out.Exports))
}
