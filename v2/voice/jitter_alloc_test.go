//go:build !tinygo

package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestAllocsJitter(t *testing.T) {
	j := newJitter(t, nil)
	var raw layer2.RawBurst
	var p voice.JitterPacket
	now := time.Duration(0)
	seq := uint16(0)
	n := testing.AllocsPerRun(1000, func() {
		j.Push(now, &raw, layer2.Hint{Seq: seq, HasSeq: true, SeqBits: 8}, 1)
		seq++
		now += burst
		for {
			at, ok := j.Deadline()
			if !ok || at > now {
				break
			}
			j.Pop(now, &p)
		}
	})
	if n != 0 {
		t.Fatalf("%v allocations", n)
	}
}
