// Command txaudio runs the Tier 0 transmit modulator, an
// fsktx.MCUAudio, in static memory on a microcontroller, as a hotspot
// board drives the modulator input of its radio from a 12-bit DAC at 24
// kHz. It allocates nothing: it builds with -gc=none, which fails the link
// of any program that allocates.
package main

import "github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"

var (
	mod   fsktx.MCUAudio
	burst [33]byte
	syms  [132]int8
	out   [5]int16
	sum   int32
)

func write(x []int16) {
	for _, v := range x {
		sum += int32(v)
	}
}

func main() {
	if err := mod.Init(fsktx.Table24k(), 1080, 2048); err != nil {
		return
	}
	x := byte(1)
	for i := range burst {
		x = x*37 + 11
		burst[i] = x
	}
	for range 100 {
		s := fsktx.Dibits(syms[:], burst[:])
		for _, v := range s {
			write(out[:mod.Symbol(out[:], v)])
		}
	}
	for range 4 {
		write(out[:mod.Symbol(out[:], 0)])
	}
	println("sum", sum)
}
