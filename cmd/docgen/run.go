package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func offlineEnv(extra ...string) []string {
	env := []string{}
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		switch k {
		case "GOPROXY", "GOFLAGS", "GOMAXPROCS", "GOWORK":
			continue
		}
		env = append(env, e)
	}
	env = append(env, "GOPROXY=off", "GOFLAGS=-p=2", "GOMAXPROCS=2", "GOWORK=off")
	return append(env, extra...)
}

func (g *gen) goCmd(ctx context.Context, dir string, env []string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = dir
	cmd.Env = env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.String(), errb.String(), err
}

func (g *gen) runExamples() error {
	g.exOut = map[string]string{}
	g.exErr = map[string]error{}
	names := make([]string, 0, len(g.examples))
	for ex := range g.examples {
		names = append(names, ex)
	}
	sort.Strings(names)
	for _, ex := range names {
		dir := filepath.Join(g.v2, "examples", filepath.FromSlash(ex))
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		fmt.Fprintf(g.log, "docgen: running example %s\n", ex)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		runDir, target := g.v2, "./examples/"+ex
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			runDir, target = dir, "."
		}
		out, errOut, err := g.goCmd(ctx, runDir, offlineEnv(), "run", target)
		cancel()
		if err != nil {
			g.exErr[ex] = fmt.Errorf("go run ./examples/%s: %w\n%s", ex, err, errOut)
			continue
		}
		g.exOut[ex] = out
	}
	return nil
}

func (g *gen) runTests() {
	g.testsOK = map[string]error{}
	pkgs := make([]string, 0, len(g.tests))
	for ip := range g.tests {
		pkgs = append(pkgs, ip)
	}
	sort.Strings(pkgs)
	for _, ip := range pkgs {
		names := make([]string, 0, len(g.tests[ip]))
		for n := range g.tests[ip] {
			names = append(names, regexp.QuoteMeta("Example"+n))
		}
		sort.Strings(names)
		fmt.Fprintf(g.log, "docgen: running %d examples of %s\n", len(names), shortPath(ip))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		out, errOut, err := g.goCmd(ctx, g.v2, offlineEnv(), "test", "-count=1", "-run", "^("+strings.Join(names, "|")+")$", ip)
		cancel()
		if err != nil {
			g.testsOK[ip] = fmt.Errorf("go test %s: %w\n%s%s", shortPath(ip), err, out, errOut)
		}
	}
}
