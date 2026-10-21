package mmdvm_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func TestAllocsMMDVM(t *testing.T) {
	var stream []byte
	for _, s := range fuzzSeeds() {
		stream = append(stream, s...)
	}
	buf := make([]byte, 0, 128)
	out := make([]byte, 0, 128)
	n := testing.AllocsPerRun(100, func() {
		p := stream
		for len(p) > 0 {
			f, n, st := mmdvm.Next(p)
			if st == mmdvm.StatusMore {
				break
			}
			p = p[n:]
			if st != mmdvm.StatusFrame {
				continue
			}
			if d, err := mmdvm.DecodeDMRData(f); err == nil {
				buf, _ = d.AppendBinary(buf[:0])
				out, _ = mmdvm.AppendFrame(out[:0], d.Command(), buf)
				_ = d.Hint()
			}
			if s, err := mmdvm.DecodeStatusV1(f.Payload); err == nil {
				buf, _ = s.AppendBinary(buf[:0])
			}
			if c, err := mmdvm.DecodeConfigV2(f.Payload); err == nil {
				buf, _ = c.AppendBinary(buf[:0])
			}
			if v, err := mmdvm.DecodeVersion(f.Payload); err == nil {
				buf, _ = v.AppendBinary(buf[:0])
			}
			if d, err := mmdvm.DecodeDebug(f); err == nil {
				buf, _ = d.AppendBinary(buf[:0])
			}
		}
	})
	if n != 0 {
		t.Fatalf("%v allocations", n)
	}
}
