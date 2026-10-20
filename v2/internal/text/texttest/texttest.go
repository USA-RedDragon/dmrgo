// Package texttest checks generated String methods against their fmt
// reference. Only tests import it.
package texttest

import (
	"math"
	"math/rand"
	"reflect"
	"testing"
)

// Check compares str with ref for the zero value of T and for random
// values.
func Check[T any](t *testing.T, str, ref func(*T) string) {
	t.Helper()
	r := rand.New(rand.NewSource(1))
	for i := range 300 {
		v := new(T)
		if i > 0 {
			fill(reflect.ValueOf(v).Elem(), r, 0)
		}
		if got, want := str(v), ref(v); got != want {
			t.Fatalf("String() = %q\nwant       %q", got, want)
		}
	}
}

func fill(v reflect.Value, r *rand.Rand, depth int) {
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(r.Intn(2) == 1)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		x := r.Int63() >> r.Intn(63)
		if r.Intn(2) == 0 {
			x = -x - 1
		}
		v.SetInt(x)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		v.SetUint(r.Uint64() >> r.Intn(64))
	case reflect.Float32, reflect.Float64:
		switch r.Intn(3) {
		case 0:
			v.SetFloat(float64(r.Int31n(1<<24)-1<<23) * 360 / (1 << 24))
		case 1:
			v.SetFloat(r.NormFloat64() * 100)
		default:
			v.SetFloat(math.Float64frombits(r.Uint64()))
		}
	case reflect.Array:
		for i := range v.Len() {
			fill(v.Index(i), r, depth)
		}
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), r.Intn(5), r.Intn(5)+5)
		for i := range s.Len() {
			fill(s.Index(i), r, depth)
		}
		v.Set(s)
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Field(i).CanSet() {
				fill(v.Field(i), r, depth)
			}
		}
	case reflect.Pointer:
		if depth < 3 && r.Intn(2) == 0 {
			p := reflect.New(v.Type().Elem())
			fill(p.Elem(), r, depth+1)
			v.Set(p)
		}
	default:
	}
}
