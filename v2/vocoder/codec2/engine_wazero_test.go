//go:build !nocodec2 && !tinygo && !codec2native && !(js && wasm)

package codec2

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"maps"
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/tetratelabs/wazero"
)

func interpretedModule(t *testing.T) *Module {
	t.Helper()
	rt, err := newRuntime(wazero.NewRuntimeConfigInterpreter())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close(t.Context()) })
	c, err := compileIn(rt, embeddedWASM)
	if err != nil {
		t.Fatal(err)
	}
	return &Module{compiled: c}
}

func wazeroModule(t *testing.T) *Module {
	t.Helper()
	m, err := NewModule(embeddedWASM)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func aotModule() *Module {
	return &Module{compiled: &engineModule{aot: true}}
}

func testModule(t *testing.T) *Module {
	t.Helper()
	switch e := os.Getenv("CODEC2_ENGINE"); e {
	case "":
		return nil
	case "wazero":
		return wazeroModule(t)
	case "interpreter":
		return interpretedModule(t)
	default:
		t.Fatalf("CODEC2_ENGINE=%s: want wazero or interpreter", e)
		return nil
	}
}

func allocModules(*testing.T) []*Module {
	if !embeddedAOT {
		return []*Module{nil}
	}
	return []*Module{nil, aotModule()}
}

func corpus(frames int) []int16 {
	rng := rand.New(rand.NewPCG(7, 7)) //nolint:gosec
	pcm := voiced(frames)
	for i := range pcm {
		switch (i / FrameSamples / 25) % 3 {
		case 1:
			pcm[i] = int16(rng.IntN(20000) - 10000)
		case 2:
			pcm[i] /= 16
		}
	}
	return pcm
}

var update = flag.Bool("update", false, "rewrite testdata/engines.txt from the wazero interpreter")

const enginesGolden = "testdata/engines.txt"

func wazeroCompiles() bool {
	return runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"
}

var engineModes = []Mode{Mode2400, Mode3200}

var engineCorpus = sync.OnceValue(func() []int16 { return corpus(300) })

func engineKeys() []string {
	ops := []string{"silence", "encode", "decode", "auto"}
	keys := make([]string, 1, 1+len(ops)*len(engineModes))
	keys[0] = "corpus"
	for _, mode := range engineModes {
		for _, op := range ops {
			keys = append(keys, strconv.Itoa(int(mode))+"/"+op)
		}
	}
	return keys
}

func digestFrames(frames [][9]byte) string {
	h := sha256.New()
	for _, f := range frames {
		h.Write(f[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

func engineDigest(t *testing.T, m *Module, key string, frames func(Mode) [][9]byte) string {
	t.Helper()
	if key == "corpus" {
		return digestPCM(engineCorpus())
	}
	ms, op, _ := strings.Cut(key, "/")
	n, err := strconv.Atoi(ms)
	if err != nil {
		t.Fatalf("key %q", key)
	}
	mode := Mode(n)
	switch op {
	case "silence":
		s := newCodecOf(t, Config{Mode: mode, WASM: m}).Silence()
		return digestBytes(s[:])
	case "encode":
		return digestFrames(encodeAll(t, newCodecOf(t, Config{Mode: mode, WASM: m}), engineCorpus()))
	case "decode":
		return digestPCM(decodeAll(t, newCodecOf(t, Config{Mode: mode, WASM: m}), frames(mode)))
	case "auto":
		return digestPCM(decodeAll(t, newCodecOf(t, Config{WASM: m}), frames(mode)))
	}
	t.Fatalf("key %q", key)
	return ""
}

func engineDigests(t *testing.T, m *Module) map[string]string {
	t.Helper()
	enc := map[Mode][][9]byte{}
	for _, mode := range engineModes {
		enc[mode] = encodeAll(t, newCodecOf(t, Config{Mode: mode, WASM: m}), engineCorpus())
	}
	d := map[string]string{}
	for _, k := range engineKeys() {
		d[k] = engineDigest(t, m, k, func(mode Mode) [][9]byte { return enc[mode] })
	}
	return d
}

type goldenFrames struct {
	once   sync.Once
	frames [][9]byte
	err    error
}

func (g *goldenFrames) get(mode Mode, want string) ([][9]byte, error) {
	g.once.Do(func() {
		c, err := New(&Config{Mode: mode})
		if err != nil {
			g.err = err
			return
		}
		defer func() { _ = c.Close() }()
		pcm := engineCorpus()
		for off := 0; off < len(pcm); off += FrameSamples {
			slot, err := c.Encode(pcm[off : off+FrameSamples])
			if err != nil {
				g.err = err
				return
			}
			g.frames = append(g.frames, slot)
		}
		if got := digestFrames(g.frames); got != want {
			g.err = errors.New("embedded engine frames digest " + got + ", want " + want)
		}
	})
	return g.frames, g.err
}

func digestBytes(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func digestPCM(pcm []int16) string {
	b := make([]byte, 0, 2*len(pcm))
	for _, v := range pcm {
		b = binary.LittleEndian.AppendUint16(b, uint16(v))
	}
	return digestBytes(b)
}

func readEngineGolden(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(enginesGolden)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{}
	for line := range strings.Lines(string(data)) {
		k, v, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			t.Fatalf("%s: bad line %q", enginesGolden, line)
		}
		want[k] = v
	}
	return want
}

func TestEnginesMatch(t *testing.T) {
	t.Parallel()
	if *update {
		got := engineDigests(t, interpretedModule(t))
		keys := slices.Sorted(maps.Keys(got))
		var sb strings.Builder
		for _, k := range keys {
			sb.WriteString(k + " " + got[k] + "\n")
		}
		if err := os.WriteFile(enginesGolden, []byte(sb.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want := readEngineGolden(t)
	type engine struct {
		name string
		m    func(*testing.T) *Module
	}
	engines := []engine{{"embedded", func(*testing.T) *Module { return nil }}}
	if embeddedAOT {
		engines = append(engines, engine{"aot", func(*testing.T) *Module { return aotModule() }})
	}
	env := os.Getenv("CODEC2_ENGINE")
	if wazeroCompiles() || env == "wazero" {
		engines = append(engines, engine{"wazero", wazeroModule})
	}
	if wazeroCompiles() || env == "interpreter" {
		engines = append(engines, engine{"interpreter", interpretedModule})
	}
	golden := map[Mode]*goldenFrames{}
	for _, mode := range engineModes {
		golden[mode] = &goldenFrames{}
	}
	keys := engineKeys()
	if len(keys) != len(want) {
		t.Errorf("%d digests, %s has %d", len(keys), enginesGolden, len(want))
	}
	for _, e := range engines {
		t.Run(e.name, func(t *testing.T) {
			t.Parallel()
			m := e.m(t)
			for _, k := range keys {
				t.Run(strings.ReplaceAll(k, "/", "_"), func(t *testing.T) {
					t.Parallel()
					frames := func(mode Mode) [][9]byte {
						f, err := golden[mode].get(mode, want[strconv.Itoa(int(mode))+"/encode"])
						if err != nil {
							t.Fatal(err)
						}
						return f
					}
					w, ok := want[k]
					if !ok {
						t.Fatalf("%s has no digest %s", enginesGolden, k)
					}
					if got := engineDigest(t, m, k, frames); got != w {
						t.Errorf("%s: digest %s, want %s from the interpreter", k, got, w)
					}
				})
			}
		})
	}
}

func TestAOTTrap(t *testing.T) {
	t.Parallel()
	if !embeddedAOT {
		if _, err := aotModule().instantiate(); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("instantiate of the translated module in a build without it: %v", err)
		}
		t.Skip("this build runs the embedded module in wazero; -tags codec2aot selects the translated engine")
	}
	inst, err := aotModule().instantiate()
	if err != nil {
		t.Fatal(err)
	}
	if err := inst.call3(fnEncode, 0xFFFFFFF0, 0xFFFFFFF0, 0xFFFFFFF0); !errors.Is(err, ErrUnavailable) {
		t.Errorf("encode with a state outside the memory: %v, want a trap", err)
	}
	if _, err := inst.call1(fnFree, 0xFFFFFFF0); !errors.Is(err, ErrUnavailable) {
		t.Errorf("free of a pointer outside the memory: %v, want a trap", err)
	}
	var b [4]byte
	if inst.read(0xFFFFFFFE, b[:]) || inst.write(0xFFFFFFFE, b[:]) {
		t.Error("read or write past the memory succeeded")
	}
	st, err := inst.call1(fnCreate, Mode2400.wasmMode())
	if err != nil || st == 0 {
		t.Fatalf("codec2_create after a trap: %d, %v", st, err)
	}
}

func TestAOTWASI(t *testing.T) {
	t.Parallel()
	mem := make([]byte, 64)
	w := &aotWASI{mem: &mem}
	copy(mem[16:], []byte{0, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0, 7, 0, 0, 0})
	if r := w.Xfd_write(2, 16, 2, 40); r != wasiSuccess || mem[40] != 12 {
		t.Errorf("fd_write: %d, wrote %d, want 0 and 12", r, mem[40])
	}
	for _, c := range [][3]int32{{60, 1, 0}, {16, 2, 62}, {-8, 1, 0}, {16, 1 << 30, 0}} {
		if r := w.Xfd_write(2, c[0], c[1], c[2]); r != wasiEBADF {
			t.Errorf("fd_write(iovs %d, n %d, written %d) = %d, want EBADF", c[0], c[1], c[2], r)
		}
	}
	if w.Xfd_close(2) != wasiSuccess || w.Xfd_seek(2, 0, 0, 0) != wasiEBADF {
		t.Error("fd_close or fd_seek")
	}
}
