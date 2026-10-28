// Command txiq runs the Tier 0 I/Q transmitter, an rf.Transmitter that
// Init sets up in static memory, as a microcontroller feeds the I/Q DAC of
// a software radio at 96 kHz: a few MS bursts on one TDMA channel, with
// their power ramps. It allocates nothing: it builds with -gc=none, which
// fails the link of any program that allocates. Init returns an
// error for a setup that no measurement shows to keep the limits of its
// region, and then the command prints "setup refused" and stops.
package main

import "github.com/USA-RedDragon/dmrgo/v2/rf"

var (
	tx    rf.Transmitter
	one   [1]rf.Burst
	sum   int32
	keyed bool
)

func toDAC(b []byte) {
	for _, v := range b {
		sum += int32(v)
	}
}

func main() {
	err := tx.Init(rf.TxConfig{Rate: 96000, OnSamples: toDAC, Region: rf.RegionUS, Service: rf.ServiceLandMobile, Power: 30})
	if err != nil {
		println("setup refused")
		return
	}
	x := byte(1)
	for i := range one[0].Data {
		x = x*37 + 11
		one[0].Data[i] = x
	}
	one[0].Stream = 1
	for i := range 100 {
		one[0].Slot = int64(2 * i)
		if _, err := tx.Write(one[:]); err != nil {
			return
		}
		keyed = tx.Active()
	}
	_ = tx.Flush()
	println("sum", sum, keyed)
}
