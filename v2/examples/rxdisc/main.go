// Command rxdisc runs the Tier 0 receiver, an rf.DiscReceiver, in static
// memory on a microcontroller, as a hotspot board feeds it the samples of
// its radio chip's FM discriminator. It allocates nothing: it builds with
// -gc=none, which fails the link of any program that allocates.
package main

import "github.com/USA-RedDragon/dmrgo/v2/rf"

// The receiver, its input buffer and the count of bursts live in static
// memory.
var (
	rx     rf.DiscReceiver
	buf    [256]int16
	bursts int
)

// onBurst counts each burst. A hotspot would hand it to its modem
// protocol here; the burst is valid only during the call.
func onBurst(*rf.Burst) { bursts++ }

// read fills x with discriminator samples at 9.6 kHz. A board reads its
// ADC here; this one gives silence.
func read(x []int16) {
	clear(x)
}

func main() {
	if err := rx.Init(rf.DiscConfig{Hz: 9600, OnBurst: onBurst}); err != nil {
		return
	}
	for range 1000 {
		read(buf[:])
		rx.Write(buf[:])
	}
	rx.Flush()
	println("bursts", bursts)
}
