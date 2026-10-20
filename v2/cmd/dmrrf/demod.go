package main

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

// Demodulated is the result of DemodulateBytes.
type Demodulated struct {
	Bursts      []rf.Burst
	OffsetHz    float64
	DeviationHz float64
}

// DemodulateBytes recovers DMR bursts from 8-bit I/Q at rate with a
// receiver of the given tier.
func DemodulateBytes(iq []byte, rate float64, signed bool, tier rf.Tier) (Demodulated, error) {
	rx, err := rf.New(rf.Config{Rate: rate, Signed: signed, Tier: tier})
	if err != nil {
		return Demodulated{}, err
	}
	var res Demodulated
	const chunk = 1 << 16
	for len(iq) > 0 {
		n := min(len(iq), chunk)
		b, err := rx.Write(iq[:n])
		if err != nil {
			return Demodulated{}, err
		}
		res.Bursts = append(res.Bursts, b...)
		iq = iq[n:]
	}
	res.Bursts = append(res.Bursts, rx.Flush()...)
	res.OffsetHz, res.DeviationHz = rx.OffsetHz(), rx.DeviationHz()
	return res, nil
}

// Demodulate recovers DMR bursts from complex samples at rate, as rtl_sdr
// unsigned 8-bit I/Q scaled by rftx.FromRTL, with rf.Tier3.
func Demodulate(x []complex128, rate float64) (Demodulated, error) {
	b := make([]byte, 2*len(x))
	for i, v := range x {
		b[2*i] = byte(math.Max(0, math.Min(255, math.Round(real(v)*127.5+127.5))))
		b[2*i+1] = byte(math.Max(0, math.Min(255, math.Round(imag(v)*127.5+127.5))))
	}
	return DemodulateBytes(b, rate, false, rf.Tier3)
}
