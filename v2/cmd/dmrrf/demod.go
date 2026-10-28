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

// DemodulateBytes recovers DMR bursts from 8-bit I/Q at rate, signed (cs8)
// or unsigned (cu8), with a receiver of the given tier.
func DemodulateBytes(iq []byte, rate float64, signed bool, tier rf.Tier) (Demodulated, error) {
	f := rf.IQFormatCU8
	if signed {
		f = rf.IQFormatCS8
	}
	rx, err := rf.NewReceiver(&rf.RxConfig{Rate: int(rate), Format: f, Tier: tier})
	if err != nil {
		return Demodulated{}, err
	}
	var res Demodulated
	const chunk = 1 << 16
	for len(iq) > 0 {
		n := min(len(iq), chunk)
		res.Bursts = rx.AppendBursts(res.Bursts, iq[:n])
		iq = iq[n:]
	}
	res.Bursts = rx.AppendFlush(res.Bursts)
	res.OffsetHz, res.DeviationHz = rx.OffsetHz(), rx.DeviationHz()
	return res, nil
}

// Demodulate recovers DMR bursts from complex samples at rate, as unsigned
// 8-bit I/Q (cu8) scaled by rftx.FromCU8, with rf.Tier3.
func Demodulate(x []complex128, rate float64) (Demodulated, error) {
	b := make([]byte, 2*len(x))
	for i, v := range x {
		b[2*i] = byte(math.Max(0, math.Min(255, math.Round(real(v)*127.5+127.5))))
		b[2*i+1] = byte(math.Max(0, math.Min(255, math.Round(imag(v)*127.5+127.5))))
	}
	return DemodulateBytes(b, rate, false, rf.Tier3)
}
