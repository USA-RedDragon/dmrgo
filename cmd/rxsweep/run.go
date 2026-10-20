package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type runner struct {
	root, out string
	jobs      int
	space     *space
	m4        string
	cost      bool

	app    appender
	logm   sync.Mutex
	done   map[string]map[sweepKey]bool
	buildm sync.Mutex
}

type sweepKey struct {
	cn   float64
	seed uint64
}

type prepared struct {
	v      variant
	id     string
	dir    string
	tree   string
	binary string
}

type workerCase struct {
	CN   float64 `json:"cn"`
	Seed uint64  `json:"seed"`
}

type workerJob struct {
	Tier        int             `json:"tier"`
	Opt         json.RawMessage `json:"opt,omitempty"`
	Lookback    int             `json:"lookback"`
	Hz          int             `json:"hz,omitempty"`
	Superframes int             `json:"superframes"`
	Offset      float64         `json:"offset"`
	Cases       []workerCase    `json:"cases"`
	Cost        bool            `json:"cost,omitempty"`
	Disc        bool            `json:"disc,omitempty"`
	Stream      string          `json:"stream,omitempty"`
}

type workerResult struct {
	Cases []caseResult `json:"cases"`
	Cost  *costRecord  `json:"cost,omitempty"`
}

func (r *runner) logf(format string, a ...any) {
	r.logm.Lock()
	defer r.logm.Unlock()
	fmt.Fprintf(os.Stderr, "%s %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
}

func (r *runner) run(ctx context.Context, only []string) error {
	for _, d := range []string{"src", "bin", "jobs"} {
		if err := os.MkdirAll(filepath.Join(r.out, d), 0o755); err != nil {
			return err
		}
	}
	prev, err := readLines[caseResult](filepath.Join(r.out, resultsFile))
	if err != nil {
		return err
	}
	r.done = map[string]map[sweepKey]bool{}
	for _, c := range prev {
		if r.done[c.ID] == nil {
			r.done[c.ID] = map[sweepKey]bool{}
		}
		r.done[c.ID][sweepKey{c.CN, c.Seed}] = true
	}
	known, err := readLines[variantRecord](filepath.Join(r.out, variantsFile))
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, k := range known {
		seen[k.ID] = true
	}
	commit := r.commit(ctx)
	var ps []*prepared
	for _, v := range r.space.Variants {
		if len(only) > 0 && !slices.Contains(only, v.Name) {
			continue
		}
		p, err := r.prepare(ctx, v)
		if err != nil {
			return fmt.Errorf("variant %s: %w", v.Name, err)
		}
		if !seen[p.id] {
			if err := r.app.add(filepath.Join(r.out, variantsFile), variantRecord{ID: p.id, Name: v.Name, Tree: p.tree, Commit: commit, Variant: v}); err != nil {
				return err
			}
			seen[p.id] = true
		}
		ps = append(ps, p)
	}
	r.logf("%d variants; sensitivity with %d workers of %d cores", len(ps), r.jobs, r.cores())
	next := make(chan *prepared)
	errc := make(chan error, r.jobs)
	var wg sync.WaitGroup
	for range r.jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range next {
				if err := r.sensitivity(ctx, p); err != nil {
					errc <- fmt.Errorf("variant %s: %w", p.v.Name, err)
					return
				}
			}
		}()
	}
feed:
	for _, p := range ps {
		select {
		case next <- p:
		case err := <-errc:
			close(next)
			wg.Wait()
			return err
		case <-ctx.Done():
			break feed
		}
	}
	close(next)
	wg.Wait()
	select {
	case err := <-errc:
		return err
	default:
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if r.cost {
		if err := r.costs(ctx, ps); err != nil {
			return err
		}
	}
	if r.m4 != "" {
		return r.m4Costs(ctx, ps)
	}
	return nil
}

func (r *runner) cores() int { return max(runtime.NumCPU()/r.jobs, 1) }

func (r *runner) commit(ctx context.Context) string {
	rev := exec.CommandContext(ctx, "git", "rev-parse", "--short=12", "HEAD")
	rev.Dir = r.root
	out, err := rev.Output()
	if err != nil {
		return ""
	}
	c := strings.TrimSpace(string(out))
	st := exec.CommandContext(ctx, "git", "status", "--porcelain", "--", "v2")
	st.Dir = r.root
	if b, _ := st.Output(); len(b) > 0 {
		c += "+dirty"
	}
	return c
}

