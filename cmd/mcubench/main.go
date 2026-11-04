// Command mcubench measures the Tier 0 receivers and transmitters of
// v2/rf on a Cortex-M4F, and the higher audio receiver tiers on a
// Cortex-M7. For each row it builds one bench program with TinyGo -opt z,
// runs it in qemu-system-arm (mps2-an386 or mps2-an500) with an
// instruction counter plugin, and takes the CPU from a cycle model of the
// counted instructions, the RAM from the static data, the heap and a
// painted stack, and the flash from the sections of the ELF. With -update
// it writes v2/rf/testdata/rxbench_mcu.json, v2/rf/testdata/rxbench_m7.json
// and v2/internal/fsktx/testdata/tier0cost.json.
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
	"strconv"
	"strings"
)

//go:embed bench
var benchFS embed.FS

const (
	progRxAudio = "rxaudio"
	progRxPE    = "rxaudiope"
	progRxIQ    = "rxiq"
	progTxAudio = "txaudio"
	progTxPE    = "txaudiope"
	progTxIQ    = "txiq"
	progRxTier  = "rxtier"
	modeAudio   = "audio"
)

const (
	tinygoVersion = "0.44."
	rxDataFile    = "v2/rf/testdata/rxbench_mcu.json"
	m7DataFile    = "v2/rf/testdata/rxbench_m7.json"
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

func findTools(ctx context.Context, root string) (tools, error) {
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
	want, err := goDirective(filepath.Join(root, "go.mod"))
	if err != nil {
		return t, err
	}
	out, err = exec.CommandContext(ctx, "go", "env", "GOVERSION").Output()
	if err != nil {
		return t, skipError{"go env GOVERSION: " + err.Error()}
	}
	if got := strings.TrimSpace(string(out)); got != "go"+want {
		return t, skipError{"go on PATH is " + got + ", the data is measured with go" + want + ", the go directive of go.mod that CI sets up, as TinyGo compiles the standard library of that Go"}
	}
	return t, nil
}

func goDirective(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, l := range strings.Split(string(b), "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[0] == "go" {
			return f[1], nil
		}
	}
	return "", errors.New(path + " has no go directive")
}

type rxRow struct {
	Mode     string  `json:"mode"`
	Tier     int     `json:"tier"`
	Rate     int     `json:"rate"`
	PreEmph  bool    `json:"pre_emphasis,omitempty"`
	MHz      float64 `json:"mhz"`
	RAMBytes uint64  `json:"ram_bytes"`
	FlashKB  float64 `json:"flash_kb"`
}

type rxData struct {
	Source string  `json:"source"`
	Rows   []rxRow `json:"rows"`
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
	prog, mode string
	rate, tier int
	name       string
}

func rxRows() []benchRow {
	return []benchRow{{prog: progRxAudio, mode: modeAudio, rate: 9600}, {prog: progRxPE, mode: modeAudio, rate: 9600}, {prog: progRxIQ, mode: "iqlow", rate: 9600}}
}

func txRows() []benchRow {
	return []benchRow{
		{prog: progTxIQ, rate: 96000, name: "I/Q at 96 kHz, CS16"},
		{prog: progTxAudio, rate: 48000, name: "Audio at 48 kHz, 12-bit"},
		{prog: progTxAudio, rate: 24000, name: "Audio at 24 kHz, 12-bit"},
		{prog: progTxPE, rate: 48000, name: "Audio at 48 kHz, 12-bit, pre-emphasis"},
		{prog: progTxPE, rate: 24000, name: "Audio at 24 kHz, 12-bit, pre-emphasis"},
	}
}

func m7Rows() []benchRow {
	return []benchRow{{prog: progRxTier, mode: modeAudio, rate: 9600, tier: 1}, {prog: progRxTier, mode: modeAudio, rate: 9600, tier: 2}}
}

func signalConds() []string { return []string{"30", "10", "6", "2", "-1", "noise", "ms10", "ms2"} }

