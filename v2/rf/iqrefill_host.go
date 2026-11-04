//go:build !tinygo

package rf

func (r *iqRx) keepRaw(v complex64) { r.keepRawExt(v) }

func (r *iqRx) noteTune(hz float32) { r.noteTuneExt(hz) }

func (r *iqRx) noteOut() { r.noteOutExt() }

func (r *iqRx) unsettled() float32 { return r.unsettledExt() }

func (r *iqRx) endNarrow() bool { return r.endNarrowExt() }

func (r *iqRx) writeAudio() { r.writeAudioExt() }