func (r *runner) prepare(ctx context.Context, v variant) (*prepared, error) {
	r.buildm.Lock()
	defer r.buildm.Unlock()
	dir := filepath.Join(r.root, "v2")
	if v.patched() {
		dir = filepath.Join(r.out, "src", v.patchKey(), "v2")
		if err := os.RemoveAll(dir); err != nil {
			return nil, err
		}
		if err := os.CopyFS(dir, os.DirFS(filepath.Join(r.root, "v2"))); err != nil {
			return nil, err
		}
		if err := v.apply(dir); err != nil {
			return nil, err
		}
		gen := exec.CommandContext(ctx, "go", "test", "./rf", "-run", "^TestTablesGenerated$", "-update", "-count", "1")
		gen.Dir = dir
		gen.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
		if out, err := gen.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("generate tables: %w: %s", err, out)
		}
	}
	tree, err := treeHash(dir)
	if err != nil {
		return nil, err
	}
	bin := filepath.Join(r.out, "bin", tree+".test")
	if _, err := os.Stat(bin); errors.Is(err, fs.ErrNotExist) {
		r.logf("build %s for %s", tree, v.Name)
		cmd := exec.CommandContext(ctx, "go", "test", "-c", "-o", bin, "./rf")
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("build: %w: %s", err, out)
		}
	}
	key, err := json.Marshal(struct {
		Tree     string
		Tier     int
		Lookback int
		Hz       int
		Opt      json.RawMessage
		SF       int
		Off      float64
		Disc     bool
		Stream   string `json:",omitempty"`
	}{tree, v.Tier, v.Lookback, v.Hz, v.jobOpt(), r.space.Superframes, r.space.Offset, v.Disc, r.space.Stream})
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256(key)
	return &prepared{v: v, id: hex.EncodeToString(h[:8]), dir: dir, tree: tree, binary: bin}, nil
}

