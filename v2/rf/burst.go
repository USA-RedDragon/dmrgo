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
	Data  layer2.RawBurst
	stamp int16
	// Seconds is the time of the first burst bit from the first sample,
	// within 2 µs on a clean signal at every tier, input and rate. An MS
	// burst stamped before the Receiver knows the clock rate of the call
	// is projected from its timing anchor at the nominal symbol period, so
	// the clock error times up to one burst adds to that.
	Seconds float64
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
