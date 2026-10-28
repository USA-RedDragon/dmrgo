package capigen

import (
	"fmt"
	"go/types"
	"testing"
)

func TestEffects(t *testing.T) {
	t.Parallel()

	l := newLoader("testdata/eff")
	pkg, err := l.load(modulePath + "/x")
	if err != nil {
		t.Fatal(err)
	}
	m, ok := pkg.Scope().Lookup("M").Type().(*types.Named)
	if !ok {
		t.Fatal("no type M")
	}
	method := func(name string) *types.Func {
		for f := range m.Methods() {
			if f.Name() == name {
				return f
			}
		}
		t.Fatalf("no method %s", name)
		return nil
	}
	str := func(e *effect) string {
		if e.all {
			return "all"
		}
		return fmt.Sprint(e.f)
	}
	for _, c := range []struct {
		name string
		want string
	}{
		{"Read", "map[]"},
		{"SetN", "map[N:1]"},
		{"Fill", "map[Data:2]"},
		{"Grow", "map[Data:3]"},
		{"Copy", "map[]"},
		{"CopyWrite", "map[Data:2]"},
		{"Via", "map[N:1]"},
		{"Alias", "map[Data:2]"},
		{"Sub2", "map[Sub:1]"},
		{"Ptr", "map[P:1]"},
		{"Unknown", "all"},
		{"Closure", "map[N:3]"},
		{"Box", "map[]"},
		{"CommaOK", "map[Any:1]"},
		{"MapOK", "map[Map:1]"},
		{"TypeSwitch", "map[Any:1]"},
		{"Reassign", "all"},
		{"CopyFill", "map[Data:2]"},
		{"BoxFill", "map[Data:2]"},
		{"Promoted", "map[E:1]"},
		{"PromotedMethod", "map[E:1]"},
		{"Builder", "map[Sb:3]"},
		{"As", "map[Err:3]"},
		{"Del", "map[Map:1]"},
		{"Send", "map[Data:3]"},
		{"ValuePoke", "map[Data:2]"},
	} {
		if got := str(l.effects(method(c.name), -1)); got != c.want {
			t.Errorf("M.%s: effects %s, want %s", c.name, got, c.want)
		}
	}
	fn := func(name string) *types.Func {
		f, ok := pkg.Scope().Lookup(name).(*types.Func)
		if !ok {
			t.Fatalf("no func %s", name)
		}
		return f
	}
	for _, c := range []struct {
		name  string
		param int
		want  string
	}{
		{"Decode", 0, "map[]"},
		{"Sign", 0, "map[:2]"},
		{"Into", 0, "map[Data:1]"},
		{"Into", 1, "map[]"},
		{"Look", 0, "map[]"},
		{"Fill", 0, "map[:2]"},
		{"Fill", 1, "map[:2]"},
		{"Wrapped", 0, "map[Any:2 Ch:2 Data:2 E:2 Err:2 Map:2 P:2 S2:2 Sb:2 Sub:2]"},
		{"PeekUnsafe", 0, "map[]"},
		{"PokeUnsafe", 0, "map[:2]"},
		{"ViewUnsafe", 0, "map[:2]"},
		{"EvenRead", 0, "map[]"},
		{"OddRead", 0, "map[]"},
		{"OddWrite", 0, "map[:2]"},
		{"EvenWrite", 0, "map[:2]"},
		{"ToRSink", 1, "map[]"},
		{"ToSink", 1, "map[:2]"},
		{"ToStringer", 1, "map[:2]"},
	} {
		if got := str(l.effects(fn(c.name), c.param)); got != c.want {
			t.Errorf("%s param %d: effects %s, want %s", c.name, c.param, got, c.want)
		}
	}
	if !l.overwrites(fn("DecodeInto"), 0) || l.overwrites(fn("ReadFirst"), 0) || l.overwrites(fn("Into"), 0) {
		t.Error("overwrites: DecodeInto sets all of m first; ReadFirst and Into do not")
	}
}
