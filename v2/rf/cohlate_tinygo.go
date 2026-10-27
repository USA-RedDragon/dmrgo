//go:build tinygo

package rf

import "unsafe"

const cohSaveHeap = true

type cohSaver struct {
	save *cohStage
}

//go:noinline
func (x *softExt) cohLate(c *leanCore, lb *Burst) bool {
	saved, cur := unsafe.Slice(x.saver.save, 1), unsafe.Slice(x.coh, 1)
	copy(saved, cur)
	x.coh.fitOnly = false
	ok := x.coh.detect(c, lb, &x.llr)
	gen, stamp := x.coh.gen, x.coh.symStamp
	copy(cur, saved)
	x.coh.gen, x.coh.symStamp, x.coh.symKey = gen+1, stamp+1, cohSymKey{}
	return ok
}
