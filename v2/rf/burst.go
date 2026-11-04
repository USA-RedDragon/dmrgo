package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Burst is one burst that a Receiver recovered.
type Burst struct {
	// Data is the 264 burst bits as detected, in transmit order. Tier3
	// detects a burst twice, with the discriminator and coherently, and
	// gives the detection nearer to Fixed.
	Data layer2.RawBurst
	// Seconds is the time of the first burst bit from the first sample,
	// within 2 µs on a clean signal at every tier, input and rate, plus the
	// bound that SecondsSymbols and SecondsError give.
	Seconds float64
	// SecondsSymbols is the number of symbols over which Seconds was
	// projected at the nominal symbol period (4800 symbols per second, ETSI
	// TS 102 361-1 clause 10.2) because the clock rate of the transmitter
	// was not yet known well enough: Seconds can then be off by up to
	// SecondsSymbols/4800 times the relative clock error. It is 0 once the
	// rate is known.
	SecondsSymbols int
	// SecondsError is the error of Seconds, in seconds, that the uncertainty
	// of the clock rate estimate gives over the projection: the rate error
	// of the timing loop, or the slope uncertainty of the line through the
	// SYNC fits, times the symbols from the timing anchor to the burst.
	SecondsError float32
	// Distance is the Hamming distance of the SYNC field from Sync, or -1
	// for a burst placed by timing without a SYNC (voice bursts B to F).
	Distance int
	// Sync is the SYNC pattern of the burst.
	Sync enums.SyncPattern
	// Voice is the superframe position of a voice burst whose position
	// comes from the slot timing, or VoiceBurstUnknown when the SYNC field
	// alone classifies the burst.
	Voice enums.VoiceBurstType
	// CACH is the 24 bits before the burst in transmit order, when HasCACH.
	CACH    [3]byte
	HasCACH bool
	// BS is set when the burst is BS-sourced, so that the bits before it
	// are a CACH.
	BS bool
	// Stream numbers the transmissions that the Receiver locked to, from
	// 1. Slot is the 30 ms slot of the burst in its stream.
	Stream int
	Slot   int64
	// Fixed and FixedCACH hold Data and CACH with each FEC field replaced
	// by the codeword that soft-decision decoding picked, when HasFixed.
	// The voice slots of a call whose voice LC has FID 0xC2 (Codec2) carry
	// no FEC and keep the bits of Data, from the LC that gives the FID on.
	Fixed     layer2.RawBurst
	FixedCACH [3]byte
	HasFixed  bool
	// Codec2 is the bit rate, 2400 or 3200, of the Codec2 frames of a
	// voice burst of a call whose voice LC has FID 0xC2, once the frames
	// since the start of the call identify it, which 5 clean frames do. It
	// is 0 for other bursts, before that, and at Tier0.
	Codec2 int
}

// Decoded returns the burst and CACH bits to decode: Fixed and FixedCACH
// when the burst has them, Data and CACH otherwise.
func (b *Burst) Decoded() (layer2.RawBurst, [3]byte) {
	if b == nil {
		return layer2.RawBurst{}, [3]byte{}
	}

	if b.HasFixed {
		return b.Fixed, b.FixedCACH
	}
	return b.Data, b.CACH
}

// Timed reports whether the burst was placed by timing instead of a SYNC.
func (b *Burst) Timed() bool {
	if b == nil {
		return false
	}
	return b.Distance < 0
}
