// Command tinygo decodes and re-encodes the bursts of a short voice call on a microcontroller, and follows the call with a voice.Receiver.
package main

import (
	_ "embed"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

//go:embed parrot_kerchunk.bin
var capture string

type summary struct {
	bursts, voice, data int
	reencoded           int
	corrected           int
	target, source      uint32
	calls               int
	ended               voice.EndReason
	err                 error
}

// follower runs the bursts through a voice.Receiver, 60 ms apart.
type follower struct {
	rx     *voice.Receiver
	events []voice.Event
	now    time.Duration
}

func newFollower() *follower {
	rx, err := voice.NewReceiver(nil)
	if err != nil {
		panic(err)
	}
	return &follower{rx: rx, events: make([]voice.Event, 0, voice.MaxEvents)}
}

func (f *follower) count(s *summary) {
	for i := range f.events {
		switch f.events[i].Kind {
		case voice.EventCallStart:
			s.calls++
		case voice.EventCallEnd:
			s.ended = f.events[i].Reason
		default:
		}
	}
}

func run(b *layer2.Burst, f *follower, stream string) summary {
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
		f.now += 60 * time.Millisecond
		f.events = f.rx.Decoded(f.now, b, layer2.Hint{}, f.events[:0])
		f.count(&s)
		out, err := b.Encode()
		if err != nil {
			s.err = err
			return s
		}
		if out == in {
			s.reencoded++
		}
	}
	f.now += 10 * time.Second
	f.events = f.rx.Tick(f.now, f.events[:0])
	return s
}

func main() {
	var b layer2.Burst
	f := newFollower()
	for {
		s := run(&b, f, capture)
		if s.err != nil {
			println("error at burst", s.bursts, s.err.Error())
		} else {
			println("bursts", s.bursts, "voice", s.voice, "data", s.data, "re-encoded", s.reencoded,
				"corrected", s.corrected, "call from", s.source, "to", s.target, "calls", s.calls, "end", s.ended.String())
		}
		time.Sleep(time.Second)
	}
}
