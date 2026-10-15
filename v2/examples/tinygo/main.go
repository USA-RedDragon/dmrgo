// Command tinygo decodes and re-encodes the bursts of a short voice call on a microcontroller.
package main

import (
	_ "embed"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

//go:embed parrot_kerchunk.bin
var capture string

type summary struct {
	bursts, voice, data int
	reencoded           int
	corrected           int
	target, source      uint32
	err                 error
}

func run(b *layer2.Burst, stream string) summary {
	var s summary
	var in layer2.RawBurst
	for off := 0; off+len(in) <= len(stream); off += len(in) {
		copy(in[:], stream[off:])
		s.bursts++
		if err := b.DecodeFromBytes(in); err != nil {
			s.err = err
			return s
		}
		switch b.Kind {
		case layer2.KindVoice:
			s.voice++
		case layer2.KindData:
			s.data++
		default:
		}
		if lc, ok := b.Data.(*pdu.FullLinkControl); ok && s.source == 0 {
			s.target, s.source, _ = lc.Addresses()
		}
		s.corrected += b.FEC.Aggregate().ErrorsCorrected
		out, err := b.Encode()
		if err != nil {
			s.err = err
			return s
		}
		if out == in {
			s.reencoded++
		}
	}
	return s
}

func main() {
	var b layer2.Burst
	for {
		s := run(&b, capture)
		if s.err != nil {
			println("error at burst", s.bursts, s.err.Error())
		} else {
			println("bursts", s.bursts, "voice", s.voice, "data", s.data, "re-encoded", s.reencoded,
				"corrected", s.corrected, "call from", s.source, "to", s.target)
		}
		time.Sleep(time.Second)
	}
}
