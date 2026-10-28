package rf

import "testing"

func TestAllocsTransmitter(t *testing.T) {
	b := msBursts(t, "voice")
	for _, c := range []TxConfig{usIQ(Tier0, 96000, IQFormatCS16), usIQ(Tier1, 2400000, IQFormatCF32), usIQ(Tier2, 250000, IQFormatCU8)} {
		c.OnSamples = func([]byte) {}
		tx, err := newTransmitter(c, openGate)
		if err != nil {
			t.Fatal(err)
		}
		i := 0
		n := testing.AllocsPerRun(len(b)-1, func() {
			_, _ = tx.Write(b[i : i+1])
			i++
		})
		if n != 0 || tx.Flush() != nil {
			t.Errorf("%+v: %v allocations per Write", c, n)
		}
		c.OnSamples = nil
		ap, _ := newTransmitter(c, openGate)
		dst := make([]byte, 0, ap.MaxOutput(1)*ap.SampleBytes())
		i = 0
		n = testing.AllocsPerRun(len(b)-1, func() {
			dst = ap.AppendSamples(dst[:0], b[i:i+1])
			i++
		})
		if n != 0 {
			t.Errorf("%+v: %v allocations per AppendSamples", c, n)
		}
	}
	for _, c := range []AudioTxConfig{usAudio(Tier0, 48000, 12), usAudio(Tier1, 44100, 16), usAudio(Tier2, 48000, 16)} {
		c.OnSamples = func([]int16) {}
		tx, err := newAudioTransmitter(c, openGate)
		if err != nil {
			t.Fatal(err)
		}
		i := 0
		n := testing.AllocsPerRun(len(b)-1, func() {
			_, _ = tx.Write(b[i : i+1])
			i++
		})
		if n != 0 || tx.Flush() != nil {
			t.Errorf("%+v: %v allocations per Write", c, n)
		}
	}
}
