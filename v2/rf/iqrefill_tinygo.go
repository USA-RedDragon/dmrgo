//go:build tinygo

package rf

func (r *iqRx) keepRaw(v complex64) {
	if r.d.hasExt() {
		r.keepRawExt(v)
	}
}

func (r *iqRx) noteTune(hz float32) {
	if r.d.hasExt() {
		r.noteTuneExt(hz)
	}
}

func (r *iqRx) noteOut() {
	if r.d.hasExt() {
		r.noteOutExt()
	}
}

func (r *iqRx) unsettled() float32 {
	if r.d.hasExt() {
		return r.unsettledExt()
	}
	return 0
}

func (r *iqRx) endNarrow() bool { return r.d.hasExt() && r.endNarrowExt() }

func (r *iqRx) writeAudio() {
	if r.d.hasExt() {
		r.writeAudioExt()
	} else {
		r.d.writeIQ(r.audio)
		r.audio = r.audio[:0]
	}
	r.lockTune()
}
