package c2mode

import "testing"

func FuzzScore(f *testing.F) {
	f.Add([]byte{0xB9, 0xE8, 0x81, 0x52, 0x61, 0x73, 0, 0, 0})
	f.Add([]byte{0xB9, 0xE8, 0x81, 0x52, 0x61, 0x73, 0x21, 0x08, 0})
	f.Fuzz(func(t *testing.T, raw []byte) {
		var d Detector
		for off := 0; off < len(raw); off += 9 {
			var s [9]byte
			copy(s[:], raw[off:])
			high := d.Add(&s)
			if d.score < -Limit || d.score > Limit {
				t.Fatalf("score %d out of range", d.score)
			}
			if m := d.Mode(); m != 0 && m != 2400 && m != 3200 || m == 3200 && !high || m == 2400 && high {
				t.Fatalf("Mode %d with Add %v", m, high)
			}
		}
	})
}
