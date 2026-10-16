//go:build !tinygo

package allocgate_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// listedPackage is the part of the go list -json output that the gate
// reads.
type listedPackage struct {
	ImportPath      string
	Dir             string
	Export          string
	CompiledGoFiles []string
	CgoFiles        []string
	Standard        bool
	DepOnly         bool
	Error           *struct{ Err string }
}

// gateEnv is the environment of the go commands of the gate. The gate reads
// amd64 machine code, so it builds for linux/amd64 on every host; cgo stays
// as the environment sets it, and go turns it off when it cross-compiles.
func gateEnv() []string {
	return append(os.Environ(), "GOFLAGS=", "GOOS=linux", "GOARCH=amd64")
}

// listPackages builds the library packages and their dependencies with go
// list -export and returns them in dependency order.
func listPackages(t *testing.T, patterns []string, env ...string) []listedPackage {
	t.Helper()
	root, err := filepath.Abs(moduleRoot)
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{"list", "-deps", "-export", "-compiled", "-json=ImportPath,Dir,Export,CompiledGoFiles,CgoFiles,Standard,DepOnly,Error"}, patterns...)
	cmd := exec.CommandContext(t.Context(), "go", args...)
	cmd.Dir = root
	cmd.Env = append(gateEnv(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.String())
	}
	var pkgs []listedPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p listedPackage
		if err := dec.Decode(&p); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if p.Error != nil {
			t.Fatalf("go list %s: %s", p.ImportPath, p.Error.Err)
		}
		if p.Export == "" && p.ImportPath != "unsafe" {
			t.Fatalf("go list %s: no export file", p.ImportPath)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

// position is a source position as objdump prints it: the base name of the
// file and the line.
type position struct {
	file string
	line int
}

// inst is one machine instruction.
type inst struct {
	pos position
	// op is the mnemonic, such as CALL or JNE.
	op string
	// target is the address that a direct jump goes to, or -1.
	target int64
	// call is the symbol that a CALL, or a JMP that is a tail call, goes
	// to.
	call string
	// indirect is set for a call or jump through a register or memory.
	indirect bool
	addr     int64
	// refs holds the text symbols whose address the instruction takes,
	// for a func value, a closure or a deferred call.
	refs []string
}

// body is the machine code of one TEXT symbol.
type body struct {
	name  string
	file  string
	insts []inst
	// ifaceMethods holds the interface methods that the code calls, as
	// the R_USEIFACEMETHOD relocations name them: "type:pkg.I+off".
	ifaceMethods map[string]bool
}

var (
	textRE  = regexp.MustCompile(`^TEXT (.*)\(SB\) ?(.*)$`)
	relocRE = regexp.MustCompile(`\[\d+:\d+\]R_([A-Z0-9_]+)`)
)

// symName strips the ABI suffix, such as <1>, from a symbol name.
func symName(s string) string {
	if strings.HasSuffix(s, ">") {
		if i := strings.LastIndexByte(s, '<'); i > 0 {
			return s[:i]
		}
	}
	return s
}

// relocSym returns the symbol of a relocation target such as "pkg.f+16".
func relocSym(s string) string {
	if i := strings.LastIndexByte(s, '+'); i > 0 {
		if _, err := strconv.Atoi(s[i+1:]); err == nil {
			s = s[:i]
		}
	}
	return symName(s)
}

// parseObjdump parses the output of go tool objdump.
func parseObjdump(r io.Reader) ([]*body, error) {
	var bodies []*body
	var cur *body
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		line := sc.Text()
		if m := textRE.FindStringSubmatch(line); m != nil {
			cur = &body{name: symName(m[1]), file: m[2], ifaceMethods: map[string]bool{}}
			bodies = append(bodies, cur)
			continue
		}
		if cur == nil || !strings.HasPrefix(line, "  ") {
			continue
		}
		var fields []string
		for f := range strings.SplitSeq(line, "\t") {
			if f = strings.TrimSpace(f); f != "" {
				fields = append(fields, f)
			}
		}
		if len(fields) < 4 {
			return nil, fmt.Errorf("objdump line %q: too few fields", line)
		}
		in := inst{target: -1}
		if i := strings.LastIndexByte(fields[0], ':'); i > 0 {
			in.pos.file = fields[0][:i]
			in.pos.line, _ = strconv.Atoi(fields[0][i+1:])
		}
		addr, err := strconv.ParseInt(strings.TrimPrefix(fields[1], "0x"), 16, 64)
		if err != nil {
			return nil, fmt.Errorf("objdump line %q: %w", line, err)
		}
		in.addr = addr
		asm := fields[3]
		op, operand, _ := strings.Cut(asm, " ")
		in.op = op
		relocs := ""
		if len(fields) > 4 {
			relocs = strings.Join(fields[4:], " ")
		}
		idx := relocRE.FindAllStringSubmatchIndex(relocs, -1)
		for i, ix := range idx {
			end := len(relocs)
			if i+1 < len(idx) {
				end = idx[i+1][0]
			}
			arg := strings.TrimPrefix(strings.TrimSpace(relocs[ix[1]:end]), ":")
			switch relocs[ix[2]:ix[3]] {
			case "CALL":
				in.call = symName(arg)
			case "CALLIND":
				in.indirect = true
			case "USEIFACEMETHOD":
				cur.ifaceMethods[arg] = true
			case "PCREL", "ADDR":
				in.refs = append(in.refs, strings.TrimSuffix(relocSym(arg), "·f"))
			}
		}
		if op == "JMP" || (len(op) > 1 && op[0] == 'J') || strings.HasPrefix(op, "LOOP") {
			switch {
			case in.call != "":
			case strings.HasPrefix(operand, "0x"):
				in.target, err = strconv.ParseInt(strings.TrimPrefix(operand, "0x"), 16, 64)
				if err != nil {
					return nil, fmt.Errorf("objdump line %q: %w", line, err)
				}
			case strings.HasSuffix(operand, "(SB)") && symName(strings.TrimSuffix(operand, "(SB)")) == cur.name:
				in.target = cur.insts[0].addr
			default:
				in.indirect = true
			}
		}
		if op == "CALL" && in.call == "" && !strings.HasPrefix(operand, "0x") {
			in.indirect = true
		}
		cur.insts = append(cur.insts, in)
	}
	return bodies, sc.Err()
}

