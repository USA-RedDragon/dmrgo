// Command capi builds libdmrgo, the C API of dmrgo, declared in dmrgo.h:
//
//	go build -buildmode=c-shared -o libdmrgo.so .
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
