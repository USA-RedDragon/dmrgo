package main

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/mod/modfile"
)

func (g *gen) fillGoMod(m marker) (string, []outFile, error) {
	p := filepath.Join(g.v2, "go.mod")
	if f := m.args["file"]; f != "" {
		p = filepath.Join(g.root, filepath.FromSlash(f))
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", nil, err
	}
	f, err := modfile.ParseLax(p, b, nil)
	if err != nil {
		return "", nil, err
	}
	var v string
	switch m.args["field"] {
	case "go":
		if f.Go != nil {
			v = f.Go.Version
		}
	case "toolchain":
		if f.Toolchain != nil {
			v = f.Toolchain.Name
		}
	case "module":
		if f.Module != nil {
			v = f.Module.Mod.Path
		}
	default:
		return "", nil, fmt.Errorf("field=%s: want go, toolchain or module", m.args["field"])
	}
	if v == "" {
		return "", nil, fmt.Errorf("%s has no %s line", filepath.Base(filepath.Dir(p))+"/go.mod", m.args["field"])
	}
	return "`" + v + "`", nil, nil
}
