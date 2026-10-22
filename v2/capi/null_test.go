package main

import (
	"reflect"
	"slices"
	"testing"
	"unsafe"
)

const errPanic = -4

const zeroedLen = 256

// TestNullArguments calls every generated entry point with all arguments
// NULL or zero, and again with each pointer argument pointing to
// zeroedLen zeroed elements, more than the longest C array parameter such
// as the uint8_t out[32] that dmrgo_dmrd_auth_token fills, and fails for
// each call that returns DMRGO_ERR_PANIC. Function pointers and handles,
// whose C type is incomplete, stay NULL.
func TestNullArguments(t *testing.T) {
	t.Parallel()

	names := make([]string, 0, len(entryPoints))
	for n := range entryPoints {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, name := range names {
		f := reflect.ValueOf(entryPoints[name])
		ft := f.Type()
		for _, zeroed := range []bool{false, true} {
			args := make([]reflect.Value, ft.NumIn())
			for i := range args {
				at := ft.In(i)
				args[i] = reflect.New(at).Elem()
				if zeroed && at.Kind() == reflect.Pointer && (at.Elem().Kind() != reflect.Array || at.Elem().Len() != 0) {
					if p, ok := zeroedMemory(at.Elem()); ok {
						args[i] = p
					}
				}
			}
			out := f.Call(args)
			if len(out) == 1 && out[0].CanInt() && out[0].Int() == errPanic {
				t.Errorf("%s (zeroed pointers %t): DMRGO_ERR_PANIC", name, zeroed)
			}
		}
	}
}

// TestHugeLengths calls every generated entry point with each unsigned
// argument, such as a length or a capacity, at its largest value, each
// pointer argument pointing to zeroed memory and each arena pointing to a
// buffer, and fails for each call that returns DMRGO_ERR_PANIC.
func TestHugeLengths(t *testing.T) {
	t.Parallel()

	names := make([]string, 0, len(entryPoints))
	for n := range entryPoints {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, name := range names {
		f := reflect.ValueOf(entryPoints[name])
		ft := f.Type()
		args := make([]reflect.Value, ft.NumIn())
		for i := range args {
			at := ft.In(i)
			args[i] = reflect.New(at).Elem()
			switch at.Kind() {
			case reflect.Uint, reflect.Uint64, reflect.Uintptr, reflect.Uint32:
				args[i].SetUint(^uint64(0) >> (64 - 8*at.Size()))
			case reflect.Pointer:
				if at.Elem().Kind() != reflect.Array || at.Elem().Len() != 0 {
					if p, ok := zeroedMemory(at.Elem()); ok {
						args[i] = p
					}
				}
			case reflect.UnsafePointer:
				args[i] = reflect.ValueOf(unsafe.Pointer(&make([]byte, 4096)[0]))
			default:
			}
		}
		out := f.Call(args)
		if len(out) == 1 && out[0].CanInt() && out[0].Int() == errPanic {
			t.Errorf("%s: DMRGO_ERR_PANIC", name)
		}
	}
}

func zeroedMemory(t reflect.Type) (p reflect.Value, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return reflect.MakeSlice(reflect.SliceOf(t), zeroedLen, zeroedLen).Index(0).Addr(), true
}
