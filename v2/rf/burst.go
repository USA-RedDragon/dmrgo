package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Burst is one burst that a Receiver recovered.
type Burst struct {
	// Data is the 264 burst bits as detected, in transmit order.
	Data layer2.RawBurst
	// Seconds is the time of the first burst bit from the first sample.
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
	Fixed     layer2.RawBurst
	FixedCACH [3]byte
	HasFixed  bool
}

// Decoded returns the burst and CACH bits to decode: Fixed and FixedCACH
// when the burst has them, Data and CACH otherwise.
func (b *Burst) Decoded() (layer2.RawBurst, [3]byte) {
	if b.HasFixed {
		return b.Fixed, b.FixedCACH
	}
	return b.Data, b.CACH
}

// Timed reports whether the burst was placed by timing instead of a SYNC.
func (b *Burst) Timed() bool { return b.Distance < 0 }

type received struct {
	data      layer2.RawBurst
	seconds   float64
	distance  int
	sync      enums.SyncPattern
	voice     enums.VoiceBurstType
	cach      [layer2.CACHBits]unpacked.Bit
	hasCACH   bool
	bs        bool
	stream    int
	slot      int64
	fixed     layer2.RawBurst
	fixedCACH [layer2.CACHBits]unpacked.Bit
	hasFixed  bool
	side      combSide
}

func (x *received) put(b *Burst) {
	*b = Burst{Data: x.data, Seconds: x.seconds, Distance: x.distance, Sync: x.sync, Voice: x.voice, CACH: cachBytes(x.cach), HasCACH: x.hasCACH,
		BS: x.bs, Stream: x.stream, Slot: x.slot, Fixed: x.fixed, FixedCACH: cachBytes(x.fixedCACH), HasFixed: x.hasFixed}
}
