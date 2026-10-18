// Command txiq runs the Tier 0 I/Q transmit modulator, an fsktx.MCUIQ, in
// static memory on a microcontroller, as a board feeds the I/Q DAC of a
// software radio at 48 kHz: one TDMA burst with its power ramps. It
// allocates nothing: it builds with -gc=none, which fails the link of any
// program that allocates.
package main

import "github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"

// The modulator, a burst, its symbols and the I/Q of one symbol live in
// static memory.
var (
	mod   fsktx.MCUIQ
	burst [33]byte
	syms  [132]int8
	out   [20]int16
	sum   int32
)

// write hands the I/Q of one symbol period to the DAC. A board writes them
// to its DAC here, from a timer interrupt or DMA; this one sums them.
func write(x []int16) {
	for _, v := range x {
		sum += int32(v)
	}
}

func main() {
	// 1 ms of ramp at 48 kHz, inside the 1.25 ms guard time of a burst.
	if err := mod.Init(48000, 0, 48); err != nil {
		return
	}
	x := byte(1)
	for i := range burst {
		x = x*37 + 11
		burst[i] = x
	}
	for range 100 {
		mod.Key(true)
		for range 6 {
			write(out[:mod.Symbol(out[:], 0)])
		}
		for _, v := range fsktx.Dibits(syms[:], burst[:]) {
			write(out[:mod.Symbol(out[:], v)])
		}
		mod.Key(false)
		for range 150 {
			write(out[:mod.Symbol(out[:], 0)])
		}
	}
	println("sum", sum)
}
