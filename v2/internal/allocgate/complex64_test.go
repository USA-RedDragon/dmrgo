//go:build !tinygo

package allocgate_test

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"testing"
)

// complexProducts returns the complex64 products and quotients of p whose
// operands are both complex. gc computes such a product in float64 and TinyGo
// in float32, so the receiver of a microcontroller would not give the bits of
// the host. A product with a real factor, a constant or complex(x, 0), rounds
// the same on both.
func complexProducts(p *program) []string {
	var out []string
	for _, l := range p.order {
		for _, f := range l.files {
			ast.Inspect(f, func(n ast.Node) bool {
				var x, y ast.Expr
				var at token.Pos
				switch e := n.(type) {
				case *ast.BinaryExpr:
					if e.Op != token.MUL && e.Op != token.QUO || !isComplex64(l.info.TypeOf(e)) {
						return true
					}
					x, y, at = e.X, e.Y, e.OpPos
				case *ast.AssignStmt:
					if e.Tok != token.MUL_ASSIGN && e.Tok != token.QUO_ASSIGN || !isComplex64(l.info.TypeOf(e.Lhs[0])) {
						return true
					}
					x, y, at = e.Lhs[0], e.Rhs[0], e.TokPos
				default:
					return true
				}
				if !isReal(l.info, x) && !isReal(l.info, y) {
					out = append(out, p.fset.Position(at).String())
				}
				return true
			})
		}
	}
	return out
}

func isComplex64(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Complex64
}

func isReal(info *types.Info, e ast.Expr) bool {
	if tv, ok := info.Types[e]; ok && tv.Value != nil {
		return constant.Sign(constant.Imag(tv.Value)) == 0
	}
	call, ok := ast.Unparen(e).(*ast.CallExpr)
	if !ok || len(call.Args) != 2 {
		return false
	}
	if b, ok := info.Uses[identOf(call.Fun)].(*types.Builtin); !ok || b.Name() != "complex" {
		return false
	}
	tv, ok := info.Types[call.Args[1]]
	return ok && tv.Value != nil && constant.Sign(tv.Value) == 0
}

func identOf(e ast.Expr) *ast.Ident {
	id, _ := ast.Unparen(e).(*ast.Ident)
	return id
}

func TestNoComplex64Products(t *testing.T) {
	t.Parallel()

	for _, at := range complexProducts(loadLibrary(t, libraryPackages(t))) {
		t.Errorf("%s: complex64 product of two complex operands rounds differently under gc and TinyGo; use cmul", at)
	}
}

func TestComplex64ProductsCases(t *testing.T) {
	t.Parallel()

	got := complexProducts(loadLibrary(t, []string{"./internal/allocgate/testdata/complex64"}))
	if len(got) != 3 {
		t.Fatalf("found %q, want the bare product, its assignment and the quotient", got)
	}
}
