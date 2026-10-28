// Command rxaudio runs the Tier 0 receiver, an rf.AudioReceiver, in static
// memory on a microcontroller, as a hotspot board feeds it the samples of
// its radio chip's FM discriminator. It allocates nothing: it builds with
// -gc=none, which fails the link of any program that allocates.
package main

import "github.com/USA-RedDragon/dmrgo/v2/rf"

var (
	rx     rf.AudioReceiver
	buf    [256]int16
	bursts int
)

func onBursts(bs []rf.Burst) { bursts += len(bs) }

func read(x []int16) {
	clear(x)
}

func main() {
	if err := rx.Init(rf.AudioRxConfig{Rate: 9600, OnBursts: onBursts}); err != nil {
		return
	}
	for range 1000 {
		read(buf[:])
		if _, err := rx.Write(buf[:]); err != nil {
			return
		}
	}
	if err := rx.Flush(); err != nil {
		return
	}
	println("bursts:", bursts > 0)
}
