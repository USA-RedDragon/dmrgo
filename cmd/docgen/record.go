package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type recording struct {
	run    *stateRun
	im     *imports
	byType map[string][]*machine
	order  []string
	runRe  []string
	edits  map[string][]edit
	hook   strings.Builder
}

func (g *gen) recordStates(ip string, want map[string]string) (*stateRun, error) {
	pi := g.loaded[ip]
	r, err := resolveAll(pi, want)
	if err != nil {
		return nil, err
	}
	var body strings.Builder
	for _, typ := range r.order {
		body.WriteString(r.instrument(pi, typ))
	}
	fmt.Fprintf(&r.hook, "package %s\n\nimport (\n\t\"os\"\n\t\"strconv\"\n\t\"sync\"\n", pi.pkg.Name)
	paths := make([]string, 0, len(r.im.alias))
	for p := range r.im.alias {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&r.hook, "\t%s %q\n", r.im.alias[p], p)
	}
	r.hook.WriteString(")\n\n" + hookCommon + body.String())
	work := filepath.Join(g.root, "artifacts", "docgen", strings.ReplaceAll(shortPath(ip), "/", "-"))
	ovPath, err := r.writeOverlay(work, filepath.Dir(pi.pkg.GoFiles[0]))
	if err != nil {
		return nil, err
	}
	rec := filepath.Join(work, "states.txt")
	if err := g.runRecorded(ip, ovPath, r.runRe, rec); err != nil {
		return nil, err
	}
	return r.run, readRecord(rec, r.run)
}

func resolveAll(pi *pkgInfo, want map[string]string) (*recording, error) {
	r := &recording{
		run:    &stateRun{machines: map[string]*machine{}},
		im:     &imports{here: pi.pkg.Types, alias: map[string]string{}},
		byType: map[string][]*machine{},
		edits:  map[string][]edit{},
	}
	names := make([]string, 0, len(want))
	seen := map[string]bool{}
	for name, re := range want {
		names = append(names, name)
		if re != "" && !seen[re] {
			seen[re] = true
			r.runRe = append(r.runRe, re)
		}
	}
	sort.Strings(names)
	sort.Strings(r.runRe)
	for _, name := range names {
		m, err := resolveView(pi.pkg.Types, name, r.im)
		if err != nil {
			return nil, err
		}
		if n, ok := pi.pkg.Types.Scope().Lookup(m.typ).Type().(*types.Named); ok && n.TypeParams().Len() > 0 {
			return nil, fmt.Errorf("%s is generic", m.typ)
		}
		if r.byType[m.typ] == nil {
			r.order = append(r.order, m.typ)
		}
		r.byType[m.typ] = append(r.byType[m.typ], m)
		r.run.machines[name] = m
	}
	return r, nil
}

// instrument adds the edits for the exported methods of typ and returns
// the hook code of typ.
func (r *recording) instrument(pi *pkgInfo, typ string) string {
	views := r.byType[typ]
	observed := map[string]bool{"String": true}
	var comps, viewOf []string
	for _, m := range views {
		if m.accessor != "" {
			observed[m.accessor] = true
		}
		for _, o := range m.observers {
			observed[o.name] = true
		}
		for _, e := range m.stateExprs() {
			comps = append(comps, e)
			viewOf = append(viewOf, strconv.Quote(m.name))
		}
	}
	var methods, skipped []string
	for _, f := range pi.pkg.Syntax {
		file := pi.pkg.Fset.File(f.Pos()).Name()
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || observed[fd.Name.Name] || recvName(fd) != typ {
				continue
			}
			if len(fd.Recv.List[0].Names) == 0 || fd.Recv.List[0].Names[0].Name == "_" {
				skipped = append(skipped, fd.Name.Name)
				continue
			}
			rv := fd.Recv.List[0].Names[0].Name
			off := pi.pkg.Fset.Position(fd.Body.Lbrace).Offset + 1
			r.edits[file] = append(r.edits[file], edit{off, fmt.Sprintf(" if __dgIn%s(%s) { defer __dgOut%s(%s, %q) };", typ, rv, typ, rv, fd.Name.Name)})
			methods = append(methods, fd.Name.Name)
		}
	}
	sort.Strings(methods)
	for _, m := range views {
		m.methods, m.skipped = methods, skipped
	}
	hk := strings.ReplaceAll(hookType, "T_", typ)
	hk = strings.Replace(hk, "VIEWS", strings.Join(viewOf, ", "), 1)
	return strings.Replace(hk, "COMPS", strings.Join(comps, ", "), 1)
}

