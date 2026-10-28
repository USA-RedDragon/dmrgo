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

const (
	lowpass8k = 8000
	lowpass5k = 5000
	lowpass3k = 3000
)

const (
	audioRateTop = 96000
	iqRateTop    = 20000000
	minRateUnit  = 500
)

// A Setup is a transmit setup that MinRate and Allows check against the
// waveform limits of a region, service and power class.
type Setup struct {
	// Tier is the modulator; TierDefault is Tier1.
	Tier Tier
	// IQ is true for I/Q and false for audio.
	IQ bool
	// Bits is the resolution of the audio converter, from 8 to 16. A depth
	// between the measured 8, 10, 12 and 16 takes the next lower one. For
	// I/Q it is 16 for 16-bit and float samples and 8 for 8-bit samples.
	Bits int
	// Region, Service and Power are the Limits constants.
	Region, Service, Power int
	// InputLowpassHz is the cutoff in Hz of a low pass of at least two
	// poles that the radio puts before its modulator input, as the user
	// states it. 0 means none: the DAC drives the modulator with no filter.
	// The check takes the radio as a two-pole Butterworth low pass at the
	// next looser of the measured cutoffs 8000, 5000 and 3000 Hz, so 6000
	// counts as 8000 and a cutoff above 8000 as none. Audio only.
	InputLowpassHz int
}

func lowpassClass(hz int) int {
	switch {
	case hz <= 0 || hz > lowpass8k:
		return 0
	case hz > lowpass5k:
		return 1
	case hz > lowpass3k:
		return 2
	}
	return 3
}

// MinRate returns the least sample rate in Hz from which the modulator of
// s keeps every waveform limit of its region, service and power with
// margin, as TestMinRatesGenerated measures it on its rate grid, or 0 when
// no rate of the grid does or a field is out of range. The rate of I/Q is
// never under 62500 Hz, where the image of the main lobe stays out of the
// alternate channel.
func (s Setup) MinRate() int {
	t := s.Tier
	if t == TierDefault {
		t = Tier1
	}
	ti := int(t - Tier0)
	if ti < 0 || ti >= len(minRateAudio) || s.Region < 0 || s.Region >= len(minRateIQ[0][0]) ||
		s.Service < 0 || s.Service >= len(minRateIQ[0][0][0]) || s.Power < 0 || s.Power >= len(minRateIQ[0][0][0][0]) {
		return 0
	}
	if s.IQ {
		switch s.Bits {
		case 16:
			return int(minRateIQ[ti][0][s.Region][s.Service][s.Power])
		case 8:
			return int(minRateIQ[ti][1][s.Region][s.Service][s.Power])
		}
		return 0
	}
	var c int
	switch {
	case s.Bits == 16:
		c = 3
	case s.Bits >= 12 && s.Bits < 16:
		c = 2
	case s.Bits >= 10 && s.Bits < 12:
		c = 1
	case s.Bits >= 8 && s.Bits < 10:
		c = 0
	default:
		return 0
	}
	return int(minRateAudio[ti][c][lowpassClass(s.InputLowpassHz)][s.Region][s.Service][s.Power]) * minRateUnit
}

// Allows reports whether the modulator of s may run at hz: a rate that
// the modulator makes, within the CPU budget of its tier, from MinRate up
// to the top of the measured grid, 96000 Hz for audio and 20 MHz for I/Q.
// Tier0 makes audio at 24000 and 48000 Hz and I/Q at 96000 Hz only: I/Q at
// 192000 Hz takes 7.4 MHz of a Cortex-M4, over the 4 MHz budget, and
// 48000 Hz is under the least rate of I/Q.
func (s Setup) Allows(hz int) bool {
	t := s.Tier
	if t == TierDefault {
		t = Tier1
	}
	switch {
	case t == Tier0 && s.IQ:
		if hz != 96000 {
			return false
		}
	case t == Tier0:
		if hz != 24000 && hz != 48000 {
			return false
		}
	case s.IQ:
		if hz > iqRateTop {
			return false
		}
	case hz < 8000 || hz > audioRateTop:
		return false
	default:
		if l, _ := ratio(hz); l > polyMaxPhases {
			return false
		}
	}
	least := s.MinRate()
	return least > 0 && hz >= least
}
