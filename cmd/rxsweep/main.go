// Command rxsweep measures many variants of the receiver in v2/rf and finds
// the Pareto front of sensitivity against cost.
//
// A space file lists the variants: a base tier with options, constants and
// source edits that replace those of the tier. rxsweep run builds the test
// binary of each distinct source tree, receives the sweep stream at a coarse
// C/N grid, then on a fine grid around the 90% recovery point of each
// payload, and last measures the cost of each variant alone on the machine.
// It appends every result to files in the output directory, so a run that
// stops resumes where it was, and a variant whose sources change gets new
// results. rxsweep report reads the results and writes the tables, the
// Pareto fronts and the graphs.
//
//	go run -C cmd/rxsweep . run -space spaces/tiers.json -out ~/rxsweep
//	go run -C cmd/rxsweep . report -out ~/rxsweep
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "run":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		err = runCmd(ctx, os.Args[2:])
		stop()
	case "report":
		err = reportCmd(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "rxsweep:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: rxsweep run -space FILE -out DIR [flags] | rxsweep report -out DIR")
	os.Exit(2)
}

// repoRoot returns the root of the dmrgo checkout that holds this source.
func repoRoot() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot find the rxsweep source directory")
	}
	return filepath.Join(filepath.Dir(file), "..", ".."), nil
}

func runCmd(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	space := fs.String("space", "", "space file of the variants")
	out := fs.String("out", "", "output directory")
	jobs := fs.Int("jobs", 2, "worker processes at once; each gets an equal share of the cores")
	only := fs.String("only", "", "comma-separated variant names to run; all when empty")
	noCost := fs.Bool("no-cost", false, "skip the cost phase")
	m4 := fs.String("m4", "", "command that measures a variant on a Cortex-M4F: it gets the patched v2 directory and the variant JSON file, and prints one JSON object")
	_ = fs.Parse(args)
	if *space == "" || *out == "" {
		return fmt.Errorf("run needs -space and -out")
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	sp, err := loadSpace(*space)
	if err != nil {
		return err
	}
	r := &runner{root: root, out: *out, jobs: max(*jobs, 1), space: sp, m4: *m4, cost: !*noCost}
	return r.run(ctx, splitList(*only))
}

func reportCmd(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	out := fs.String("out", "", "output directory of rxsweep run")
	floor := fs.String("floor", "", "floor file; default v2/rf/testdata/rxbench_floor.json of this checkout")
	_ = fs.Parse(args)
	if *out == "" {
		return fmt.Errorf("report needs -out")
	}
	if *floor == "" {
		root, err := repoRoot()
		if err != nil {
			return err
		}
		*floor = filepath.Join(root, "v2/rf/testdata/rxbench_floor.json")
	}
	return report(*out, *floor)
}
