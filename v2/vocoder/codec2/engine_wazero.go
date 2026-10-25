//go:build !nocodec2 && !tinygo && !codec2native && (!cgo || codec2wasm) && !(js && wasm)

package codec2

import (
	"context"
	"math"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

var sharedRuntime = sync.OnceValues(func() (wazero.Runtime, error) {
	return newRuntime(wazero.NewRuntimeConfig())
})

func newRuntime(cfg wazero.RuntimeConfig) (wazero.Runtime, error) {
	ctx := context.Background()
	rt := wazero.NewRuntimeWithConfig(ctx, cfg)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, newCauseError("codec2: WASI", err)
	}
	return rt, nil
}

type engineModule struct {
	rt       wazero.Runtime
	compiled wazero.CompiledModule
	aot      bool
}

func compileEmbedded() (*engineModule, error) {
	if embeddedAOT {
		return &engineModule{aot: true}, nil
	}
	return compile(embeddedWASM)
}

func compile(wasm []byte) (*engineModule, error) {
	rt, err := sharedRuntime()
	if err != nil {
		return nil, err
	}
	return compileIn(rt, wasm)
}

func compileIn(rt wazero.Runtime, wasm []byte) (*engineModule, error) {
	ctx := context.Background()
	c, err := rt.CompileModule(ctx, wasm)
	if err != nil {
		return nil, newCauseError("codec2: compile the WebAssembly module", err)
	}
	if !hasExports(c) {
		_ = c.Close(ctx)
		return nil, errExports
	}
	return &engineModule{rt: rt, compiled: c}, nil
}

func hasExports(c wazero.CompiledModule) bool {
	if _, ok := c.ExportedMemories()["memory"]; !ok {
		return false
	}
	fns := c.ExportedFunctions()
	for fn := range exportCount {
		name, params, results := fn.spec()
		def, ok := fns[name]
		if !ok || len(def.ParamTypes()) != params || len(def.ResultTypes()) != results {
			return false
		}
		for _, types := range [][]api.ValueType{def.ParamTypes(), def.ResultTypes()} {
			for _, t := range types {
				if t != api.ValueTypeI32 {
					return false
				}
			}
		}
	}
	return true
}

func (w *engineModule) close() error {
	if w.aot {
		return nil
	}
	return w.compiled.Close(context.Background())
}

func (w *engineModule) instantiate() (*engineInstance, error) {
	if w.aot {
		if !embeddedAOT {
			return nil, errExports
		}
		a, err := newAOTInstance()
		if err != nil {
			return nil, err
		}
		return &engineInstance{aot: a}, nil
	}
	mod, err := w.rt.InstantiateModule(context.Background(), w.compiled,
		wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize"))
	if err != nil {
		return nil, newCauseError("codec2: instantiate the WebAssembly module", err)
	}
	inst := &engineInstance{mod: mod, mem: mod.Memory()}
	for fn := range exportCount {
		inst.fns[fn] = mod.ExportedFunction(fn.name())
	}
	return inst, nil
}

type engineInstance struct {
	mod   api.Module
	mem   api.Memory
	fns   [exportCount]api.Function
	stack [3]uint64
	aot   *aotInstance
}

func (w *engineInstance) call1(fn export, a uint32) (uint32, error) {
	if embeddedAOT && w.aot != nil {
		return w.aot.call1(fn, a)
	}
	w.stack[0] = uint64(a)
	if err := w.fns[fn].CallWithStack(context.Background(), w.stack[:]); err != nil {
		return 0, err
	}
	if _, _, results := fn.spec(); results == 0 {
		return 0, nil
	}
	return api.DecodeU32(w.stack[0]), nil
}

func (w *engineInstance) call3(fn export, a, b, c uint32) error {
	if embeddedAOT && w.aot != nil {
		return w.aot.call3(fn, a, b, c)
	}
	w.stack = [3]uint64{uint64(a), uint64(b), uint64(c)}
	return w.fns[fn].CallWithStack(context.Background(), w.stack[:])
}

func (w *engineInstance) read(off uint32, dst []byte) bool {
	if embeddedAOT && w.aot != nil {
		return w.aot.read(off, dst)
	}
	n := uint64(len(dst))
	if n > math.MaxUint32 {
		return false
	}
	b, ok := w.mem.Read(off, uint32(n))
	if !ok {
		return false
	}
	copy(dst, b)
	return true
}

func (w *engineInstance) write(off uint32, src []byte) bool {
	if embeddedAOT && w.aot != nil {
		return w.aot.write(off, src)
	}
	return w.mem.Write(off, src)
}

func (w *engineInstance) close() {
	if embeddedAOT && w.aot != nil {
		return
	}
	_ = w.mod.Close(context.Background())
}
