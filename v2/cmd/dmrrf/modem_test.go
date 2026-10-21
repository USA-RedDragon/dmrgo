package main

import (
	"math"
	"math/cmplx"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
)

func testConfig() rftx.SeqConfig {
	return rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
}

func TestLoopbackDecodesEverySequence(t *testing.T) {
	t.Parallel()

	for _, seq := range []string{"data12", "data34", "data1", "data12u", "data34u", "data1u"} {
		cfg := testConfig()
		built, err := rftx.Sequence(seq, cfg)
		if err != nil {
			t.Fatal(err)
		}
		bursts := built.Bursts
		fm, env := rftx.Modulate(bursts, rftx.Schedule{Slot: 0})
		iq := rftx.ToIQ(fm, env, 2e6, 0, 0.9)
		rx := rftx.Channel(iq, 2e6, 2.048e6, 3000, 25, 1)
		d := demodulate(t, rftx.FromCU8(rx), 2.048e6)
		if len(d.Bursts) != len(bursts) {
			t.Fatalf("%s: got %d bursts, want %d", seq, len(d.Bursts), len(bursts))
		}
		asm := packet.NewAssembler(127)
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
	t.Parallel()

	cfg := testConfig()
	cfg.Wakeups = 40
	built, err := rftx.Sequence("wakeup", cfg)
	if err != nil {
		t.Fatal(err)
	}
	bursts := built.Bursts
	fm, env := rftx.Modulate(bursts, rftx.Schedule{Slot: -1})
	iq := rftx.ToIQ(fm, env, 2e6, 0, 0.9)
	for _, snr := range []float64{30, 20, 15} {
		d := demodulate(t, rftx.FromCU8(rftx.Channel(iq, 2e6, 2.048e6, -2500, snr, 7)), 2.048e6)
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

func TestBitErrorRateWeakBS(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes = true, 2
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.ModulateBS(built.Slots)
	iq := rftx.ToIQ(fm, env, 2e6, 0, 0.9)
	for seed := range uint64(3) {
		d := demodulate(t, rftx.FromCU8(rftx.Channel(iq, 2e6, 2.4e6, 3000, -10, seed)), 2.4e6)
		if len(d.Bursts) != len(built.Slots) {
			t.Errorf("seed %d: %d bursts, want %d", seed, len(d.Bursts), len(built.Slots))
			continue
		}
		var errs int
		for i, r := range d.Bursts {
			for k := range r.Data {
				errs += popcount(r.Data[k] ^ built.Slots[i].Burst[k])
			}
		}
		if total := len(built.Slots) * layer2.BurstBits; errs*1000 > total {
			t.Errorf("seed %d: %d bit errors in %d bits", seed, errs, total)
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
	t.Parallel()

	cfg := testConfig()
	cfg.Wakeups = 30
	built, err := rftx.Sequence("wakeup", cfg)
	if err != nil {
		t.Fatal(err)
	}
	bursts := built.Bursts
	fm, _ := rftx.Modulate(bursts, rftx.Schedule{Slot: -1})
	env := make([]float64, len(fm))
	for i := range env {
		env[i] = 1
	}
	x := rftx.FromCS8(rftx.ToIQ(fm, env, basebandRate, 0, 0.9))
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
	t.Parallel()

	iq := rftx.CW(0.2, 2e6, 0, 0.9)
	rx := rftx.Channel(iq, 2e6, 2.048e6, -1234, 30, 3)
	off := carrierOffset(decimate(rftx.FromCU8(rx), 2.048e6))
	if math.Abs(off+1234) > 20 {
		t.Errorf("offset %.1f Hz, want -1234", off)
	}
}

func TestSteadySymbolsHoldSpecDeviation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		fill byte
		want float64
	}{{0x55, 1944}, {0x00, 648}, {0xAA, -648}, {0xFF, -1944}} {
		var b [33]byte
		for i := range b {
			b[i] = tc.fill
		}
		fm, _ := rftx.Modulate([]layer2.RawBurst{b}, rftx.Schedule{Slot: 0})
		mid := int((rftx.SlotSeconds/2 - 0.005) * basebandRate)
		for i := mid; i < mid+int(0.01*basebandRate); i++ {
			if math.Abs(fm[i]-tc.want) > 0.01*math.Abs(tc.want) {
				t.Fatalf("fill %#x: %.1f Hz at sample %d, want %.0f", tc.fill, fm[i], i, tc.want)
			}
		}
	}
}
