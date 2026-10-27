//go:build !tinygo

package rf

const cohSaveHeap = false

type cohSaver struct {
	save *cohStage
}

//go:noinline
func (x *softExt) cohLate(c *leanCore, lb *Burst) bool {
	saved := *x.coh
	x.coh.fitOnly = false
	ok := x.coh.detect(c, lb, &x.llr)
	gen, stamp := x.coh.gen, x.coh.symStamp
	*x.coh = saved
	x.coh.gen, x.coh.symStamp, x.coh.symKey = gen+1, stamp+1, cohSymKey{}
	return ok
}
