package fsktx

// The regions, services and power classes of MinRate.
const (
	LimitsWorld = iota
	LimitsUS
	LimitsCEPT
	LimitsChina
	LimitsJapan
)

// The services of MinRate.
const (
	LimitsLandMobile = iota
	LimitsAmateur
)

// The power classes of MinRate: 1, 10, 100 and 1000 W.
const (
	Limits1W = iota
	Limits10W
	Limits100W
	Limits1000W
)

// MinRate returns the least sample rate in Hz from which the modulator of
// tier t keeps every waveform limit of region, service and power with
// margin, as TestMinRatesGenerated measures it on its rate grid, or 0 when
// no rate of the grid does or an argument is out of range. For audio, bits
// is the resolution of the converter, from 8 to 16, and a depth between
// the measured 8, 10, 12 and 16 takes the next lower one. For I/Q, bits is
// 16 for 16-bit and float samples and 8 for 8-bit samples. The rate of I/Q
// is never under 62500 Hz, where the image of the main lobe stays out of
// the alternate channel.
func MinRate(t Tier, iq bool, bits, region, service, power int) int {
	if t == TierDefault {
		t = Tier1
	}
	ti := int(t - Tier0)
	if ti < 0 || ti >= len(minRateAudio) || region < 0 || region >= len(minRateAudio[0][0]) ||
		service < 0 || service >= len(minRateAudio[0][0][0]) || power < 0 || power >= len(minRateAudio[0][0][0][0]) {
		return 0
	}
	if iq {
		switch bits {
		case 16:
			return int(minRateIQ[ti][0][region][service][power])
		case 8:
			return int(minRateIQ[ti][1][region][service][power])
		}
		return 0
	}
	var c int
	switch {
	case bits == 16:
		c = 3
	case bits >= 12 && bits < 16:
		c = 2
	case bits >= 10 && bits < 12:
		c = 1
	case bits >= 8 && bits < 10:
		c = 0
	default:
		return 0
	}
	return int(minRateAudio[ti][c][region][service][power])
}