func (r *runner) work(ctx context.Context, p *prepared, cases []workerCase, cost bool, procs int) (*workerResult, error) {
	job := workerJob{Tier: p.v.Tier, Opt: p.v.jobOpt(), Lookback: p.v.Lookback, Hz: p.v.Hz, Superframes: r.space.Superframes, Offset: r.space.Offset, Cases: cases, Cost: cost, Disc: p.v.Disc, Stream: r.space.Stream}
	b, err := json.Marshal(job)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(filepath.Join(r.out, "jobs"), p.id+"-*.json")
	if err != nil {
		return nil, err
	}
	jobPath := f.Name()
	outPath := strings.TrimSuffix(jobPath, ".json") + ".out.json"
	defer os.Remove(jobPath)
	defer os.Remove(outPath)
	if _, err := f.Write(b); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()
	cmd := exec.CommandContext(ctx, p.binary, "-test.run", "^TestSweepWorker$", "-test.count", "1", "-test.timeout", "6h")
	cmd.Dir = filepath.Join(p.dir, "rf")
	cmd.Env = append(os.Environ(), "DMRRF_SWEEP_JOB="+jobPath, "DMRRF_SWEEP_OUT="+outPath, "GOMAXPROCS="+strconv.Itoa(procs))
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("worker: %w: %s", err, buf.Bytes())
	}
	ob, err := os.ReadFile(outPath)
	if err != nil {
		return nil, err
	}
	var res workerResult
	if err := json.Unmarshal(ob, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func (r *runner) receive(ctx context.Context, p *prepared, cases []workerCase) error {
	var todo []workerCase
	r.logm.Lock()
	for _, c := range cases {
		if !r.done[p.id][sweepKey{c.CN, c.Seed}] {
			todo = append(todo, c)
		}
	}
	r.logm.Unlock()
	if len(todo) == 0 {
		return nil
	}
	start := time.Now()
	res, err := r.work(ctx, p, todo, false, r.cores())
	if err != nil {
		return err
	}
	r.logm.Lock()
	if r.done[p.id] == nil {
		r.done[p.id] = map[sweepKey]bool{}
	}
	for _, c := range res.Cases {
		r.done[p.id][sweepKey{c.CN, c.Seed}] = true
	}
	r.logm.Unlock()
	for _, c := range res.Cases {
		c.ID = p.id
		if err := r.app.add(filepath.Join(r.out, resultsFile), c); err != nil {
			return err
		}
	}
	r.logf("%s: %d cases in %s", p.v.Name, len(todo), time.Since(start).Round(time.Second))
	return nil
}

func gridCases(g grid, at float64) []workerCase {
	var out []workerCase
	for i := 0; ; i++ {
		cn := math.Round((at+g.Lo+float64(i)*g.Step)*100) / 100
		if cn > at+g.Hi+1e-9 {
			break
		}
		for s := range g.Seeds {
			out = append(out, workerCase{cn, uint64(100 + s)})
		}
	}
	return out
}

func (r *runner) sensitivity(ctx context.Context, p *prepared) error {
	if err := r.receive(ctx, p, gridCases(r.space.Coarse, 0)); err != nil {
		return err
	}
	cases, err := r.cases(p.id)
	if err != nil {
		return err
	}
	pts := curveOf(cases)
	var fine []workerCase
	have := map[sweepKey]bool{}
	for _, k := range r.space.Payloads {
		cn, y := recovery(pts, k)
		c := at90(cn, y)
		if !c.OK || c.Below {
			continue
		}
		at := math.Round(c.X/r.space.Fine.Step) * r.space.Fine.Step
		for _, fc := range gridCases(r.space.Fine, at) {
			if key := (sweepKey{fc.CN, fc.Seed}); !have[key] {
				have[key] = true
				fine = append(fine, fc)
			}
		}
	}
	return r.receive(ctx, p, fine)
}

func (r *runner) cases(id string) ([]caseResult, error) {
	r.app.mu.Lock()
	defer r.app.mu.Unlock()
	all, err := readLines[caseResult](filepath.Join(r.out, resultsFile))
	if err != nil {
		return nil, err
	}
	var out []caseResult
	for _, c := range all {
		if c.ID == id {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *runner) costs(ctx context.Context, ps []*prepared) error {
	prev, err := readLines[costRecord](filepath.Join(r.out, costFile))
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, c := range prev {
		have[c.ID] = true
	}
	for _, p := range ps {
		if have[p.id] {
			continue
		}
		res, err := r.work(ctx, p, []workerCase{{10, 1}, {5, 1}}, true, 1)
		if err != nil {
			return fmt.Errorf("cost of %s: %w", p.v.Name, err)
		}
		c := res.Cost
		c.ID = p.id
		c.LockedNsPerSecond = map[string]float64{}
		for _, k := range res.Cases {
			c.LockedNsPerSecond[strconv.FormatFloat(k.CN, 'f', -1, 64)] = k.CPUSeconds * 1e9 / k.Seconds
		}
		if err := r.app.add(filepath.Join(r.out, costFile), c); err != nil {
			return err
		}
		have[p.id] = true
		r.logf("%s: cost %d B, %.1f ms/s locked at 10 dB, %.1f ms/s search", p.v.Name, c.RAMBytes, c.LockedNsPerSecond["10"]/1e6, c.SearchNsPerSecond/1e6)
	}
	return nil
}

func (r *runner) m4Costs(ctx context.Context, ps []*prepared) error {
	prev, err := readLines[m4Record](filepath.Join(r.out, m4File))
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, c := range prev {
		have[c.ID] = true
	}
	for _, p := range ps {
		if have[p.id] {
			continue
		}
		vb, err := json.Marshal(p.v)
		if err != nil {
			return err
		}
		vf := filepath.Join(r.out, "jobs", p.id+"-m4.json")
		if err := os.WriteFile(vf, vb, 0o600); err != nil {
			return err
		}
		out, err := exec.CommandContext(ctx, r.m4, p.dir, vf).Output()
		os.Remove(vf)
		if err != nil {
			return fmt.Errorf("m4 of %s: %w", p.v.Name, err)
		}
		lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
		var res map[string]json.RawMessage
		if err := json.Unmarshal(lines[len(lines)-1], &res); err != nil {
			return fmt.Errorf("m4 of %s: last line is not a JSON object: %w", p.v.Name, err)
		}
		if err := r.app.add(filepath.Join(r.out, m4File), m4Record{ID: p.id, Result: res}); err != nil {
			return err
		}
		have[p.id] = true
		r.logf("%s: m4 %s", p.v.Name, lines[len(lines)-1])
	}
	return nil
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}