// recvName returns the receiver type name of an exported method with a
// pointer receiver and a body, and "" for any other func.
func recvName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || fd.Body == nil || !fd.Name.IsExported() {
		return ""
	}
	star, ok := fd.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return ""
	}
	id, ok := star.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return id.Name
}

func (r *recording) writeOverlay(work, pkgDir string) (string, error) {
	if err := os.RemoveAll(work); err != nil {
		return "", err
	}
	if err := os.MkdirAll(work, 0o750); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(work)
	if err != nil {
		return "", err
	}
	defer root.Close()
	write := func(name string, data []byte) error {
		f, err := root.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		if _, err := f.Write(data); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	}
	ov := map[string]string{}
	files := make([]string, 0, len(r.edits))
	for file := range r.edits {
		files = append(files, file)
	}
	sort.Strings(files)
	for n, file := range files {
		es := r.edits[file]
		src, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		sort.Slice(es, func(i, j int) bool { return es[i].off > es[j].off })
		for _, e := range es {
			src = append(src[:e.off:e.off], append([]byte(e.text), src[e.off:]...)...)
		}
		name := fmt.Sprintf("%d.go", n+1)
		if err := write(name, src); err != nil {
			return "", err
		}
		ov[file] = filepath.Join(work, name)
	}
	if err := write(hookFile, []byte(r.hook.String())); err != nil {
		return "", err
	}
	ov[filepath.Join(pkgDir, hookFile)] = filepath.Join(work, hookFile)
	ovJSON, err := json.Marshal(map[string]map[string]string{"Replace": ov})
	if err != nil {
		return "", err
	}
	return filepath.Join(work, "overlay.json"), write("overlay.json", ovJSON)
}

func (g *gen) runRecorded(ip, ovPath string, runRe []string, rec string) error {
	args := []string{"test", "-count=1", "-overlay", ovPath}
	if len(runRe) > 0 {
		args = append(args, "-run", strings.Join(runRe, "|"))
	}
	args = append(args, ip)
	fmt.Fprintf(g.log, "docgen: recording the states of %s\n", shortPath(ip))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	out, errOut, err := g.goCmd(ctx, g.v2, offlineEnv("DOCGEN_STATES_OUT="+rec), args...)
	if err == nil {
		return nil
	}
	var fails []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "--- FAIL") {
			fails = append(fails, strings.TrimSpace(l))
		}
	}
	if len(fails) == 0 {
		return fmt.Errorf("go test: %w\n%s%s", err, out, errOut)
	}
	fmt.Fprintf(g.log, "docgen: %s: tests that fail with the state hook (their transitions up to the failure still count):\n  %s\n", shortPath(ip), strings.Join(fails, "\n  "))
	return nil
}

func readRecord(rec string, run *stateRun) error {
	f, err := os.Open(rec)
	if err != nil {
		return fmt.Errorf("the tests recorded no state: %w", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Split(sc.Text(), "\t")
		if len(fs) < 3 {
			continue
		}
		m := run.machines[fs[1]]
		if m == nil {
			continue
		}
		switch {
		case fs[0] == "init" && len(fs) == 3:
			m.initial[fs[2]] = true
		case fs[0] == "edge" && len(fs) == 5:
			k := [2]string{fs[2], fs[4]}
			if m.edges[k] == nil {
				m.edges[k] = map[string]bool{}
			}
			m.edges[k][fs[3]] = true
		}
	}
	return sc.Err()
}
