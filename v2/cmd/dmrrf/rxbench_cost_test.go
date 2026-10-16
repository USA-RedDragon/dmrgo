package main

// m4fCyclesPerOp is the Cortex-M4F cycles per arithmetic operation that the
// cost estimate assumes: single-cycle FPU multiply-accumulate, with loads
// and loop overhead.
const m4fCyclesPerOp = 1.5

// opsPerSample estimates the arithmetic operations per input sample at
// basebandRate of a receiver with settings o that follows a BS outbound
// stream: a multiply-add counts 2, a complex multiply-add 8, a compare 1.
// A trellis runs over each symbol about 2.2 times, forward and backward
// from the anchors on both sides, and a voice burst that the receiver
// detects again adds half of that.
func opsPerSample(o Options) float64 {
	const (
		interpOps = sincTaps*4 + 8
		symN      = samplesPerSy
	)
	ops := float64(chanTaps * 2 * 2)
	ops += float64(fftSize/2*8*10+fftSize*5) / (fftSize / 2)
	if o.SoftFEC {
		ops += 60000.0 / slotSamplesN
	}
	if !o.Coherent {
		ops += float64(discTaps*2*2 + 6 + 40 + len(rrcTapsAt(samplesPerSy))*2)
		return ops + float64(13*syncSymbols*2*2)/slotSamplesN
	}
	ops += float64(13*syncSpan*interpOps+13*2*syncSpan*8) / slotSamplesN
	windows := 1 << (4*o.Window + 2)
	states := windows / 4
	run := float64(windows*o.TrellisSPS*8 + o.TrellisSPS*interpOps)
	if o.ExactPhase {
		np := o.PhaseStates
		if np == 0 {
			np = phaseStates
		}
		run += float64(windows*np*(5+7) + 2*states*np*4)
	} else {
		run += float64(states*4*9 + windows*4)
	}
	runs := 2.2
	if o.RefinePrior > 0 {
		runs *= 1.5
	}
	ops += run * runs / symN
	if o.TrackCarrier {
		ops += float64(19*symN*2+symN*interpOps) / symN
	}
	return ops
}
