//go:build js && wasm

package main

import "syscall/js"

const (
	errKey     = "error"
	errNotOpen = "open first"
)

type bridge struct {
	dec *decoder
	in  []byte
}

func (b *bridge) result(out []byte, err error) any {
	r := map[string]any{"events": string(out)}
	if err != nil {
		r[errKey] = err.Error()
	}
	if b.dec != nil {
		r["seconds"] = b.dec.seconds()
	}
	return r
}

func (b *bridge) open(_ js.Value, args []js.Value) any {
	o := args[0]
	opt := options{format: o.Get("format").String(), audioTier: -1, iqTier: -1}
	if v := o.Get("rate"); v.Truthy() {
		opt.rate = v.Float()
	}
	if v := o.Get("audioTier"); v.Type() == js.TypeNumber {
		opt.audioTier = v.Int()
	}
	if v := o.Get("iqTier"); v.Type() == js.TypeNumber {
		opt.iqTier = v.Int()
	}
	var err error
	b.dec, err = newDecoder(opt)
	return b.result(nil, err)
}

func (b *bridge) write(_ js.Value, args []js.Value) any {
	if b.dec == nil {
		return map[string]any{errKey: errNotOpen}
	}
	src := args[0]
	n := src.Get("length").Int()
	if cap(b.in) < n {
		b.in = make([]byte, n)
	}
	js.CopyBytesToGo(b.in[:n], src)
	err := b.dec.write(b.in[:n])
	return b.result(b.dec.rep.take(), err)
}

func (b *bridge) flush(js.Value, []js.Value) any {
	if b.dec == nil {
		return map[string]any{errKey: errNotOpen}
	}
	b.dec.flush()
	b.dec.rep.emit(b.dec.rep.summary(b.dec.seconds()))
	return b.result(b.dec.rep.take(), nil)
}

func (b *bridge) stats(js.Value, []js.Value) any {
	if b.dec == nil {
		return map[string]any{errKey: errNotOpen}
	}
	b.dec.rep.emit(b.dec.rep.summary(b.dec.seconds()))
	return b.result(b.dec.rep.take(), nil)
}

func main() {
	b := &bridge{}
	js.Global().Set("dmrgo", js.ValueOf(map[string]any{
		"open":  js.FuncOf(b.open),
		"write": js.FuncOf(b.write),
		"flush": js.FuncOf(b.flush),
		"stats": js.FuncOf(b.stats),
	}))
	select {}
}
