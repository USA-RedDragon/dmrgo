package rf

import "github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"

// A PreEmphasis is the Tier0 modulator of an AudioTransmitter for a radio
// that pre-emphasizes its modulator input from 300 Hz to 3 kHz, which
// AudioTxConfig.PreEmphasis takes. The zero PreEmphasis is not set up: Init
// sets it up in memory of the caller, such as a package variable, and
// NewPreEmphasis allocates one. A program that sets up none links none of
// its code or tables. It holds the state of the modulator, so each
// AudioTransmitter takes a PreEmphasis of its own.
type PreEmphasis struct {
	m      fsktx.MCUAudioPE
	init   func(m *fsktx.MCUAudioPE, level, offset int) error
	symbol func(m *fsktx.MCUAudioPE, dst []int16, s int8) int
	peak   func(level int) int
}

// NewPreEmphasis allocates a PreEmphasis and returns it set up.
func NewPreEmphasis() *PreEmphasis {
	return new(PreEmphasis).Init()
}

// Init sets up p, in place of any state it holds, and returns p, for
// AudioTxConfig.PreEmphasis. It does not allocate. A nil p returns nil.
func (p *PreEmphasis) Init() *PreEmphasis {
	if p == nil {
		return nil
	}
	*p = PreEmphasis{init: (*fsktx.MCUAudioPE).Init, symbol: (*fsktx.MCUAudioPE).Symbol, peak: fsktx.PeakPE}
	return p
}

func (p *PreEmphasis) set() bool {
	return p != nil && p.init != nil && p.symbol != nil && p.peak != nil
}

//go:noinline
func (p *PreEmphasis) emit(dst []int16, s int8) int { return p.symbol(&p.m, dst, s) }
