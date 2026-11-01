//go:build tinygo

package rf

type iqRefill struct{}

func (r *iqRx) attachRefill(*softExt) {}

func (r *iqRx) keepRaw(complex64) {}

func (r *iqRx) noteTune(float32) {}

func (r *iqRx) noteOut() {}

func (r *iqRx) unsettled() float32 { return 0 }
