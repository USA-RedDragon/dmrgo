package main

/*
#define DMRGO_NO_PROTOTYPES
#include "dmrgo.h"
#include "dmrgo_gen_internal.h"

void *dmrgo__ptr(uintptr_t h);
const char *dmrgo_strerror(int status);
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"runtime/debug"
	"strings"
	"unsafe"
)

const modulePath = "github.com/USA-RedDragon/dmrgo/v2"

type handlePanic struct{}

type argPanic struct{}

type codeError C.int

func (e codeError) Error() string {
	return fmt.Sprintf("dmrgo: C status %d: %s", int(e), C.GoString(C.dmrgo_strerror(C.int(e))))
}

func recoverTo(rc *C.int) {
	r := recover()
	switch v := r.(type) {
	case nil:
	case handlePanic:
		*rc = fail(C.DMRGO_ERR_HANDLE)
	case argPanic:
		*rc = fail(C.DMRGO_ERR_ARG)
	case string:
		if strings.Contains(v, "invalid Handle") {
			*rc = fail(C.DMRGO_ERR_HANDLE)
			return
		}
		*rc = setLast(C.DMRGO_ERR_PANIC, "dmrgo: panic: "+v)
	default:
		*rc = setLast(C.DMRGO_ERR_PANIC, fmt.Sprintf("dmrgo: panic: %v", v))
	}
}

func lookup[T any](p unsafe.Pointer) T {
	h, ok := handleOf(p)
	if !ok {
		panic(handlePanic{})
	}
	v, ok := h.Value().(T)
	if !ok {
		panic(handlePanic{})
	}
	return v
}

func hvalue[T any](h uintptr) T {
	if h == 0 {
		panic(handlePanic{})
	}
	v, ok := cgo.Handle(h).Value().(T)
	if !ok {
		panic(handlePanic{})
	}
	return v
}

func newHandle(v any) unsafe.Pointer {
	return C.dmrgo__ptr(C.uintptr_t(cgo.NewHandle(v)))
}

func newUser(v any) C.uintptr_t {
	return C.uintptr_t(cgo.NewHandle(v))
}

func deleteHandle(p unsafe.Pointer) {
	if h, ok := handleOf(p); ok {
		h.Delete()
	}
}

func fail(code C.int) C.int {
	C.dmrgo__fail(code)
	return code
}

func setLast(code C.int, msg string) C.int {
	l := C.dmrgo__last()
	l.code = code
	l.kind = 0
	l.msg_len = C.size_t(len(msg))
	b := unsafe.Slice((*byte)(unsafe.Pointer(&l.msg[0])), len(l.msg))
	b[copy(b[:len(b)-1], msg)] = 0
	return code
}

type outc struct {
	short  bool
	ashort bool
	arena  []byte
	need   int
	slack  int
}

func (o *outc) reported() int {
	if o.ashort {
		return o.need + o.slack
	}
	return o.need
}

func (o *outc) setArena(p unsafe.Pointer, n C.size_t) {
	if p != nil && n > 0 {
		o.arena = unsafe.Slice((*byte)(p), int(n))
	}
}

func (o *outc) take(size, align int) unsafe.Pointer {
	base := uintptr(unsafe.Pointer(unsafe.SliceData(o.arena)))
	off := o.need
	if pad := int((base + uintptr(off)) % uintptr(align)); pad != 0 {
		off += align - pad
	}
	o.need = off + size
	if size == 0 {
		return nil
	}
	o.slack += align - 1
	if o.need > len(o.arena) {
		o.short = true
		o.ashort = true
		return nil
	}
	return unsafe.Pointer(&o.arena[off])
}

func arenaCopy[E any](o *outc, s []E) unsafe.Pointer {
	var z E
	p := o.take(len(s)*int(unsafe.Sizeof(z)), int(unsafe.Alignof(z)))
	if p != nil {
		copy(unsafe.Slice((*E)(p), len(s)), s)
	}
	return p
}

func arenaTake[E any](o *outc, n int) []E {
	var z E
	p := o.take(n*int(unsafe.Sizeof(z)), int(unsafe.Alignof(z)))
	if p == nil {
		return nil
	}
	s := unsafe.Slice((*E)(p), n)
	clear(s)
	return s
}

func arenaStr(o *outc, s string) *C.char {
	p := o.take(len(s)+1, 1)
	if p == nil {
		return nil
	}
	b := unsafe.Slice((*byte)(p), len(s)+1)
	b[copy(b, s)] = 0
	return (*C.char)(p)
}

func putStr(buf *C.char, capx C.size_t, n, need *C.size_t, s string, oc *outc) {
	if need != nil {
		*need = C.size_t(len(s) + 1)
	}
	if len(s) >= int(capx) {
		oc.short = true
	}
	w := 0
	if buf != nil && capx > 0 {
		b := unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(capx))
		w = copy(b[:len(b)-1], s)
		b[w] = 0
	}
	if n != nil {
		*n = C.size_t(w)
	}
}

func putSlice[E any](dst []E, n, need *C.size_t, src []E, oc *outc) {
	if need != nil {
		*need = C.size_t(len(src))
	}
	if len(src) > len(dst) {
		oc.short = true
	}
	*n = C.size_t(copy(dst, src))
}

func putAppend[E any](full []E, fill int, need *C.size_t, src []E, oc *outc) int {
	if need != nil {
		*need = C.size_t(len(src))
	}
	if len(src) > len(full) {
		oc.short = true
		return fill
	}
	if len(src) > 0 && &src[0] != &full[0] {
		copy(full, src)
	}
	return len(src)
}

//export dmrgo_last_error_message
func dmrgo_last_error_message(buf *C.char, capx C.size_t, n, need *C.size_t) C.int {
	l := C.dmrgo__last()
	msg := unsafe.String((*byte)(unsafe.Pointer(&l.msg[0])), min(int(l.msg_len), len(l.msg)-1))
	oc := &outc{}
	putStr(buf, capx, n, need, msg, oc)
	if oc.short || int(l.msg_len) > len(msg) {
		return C.DMRGO_ERR_SHORT
	}
	return C.DMRGO_OK
}

//export dmrgo_last_error_code
func dmrgo_last_error_code() C.int {
	return C.dmrgo__last().code
}

//export dmrgo_version
func dmrgo_version(buf *C.char, capx C.size_t, n, need *C.size_t) (rc C.int) {
	defer recoverTo(&rc)
	oc := &outc{}
	v := "(unknown)"
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, d := range bi.Deps {
			if d.Path == modulePath {
				v = d.Version
				if d.Replace != nil && d.Replace.Version != "" {
					v = d.Replace.Version
				}
			}
		}
	}
	putStr(buf, capx, n, need, v, oc)
	if oc.short {
		return fail(C.DMRGO_ERR_SHORT)
	}
	return C.DMRGO_OK
}
