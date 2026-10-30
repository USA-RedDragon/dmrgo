package rf

const (
	discRate = basebandRate / 2
	halfTaps = 11
	halfSide = (halfTaps/2 + 1) / 2
	halfBeta = 6
	preTaps  = 17
	discSpan = 4
	discSPS  = samplesPerSy / 2
	rrcTaps  = 2*discSpan*discSPS + 1
)

// ETSI TS 102 361-1 clause 10.2
const (
	symbolRate   = 4800
	deviationHz  = 648.0
	basebandRate = 48000
	samplesPerSy = basebandRate / symbolRate
	rrcAlpha     = 0.2
	rrcSpan      = 8
	burstSymbols = 132
	cachSymbols  = 12
	slotSeconds  = 0.030
)

const (
	slotSymbols   = 144
	slotSamplesN  = slotSymbols * samplesPerSy
	syncFirstSym  = cachSymbols + 54
	exactGapSlots = 3
	discLock      = 0.9
	levelJump     = 4.0
	lostSlots     = 24
)

const (
	discCutHz  = 4500.0
	discStepHz = 64
)