func run(ctx context.Context, root, work string, update bool) error {
	t, err := findTools(ctx, root)
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
	rx := rxData{Source: "Built with TinyGo " + t.version + " `-opt z -gc=none` and run in QEMU (`mps2-an386`, Cortex-M4F) by `cmd/mcubench`. CPU is the most, over the test signals, of the cycles that the receiver takes per second of signal. RAM is the static data of the program less the bench, with the most stack. Flash is the code and constant data of the whole program."}
	for _, r := range rxRows() {
		m, err := b.rx(ctx, r)
		if err != nil {
			return fmt.Errorf("%s %d: %w", r.prog, r.rate, err)
		}
		rx.Rows = append(rx.Rows, rxRow{Mode: r.mode, Rate: r.rate, PreEmph: r.prog == progRxPE, MHz: m.mhz, RAMBytes: m.ram, FlashKB: m.flashKB})
	}
	tx := txData{Source: "Built with TinyGo " + t.version + " `-opt z -gc=none` and run in QEMU (`mps2-an386`, Cortex-M4F) by `cmd/mcubench`, with BS-sourced bursts on every slot. CPU counts the cycles of the `rf` and `fsktx` code and of the bench loop per second of signal. RAM is the `Transmitter` or `AudioTransmitter`, with its `PreEmphasis` when it has one, and the most stack under it. Flash is the code and constant data of the whole program."}
	for _, r := range txRows() {
		m, err := b.tx(ctx, r)
		if err != nil {
			return fmt.Errorf("%s %d: %w", r.prog, r.rate, err)
		}
		tx.Rows = append(tx.Rows, txRow{Name: r.name, MHz: m.mhz, RAMBytes: m.ram, FlashKB: m.flashKB})
	}
	m7 := rxData{Source: "Built with TinyGo " + t.version + " `-opt z -gc=leaking` for a Cortex-M7 with an FPv5-D16 FPU and run in QEMU (`mps2-an500`) by `cmd/mcubench`. The receiver is the one of `NewAudioReceiver`, and each run gives the bursts of the receiver of the Go compiler, with `Burst.Seconds` to the microsecond, as the math package of TinyGo differs from the assembly of gc in the last bit of some results. CPU is the most, over the test signals, of the cycles that the receiver takes per second of signal, by the cycle model of the Cortex-M4F rows. RAM is the static data of the program less the bench, the heap that `NewAudioReceiver` takes, and the most stack. The receiver allocates nothing after it starts. Flash is the code and constant data of the whole program."}
	for _, r := range m7Rows() {
		m, err := b.rxTier(ctx, r)
		if err != nil {
			return fmt.Errorf("%s %d tier %d: %w", r.prog, r.rate, r.tier, err)
		}
		m7.Rows = append(m7.Rows, rxRow{Mode: r.mode, Tier: r.tier, Rate: r.rate, MHz: m.mhz, RAMBytes: m.ram, FlashKB: m.flashKB})
	}
	var bad []string
	for _, f := range []struct {
		path string
		v    any
	}{{rxDataFile, rx}, {m7DataFile, m7}, {txDataFile, tx}} {
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
	m7     string
	sig    string
	refs   map[string]sigRef
}

type sigRef struct {
	samples int
	bursts  string
	hash    string
	rate    int
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
	for _, n := range []string{"m4count.c", "startup.s", "m7.ld", "common.go.txt", progRxAudio + ".go.txt", progRxPE + ".go.txt", progRxIQ + ".go.txt", progTxAudio + ".go.txt", progTxPE + ".go.txt", progTxIQ + ".go.txt", progRxTier + ".go.txt"} {
		data, err := benchFS.ReadFile("bench/" + n)
		if err != nil {
			return err
		}
		files[n] = string(data)
	}
	for _, prog := range []string{progRxAudio, progRxPE, progRxIQ, progTxAudio, progTxPE, progTxIQ} {
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
	tierDir := filepath.Join(src, "zzmcubench", progRxTier)
	if err := os.MkdirAll(tierDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tierDir, "main.go"), []byte(files[progRxTier+".go.txt"]), 0o600); err != nil {
		return err
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
	if err := b.setupM7(files["m7.ld"], rel); err != nil {
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
	return b.signals(ctx)
}

func (b *bench) signals(ctx context.Context) error {
	b.sig = filepath.Join(b.work, "sig")
	if err := os.MkdirAll(b.sig, 0o755); err != nil {
		return err
	}
	rates := map[string][]string{}
	for _, r := range rxRows() {
		rates[r.prog] = append(rates[r.prog], strconv.Itoa(r.rate))
	}
	var tiers []string
	for _, r := range m7Rows() {
		tiers = append(tiers, strconv.Itoa(r.tier))
	}
	env := []string{"DMRGO_MCUBENCH_OUT=" + b.sig, "DMRGO_MCUBENCH_TIERS=" + strings.Join(tiers, " "), "DMRGO_MCUBENCH_AUDIO=" + strings.Join(rates[progRxAudio], " "),
		"DMRGO_MCUBENCH_AUDIOPE=" + strings.Join(rates[progRxPE], " "), "DMRGO_MCUBENCH_IQ=" + strings.Join(rates[progRxIQ], " ")}
	if err := b.cmd(ctx, filepath.Join(b.root, "v2"), env, "go", "test", "-count=1", "-run", "^TestMCUBenchSignals$", "./rf"); err != nil {
		return err
	}
	ref, err := os.ReadFile(filepath.Join(b.sig, "ref.txt"))
	if err != nil {
		return err
	}
	b.refs = map[string]sigRef{}
	for _, l := range strings.Split(strings.TrimSpace(string(ref)), "\n") {
		var name string
		var r sigRef
		if _, err := fmt.Sscan(l, &name, &r.samples, &r.bursts, &r.hash, &r.rate); err != nil {
			return fmt.Errorf("ref.txt: %q: %w", l, err)
		}
		b.refs[name] = r
	}
	return nil
}

func (b *bench) build(ctx context.Context, prog string, rate int, on bool) (string, error) {
	return b.buildFor(ctx, b.target, "none", prog, rate, 0, on)
}

func (b *bench) buildFor(ctx context.Context, target, gc, prog string, rate, tier int, on bool) (string, error) {
	flag := "1"
	if !on {
		flag = "0"
	}
	ld := fmt.Sprintf("-X main.doRun=%s -X main.rateS=%d", flag, rate)
	name := fmt.Sprintf("%s-%d-%s.elf", prog, rate, flag)
	if tier > 0 {
		ld += fmt.Sprintf(" -X main.tierS=%d", tier+1)
		name = fmt.Sprintf("%s-%d-t%d-%s.elf", prog, rate, tier, flag)
	}
	elf := filepath.Join(b.work, name)
	err := b.cmd(ctx, filepath.Join(b.work, "v2"), []string{"GOFLAGS=-buildvcs=false"}, b.t.tinygo, "build", "-target", target, "-opt", "z", "-gc", gc, "-scheduler", "none",
		"-ldflags", ld, "-o", elf, "./zzmcubench/"+prog)
	return elf, err
}

type runOut struct {
	depth, heap, grow uint64
	bursts, hash      string
	counts, stdout    string
}

func (b *bench) qemu(ctx context.Context, elf, dir, input string) (runOut, error) {
	return b.qemuOn(ctx, "mps2-an386", "cortex-m4", elf, dir, input)
}

func (b *bench) qemuOn(ctx context.Context, machine, cpu, elf, dir, input string) (runOut, error) {
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
	c := exec.CommandContext(ctx, b.t.qemu, "-machine", machine, "-cpu", cpu, "-semihosting-config", "enable=on,target=native",
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
	if len(f) >= 5 {
		if _, err := fmt.Sscan(f[3]+" "+f[4], &o.heap, &o.grow); err != nil {
			return o, fmt.Errorf("qemu %s: %q: %w", elf, out, err)
		}
	}
	if o.bursts == "4294967295" {
		return o, fmt.Errorf("qemu %s: the bench could not set up the receiver or transmitter", elf)
	}
	return o, nil
}

type measure struct {
	mhz     float64
	ram     uint64
	flashKB float64
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func (b *bench) rx(ctx context.Context, r benchRow) (measure, error) {
	on, err := b.build(ctx, r.prog, r.rate, true)
	if err != nil {
		return measure{}, err
	}
	off, err := b.build(ctx, r.prog, r.rate, false)
	if err != nil {
		return measure{}, err
	}
	img, err := readELF(on)
	if err != nil {
		return measure{}, err
	}
	offImg, err := readELF(off)
	if err != nil {
		return measure{}, err
	}
	prefix := map[string]string{progRxAudio: "a", progRxPE: "p", progRxIQ: "q"}[r.prog]
	var m measure
	var depth uint64
	for _, c := range signalConds() {
		name := fmt.Sprintf("%s-%s-%d.bin", prefix, c, r.rate)
		ref, ok := b.refs[name]
		if !ok {
			return m, fmt.Errorf("no signal %s", name)
		}
		dir := filepath.Join(b.work, "run", strings.TrimSuffix(name, ".bin"))
		o1, err := b.qemu(ctx, on, dir, filepath.Join(b.sig, name))
		if err != nil {
			return m, err
		}
		if o1.bursts != ref.bursts || o1.hash != ref.hash {
			return m, fmt.Errorf("%s: the bench gave %s bursts with hash %s, the Go receiver %s with hash %s", name, o1.bursts, o1.hash, ref.bursts, ref.hash)
		}
		o0, err := b.qemu(ctx, off, dir, filepath.Join(b.sig, name))
		if err != nil {
			return m, err
		}
		c1, err := img.cycles(o1.counts, func(s string) bool { return s == "main.onBursts" || strings.HasPrefix(s, "main.mix") })
		if err != nil {
			return m, err
		}
		c0, err := offImg.cycles(o0.counts, func(string) bool { return false })
		if err != nil {
			return m, err
		}
		secs := float64(ref.samples) / float64(ref.rate)
		m.mhz = max(m.mhz, float64(c1.total-c1.matched-c0.total)/secs/1e6)
		depth = max(depth, o1.depth)
		_ = os.Remove(o1.counts)
		_ = os.Remove(o0.counts)
	}
	m.mhz = round1(m.mhz)
	m.ram = img.ram - img.mainData + depth
	m.flashKB = round1(float64(img.flash) / 1000)
	return m, nil
}

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
		return strings.Contains(s, "internal/fsktx") || strings.Contains(s, "dmrgo/v2/rf.") || strings.HasPrefix(s, "OUTLINED_FUNCTION") || s == "main.step" || s == "main.toDAC"
	})
	if err != nil {
		return measure{}, err
	}
	_ = os.Remove(o.counts)
	secs := float64(txBursts) * 0.03
	return measure{
		mhz:     round1(float64(c.matched) / secs / 1e6),
		ram:     img.symSize("main.mod") + img.symSize("main.pe") + o.depth,
		flashKB: round1(float64(img.flash) / 1000),
	}, nil
}

const txBursts = 66
