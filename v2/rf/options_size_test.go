package rf

import (
	"reflect"
	"testing"
)

// tinyGoMaxStackAlloc is the largest allocation that TinyGo moves from the
// heap to the stack on a target with the default stack size of the
// Cortex-M targets.
const tinyGoMaxStackAlloc = 256

// tinyGoARMSize returns the size of t with the layout of LLVM on 32-bit
// ARM, where a type of 8 bytes aligns to 8.
func tinyGoARMSize(tb testing.TB, t reflect.Type) int {
	off := 0
	for i := range t.NumField() {
		var n int
		switch k := t.Field(i).Type.Kind(); k {
		case reflect.Bool, reflect.Int8, reflect.Uint8:
			n = 1
		case reflect.Int16, reflect.Uint16:
			n = 2
		case reflect.Int, reflect.Uint, reflect.Int32, reflect.Uint32, reflect.Float32:
			n = 4
		case reflect.Int64, reflect.Uint64, reflect.Float64:
			n = 8
		default:
			tb.Fatalf("field %s of %s has kind %s, which tinyGoARMSize does not lay out", t.Field(i).Name, t, k)
		}
		off = (off+n-1)/n*n + n
	}
	return (off + 7) / 8 * 8
}

// TestOptionsSize checks that a copy of options stays on the stack under
// TinyGo. A DiscReceiver with options on the heap does not fold its
// settings into its code: the Tier 0 microcontroller build then links the
// float64 math.Log of the click floor and grows by 8 KiB of flash.
func TestOptionsSize(t *testing.T) {
	if n := tinyGoARMSize(t, reflect.TypeFor[options]()); n > tinyGoMaxStackAlloc {
		t.Errorf("options takes %d bytes on 32-bit ARM, more than %d: keep its fields in order of size", n, tinyGoMaxStackAlloc)
	}
}