// objdumpTool returns the path of the objdump binary of the go command.
func objdumpTool(t *testing.T) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "go", "tool", "-n", "objdump").Output()
	if err != nil {
		t.Fatalf("go tool -n objdump: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// disassemble returns the bodies of the TEXT symbols of the export files of
// pkgs, by symbol name. A name can have more than one body: objdump prints
// an ABI wrapper under the name of the function it wraps, and each package
// that instantiates a generic function holds a copy of it.
func disassemble(t *testing.T, pkgs []listedPackage) map[string][]*body {
	t.Helper()
	tool := objdumpTool(t)
	results := make([][]*body, len(pkgs))
	errs := make([]error, len(pkgs))
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	for i, p := range pkgs {
		if p.Export == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			cmd := exec.CommandContext(t.Context(), tool, p.Export)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			out, err := cmd.StdoutPipe()
			if err != nil {
				errs[i] = err
				return
			}
			if err := cmd.Start(); err != nil {
				errs[i] = err
				return
			}
			results[i], errs[i] = parseObjdump(out)
			if err := cmd.Wait(); err != nil {
				errs[i] = fmt.Errorf("%w: %s", err, stderr.String())
			}
		}()
	}
	wg.Wait()
	bodies := map[string][]*body{}
	for i, p := range pkgs {
		if errs[i] != nil {
			t.Fatalf("objdump %s: %v", p.ImportPath, errs[i])
		}
		for _, b := range results[i] {
			if len(b.insts) > 0 {
				bodies[b.name] = append(bodies[b.name], b)
			}
		}
	}
	for name, bs := range bodies {
		// An ABI wrapper only calls the function that it wraps, under the
		// same name, so the function's own body stands for both.
		own := slices.DeleteFunc(slices.Clone(bs), func(b *body) bool { return b.file == "<autogenerated>" })
		if len(own) > 0 {
			bs = own
		}
		// Each package that instantiates a generic function holds the same
		// code for it.
		bodies[name] = slices.CompactFunc(bs, func(a, b *body) bool { return a.file == b.file && len(a.insts) == len(b.insts) })
	}
	if len(bodies) == 0 {
		t.Fatal("objdump found no code")
	}
	return bodies
}
