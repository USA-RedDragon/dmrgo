//go:build !tinygo

package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	nopanicRounds = 256
	nopanicDepth  = 4
)

type nopanicGen struct {
	rng *rand.Rand
}

func (g *nopanicGen) intOf(bits int, signed bool) int64 {
	switch g.rng.IntN(8) {
	case 0:
		return 0
	case 1:
		return 1
	case 2:
		if signed {
			return -1
		}
		return 2
	case 3:
		if signed {
			return int64(-1) << (bits - 1)
		}
		return 0
	case 4:
		if signed {
			return int64(uint64(1)<<(bits-1) - 1)
		}
		return int64(uint64(1)<<(bits-1) - 1)
	case 5:
		return int64(g.rng.IntN(300))
	case 6:
		return int64(g.rng.IntN(1 << 16))
	}
	return g.rng.Int64() >> (64 - bits)
}

func (g *nopanicGen) value(t reflect.Type, depth int, zero bool) reflect.Value {
	v := reflect.New(t).Elem()
	if zero {
		return v
	}
	g.fill(v, depth)
	return v
}

func (g *nopanicGen) fill(v reflect.Value, depth int) {
	t := v.Type()
	switch t.Kind() {
	case reflect.Bool:
		v.SetBool(g.rng.IntN(2) == 1)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(g.intOf(t.Bits(), true))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		x := g.intOf(t.Bits(), false)
		if g.rng.IntN(6) == 0 {
			x = -1
		}
		v.SetUint(uint64(x) & (math.MaxUint64 >> (64 - t.Bits())))
	case reflect.Float32, reflect.Float64:
		v.SetFloat([]float64{0, 1, -1, math.NaN(), math.Inf(1), math.Inf(-1), 1e300, -1e-300, g.rng.Float64() * 1e6, g.rng.NormFloat64()}[g.rng.IntN(10)])
	case reflect.Complex64, reflect.Complex128:
		v.SetComplex(complex(g.rng.NormFloat64(), []float64{0, math.NaN(), math.Inf(1), g.rng.NormFloat64()}[g.rng.IntN(4)]))
	case reflect.String:
		b := make([]byte, g.rng.IntN(48))
		for i := range b {
			b[i] = byte(g.rng.UintN(256))
		}
		v.SetString(string(b))
	case reflect.Array:
		for i := range v.Len() {
			g.fill(v.Index(i), depth)
		}
	case reflect.Slice:
		if depth <= 0 || g.rng.IntN(5) == 0 {
			return
		}
		n := []int{0, 1, 2, 7, 33, 64, 300, 1500}[g.rng.IntN(8)]
		if t.Elem().Kind() != reflect.Uint8 && n > 64 {
			n = 64
		}
		s := reflect.MakeSlice(t, n, n)
		for i := range n {
			g.fill(s.Index(i), depth-1)
		}
		v.Set(s)
	case reflect.Map:
		if depth <= 0 || g.rng.IntN(3) == 0 {
			return
		}
		m := reflect.MakeMap(t)
		for range g.rng.IntN(4) {
			m.SetMapIndex(g.value(t.Key(), depth-1, false), g.value(t.Elem(), depth-1, false))
		}
		v.Set(m)
	case reflect.Pointer:
		if depth <= 0 || g.rng.IntN(4) == 0 {
			return
		}
		p := reflect.New(t.Elem())
		g.fill(p.Elem(), depth-1)
		v.Set(p)
	case reflect.Struct:
		for i := range t.NumField() {
			if t.Field(i).IsExported() {
				g.fill(v.Field(i), depth-1)
			}
		}
	case reflect.Func:
		if g.rng.IntN(2) == 0 {
			return
		}
		v.Set(reflect.MakeFunc(t, func([]reflect.Value) []reflect.Value {
			out := make([]reflect.Value, t.NumOut())
			for i := range out {
				out[i] = reflect.New(t.Out(i)).Elem()
			}
			return out
		}))
	case reflect.Interface, reflect.Chan, reflect.UnsafePointer, reflect.Invalid:
	}
}

func (g *nopanicGen) construct(c reflect.Value) (v reflect.Value) {
	ct := c.Type()
	if ct.IsVariadic() {
		return v
	}
	in := make([]reflect.Value, ct.NumIn())
	for k := range in {
		in[k] = g.value(ct.In(k), nopanicDepth, g.rng.IntN(3) == 0)
	}
	defer func() {
		if recover() != nil {
			v = reflect.Value{}
		}
	}()
	out := c.Call(in)
	if out[0].IsNil() {
		return reflect.Value{}
	}
	return out[0]
}

