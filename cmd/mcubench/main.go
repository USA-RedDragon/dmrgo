// Command mcubench measures the Tier 0 transmitters of
// v2/rf on a Cortex-M4F. For each row it builds one bench program with
// TinyGo -opt z, runs it in qemu-system-arm (mps2-an386) with an
// instruction counter plugin, and takes the CPU from a cycle model of the
// counted instructions, the RAM from the static data and a painted stack,
// and the flash from the sections of the ELF. With -update it writes
// v2/internal/fsktx/testdata/tier0cost.json.
// With -check it measures again and fails when a value differs, or skips
// with the reason when a tool is missing.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed bench
var benchFS embed.FS

const (
	progTxAudio = "txaudio"
	progTxIQ    = "txiq"
)

const (
	tinygoVersion = "0.43."
	txDataFile    = "v2/internal/fsktx/testdata/tier0cost.json"
)

func main() {
	update := flag.Bool("update", false, "measure every row and write the data files")
	check := flag.Bool("check", false, "measure every row and fail when the data files differ, or skip when a tool is missing")
	work := flag.String("work", "", "directory for the build tree and the runs (default: a new directory under TMPDIR)")
	flag.Parse()
	if *update == *check {
		fmt.Fprintln(os.Stderr, "mcubench: give -update or -check")
		os.Exit(2)
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "mcubench: cannot find the mcubench source directory")
		os.Exit(1)
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, root, *work, *update)
	stop()
	var skip skipError
	switch {
	case errors.As(err, &skip) && !*update:
		fmt.Fprintln(os.Stderr, "mcubench: skipped:", skip.reason)
	case err != nil:
		fmt.Fprintln(os.Stderr, "mcubench:", err)
		os.Exit(1)
	}
}

type skipError struct{ reason string }

func (e skipError) Error() string { return e.reason }

type tools struct {
	tinygo, tinygoRoot, version, qemu, cc string
}

func findTools(ctx context.Context) (tools, error) {
	var t tools
	var err error
	for _, n := range []struct {
		dst  *string
		name string
	}{{&t.tinygo, "tinygo"}, {&t.qemu, "qemu-system-arm"}, {&t.cc, "cc"}} {
		if *n.dst, err = exec.LookPath(n.name); err != nil {
			return t, skipError{n.name + " is not on PATH"}
		}
	}
	out, err := exec.CommandContext(ctx, t.tinygo, "version").Output()
	if err != nil {
		return t, skipError{"tinygo version: " + err.Error()}
	}
	f := strings.Fields(string(out))
	if len(f) < 3 || !strings.HasPrefix(f[2], tinygoVersion) {
		return t, skipError{"tinygo on PATH is " + strings.TrimSpace(string(out)) + ", the data is measured with TinyGo " + tinygoVersion + "x"}
	}
	t.version = f[2]
	out, err = exec.CommandContext(ctx, t.tinygo, "env", "TINYGOROOT").Output()
	if err != nil {
		return t, skipError{"tinygo env TINYGOROOT: " + err.Error()}
	}
	t.tinygoRoot = strings.TrimSpace(string(out))
	return t, nil
}

type txRow struct {
	Name     string  `json:"name"`
	MHz      float64 `json:"mhz"`
	RAMBytes uint64  `json:"ram_bytes"`
	FlashKB  float64 `json:"flash_kb"`
}

type txData struct {
	Source string  `json:"source"`
	Rows   []txRow `json:"rows"`
}

type benchRow struct {
	prog string
	rate int
	name string
}

func txRows() []benchRow {
	return []benchRow{
		{prog: progTxIQ, rate: 96000, name: "I/Q at 96 kHz, CS16"},
		{prog: progTxAudio, rate: 48000, name: "Audio at 48 kHz, 12-bit"},
		{prog: progTxAudio, rate: 24000, name: "Audio at 24 kHz, 12-bit"},
	}
}

