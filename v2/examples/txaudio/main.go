// Command txaudio runs the Tier 0 audio transmitter, an
// rf.AudioTransmitter that Init sets up in static memory, as a hotspot
// board drives the modulator input of its radio from a 12-bit DAC at 48
// kHz and keys the radio by Active. It allocates nothing: it builds with
// -gc=none, which fails the link of any program that allocates. Init
// returns an error for a setup that no measurement shows to keep the
// limits of its region, and then the command prints "setup refused" and
// stops.
package main

import "github.com/USA-RedDragon/dmrgo/v2/rf"

var (
	tx  rf.AudioTransmitter
	one [1]rf.Burst
	sum int32
	ptt bool
)

func toDAC(x []int16) {
	for _, v := range x {
		sum += int32(v)
	}
}

func main() {
	err := tx.Init(rf.AudioTxConfig{Rate: 48000, Bits: 12, Unsigned: true, OnSamples: toDAC,
		Region: rf.RegionUS, Service: rf.ServiceLandMobile, Power: 30})
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
		ptt = tx.Active()
	}
	_ = tx.Flush()
	println("sum", sum, ptt)
}
