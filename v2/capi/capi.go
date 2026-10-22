// Command capi builds libdmrgo, the C API of dmrgo, declared in dmrgo.h.
// make writes build/libdmrgo.so.N with the soname, the symbol versions of
// libdmrgo.map and the dmrgo_version string, and the link libdmrgo.so:
//
//	make
//
// go build -buildmode=c-shared builds the library without them.
//
// make static writes build/static/libdmrgo.a, one object in which only the
// functions of libdmrgo.map are global, so that the Go runtime and the
// internals of libdmrgo do not clash with the symbols of the program. A
// program links it with -lpthread, -lm and the codec2 library:
//
//	make static
//	cc prog.c -I. build/static/libdmrgo.a $(pkg-config --libs codec2) -lpthread -lm
//
// Its own module keeps cgo and the C toolchain out of the dmrgo module.
package main

import (
	"runtime/cgo"
	"unsafe"
)

func main() {}

const handleBase = 0x100000

func handleOf(p unsafe.Pointer) (cgo.Handle, bool) {
	v := uintptr(p)
	if v < handleBase || (v-handleBase)%16 != 0 {
		return 0, false
	}
	return cgo.Handle((v - handleBase) / 16), true
}
