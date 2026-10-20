package capigen

import "go/types"

func (g *gen) needsArena(t types.Type) bool {
	switch u := t.(type) {
	case *types.Basic:
		return u.Kind() == types.String
	case *types.Slice:
		return true
	case *types.Array:
		return g.needsArena(u.Elem())
	case *types.Pointer:
		if _, isH := g.handleC(u.Elem()); isH {
			return false
		}
		return g.needsArena(u.Elem())
	case *types.Named:
		ti := g.info(u)
		if ti == nil {
			return false
		}
		key := "arena:" + ti.id
		if v, ok := g.memo[key]; ok {
			return v.s != ""
		}
		g.memo[key] = memo{}
		need := false
		switch ti.kind {
		case kValue:
			for f := range underStruct(u).Fields() {
				if f.Exported() && g.needsArena(f.Type()) {
					need = true
				}
			}
		case kClosed:
			for _, im := range ti.impls {
				if g.needsArena(im) {
					need = true
				}
			}
		case kNArray:
			need = g.needsArena(u.Underlying())
		default:
		}
		if need {
			g.memo[key] = memo{s: "y"}
		}
		return need
	default:
		return false
	}
}
