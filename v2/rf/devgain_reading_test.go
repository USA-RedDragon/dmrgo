//go:build !tinygo

package rf

import (
	"math"
	"slices"
	"strings"
	"testing"
)

func mxRowOf(t *testing.T, name string) mxRow {
	t.Helper()
	var row mxRow
	parts := strings.Split(name, ".")
	if len(parts) < mxBaseDims || len(parts) > mxDims {
		t.Fatalf("row %s", name)
	}
	d := 0
	for _, p := range parts {
		i := slices.Index(mxValues[d], p)
		for d >= mxBaseDims && i < 0 && d+1 < mxDims {
			d++
			i = slices.Index(mxValues[d], p)
		}
		if i < 0 {
			t.Fatalf("row %s: no %s value %s", name, mxDimName(d), p)
		}
		row[d] = uint8(i)
		d++
	}
	if row.name() != name {
		t.Fatalf("row %s reads as %s", name, row.name())
	}
	return row
}

func TestIQDeviationReading(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"cs16-48k", "cu8-2048k"} {
		for _, off := range []string{"+0", "+1500", "-2500", "+4000"} {
			row := mxRowOf(t, "s0.none."+off+".ms.data12.0ppm.nom."+format+".inf")
			s, err := mxBuild(row)
			if err != nil {
				t.Fatal(err)
			}
			in := mxInputOf(s, row)
			for _, x := range mxReceivers(format, true) {
				got, dev, err := mxReceive(in, x)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) == 0 || math.Abs(dev/mxOuterHz-1) > mxDevHigh {
					t.Errorf("%s %s Hz T%d: %d bursts, %.1f Hz, want %.0f within %.0f%%", format, off, x.tier-Tier0, len(got), dev, mxOuterHz, 100*mxDevHigh)
				}
			}
		}
	}
	for _, c := range []struct {
		hz  int
		tol float64
	}{{iq0LowHz, 0.04}, {discRate / 2, mxDevHigh}} {
		for _, off := range []string{"+0", "-1500"} {
			sig, err := mxBuild(mxRowOf(t, "s0.none."+off+".ms.data12.0ppm.nom.cs16-48k.inf"))
			if err != nil {
				t.Fatal(err)
			}
			x := sig.baseband()
			var b []byte
			for i := 0; i < len(x); i += basebandRate / c.hz {
				b = appendIQ(b, false, 0.3*float64(real(x[i])), 0.3*float64(imag(x[i])))
			}
			r, err := NewReceiver(&RxConfig{Rate: c.hz, Tier: Tier0, Format: IQFormatCS16})
			if err != nil {
				t.Fatal(err)
			}
			got := r.AppendFlush(r.AppendBursts(nil, b))
			if dev := r.DeviationHz(); len(got) == 0 || math.Abs(dev/mxOuterHz-1) > c.tol {
				t.Errorf("low-rate %d Hz %s Hz: %d bursts, %.1f Hz, want %.0f within %.1f%%", c.hz, off, len(got), dev, mxOuterHz, 100*c.tol)
			}
		}
	}
}

func TestCombDeviationFallback(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "agc.noise.-1500.ms.data1.-500ppm.nom.cu8-48k.inf")
	sig, err := mxBuild(row)
	if err != nil {
		t.Fatal(err)
	}
	got, dev, err := mxReceive(mxInputOf(sig, row), mxRx{"cu8-48k", Tier3})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || math.Abs(dev/mxOuterHz-1) > mxDevHigh {
		t.Errorf("Tier3 with only its Tier2 side locked: %d bursts, %.1f Hz", len(got), dev)
	}
}

func TestDeviationAfterRetune(t *testing.T) {
	t.Parallel()
	const offHz = 1400
	row := mxRowOf(t, "s0.none.+1500.ms.data12.0ppm.nom.cs16-48k.inf")
	s, err := mxBuild(row)
	if err != nil {
		t.Fatal(err)
	}
	s.parts[0].hz = offHz
	in := mxInputOf(s, row)
	last := s.truth[len(s.truth)-1]
	cut := int((last.at+float64(burstSymbols)/symbolRate)*float64(in.rate)) * in.iqFmt.bytes()
	read := func(retune bool) (float64, int) {
		r, err := NewReceiver(&RxConfig{Rate: in.rate, Tier: Tier0, Format: in.iqFmt})
		if err != nil {
			t.Fatal(err)
		}
		got := r.AppendBursts(nil, in.iq[:cut])
		h, ok := r.full.(*rxHost)
		if !ok || h.iqd == nil {
			t.Fatal("no I/Q tier 0 receiver")
		}
		iq := h.iqd
		if retune {
			if !iq.d.locked {
				t.Fatal("the stream is not locked at the retune")
			}
			iq.d.write(iq.audio)
			iq.audio = iq.audio[:0]
			f := &iq.d.f
			iq.tune(offHz, f.dc-offHz*f.gain*iqUnitsPerHz)
		}
		got = r.AppendFlush(r.AppendBursts(got, in.iq[cut:]))
		return r.DeviationHz(), len(got)
	}
	want, n0 := read(false)
	got, n := read(true)
	if n != n0 || n == 0 || math.Abs(got/want-1) > 0.005 {
		t.Errorf("%.1f Hz after a locked retune of %d Hz, %.1f Hz without it (%d and %d bursts)", got, offHz, want, n, n0)
	}
}
