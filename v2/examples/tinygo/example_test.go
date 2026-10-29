package main

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func Example() {
	var b layer2.Burst
	s := run(&b, newFollower(), capture)
	fmt.Println("bursts", s.bursts, "voice", s.voice, "data", s.data, "re-encoded", s.reencoded, "corrected", s.corrected)
	fmt.Println("call from", s.source, "to", s.target, "calls", s.calls, "end", s.ended.String(), "relayed", s.relayed)
	// Output:
	// bursts 15 voice 13 data 2 re-encoded 15 corrected 0
	// call from 3191868 to 9990 calls 1 end Terminator relayed 20
}
