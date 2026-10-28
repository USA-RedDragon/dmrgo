package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var mdLink = regexp.MustCompile(`\]\(([^()\s]+)\)`)

func markdownFiles(repoRoot string) ([]string, error) {
	files := []string{filepath.Join(repoRoot, "README.md")}
	err := filepath.WalkDir(filepath.Join(repoRoot, "docs"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

func stripCode(text string) []string {
	var out []string
	fence := false
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			fence = !fence
			out = append(out, "")
			continue
		}
		if fence {
			out = append(out, "")
			continue
		}
		parts := strings.Split(l, "`")
		var b strings.Builder
		for i := 0; i < len(parts); i += 2 {
			b.WriteString(parts[i])
		}
		out = append(out, b.String())
	}
	return out
}

func headingSlug(h string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(h)) {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func anchors(text string) map[string]bool {
	out := map[string]bool{}
	seen := map[string]int{}
	for _, l := range stripCode(text) {
		t := strings.TrimLeft(l, "#")
		if len(t) == len(l) || len(l)-len(t) > 6 || !strings.HasPrefix(t, " ") {
			continue
		}
		s := headingSlug(t)
		if n := seen[s]; n > 0 {
			out[s+"-"+strconv.Itoa(n)] = true
		} else {
			out[s] = true
		}
		seen[s]++
	}
	return out
}

func brokenLinks(file, text string, read func(string) (string, error)) []string {
	var bad []string
	for n, l := range stripCode(text) {
		for _, m := range mdLink.FindAllStringSubmatch(l, -1) {
			target := m[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			path, frag, _ := strings.Cut(target, "#")
			dest := file
			if path != "" {
				dest = filepath.Join(filepath.Dir(file), filepath.FromSlash(path))
			}
			body, err := read(dest)
			if err != nil {
				bad = append(bad, fmt.Sprintf("%s:%d: %s: %v", file, n+1, target, err))
				continue
			}
			if frag != "" && strings.HasSuffix(dest, ".md") && !anchors(body)[frag] {
				bad = append(bad, fmt.Sprintf("%s:%d: %s: no heading #%s", file, n+1, target, frag))
			}
		}
	}
	return bad
}

func checkLinks(repoRoot string) int {
	files, err := markdownFiles(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: cannot list the Markdown files: %v\n", err)
		return 1
	}
	read := func(rel string) (string, error) {
		p := filepath.Join(repoRoot, rel)
		fi, err := os.Stat(p)
		if err != nil {
			return "", err
		}
		if fi.IsDir() || !strings.HasSuffix(p, ".md") {
			return "", nil
		}
		b, err := os.ReadFile(p)
		return string(b), err
	}
	errs := 0
	for _, f := range files {
		rel, err := filepath.Rel(repoRoot, f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			errs++
			continue
		}
		body, err := read(rel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			errs++
			continue
		}
		for _, msg := range brokenLinks(rel, body, read) {
			fmt.Fprintln(os.Stderr, "ERROR: broken link:", msg)
			errs++
		}
	}
	return errs
}
