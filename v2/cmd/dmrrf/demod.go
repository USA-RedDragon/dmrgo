package main

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Received is one burst recovered from I/Q.
type Received struct {
	Data    layer2.RawBurst
	Seconds float64
	// Distance is the Hamming distance of the SYNC field from Sync, or -1
	// for a burst placed by timing without a SYNC (voice bursts B to F).
	Distance int
	Sync     enums.SyncPattern
	// Voice is the superframe position of a voice burst whose position
	// comes from the slot timing, or VoiceBurstUnknown when the SYNC field
	// alone classifies the burst.
	Voice enums.VoiceBurstType
	// CACH is the 24 bits before the burst in transmit order. BS is set
	// when the burst is BS-sourced, so that those bits are a CACH.
	CACH    [layer2.CACHBits]unpacked.Bit
	HasCACH bool
	BS      bool
	// Stream numbers the transmissions a Receiver locked to, from 1, and
	// Slot is the 30 ms slot of the burst in its stream. Stream 0 means
	// unknown.
	Stream int
	Slot   int64
	// Fixed and FixedCACH hold Data and CACH with each FEC field replaced
	// by the codeword that soft-decision decoding picked, when HasFixed.
	Fixed     layer2.RawBurst
	FixedCACH [layer2.CACHBits]unpacked.Bit
	HasFixed  bool
}

// decoded returns the burst and CACH bits to decode: Fixed and FixedCACH
// when the burst has them, Data and CACH otherwise.
func (r Received) decoded() (layer2.RawBurst, [layer2.CACHBits]unpacked.Bit) {
	if r.HasFixed {
		return r.Fixed, r.FixedCACH
	}
	return r.Data, r.CACH
}

// Timed reports whether the burst was placed by timing instead of a SYNC.
func (r Received) Timed() bool { return r.Distance < 0 }

// Demodulated is the result of Demodulate.
type Demodulated struct {
	Bursts      []Received
	OffsetHz    float64
	DeviationHz float64
}

// sampleOf returns the sample at basebandRate of the first symbol of r.
func sampleOf(r Received) int { return int(math.Round(r.Seconds * basebandRate)) }

// Demodulate recovers DMR bursts from complex samples at rate, as rtl_sdr
// unsigned 8-bit I/Q scaled by FromRTL, with TierExact.
func Demodulate(x []complex128, rate float64) Demodulated {
	b := make([]byte, 2*len(x))
	for i, v := range x {
		b[2*i] = byte(math.Max(0, math.Min(255, math.Round(real(v)*127.5+127.5))))
		b[2*i+1] = byte(math.Max(0, math.Min(255, math.Round(imag(v)*127.5+127.5))))
	}
	return DemodulateBytes(b, rate, false, TierExact)
}

// DemodulateBytes recovers DMR bursts from 8-bit I/Q at rate with a
// Receiver of the given tier.
func DemodulateBytes(iq []byte, rate float64, signed bool, tier Tier) Demodulated {
	rx := NewReceiver(ReceiverConfig{Rate: rate, Signed: signed, Tier: tier})
	var res Demodulated
	const chunk = 1 << 16
	for len(iq) > 0 {
		n := min(len(iq), chunk)
		res.Bursts = append(res.Bursts, rx.Write(iq[:n])...)
		iq = iq[n:]
	}
	res.Bursts = append(res.Bursts, rx.Flush()...)
	res.OffsetHz, res.DeviationHz = rx.OffsetHz, rx.DeviationHz
	return res
}
