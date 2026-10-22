package main

import (
	"bytes"
	"os"
	"regexp"
	"slices"
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
	checkWordSplits(t, out)
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
	if m := regexp.MustCompile(`[^.]*\bgoroutines?\b`).FindAll(out.H, 3); m != nil {
		t.Errorf("dmrgo_gen.h comments say goroutine, which C has no word for: %q", m)
	}
	if m := regexp.MustCompile(`DMRGO_ERR_NIL_RECEIVER for a nil \w+`).FindAll(out.H, 3); m != nil {
		t.Errorf("a function comment gives DMRGO_ERR_NIL_RECEIVER for a receiver, which C passes as self: %q", m)
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
	checkGoWords(t, out, comment.FindAll(h, -1))
}

func checkGoWords(t *testing.T, out *capigen.Output, comments [][]byte) {
	t.Helper()
	consts, other := map[string]bool{}, map[string]bool{}
	for _, e := range out.Exports {
		n := e.Go[strings.LastIndex(e.Go, ".")+1:]
		if e.Kind == "const" && e.Err == "" {
			consts[n] = true
		} else {
			other[n] = true
		}
	}
	for n := range other {
		delete(consts, n)
	}
	word := regexp.MustCompile(`\b[A-Z]\w*\b`)
	nilRe := regexp.MustCompile(`\b[Nn]il\b`)
	var constBad, nilBad []string
	for _, c := range comments {
		for _, m := range word.FindAllIndex(c, -1) {
			before := m[0] > 0 && c[m[0]-1] == '.'
			after := m[1]+1 < len(c) && c[m[1]] == '.' && (c[m[1]+1] >= 'A' && c[m[1]+1] <= 'Z')
			if consts[string(c[m[0]:m[1]])] && m[0] != 3 && !before && !after {
				constBad = append(constBad, string(c[m[0]:m[1]]))
			}
		}
		if m := nilRe.FindAll(c, -1); m != nil {
			nilBad = append(nilBad, string(c))
		}
	}
	if len(constBad) > 0 {
		t.Errorf("dmrgo_gen.h comments name %d Go constants, such as %q", len(constBad), constBad[:min(len(constBad), 5)])
	}
	if len(nilBad) > 0 {
		t.Errorf("dmrgo_gen.h has %d comments that say nil, which C calls NULL, such as %q", len(nilBad), nilBad[:min(len(nilBad), 3)])
	}
}

func checkWordSplits(t *testing.T, out *capigen.Output) {
	t.Helper()
	single := map[string]bool{"allmsi": true, "msid": true, "tsccas": true}
	vocab := map[string]bool{}
	for _, n := range regexp.MustCompile(`\b(?:dmrgo|DMRGO)_\w+`).FindAll(out.H, -1) {
		for _, w := range strings.Split(strings.ToLower(string(n)), "_") {
			vocab[w] = true
		}
	}
	known := func(w string) bool {
		return vocab[w] || vocab[strings.TrimRight(w, "0123456789")]
	}
	split := func(w string) bool {
		for i := 2; i <= len(w)-2; i++ {
			if b := w[i:]; vocab[w[:i]] && b[0] >= 'a' && b[0] <= 'z' && known(b) {
				return true
			}
		}
		return false
	}
	caps := regexp.MustCompile(`[A-Z][A-Z0-9]{3,}`)
	var bad []string
	for _, e := range out.Exports {
		var runs []string
		for _, r := range caps.FindAllString(e.Go[strings.LastIndex(e.Go, ".")+1:], -1) {
			runs = append(runs, strings.ToLower(r))
		}
		for _, c := range e.C {
			for _, w := range strings.FieldsFunc(strings.ToLower(c), func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') }) {
				if len(w) >= 4 && !single[w] && slices.ContainsFunc(runs, func(r string) bool { return strings.Contains(r, w) }) && split(w) {
					bad = append(bad, c)
					break
				}
			}
		}
	}
	slices.Sort(bad)
	bad = slices.Compact(bad)
	if len(bad) > 0 {
		t.Errorf("dmrgo_gen.h has %d names with two words run together, such as %q", len(bad), bad[:min(len(bad), 20)])
	}
}
