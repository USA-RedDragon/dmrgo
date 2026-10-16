//go:build !tinygo

package layer2

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// fillNonZero sets v, a settable value, to a value that is not zero.
func fillNonZero(t *testing.T, v reflect.Value) {
	t.Helper()
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1)
	case reflect.Array:
		fillNonZero(t, v.Index(0))
	case reflect.Struct:
		for i := range v.NumField() {
			f := v.Field(i)
			fillNonZero(t, reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem())
		}
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
	case reflect.Interface:
		v.Set(reflect.ValueOf(&pdu.PRFill{}))
	default:
		t.Fatalf("cannot fill a %s", v.Type())
	}
}

// TestBurstReset fills every field of a Burst and checks that reset zeroes
// all but the decode store, so that a field added to Burst without a line in
// reset fails here.
func TestBurstReset(t *testing.T) {
	t.Parallel()

	var b Burst
	fillNonZero(t, reflect.ValueOf(&b).Elem())
	var data RawBurst
	data[0] = 0xA5
	b.reset(data)
	v, typ := reflect.ValueOf(&b).Elem(), reflect.TypeOf(b)
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		f := v.Field(i)
		switch name {
		case "store":
			if f.IsZero() {
				t.Error("reset cleared the decode store")
			}
		case "raw":
			if b.raw != data {
				t.Error("reset did not set the raw burst")
			}
		default:
			if !f.IsZero() {
				t.Errorf("reset left %s set", name)
			}
		}
	}
}