func run(ctx context.Context, root, work string, update bool) error {
	t, err := findTools(ctx)
	if err != nil {
		return err
	}
	if work == "" {
		if work, err = os.MkdirTemp("", "mcubench"); err != nil {
			return err
		}
		defer func() { _ = os.RemoveAll(work) }()
	}
	b := &bench{root: root, work: work, t: t}
	if err := b.setup(ctx); err != nil {
		return err
	}
	tx := txData{Source: "Built with TinyGo " + t.version + " `-opt z -gc=none` and run in QEMU (`mps2-an386`, Cortex-M4F) by `cmd/mcubench`, with BS-sourced bursts on every slot. CPU counts the cycles of the `rf` and `fsktx` code and of the bench loop per second of signal. RAM is the `Transmitter` or `AudioTransmitter` and the most stack under it. Flash is the code and constant data of the whole program."}
	for _, r := range txRows() {
		m, err := b.tx(ctx, r)
		if err != nil {
			return fmt.Errorf("%s %d: %w", r.prog, r.rate, err)
		}
		tx.Rows = append(tx.Rows, txRow{Name: r.name, MHz: m.mhz, RAMBytes: m.ram, FlashKB: m.flashKB})
	}
	var bad []string
	for _, f := range []struct {
		path string
		v    any
	}{{txDataFile, tx}} {
		js, err := json.MarshalIndent(f.v, "", "\t")
		if err != nil {
			return err
		}
		js = append(js, '\n')
		p := filepath.Join(root, f.path)
		if update {
			if err := os.WriteFile(p, js, 0o600); err != nil {
				return err
			}
			continue
		}
		old, err := os.ReadFile(p)
		if err != nil || string(old) != string(js) {
			bad = append(bad, f.path)
			fmt.Fprintf(os.Stderr, "mcubench: measured %s:\n%s", f.path, js)
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%s differ from the measurement: run go run -C cmd/mcubench . -update", strings.Join(bad, " and "))
	}
	return nil
}

type bench struct {
	root   string
	work   string
	t      tools
	plugin string
	target string
}

func (b *bench) cmd(ctx context.Context, dir string, env []string, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...)
	c.Dir = dir
	c.Env = append(os.Environ(), env...)
	out, err := c.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, out)
	}
	return nil
}

func copyTree(src, dst string) error {
	fsys := os.DirFS(src)
	return fs.WalkDir(fsys, ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" || d.Name() == "examples" || d.Name() == "cmd" && rel != "." {
				return fs.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		data, err := fs.ReadFile(fsys, rel)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o600)
	})
}

func (b *bench) setup(ctx context.Context) error {
	src := filepath.Join(b.work, "v2")
	if err := os.RemoveAll(src); err != nil {
		return err
	}
	if err := copyTree(filepath.Join(b.root, "v2"), src); err != nil {
		return err
	}
	gate := filepath.Join(src, "rf", "txregion.go")
	if s, err := os.ReadFile(gate); err == nil {
		open := strings.Replace(string(s), "var spurGate10 [regions][2]bool\n",
			"var spurGate10 = [regions][2]bool{{true, true}, {true, true}, {true, true}, {true, true}, {true, true}}\n", 1)
		if err := os.WriteFile(gate, []byte(open), 0o600); err != nil {
			return err
		}
	}
	files := map[string]string{}
	for _, n := range []string{"m4count.c", "startup.s", "common.go.txt", progTxAudio + ".go.txt", progTxIQ + ".go.txt"} {
		data, err := benchFS.ReadFile("bench/" + n)
		if err != nil {
			return err
		}
		files[n] = string(data)
	}
	for _, prog := range []string{progTxAudio, progTxIQ} {
		dir := filepath.Join(src, "zzmcubench", prog)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		main := files[prog+".go.txt"]
		if strings.HasPrefix(prog, "tx") {
			if err := os.WriteFile(filepath.Join(dir, "const.go"), []byte(fmt.Sprintf("package main\n\nconst txBursts = %d\n", txBursts)), 0o600); err != nil {
				return err
			}
		}
		if err := os.WriteFile(filepath.Join(dir, "common.go"), []byte(files["common.go.txt"]), 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(main), 0o600); err != nil {
			return err
		}
	}
	startup := filepath.Join(b.work, "startup.s")
	if err := os.WriteFile(startup, []byte(files["startup.s"]), 0o600); err != nil {
		return err
	}
	rel, err := filepath.Rel(b.t.tinygoRoot, startup)
	if err != nil {
		return err
	}
	target := map[string]any{
		"inherits":           []string{"cortex-m4"},
		"build-tags":         []string{"qemu", "lm3s6965", "mps2_an386"},
		"linkerscript":       "targets/lm3s6965.ld",
		"default-stack-size": 4096,
		"extra-files":        []string{rel},
		"features":           "+armv7e-m,+dsp,+hwdiv,+thumb-mode,+fpregs,+vfp2sp,+vfp3d16sp,+vfp4d16sp,-aes,-bf16,-cdecp0,-cdecp1,-cdecp2,-cdecp3,-cdecp4,-cdecp5,-cdecp6,-cdecp7,-crc,-crypto,-d32,-dotprod,-fp-armv8,-fp-armv8d16,-fp-armv8d16sp,-fp-armv8sp,-fp16,-fp16fml,-fp64,-fullfp16,-hwdiv-arm,-i8mm,-lob,-mve,-mve.fp,-neon,-pacbti,-ras,-sb,-sha2,-vfp2,-vfp3,-vfp3d16,-vfp3sp,-vfp4,-vfp4d16,-vfp4sp",
		"cflags":             []string{"-mfloat-abi=softfp", "-mfpu=fpv4-sp-d16"},
	}
	js, err := json.Marshal(target)
	if err != nil {
		return err
	}
	b.target = filepath.Join(b.work, "m4.json")
	if err := os.WriteFile(b.target, js, 0o600); err != nil {
		return err
	}
	cfile := filepath.Join(b.work, "m4count.c")
	if err := os.WriteFile(cfile, []byte(files["m4count.c"]), 0o600); err != nil {
		return err
	}
	glib, err := exec.CommandContext(ctx, "pkg-config", "--cflags", "--libs", "glib-2.0").Output()
	if err != nil {
		return skipError{"pkg-config glib-2.0: " + err.Error()}
	}
	b.plugin = filepath.Join(b.work, "m4count.so")
	args := append([]string{"-shared", "-fPIC", "-O2", "-o", b.plugin, cfile}, strings.Fields(string(glib))...)
	if err := b.cmd(ctx, b.work, nil, b.t.cc, args...); err != nil {
		return skipError{"cannot build the QEMU plugin (needs qemu-plugin.h and glib): " + err.Error()}
	}
	return nil
}

