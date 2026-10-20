package rf

import (
	"reflect"
	"testing"
)

const tinyGoMaxStackAlloc = 256

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

func TestOptionsSize(t *testing.T) {
	t.Parallel()
	if n := tinyGoARMSize(t, reflect.TypeFor[options]()); n > tinyGoMaxStackAlloc {
		t.Errorf("options takes %d bytes on 32-bit ARM, more than %d: keep its fields in order of size", n, tinyGoMaxStackAlloc)
	}
}
