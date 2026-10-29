package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var usageOf = regexp.MustCompile(`(?m)^Usage of \S*?([^/\s:]+):`)

// fillFlags runs the command in the directory cmd= with -h and writes the
// usage that it prints.
func (g *gen) fillFlags(m marker) (string, []outFile, error) {
	dir := g.rootDir(m.args["cmd"])
	if !hasGo(dir) {
		return "", nil, fmt.Errorf("no Go command in %s", m.args["cmd"])
	}
	mod := dir
	for {
		if _, err := os.Stat(filepath.Join(mod, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(mod)
		if parent == mod {
			return "", nil, fmt.Errorf("no go.mod above %s", m.args["cmd"])
		}
		mod = parent
	}
	rel, err := filepath.Rel(mod, dir)
	if err != nil {
		return "", nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	fmt.Fprintf(g.log, "docgen: running %s -h\n", m.args["cmd"])
	out, errOut, err := g.goCmd(ctx, mod, offlineEnv(), "run", "./"+filepath.ToSlash(rel), "-h")
	var ee *exec.ExitError
	if err != nil && (!errors.As(err, &ee) || ee.ExitCode() != 2 && ee.ExitCode() != 1) {
		return "", nil, fmt.Errorf("go run %s -h: %w\n%s", m.args["cmd"], err, errOut)
	}
	usage := errOut + out
	usage = regexp.MustCompile(`(?m)^exit status \d+\n?`).ReplaceAllString(usage, "")
	usage = usageOf.ReplaceAllString(usage, "Usage of $1:")
	if !strings.Contains(usage, "Usage") && !strings.Contains(usage, "-") {
		return "", nil, fmt.Errorf("go run %s -h printed no usage", m.args["cmd"])
	}
	return fence("text", usage), nil, nil
}