type nopanicFinding struct {
	fn, msg, where string
}

func nopanicCall(f reflect.Value, args []reflect.Value) (msg, where string, hung bool) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				msg = fmt.Sprint(r)
				where = nopanicSite()
			}
		}()
		f.Call(args)
	}()
	select {
	case <-done:
		return msg, where, false
	case <-time.After(5 * time.Second):
		return "", "", true
	}
}

func nopanicSite() string {
	pc := make([]uintptr, 32)
	n := runtime.Callers(3, pc)
	frames := runtime.CallersFrames(pc[:n])
	for {
		fr, more := frames.Next()
		if strings.Contains(fr.Function, "dmrgo/v2") && !strings.Contains(fr.File, "nopanic_test.go") {
			return fmt.Sprintf("%s:%d", fr.File[strings.Index(fr.File, "v2/")+3:], fr.Line)
		}
		if !more {
			return "?"
		}
	}
}

func nopanicName(f any) string {
	name := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
	name = strings.TrimPrefix(name, "github.com/USA-RedDragon/dmrgo/v2")
	return strings.TrimSuffix(strings.TrimPrefix(name, "/"), "-fm")
}

// TestNoPanic calls every exported function and method of the library,
// except the internal packages, with zero values, nil pointers and random
// values of their arguments, on a receiver that is a zero value or holds
// random exported fields, and fails for each call that panics or hangs.
func TestNoPanic(t *testing.T) {
	t.Parallel()

	var found []nopanicFinding
	seen := map[string]bool{}
	ctors := map[reflect.Type][]reflect.Value{}
	for _, f := range api {
		ft := reflect.TypeOf(f)
		name := nopanicName(f)
		if strings.Contains(name, ".(") || strings.Count(name[strings.LastIndex(name, "/")+1:], ".") != 1 {
			continue
		}
		if ft.NumOut() > 0 && ft.Out(0).Kind() == reflect.Pointer && ft.Out(0).Elem().Kind() == reflect.Struct {
			ctors[ft.Out(0)] = append(ctors[ft.Out(0)], reflect.ValueOf(f))
		}
	}
	for i, f := range api {
		name := nopanicName(f)
		if strings.HasPrefix(name, "internal/") {
			continue
		}
		fv := reflect.ValueOf(f)
		ft := fv.Type()
		g := &nopanicGen{rng: rand.New(rand.NewPCG(uint64(i), 7))}
		for round := range nopanicRounds {
			args := make([]reflect.Value, ft.NumIn())
			for j := range args {
				at := ft.In(j)
				if ft.IsVariadic() && j == len(args)-1 {
					args[j] = g.value(at, nopanicDepth, round == 0)
					continue
				}
				switch {
				case j == 0 && strings.Contains(name, ".(*") && at.Kind() == reflect.Pointer:
					p := reflect.New(at.Elem())
					if round > 1 {
						g.fill(p.Elem(), nopanicDepth)
					}
					if c := ctors[at]; round > 1 && len(c) > 0 && g.rng.IntN(2) == 0 {
						if v := g.construct(c[g.rng.IntN(len(c))]); v.IsValid() {
							p = v
						}
					}
					args[j] = p
				case round == 0:
					args[j] = reflect.New(at).Elem()
				case round == 1 && at.Kind() == reflect.Pointer:
					args[j] = reflect.New(at.Elem())
				default:
					args[j] = g.value(at, nopanicDepth, round == 1)
				}
			}
			var msg, where string
			var hung bool
			if ft.IsVariadic() {
				msg, where, hung = nopanicCall(reflect.ValueOf(func(a []reflect.Value) { fv.CallSlice(a) }), []reflect.Value{reflect.ValueOf(args)})
			} else {
				msg, where, hung = nopanicCall(fv, args)
			}
			if hung {
				msg, where = "hang", "?"
			}
			if msg == "" {
				continue
			}
			key := name + " " + where
			if !seen[key] {
				seen[key] = true
				found = append(found, nopanicFinding{name, msg, where})
			}
			if hung {
				break
			}
		}
	}
	slices.SortFunc(found, func(a, b nopanicFinding) int { return strings.Compare(a.fn, b.fn) })
	for _, f := range found {
		t.Errorf("%s: %s at %s", f.fn, f.msg, f.where)
	}
}
