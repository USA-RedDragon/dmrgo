package parse

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type constValues struct {
	local   map[string]uint64
	imports map[string]string
	modRoot string
	modPath string
	pkgs    map[string]map[string]uint64
}

func newConstValues(node *ast.File, filePath string, dir bool) *constValues {
	c := &constValues{local: map[string]uint64{}, imports: map[string]string{}, pkgs: map[string]map[string]uint64{}}
	collectConsts(node, c.local)
	for _, imp := range node.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		name := filepath.Base(path)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		c.imports[name] = path
	}
	if !dir {
		return c
	}
	for _, f := range packageFiles(filepath.Dir(filePath), node.Name.Name, filePath) {
		collectConsts(f, c.local)
	}
	c.modRoot, c.modPath = findModule(filepath.Dir(filePath))
	return c
}

func packageFiles(dir, pkg, skip string) []*ast.File {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		if path == skip {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil || f.Name.Name != pkg {
			continue
		}
		files = append(files, f)
	}
	return files
}

func findModule(dir string) (root, path string) {
	for {
		f, err := os.Open(filepath.Join(dir, "go.mod"))
		if err == nil {
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				if rest, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
					path = strings.TrimSpace(rest)
					break
				}
			}
			_ = f.Close()
			return dir, path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}

func collectConsts(node *ast.File, out map[string]uint64) {
	for _, decl := range node.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Values) != len(vs.Names) {
				continue
			}
			for i, n := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.INT {
					continue
				}
				if v, err := strconv.ParseUint(strings.ReplaceAll(lit.Value, "_", ""), 0, 64); err == nil {
					out[n.Name] = v
				}
			}
		}
	}
}

func (c *constValues) value(name string) (uint64, bool) {
	if v, err := strconv.ParseUint(name, 0, 64); err == nil {
		return v, true
	}
	pkg, ident, qualified := strings.Cut(name, ".")
	if !qualified {
		v, ok := c.local[name]
		return v, ok
	}
	consts, ok := c.pkgs[pkg]
	if !ok {
		consts = map[string]uint64{}
		if path, found := c.imports[pkg]; found && c.modPath != "" {
			if rel, inModule := strings.CutPrefix(path, c.modPath); inModule {
				dir := filepath.Join(c.modRoot, filepath.FromSlash(strings.TrimPrefix(rel, "/")))
				for _, f := range packageFiles(dir, filepath.Base(dir), "") {
					collectConsts(f, consts)
				}
			}
		}
		c.pkgs[pkg] = consts
	}
	v, ok := consts[ident]
	return v, ok
}

type condSet struct {
	keys map[string]bool
	ne   string
}

func (c *constValues) key(name string) (string, error) {
	switch name {
	case litTrue:
		return "1", nil
	case litFalse:
		return "0", nil
	}
	v, ok := c.value(name)
	if !ok {
		return "", fmt.Errorf("value %s of a variant condition is not an integer literal or a constant with one", name)
	}
	return strconv.FormatUint(v, 10), nil
}

func (c *constValues) set(cond VariantCond) (condSet, error) {
	s := condSet{keys: map[string]bool{}}
	add := func(name string) error {
		k, err := c.key(name)
		s.keys[k] = true
		return err
	}
	switch {
	case cond.Guard == nil:
		for _, v := range cond.Values {
			if err := add(v); err != nil {
				return s, err
			}
		}
	case cond.Guard.Value == litTrue || cond.Guard.Value == litFalse:
		return s, add(strconv.FormatBool((cond.Guard.Value == litTrue) == (cond.Guard.Op == "==")))
	case cond.Guard.Op == "==":
		return s, add(cond.Guard.Value)
	default:
		k, err := c.key(cond.Guard.Value)
		s.ne = k
		return s, err
	}
	return s, nil
}

func disjoint(a, b condSet) bool {
	switch {
	case a.ne != "" && b.ne != "":
		return false
	case a.ne != "":
		return len(b.keys) == 1 && b.keys[a.ne]
	case b.ne != "":
		return len(a.keys) == 1 && a.keys[b.ne]
	}
	for k := range a.keys {
		if b.keys[k] {
			return false
		}
	}
	return true
}

func checkOverlaps(pdu *PDUStruct, c *constValues) error {
	sets := make([]map[string]condSet, len(pdu.Variants))
	for i, v := range pdu.Variants {
		sets[i] = map[string]condSet{}
		for _, cond := range v.Conds {
			s, err := c.set(cond)
			if err != nil {
				return fmt.Errorf("variant %s: %w", v.Name, err)
			}
			sets[i][cond.Field] = s
		}
	}
	for i, a := range pdu.Variants {
		for j := i + 1; j < len(pdu.Variants); j++ {
			if !setsDisjoint(sets[i], sets[j]) {
				return fmt.Errorf("variants %s and %s can both match the same bits: give them conditions on one field with no common value", a.Name, pdu.Variants[j].Name)
			}
		}
	}
	return nil
}

func setsDisjoint(a, b map[string]condSet) bool {
	for f, sa := range a {
		if sb, ok := b[f]; ok && disjoint(sa, sb) {
			return true
		}
	}
	return false
}
