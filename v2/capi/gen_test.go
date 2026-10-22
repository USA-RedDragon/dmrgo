package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"
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
	goLead := regexp.MustCompile(`/\* Go: [\w.]+\.`)
	if m := regexp.MustCompile(`\b[a-z][a-z0-9]*\.[A-Z]\w*`).FindAll(goLead.ReplaceAll(out.H, []byte("/*")), 5); m != nil {
		t.Errorf("dmrgo_gen.h comments name Go identifiers: %q", m)
	}
	checkDocRefs(t, out)
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

func checkDocRefs(t *testing.T, out *capigen.Output) {
	t.Helper()
	comment := regexp.MustCompile(`(?s)/\*.*?\*/`)
	h := regexp.MustCompile(`/\* Go: [\w.]+\.`).ReplaceAll(out.H, []byte("/*"))
	hand, err := os.ReadFile("dmrgo.h")
	if err != nil {
		t.Fatal(err)
	}
	code := comment.ReplaceAll(append(hand, out.H...), nil)
	members := map[string]map[string]string{}
	member := regexp.MustCompile(`(?m)^\t(?:const )?(\w+)[ *]+(\w+)(?:\[[^\]]*\])*;$`)
	for _, m := range regexp.MustCompile(`(?ms)^struct (\w+) \{\n(.*?)^\};`).FindAllSubmatch(code, -1) {
		ms := map[string]string{}
		for _, d := range member.FindAllSubmatch(m[2], -1) {
			ms[string(d[2])] = string(d[1])
		}
		members[string(m[1])] = ms
	}
	names := map[string]bool{}
	for _, n := range regexp.MustCompile(`\b(?:dmrgo|DMRGO)_\w+`).FindAll(code, -1) {
		names[string(n)] = true
	}
	goNames := map[string]bool{}
	for _, e := range out.Exports {
		if _, n, ok := strings.Cut(e.Go, "."); ok && strings.Contains(n, ".") {
			goNames[n] = true
		}
	}
	goSel := regexp.MustCompile(`(?:^|[^\w.])([A-Z]\w*\.[A-Z]\w*)`)
	cRef := regexp.MustCompile(`\b(?:dmrgo|DMRGO)_\w+(?:\.\w+)*`)
	var goBad, cBad []string
	for _, c := range comment.FindAll(h, -1) {
		for _, m := range goSel.FindAllSubmatch(c, -1) {
			if goNames[string(m[1])] {
				goBad = append(goBad, string(m[1]))
			}
		}
		for _, r := range cRef.FindAll(c, -1) {
			path := strings.Split(string(r), ".")
			ok, typ := names[path[0]], path[0]
			for _, sel := range path[1:] {
				typ, ok = members[typ][sel]
				if !ok {
					break
				}
			}
			if !ok {
				cBad = append(cBad, string(r))
			}
		}
	}
	if len(cBad) > 0 {
		t.Errorf("dmrgo_gen.h comments name %d C names that it does not declare, such as %q", len(cBad), cBad[:min(len(cBad), 5)])
	}
	if len(goBad) > 0 {
		t.Errorf("dmrgo_gen.h comments name %d Go exports, such as %q", len(goBad), goBad[:min(len(goBad), 5)])
	}
}
