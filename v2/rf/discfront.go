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
