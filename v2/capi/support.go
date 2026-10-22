package main

/*
#define DMRGO_NO_PROTOTYPES
#define DMRGO_USER_T uintptr_t
#include "dmrgo.h"
#include "dmrgo_gen_internal.h"

void *dmrgo__ptr(uintptr_t h);
const char *dmrgo_strerror(int status);
*/
import "C"

import (
	"fmt"
	"math"
	"reflect"
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

type statusError C.int

func (e statusError) Error() string {
	return fmt.Sprintf("dmrgo: C status %d is not a status of the C API", int(e))
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

func newUser(v any) C.uintptr_t {
	return C.uintptr_t(cgo.NewHandle(v))
}

func newHandle(v any) unsafe.Pointer {
	return C.dmrgo__ptr(C.uintptr_t(cgo.NewHandle(v)))
}

func deleteHandle(p unsafe.Pointer) {
	if h, ok := handleOf(p); ok {
		h.Delete()
	}
}

func lenOK(p unsafe.Pointer, n C.size_t, size uintptr) bool {
	if n == 0 {
		return true
	}
	if p == nil || uint64(n) > uint64(math.MaxInt)/uint64(max(size, 1)) {
		return false
	}
	return uintptr(n)*max(size, 1) <= ^uintptr(0)-uintptr(p)
}

const maxGrow = 1 << 24

func growOK(need C.size_t, have int) bool {
	return need <= maxGrow && int(need) > have
}

func sizedIn(dst, src unsafe.Pointer, n uintptr) bool {
	size := uintptr(*(*C.size_t)(src))
	if size < unsafe.Sizeof(C.size_t(0)) {
		return false
	}
	k := min(size, n)
	copy(unsafe.Slice((*byte)(dst), k), unsafe.Slice((*byte)(src), k))
	return true
}

func sizedOut(dst, src unsafe.Pointer, n uintptr) {
	size := *(*C.size_t)(dst)
	k := min(uintptr(size), n)
	copy(unsafe.Slice((*byte)(dst), k), unsafe.Slice((*byte)(src), k))
	*(*C.size_t)(dst) = size
}

func cSlice[E any](p unsafe.Pointer, n C.size_t) []E {
	var z E
	if !lenOK(p, n, unsafe.Sizeof(z)) {
		panic(argPanic{})
	}
	if p == nil {
		return nil
	}
	return unsafe.Slice((*E)(p), int(n))
}

func cStr(p *C.char, n C.size_t) string {
	if !lenOK(unsafe.Pointer(p), n, 1) {
		panic(argPanic{})
	}
	if n == 0 {
		return ""
	}
	return string(unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n)))
}

func fail(code C.int) C.int {
	C.dmrgo__fail(code)
	return code
}

func setLast(code C.int, msg string) C.int {
	l := C.dmrgo__last()
	l.code = code
	l.kind = 0
	l.chain_len = 0
	l.msg_len = C.size_t(len(msg))
	b := unsafe.Slice((*byte)(unsafe.Pointer(&l.msg[0])), len(l.msg))
	b[copy(b[:len(b)-1], msg)] = 0
	return code
}

func cloneDeep[T any](v T) T {
	rv := reflect.ValueOf(&v).Elem()
	cloneValue(rv)
	return v
}

func cloneValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Slice:
		if v.IsNil() || !v.CanSet() {
			return
		}
		n := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		reflect.Copy(n, v)
		for i := range n.Len() {
			cloneValue(n.Index(i))
		}
		v.Set(n)
	case reflect.Pointer:
		if v.IsNil() || !v.CanSet() {
			return
		}
		n := reflect.New(v.Type().Elem())
		n.Elem().Set(v.Elem())
		cloneValue(n.Elem())
		v.Set(n)
	case reflect.Struct:
		for i := range v.NumField() {
			if f := v.Field(i); f.CanSet() {
				cloneValue(f)
			}
		}
	case reflect.Array:
		for i := range v.Len() {
			cloneValue(v.Index(i))
		}
	default:
	}
}

func errCode(err error) C.int {
	code := C.int(C.DMRGO_ERR_OTHER)
	errWalk(err, func(c C.int) bool {
		code = c
		return true
	})
	return code
}