func (b *bench) build(ctx context.Context, prog string, rate int, on bool) (string, error) {
	flag := "1"
	if !on {
		flag = "0"
	}
	elf := filepath.Join(b.work, fmt.Sprintf("%s-%d-%s.elf", prog, rate, flag))
	err := b.cmd(ctx, filepath.Join(b.work, "v2"), []string{"GOFLAGS=-buildvcs=false"}, b.t.tinygo, "build", "-target", b.target, "-opt", "z", "-gc", "none", "-scheduler", "none",
		"-ldflags", fmt.Sprintf("-X main.doRun=%s -X main.rateS=%d", flag, rate), "-o", elf, "./zzmcubench/"+prog)
	return elf, err
}

type runOut struct {
	depth          uint64
	bursts, hash   string
	counts, stdout string
}

func (b *bench) qemu(ctx context.Context, elf, dir, input string) (runOut, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return runOut{}, err
	}
	in := filepath.Join(dir, "in.bin")
	_ = os.Remove(in)
	if input != "" {
		if err := os.Symlink(input, in); err != nil {
			return runOut{}, err
		}
	}
	o := runOut{counts: filepath.Join(dir, filepath.Base(elf)+".counts")}
	c := exec.CommandContext(ctx, b.t.qemu, "-machine", "mps2-an386", "-cpu", "cortex-m4", "-semihosting-config", "enable=on,target=native",
		"-nographic", "-monitor", "none", "-serial", "none", "-kernel", elf, "-plugin", b.plugin+",out="+o.counts)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		return o, fmt.Errorf("qemu %s: %w\n%s", elf, err, out)
	}
	o.stdout = string(out)
	f := strings.Fields(o.stdout)
	if len(f) < 3 {
		return o, fmt.Errorf("qemu %s: unexpected output %q", elf, out)
	}
	if _, err := fmt.Sscan(f[0], &o.depth); err != nil {
		return o, fmt.Errorf("qemu %s: %q: %w", elf, out, err)
	}
	o.bursts, o.hash = f[1], f[2]
	if o.bursts == "4294967295" {
		return o, fmt.Errorf("qemu %s: the bench could not set up the transmitter", elf)
	}
	return o, nil
}

type measure struct {
	mhz     float64
	ram     uint64
	flashKB float64
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func (b *bench) tx(ctx context.Context, r benchRow) (measure, error) {
	elf, err := b.build(ctx, r.prog, r.rate, true)
	if err != nil {
		return measure{}, err
	}
	img, err := readELF(elf)
	if err != nil {
		return measure{}, err
	}
	o, err := b.qemu(ctx, elf, filepath.Join(b.work, "run", fmt.Sprintf("%s-%d", r.prog, r.rate)), "")
	if err != nil {
		return measure{}, err
	}
	c, err := img.cycles(o.counts, func(s string) bool {
		return strings.Contains(s, "internal/fsktx") || strings.Contains(s, "dmrgo/v2/rf.") || s == "main.step" || s == "main.toDAC"
	})
	if err != nil {
		return measure{}, err
	}
	_ = os.Remove(o.counts)
	secs := float64(txBursts) * 0.03
	return measure{
		mhz:     round1(float64(c.matched) / secs / 1e6),
		ram:     img.symSize("main.mod") + o.depth,
		flashKB: round1(float64(img.flash) / 1000),
	}, nil
}

const txBursts = 66
