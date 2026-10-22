package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func runJitter(t *testing.T, delay, lead uint8, ops []byte) {
	t.Helper()
	j, err := voice.NewJitter(&voice.JitterConfig{Delay: time.Duration(int8(delay)) * 20 * time.Millisecond, Lead: time.Duration(lead) * time.Millisecond})
	if err != nil {
		return
	}
	var p voice.JitterPacket
	var raw layer2.RawBurst
	waiting := map[uint16]bool{}
	var seq [4]uint16
	id := uint16(0)
	now := 10 * time.Second
	var last, seen time.Duration
	hasLast := false
	for i := 0; i+2 < len(ops) && i < 3*512; i += 3 {
		a, b, c := ops[i], ops[i+1], ops[i+2]
		switch a & 7 {
		case 0, 1, 2:
			src := uint32(a >> 3 & 3)
			seq[src] += uint16(b & 3)
			h := layer2.Hint{Seq: seq[src] - uint16(b>>2&7), HasSeq: b&0x80 == 0, Stream: uint32(c & 3), HasStream: c&4 != 0}
			if c&8 != 0 {
				h.SeqBits, h.Seq = 8, h.Seq&0xFF
			}
			if c&16 != 0 {
				h.At = now - time.Duration(c>>5)*30*time.Millisecond
			}
			id++
			raw[0], raw[1] = byte(id>>8), byte(id)
			rp := &raw
			if a&0x80 != 0 {
				rp = nil
			}
			seen = now
			if r := j.Push(now, rp, h, src); r == voice.JitterBuffered && rp != nil {
				waiting[id] = true
			}
		case 3:
			now += time.Duration(b) * time.Millisecond
		case 4:
			now -= time.Duration(b) * 10 * time.Millisecond
			hasLast = false
		case 5:
			j.Reset()
			clear(waiting)
			hasLast = false
		default:
			for range int(b&7) + 1 {
				at, ok := j.Deadline()
				if !ok {
					break
				}
				if c&1 != 0 && at > now {
					now = at
				}
				reset := now < seen
				seen = now
				popped := j.Pop(now, &p)
				if !reset && popped != (at <= now) {
					t.Fatalf("op %d: Pop %v at %v with Deadline %v", i, popped, now, at)
				}
				if !popped {
					break
				}
				if p.Hint.At != p.Due {
					t.Fatalf("op %d: At %v, Due %v", i, p.Hint.At, p.Due)
				}
				if hasLast && p.Due < last+60*time.Millisecond {
					t.Fatalf("op %d: Due %v after %v", i, p.Due, last)
				}
				last, hasLast = p.Due, true
				if p.HasBurst {
					k := uint16(p.Burst[0])<<8 | uint16(p.Burst[1])
					if !waiting[k] {
						t.Fatalf("op %d: packet %d played twice or never buffered", i, k)
					}
					delete(waiting, k)
				}
			}
		}
		if j.Len() > 2*(3*int((voice.MaxJitter+59*time.Millisecond)/(60*time.Millisecond))+8) {
			t.Fatalf("op %d: %d waiting", i, j.Len())
		}
	}
}

func jitterSeeds() [][]byte {
	inorder := make([]byte, 0, 9*60)
	reorder := make([]byte, 0, 9*60)
	for i := range 60 {
		inorder = append(inorder, 0, 1, 0x18, 3, 60, 0, 6, 0, 1)
		reorder = append(reorder, byte(i%3)<<3, byte(i%4)|byte(i%3)<<2, 4|byte(i%2)<<3, 3, byte(20+i%50), 0, 7, 7, 0)
	}
	return [][]byte{inorder, reorder, {0, 1, 0, 3, 200, 0, 4, 100, 0, 0, 1, 0, 6, 7, 1}}
}

func FuzzJitter(f *testing.F) {
	for _, s := range jitterSeeds() {
		f.Add(uint8(0), uint8(0), s)
		f.Add(uint8(3), uint8(59), s)
	}
	f.Fuzz(func(t *testing.T, delay, lead uint8, ops []byte) {
		runJitter(t, delay, lead, ops)
	})
}