func setChain(err error) {
	l := C.dmrgo__last()
	errWalk(err, func(c C.int) bool {
		for _, x := range l.chain[:l.chain_len] {
			if x == c {
				return false
			}
		}
		if int(l.chain_len) < len(l.chain) {
			l.chain[l.chain_len] = c
			l.chain_len++
		}
		return false
	})
}

//export dmrgo_last_error_is
func dmrgo_last_error_is(code C.int, out *C.bool) C.int {
	if out == nil {
		return C.DMRGO_ERR_ARG
	}
	l := C.dmrgo__last()
	is := code != C.DMRGO_OK && code == l.code
	for _, x := range l.chain[:l.chain_len] {
		is = is || x == code
	}
	*out = C.bool(is)
	return C.DMRGO_OK
}

type outc struct {
	short  bool
	ashort bool
	arena  []byte
	need   int
	slack  int
	seen   map[[3]uintptr]unsafe.Pointer
}

func (o *outc) reported() int {
	if o.ashort {
		return o.need + o.slack
	}
	return o.need
}

func (o *outc) setArena(p unsafe.Pointer, n C.size_t) {
	if p != nil && n > 0 {
		o.arena = cSlice[byte](p, n)
	}
}

func (o *outc) take(size, align int) unsafe.Pointer {
	if size == 0 {
		return nil
	}
	base := uintptr(unsafe.Pointer(unsafe.SliceData(o.arena)))
	off := o.need
	if pad := int((base + uintptr(off)) % uintptr(align)); pad != 0 {
		off += align - pad
	}
	o.need = off + size
	o.slack += align - 1
	if o.need > len(o.arena) {
		o.short = true
		o.ashort = true
		return nil
	}
	return unsafe.Pointer(&o.arena[off])
}

func alignOf[E any]() uintptr {
	var z E
	return max(unsafe.Alignof(z), cAlign((*E)(nil)))
}

func arenaCopy[E any](o *outc, s []E) unsafe.Pointer {
	var z E
	var key [3]uintptr
	if len(s) > 0 {
		key = [3]uintptr{uintptr(unsafe.Pointer(&s[0])), uintptr(len(s)) * unsafe.Sizeof(z), alignOf[E]()}
		if p, ok := o.seen[key]; ok {
			return p
		}
	}
	p := o.take(len(s)*int(unsafe.Sizeof(z)), int(alignOf[E]()))
	if p != nil {
		copy(unsafe.Slice((*E)(p), len(s)), s)
		if o.seen == nil {
			o.seen = map[[3]uintptr]unsafe.Pointer{}
		}
		o.seen[key] = p
	}
	return p
}

func arenaTake[E any](o *outc, n int) []E {
	var z E
	p := o.take(n*int(unsafe.Sizeof(z)), int(alignOf[E]()))
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
	if !lenOK(unsafe.Pointer(buf), capx, 1) {
		return C.DMRGO_ERR_ARG
	}
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

const devel = "(devel)"

var version string

func moduleVersion() string {
	if version != "" {
		return version
	}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "(unknown)"
	}
	v := devel
	for _, d := range bi.Deps {
		if d.Path == modulePath {
			v = d.Version
			if d.Replace != nil && d.Replace.Version != "" {
				v = d.Replace.Version
			}
		}
	}
	if v != devel && v != "" {
		return v
	}
	rev, dirty := "", ""
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = "-dirty"
			}
		}
	}
	if rev != "" {
		return devel + " " + rev[:min(len(rev), 12)] + dirty
	}
	return devel
}

//export dmrgo_version
func dmrgo_version(buf *C.char, capx C.size_t, n, need *C.size_t) (rc C.int) {
	defer recoverTo(&rc)
	if !lenOK(unsafe.Pointer(buf), capx, 1) {
		return fail(C.DMRGO_ERR_ARG)
	}
	oc := &outc{}
	putStr(buf, capx, n, need, moduleVersion(), oc)
	if oc.short {
		return fail(C.DMRGO_ERR_SHORT)
	}
	return C.DMRGO_OK
}
