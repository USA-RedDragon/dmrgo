package main

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
)

func testConfig() SeqConfig {
	return SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
}

// channel resamples signed 8-bit I/Q from txRate to rxRate, adds a carrier
// offset and white noise at snrDB, and returns rtl_sdr style unsigned bytes.
func channel(iq []byte, txRate, rxRate, offsetHz, snrDB float64, seed uint64) []byte {
	x := FromHackRF(iq)
	n := int(float64(len(x)) * rxRate / txRate)
	rng := rand.New(rand.NewPCG(seed, seed+1)) //nolint:gosec // deterministic test noise
	sigma := math.Sqrt(0.81/math.Pow(10, snrDB/10)) / math.Sqrt2
	out := make([]byte, 2*n)
	for i := range n {
		pos := float64(i) * txRate / rxRate
		k := int(pos)
		v := x[k]
		if k+1 < len(x) {
			v += (x[k+1] - x[k]) * complex(pos-float64(k), 0)
		}
		v *= cmplx.Exp(complex(0, 2*math.Pi*offsetHz*float64(i)/rxRate))
		v += complex(rng.NormFloat64()*sigma, rng.NormFloat64()*sigma)
		clip := func(f float64) byte { return byte(math.Max(0, math.Min(255, math.Round(f*127.5+127.5)))) }
		out[2*i], out[2*i+1] = clip(real(v)*0.8), clip(imag(v)*0.8)
	}
	return out
}

func TestLoopbackDecodesEverySequence(t *testing.T) {
	for _, seq := range []string{"data12", "data34", "data1"} {
		cfg := testConfig()
		bursts, err := Sequence(seq, cfg)
		if err != nil {
			t.Fatal(err)
		}
		fm, env := Modulate(bursts, Schedule{Slot: 0})
		iq := ToIQ(fm, env, 2e6, 0, 0.9)
		rx := channel(iq, 2e6, 2.048e6, 3000, 25, 1)
		d := Demodulate(FromRTL(rx), 2.048e6)
		if len(d.Bursts) != len(bursts) {
			t.Fatalf("%s: got %d bursts, want %d", seq, len(d.Bursts), len(bursts))
		}
		var asm packet.Assembler
		var got string
		for i, r := range d.Bursts {
			if r.Data != bursts[i] {
				t.Errorf("%s burst %d: bits differ", seq, i)
			}
			b, err := layer2.NewBurstFromBytes(r.Data)
			if err != nil {
				t.Fatalf("%s burst %d: %v", seq, i, err)
			}
			if m, ok, err := asm.AddBurst(b); err != nil {
				t.Fatalf("%s: %v", seq, err)
			} else if ok {
				got = string(m.Data)
			}
		}
		if got != cfg.Message {
			t.Errorf("%s: message %q, want %q", seq, got, cfg.Message)
		}
		if math.Abs(d.OffsetHz-3000) > 50 {
			t.Errorf("%s: offset %.0f Hz, want 3000 within 50", seq, d.OffsetHz)
		}
		if math.Abs(d.DeviationHz-1944)/1944 > 0.05 {
			t.Errorf("%s: outer deviation %.0f Hz, want 1944 within 5%%", seq, d.DeviationHz)
		}
	}
}

func TestBitErrorRateBySNR(t *testing.T) {
	cfg := testConfig()
	cfg.Wakeups = 40
	bursts, err := Sequence("wakeup", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := Modulate(bursts, Schedule{Slot: -1})
	iq := ToIQ(fm, env, 2e6, 0, 0.9)
	for _, snr := range []float64{30, 20, 15} {
		d := Demodulate(FromRTL(channel(iq, 2e6, 2.048e6, -2500, snr, 7)), 2.048e6)
		var errs, total int
		for _, r := range d.Bursts {
			for i := range r.Data {
				errs += popcount(r.Data[i] ^ bursts[0][i])
				total += 8
			}
		}
		t.Logf("SNR %2.0f dB (in 2 MHz): %d bursts, BER %.2e", snr, len(d.Bursts), float64(errs)/float64(max(total, 1)))
		if snr >= 20 && (len(d.Bursts) != len(bursts) || errs != 0) {
			t.Errorf("SNR %.0f dB: %d of %d bursts, %d bit errors", snr, len(d.Bursts), len(bursts), errs)
		}
	}
}

func popcount(b byte) int {
	n := 0
	for ; b != 0; b &= b - 1 {
		n++
	}
	return n
}

func TestOccupiedBandwidth(t *testing.T) {
	cfg := testConfig()
	cfg.Wakeups = 30
	bursts, err := Sequence("wakeup", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, _ := Modulate(bursts, Schedule{Slot: -1})
	env := make([]float64, len(fm))
	for i := range env {
		env[i] = 1
	}
	x := FromHackRF(ToIQ(fm, env, basebandRate, 0, 0.9))
	const n = 4096
	psd := make([]float64, n)
	{
		start := 0
		for k := range n {
			var s complex128
			for i := range n {
				w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/n)
				s += x[start+i] * complex(w, 0) * cmplx.Exp(complex(0, -2*math.Pi*float64(k*i)/n))
			}
			psd[k] += real(s)*real(s) + imag(s)*imag(s)
		}
	}
	var total, inBand float64
	for k, p := range psd {
		f := float64(k) * basebandRate / n
		if f > basebandRate/2 {
			f -= basebandRate
		}
		total += p
		if math.Abs(f) <= 6250 {
			inBand += p
		}
	}
	if frac := inBand / total; frac < 0.99 {
		t.Errorf("%.4f of power within ±6.25 kHz, want at least 0.99", frac)
	} else {
		t.Logf("%.5f of power within the 12.5 kHz channel", frac)
	}
}

func TestCalMeasuresCarrier(t *testing.T) {
	iq := CW(0.2, 2e6, 0, 0.9)
	rx := channel(iq, 2e6, 2.048e6, -1234, 30, 3)
	off := carrierOffset(decimate(FromRTL(rx), 2.048e6))
	if math.Abs(off+1234) > 20 {
		t.Errorf("offset %.1f Hz, want -1234", off)
	}
}
