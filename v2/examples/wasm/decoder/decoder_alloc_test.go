package main

import "testing"

func TestAllocsWrite(t *testing.T) {
	noise := make([]byte, 1<<15)
	x := uint32(1)
	for i := range noise {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		noise[i] = byte(127 + int(x%7) - 3)
	}
	for _, opt := range []options{
		{format: "cu8", rate: 2048000, audioTier: -1, iqTier: 2},
		{format: "cu8", rate: 2048000, audioTier: -1, iqTier: 1},
		{format: "cs16", rate: 256000, audioTier: -1, iqTier: 2},
		{format: "f32", rate: 48000, audioTier: 1, iqTier: -1},
		{format: "f32", rate: 44100, audioTier: 2, iqTier: -1},
	} {
		d, err := newDecoder(opt)
		if err != nil {
			t.Fatal(err)
		}
		if err := d.write(noise); err != nil {
			t.Fatal(err)
		}
		n := testing.AllocsPerRun(20, func() {
			if err := d.write(noise[:len(noise)-3]); err != nil {
				t.Fatal(err)
			}
		})
		if n != 0 || d.rep.bursts != 0 {
			t.Errorf("%s at %g Hz: %v allocations per write of noise, %d bursts", opt.format, opt.rate, n, d.rep.bursts)
		}
	}
}
