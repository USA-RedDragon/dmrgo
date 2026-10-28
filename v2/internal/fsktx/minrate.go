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
	// PreEmphasis states that the radio pre-emphasizes its modulator
	// input with a zero at 300 Hz and a pole at 3 kHz, and that the
	// modulator is MCUAudioPE. It is measured for Tier0
	// audio at 48000 Hz only.
	PreEmphasis bool
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
	if s.PreEmphasis && (s.IQ || t != Tier0) {
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
	if s.PreEmphasis {
		return int(minRatePE[c][lowpassClass(s.InputLowpassHz)][s.Region][s.Service][s.Power]) * minRateUnit
	}
	return int(minRateAudio[ti][c][lowpassClass(s.InputLowpassHz)][s.Region][s.Service][s.Power]) * minRateUnit
}

// Allows reports whether the modulator of s may run at hz: a rate that
// the modulator makes, within the CPU budget of its tier, from MinRate up
// to the top of the measured grid, 96000 Hz for audio and 20 MHz for I/Q.
// Tier0 makes audio at 24000 and 48000 Hz and I/Q at 96000 Hz only: I/Q at
// 192000 Hz takes 7.4 MHz of a Cortex-M4, over the 4 MHz budget, and
// 48000 Hz is under the least rate of I/Q. With PreEmphasis, Tier0 makes
// audio at 48000 Hz only.
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
		if hz != 24000 && hz != 48000 || s.PreEmphasis && hz != 48000 {
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

// Tier0Level returns the Level of the audio of s that MinRate measures
// Tier0 at, and the most that it measures Tier1 and Tier2 at, or 0 when s
// is I/Q or Bits is out of range. At 16 bits it is 15000. Under 16 bits it
// is about half of full scale of MeasuredBits, 65 codes at 8 bits, 260 at
// 10 and 1040 at 12, times 2 for each bit of Bits over MeasuredBits. Under
// 16 bits, Tier1 and Tier2 at 24000 Hz and above reach 2.12 times Level,
// so that 1040 codes do not fit 12 bits: there MinRate measures at most
// the largest Level whose Peak at Offset 0 fits MeasuredBits, times 2 for
// each bit over it.
func (s Setup) Tier0Level() int {
	depth, level := measuredDepth(s)
	return level << (s.Bits - depth)
}

// MeasuredBits returns the depth of the audio rows of MinRate that s
// takes, 8, 10, 12 or 16, or 0 when s is I/Q or Bits is out of range.
func (s Setup) MeasuredBits() int {
	if depth, level := measuredDepth(s); level > 0 {
		return depth
	}
	return 0
}

func measuredDepth(s Setup) (depth, level int) {
	switch {
	case s.IQ:
		return s.Bits, 0
	case s.Bits == 16:
		return 16, tableUnits
	case s.Bits >= 12 && s.Bits < 16:
		return 12, 1040
	case s.Bits >= 10 && s.Bits < 12:
		return 10, 260
	case s.Bits >= 8 && s.Bits < 10:
		return 8, 65
	}
	return s.Bits, 0
}
