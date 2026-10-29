package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

func run(args []string, stderr io.Writer) int {
	fl := flag.NewFlagSet("docgen", flag.ContinueOnError)
	update := fl.Bool("update", false, "rewrite the body of each marker in docs/**/*.md")
	check := fl.Bool("check", false, "exit 1 and list each marker whose body differs from what docgen writes")
	root := fl.String("root", "", "repository root (default: two directories above the docgen source)")
	docs := fl.String("docs", "docs", "directory of the Markdown pages, relative to the root")
	fl.SetOutput(stderr)
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if *update == *check {
		fmt.Fprintln(stderr, "docgen: give exactly one of -update or -check")
		return 2
	}
	if *root == "" {
		_, this, _, ok := runtime.Caller(0)
		if !ok {
			fmt.Fprintln(stderr, "docgen: cannot find the docgen source directory")
			return 1
		}
		*root = filepath.Join(filepath.Dir(this), "..", "..")
	}
	g := newGen(*root, stderr)
	dir := *docs
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(*root, dir)
	}
	stale, err := g.run(dir, *update)
	if err != nil {
		fmt.Fprintln(stderr, "docgen:", err)
		return 1
	}
	if *check && len(stale) > 0 {
		for _, s := range stale {
			fmt.Fprintln(stderr, "stale:", s)
		}
		fmt.Fprintf(stderr, "docgen: %d stale; run go run -C cmd/docgen . -update and commit the result\n", len(stale))
		return 1
	}
	return 0
}

type page struct {
	path    string
	rel     string
	text    string
	markers []marker
}

type outFile struct {
	path string
	data []byte
}

func (g *gen) run(docsDir string, write bool) ([]string, error) {
	var pages []*page
	root, err := os.OpenRoot(docsDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	err = fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(name, ".md") {
			return nil
		}
		b, err := fs.ReadFile(root.FS(), name)
		if err != nil {
			return err
		}
		p := filepath.Join(docsDir, filepath.FromSlash(name))
		rel, _ := filepath.Rel(g.root, p)
		ms, err := parseMarkers(string(b))
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		if len(ms) > 0 {
			pages = append(pages, &page{path: p, rel: filepath.ToSlash(rel), text: string(b), markers: ms})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, pg := range pages {
		for _, m := range pg.markers {
			g.plan(m)
		}
	}
	if err := g.prepare(); err != nil {
		return nil, err
	}
	var stale []string
	var errs []error
	for _, pg := range pages {
		var b strings.Builder
		last := 0
		for _, m := range pg.markers {
			content, files, err := g.fill(pg, m)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s:%d: %s: %w", pg.rel, m.line, m, err))
				content = strings.Trim(m.body, "\n")
			}
			body := wrapBody(m, content)
			if body != m.body {
				stale = append(stale, fmt.Sprintf("%s:%d: %s", pg.rel, m.line, m))
			}
			for _, f := range files {
				old, _ := os.ReadFile(f.path)
				if !bytes.Equal(old, f.data) {
					rel, _ := filepath.Rel(g.root, f.path)
					stale = append(stale, fmt.Sprintf("%s (from %s:%d: %s)", filepath.ToSlash(rel), pg.rel, m.line, m))
					if write {
						if err := os.MkdirAll(filepath.Dir(f.path), 0o750); err != nil {
							return nil, err
						}
						if err := os.WriteFile(f.path, f.data, 0o600); err != nil {
							return nil, err
						}
					}
				}
			}
			b.WriteString(pg.text[last:m.start])
			b.WriteString(body)
			last = m.end
		}
		b.WriteString(pg.text[last:])
		if write && b.String() != pg.text {
			if err := os.WriteFile(pg.path, []byte(b.String()), 0o600); err != nil {
				return nil, err
			}
		}
	}
	sort.Strings(stale)
	if len(errs) > 0 {
		return stale, errors.Join(errs...)
	}
	return stale, nil
}
