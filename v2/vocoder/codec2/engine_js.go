//go:build !nocodec2 && !tinygo && !codec2native && js && wasm

package codec2

import (
	"errors"
	"fmt"
	"syscall/js"
)

const (
	wasiSuccess = 0
	wasiEBADF   = 8
)

var errNoWebAssembly = errors.New("the JavaScript host has no WebAssembly")

type engineModule struct {
	mod js.Value
}

func jsTry(f func() js.Value) (v js.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
				return
			}
			err = fmt.Errorf("%v", r)
		}
	}()
	return f(), nil
}

func compile(wasm []byte) (*engineModule, error) {
	wa := js.Global().Get("WebAssembly")
	if wa.Type() != js.TypeObject {
		return nil, newCauseError("codec2: compile the WebAssembly module", errNoWebAssembly)
	}
	src := js.Global().Get("Uint8Array").New(len(wasm))
	js.CopyBytesToJS(src, wasm)
	mod, err := jsTry(func() js.Value { return wa.Get("Module").New(src) })
	if err != nil {
		return nil, newCauseError("codec2: compile the WebAssembly module", err)
	}
	if !hasExports(wa, mod) {
		return nil, errExports
	}
	return &engineModule{mod: mod}, nil
}

func hasExports(wa, mod js.Value) bool {
	list, err := jsTry(func() js.Value { return wa.Get("Module").Call("exports", mod) })
	if err != nil {
		return false
	}
	kinds := map[string]string{}
	for i := range list.Length() {
		e := list.Index(i)
		kinds[e.Get("name").String()] = e.Get("kind").String()
	}
	if kinds["memory"] != "memory" {
		return false
	}
	for fn := range exportCount {
		if kinds[fn.name()] != "function" {
			return false
		}
	}
	return true
}

func (m *engineModule) close() error {
	m.mod = js.Undefined()
	return nil
}

func (m *engineModule) instantiate() (*engineInstance, error) {
	inst := &engineInstance{}
	inst.wasi = [...]js.Func{
		js.FuncOf(func(js.Value, []js.Value) any { return wasiSuccess }),
		js.FuncOf(func(js.Value, []js.Value) any { return wasiEBADF }),
		js.FuncOf(inst.fdWrite),
	}
	wasi := js.Global().Get("Object").New()
	wasi.Set("fd_close", inst.wasi[0])
	wasi.Set("fd_seek", inst.wasi[1])
	wasi.Set("fd_write", inst.wasi[2])
	imports := js.Global().Get("Object").New()
	imports.Set("wasi_snapshot_preview1", wasi)
	instance, err := jsTry(func() js.Value {
		return js.Global().Get("WebAssembly").Get("Instance").New(m.mod, imports)
	})
	if err != nil {
		inst.close()
		return nil, newCauseError("codec2: instantiate the WebAssembly module", err)
	}
	exports := instance.Get("exports")
	inst.mem = exports.Get("memory")
	for fn := range exportCount {
		f := exports.Get(fn.name())
		if _, params, _ := fn.spec(); f.Type() != js.TypeFunction || f.Get("length").Int() != params {
			inst.close()
			return nil, errExports
		}
		inst.fns[fn] = f
	}
	if init := exports.Get("_initialize"); init.Type() == js.TypeFunction {
		if _, err := jsTry(func() js.Value { return init.Invoke() }); err != nil {
			inst.close()
			return nil, newCauseError("codec2: initialize the WebAssembly module", err)
		}
	}
	return inst, nil
}

type engineInstance struct {
	mem  js.Value
	fns  [exportCount]js.Value
	wasi [3]js.Func
}

func (w *engineInstance) fdWrite(_ js.Value, args []js.Value) any {
	if len(args) != 4 {
		return wasiEBADF
	}
	iovs, n, written := u32(args[1]), u32(args[2]), u32(args[3])
	var total uint32
	var iov [8]byte
	for i := range n {
		if !w.read(iovs+8*i, iov[:]) {
			return wasiEBADF
		}
		total += uint32(iov[4]) | uint32(iov[5])<<8 | uint32(iov[6])<<16 | uint32(iov[7])<<24
	}
	out := [4]byte{byte(total), byte(total >> 8), byte(total >> 16), byte(total >> 24)}
	if !w.write(written, out[:]) {
		return wasiEBADF
	}
	return wasiSuccess
}

func u32(v js.Value) uint32 {
	f := v.Float()
	if f < 0 {
		f += 1 << 32
	}
	return uint32(f)
}

func (w *engineInstance) call(fn export, args ...any) (js.Value, error) {
	return jsTry(func() js.Value { return w.fns[fn].Invoke(args...) })
}

func (w *engineInstance) call1(fn export, a uint32) (uint32, error) {
	r, err := w.call(fn, a)
	if err != nil {
		return 0, err
	}
	if _, _, results := fn.spec(); results == 0 {
		return 0, nil
	}
	return u32(r), nil
}

func (w *engineInstance) call3(fn export, a, b, c uint32) error {
	_, err := w.call(fn, a, b, c)
	return err
}

func (w *engineInstance) view(off uint32, n int) (js.Value, bool) {
	buf := w.mem.Get("buffer")
	if float64(off)+float64(n) > buf.Get("byteLength").Float() {
		return js.Value{}, false
	}
	return js.Global().Get("Uint8Array").New(buf, off, n), true
}

func (w *engineInstance) read(off uint32, dst []byte) bool {
	v, ok := w.view(off, len(dst))
	if !ok {
		return false
	}
	return js.CopyBytesToGo(dst, v) == len(dst)
}

func (w *engineInstance) write(off uint32, src []byte) bool {
	v, ok := w.view(off, len(src))
	if !ok {
		return false
	}
	return js.CopyBytesToJS(v, src) == len(src)
}

func (w *engineInstance) close() {
	for _, f := range w.wasi {
		f.Release()
	}
	w.mem = js.Undefined()
}
