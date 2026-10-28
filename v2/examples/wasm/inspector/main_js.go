//go:build js && wasm

package main

import "syscall/js"

func main() {
	js.Global().Set("dmrgoInspect", js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) < 1 {
			return `{"errors":["no input"]}`
		}
		in := make([]byte, args[0].Get("length").Int())
		js.CopyBytesToGo(in, args[0])
		var opt Options
		if len(args) > 1 {
			opt.Format = args[1].String()
		}
		if len(args) > 2 {
			opt.Protocol = args[2].String()
		}
		return InspectJSON(in, opt)
	}))
	if cb := js.Global().Get("dmrgoReady"); cb.Type() == js.TypeFunction {
		cb.Invoke()
	}
	select {}
}
