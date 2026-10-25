//go:build !nocodec2 && !tinygo && !codec2native && !(js && wasm)

package codec2

import (
	"errors"
	"math/rand/v2"
	"os"
	"slices"
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

func TestEnginesMatch(t *testing.T) {
	t.Parallel()
	engines := []struct {
		name string
		m    *Module
	}{
		{"embedded", nil},
		{"wazero", wazeroModule(t)},
	}
	if embeddedAOT {
		engines = append(engines, struct {
			name string
			m    *Module
		}{"aot", aotModule()})
	}
	interp := interpretedModule(t)
	pcm := corpus(300)
	for _, mode := range []Mode{Mode2400, Mode3200} {
		ref := newCodecOf(t, Config{Mode: mode, WASM: interp})
		want := encodeAll(t, ref, pcm)
		wantPCM := decodeAll(t, ref, want)
		wantAuto := decodeAll(t, newCodecOf(t, Config{WASM: interp}), want)
		for _, e := range engines {
			c := newCodecOf(t, Config{Mode: mode, WASM: e.m})
			if c.Silence() != ref.Silence() {
				t.Errorf("%s, mode %d: Silence differs from the interpreter", e.name, mode)
			}
			if got := encodeAll(t, c, pcm); !slices.Equal(got, want) {
				t.Errorf("%s, mode %d: encodes otherwise than the interpreter", e.name, mode)
			}
			if !slices.Equal(decodeAll(t, c, want), wantPCM) {
				t.Errorf("%s, mode %d: decodes otherwise than the interpreter", e.name, mode)
			}
			if !slices.Equal(decodeAll(t, newCodecOf(t, Config{WASM: e.m}), want), wantAuto) {
				t.Errorf("%s, mode %d: decodes in ModeAuto otherwise than the interpreter", e.name, mode)
			}
		}
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
