//go:build !nocodec2 && !tinygo && !codec2native && (!cgo || codec2wasm) && !(js && wasm)

package codec2

import (
	"math/rand/v2"
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

func TestCompiledMatchesInterpreted(t *testing.T) {
	t.Parallel()
	interp := interpretedModule(t)
	pcm := corpus(300)
	for _, mode := range []Mode{Mode2400, Mode3200} {
		compiled := newCodec(t, Config{Mode: mode})
		reference := newCodec(t, Config{Mode: mode, WASM: interp})
		got, want := encodeAll(t, compiled, pcm), encodeAll(t, reference, pcm)
		if !slices.Equal(got, want) {
			t.Fatalf("mode %d: the compiled module encodes otherwise than the interpreted one", mode)
		}
		if !slices.Equal(decodeAll(t, compiled, got), decodeAll(t, reference, want)) {
			t.Fatalf("mode %d: the compiled module decodes otherwise than the interpreted one", mode)
		}
	}
}
