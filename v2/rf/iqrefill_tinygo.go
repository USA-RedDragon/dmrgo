//go:build tinygo

package rf

type iqRefill struct{}

func (r *iqRx) attachRefill(*softExt) {}

func (r *iqRx) keepRaw(complex64) {}

func (r *iqRx) noteTune(float32) {}

func (r *iqRx) noteOut() {}

func (r *iqRx) unsettled() float32 { return 0 }

func (r *iqRx) endNarrow() bool { return false }

func (r *iqRx) writeAudio() {
	r.d.writeIQ(r.audio)
	r.audio = r.audio[:0]
}
