package rf

import (
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
)

func TestRxConfigSize(t *testing.T) {
	t.Parallel()
	for _, typ := range []reflect.Type{reflect.TypeFor[RxConfig](), reflect.TypeFor[AudioRxConfig]()} {
		off := 0
		for i := range typ.NumField() {
			n, align := 4, 4
			switch k := typ.Field(i).Type.Kind(); k {
			case reflect.Bool, reflect.Uint8:
				n, align = 1, 1
			case reflect.Float64:
				n, align = 8, 8
			case reflect.Func:
				n = 8
			case reflect.Int, reflect.Pointer:
			default:
				t.Fatalf("field %s of %s has kind %s", typ.Field(i).Name, typ, k)
			}
			off = (off+align-1)/align*align + n
		}
		if off > 64 {
			t.Errorf("%s takes %d bytes on 32-bit ARM, more than 64", typ, off)
		}
	}
}

func TestRxModes(t *testing.T) {
	t.Parallel()
	var nilRx *Receiver
	if _, err := nilRx.Write(nil); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil Receiver Write: %v", err)
	}
	if err := nilRx.Flush(); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil Receiver Flush: %v", err)
	}
	if nilRx.Init(RxConfig{Rate: 9600}) == nil || nilRx.AppendBursts(nil, []byte{1}) != nil || nilRx.AppendFlush(nil) != nil ||
		nilRx.Active() || nilRx.Lag() != 0 || nilRx.MaxOutput(9) != 0 || nilRx.SampleBytes() != 0 || nilRx.OffsetHz() != 0 || nilRx.DeviationHz() != 0 {
		t.Error("a nil Receiver did something")
	}
}

func TestRxModesZero(t *testing.T) {
	t.Parallel()
	var ce *dmrgo.ConfigError
	var zero Receiver
	if _, err := zero.Write([]byte{1, 2}); !errors.Is(err, ErrNotSetUp) {
		t.Errorf("zero Receiver Write: %v", err)
	}
	if err := zero.Flush(); !errors.Is(err, ErrNotSetUp) {
		t.Errorf("zero Receiver Flush: %v", err)
	}
	if zero.AppendBursts(nil, []byte{1}) != nil || zero.AppendFlush(nil) != nil || zero.Lag() != 0 || zero.SampleBytes() != 0 {
		t.Error("a zero Receiver did something")
	}
	app := mustNew(RxConfig{Tier: Tier0})
	if _, err := app.Write([]byte{1}); !errors.As(err, &ce) || ce.Field != "RxConfig.OnBursts" {
		t.Errorf("append mode Write: %v", err)
	}
	if err := app.Flush(); !errors.As(err, &ce) || ce.Field != "RxConfig.OnBursts" {
		t.Errorf("append mode Flush: %v", err)
	}
}

func TestRxModesStream(t *testing.T) {
	t.Parallel()
	st := mustNew(RxConfig{Tier: Tier0, OnBursts: func([]Burst) {}})
	dst := make([]Burst, 1, 4)
	if got := st.AppendBursts(dst, make([]byte, 64)); len(got) != 1 || cap(got) != 4 {
		t.Error("stream mode AppendBursts changed dst")
	}
	if got := st.AppendFlush(dst); len(got) != 1 || cap(got) != 4 {
		t.Error("stream mode AppendFlush changed dst")
	}
	if n, err := st.Write(make([]byte, 7)); n != 7 || err != nil {
		t.Errorf("stream mode Write: %d, %v", n, err)
	}
	for _, c := range []struct {
		f IQFormat
		n int
	}{{IQFormatDefault, 4}, {IQFormatCU8, 2}, {IQFormatCS8, 2}, {IQFormatCS16, 4}, {IQFormatCF32, 8}} {
		if n := mustNew(RxConfig{Format: c.f}).SampleBytes(); n != c.n {
			t.Errorf("format %d: SampleBytes %d, want %d", c.f, n, c.n)
		}
	}
}

func TestAudioModes(t *testing.T) {
	t.Parallel()
	var ce *dmrgo.ConfigError
	var nilAudio *AudioReceiver
	if _, err := nilAudio.Write(nil); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Errorf("nil AudioReceiver Write: %v", err)
	}
	if nilAudio.Init(AudioRxConfig{}) == nil || nilAudio.AppendBursts(nil, []int16{1}) != nil || nilAudio.Active() || nilAudio.Level() != 0 || nilAudio.Offset() != 0 || nilAudio.MaxOutput(0) != 0 {
		t.Error("a nil AudioReceiver did something")
	}
	var za AudioReceiver
	if _, err := za.Write([]int16{1}); !errors.Is(err, ErrNotSetUp) {
		t.Errorf("zero AudioReceiver Write: %v", err)
	}
	if err := za.Flush(); !errors.Is(err, ErrNotSetUp) {
		t.Errorf("zero AudioReceiver Flush: %v", err)
	}
	if err := za.Init(AudioRxConfig{Tier: Tier1}); !errors.As(err, &ce) || ce.Field != "AudioRxConfig.Tier" || za.set {
		t.Errorf("Init at Tier1: %v", err)
	}
	if err := za.Init(AudioRxConfig{}); err != nil {
		t.Fatal(err)
	}
	if _, err := za.Write([]int16{1}); !errors.As(err, &ce) || ce.Field != "AudioRxConfig.OnBursts" {
		t.Errorf("append mode Write: %v", err)
	}
}

func TestRxFormats(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 3)
	x := noisyBaseband(slots, 1400, 25, 7)
	for _, rate := range []int{basebandRate, 5 * basebandRate} {
		y := x
		if rate != basebandRate {
			y = dgUpsample(x, rate)
		}
		for _, f := range []IQFormat{IQFormatCU8, IQFormatCS8, IQFormatCS16, IQFormatCF32} {
			in := iqBytes(y, f)
			for _, tier := range []Tier{Tier0, Tier1} {
				r := mustNew(RxConfig{Tier: tier, Rate: rate, Format: f})
				var got []Burst
				for k := 0; k < len(in); k += 3001 {
					got = r.AppendBursts(got, in[k:min(k+3001, len(in))])
				}
				got = r.AppendFlush(got)
				if exact, total := exactFrames(slots, got); exact < total*9/10 || total == 0 {
					t.Errorf("tier %d, format %d at %d Hz: %d of %d voice frames exact", tier-Tier0, f, rate, exact, total)
				}
			}
		}
	}
}

func dgUpsample(x []complex64, rate int) []complex64 {
	n := len(x) * rate / basebandRate
	y := make([]complex64, n)
	for i := range y {
		u := float64(i) * basebandRate / float64(rate)
		k := int(u)
		if k+1 >= len(x) {
			break
		}
		f := float32(u - float64(k))
		y[i] = x[k]*complex(1-f, 0) + x[k+1]*complex(f, 0)
	}
	return y
}

func iqBytes(x []complex64, f IQFormat) []byte {
	b := make([]byte, 0, f.bytes()*len(x))
	for _, v := range x {
		for _, c := range [2]float32{real(v), imag(v)} {
			switch f {
			case IQFormatCU8:
				b = append(b, byte(max(0, min(255, math.Round(float64(c)*0.5*127.5+127.5)))))
			case IQFormatCS8:
				b = append(b, byte(int8(max(-127, min(127, math.Round(float64(c)*0.5*127.5))))))
			case IQFormatCS16:
				b = binary.LittleEndian.AppendUint16(b, uint16(int16(max(-32767, min(32767, math.Round(float64(c)*0.5*32767))))))
			default:
				b = binary.LittleEndian.AppendUint32(b, math.Float32bits(c*0.5))
			}
		}
	}
	return b
}

func TestAudioBits(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 3)
	a := discAudio(noisyBaseband(slots, 1400, 20, 8), 9600)
	for _, c := range []struct {
		bits     int
		unsigned bool
	}{{12, true}, {12, false}, {8, true}, {16, true}} {
		shift := 16 - c.bits
		codes := make([]int16, len(a))
		ref := make([]int16, len(a))
		for i, v := range a {
			k := int32(v) >> shift
			ref[i] = int16(k << shift)
			if c.unsigned {
				k += 1 << (c.bits - 1)
			}
			codes[i] = int16(uint16(k))
		}
		want := mustAudio(AudioRxConfig{Rate: 9600, Tier: Tier0})
		wb := want.AppendFlush(want.AppendBursts(nil, ref))
		r := mustAudio(AudioRxConfig{Rate: 9600, Tier: Tier0, Bits: c.bits, Unsigned: c.unsigned})
		got := r.AppendFlush(r.AppendBursts(nil, codes))
		if len(wb) == 0 {
			t.Fatalf("%+v: no bursts", c)
		}
		sameBursts(t, "bits", wb, got)
		g := float64(int(1) << shift)
		if l := r.Level(); math.Abs(float64(l)-float64(want.Level())/g) > 1 || l <= 0 {
			t.Errorf("%+v: Level %d, 16-bit level %d", c, l, want.Level())
		}
		off := float64(want.Offset()) / g
		if c.unsigned {
			off += float64(int(1) << (c.bits - 1))
		}
		if o := r.Offset(); math.Abs(float64(o)-off) > 1 {
			t.Errorf("%+v: Offset %d, want %.1f", c, o, off)
		}
	}
}

func TestRxLag(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 4)
	long, _ := benchStream(t, 16)
	tiers := []Tier{Tier0, Tier1, Tier2}
	if !testing.Short() {
		tiers = append(tiers, Tier3)
	}
	const chunk = 16
	for _, tier := range tiers {
		x := noisyBaseband(slots, 1400, 12, 3)
		if tier == Tier3 {
			x = noisyBaseband(long, 1400, 12, 3)
		}
		x = x[int(0.9*basebandRate):]
		for _, hz := range []int{9600, 48000} {
			a := discAudio(x, hz)
			fed, worst, n := 0, 0.0, 0
			r := mustAudio(AudioRxConfig{Rate: hz, Tier: tier, OnBursts: func(bs []Burst) {
				for _, b := range bs {
					worst, n = max(worst, float64(fed)-(b.Seconds+burstSeconds)*float64(hz)), n+1
				}
			}})
			for k := 0; k < len(a); k += chunk {
				fed = min(k+chunk, len(a))
				_, _ = r.Write(a[k:fed])
			}
			if tier == Tier3 {
				_ = r.Flush()
			}
			if n == 0 || worst > float64(r.Lag()+chunk) {
				t.Errorf("audio tier %d at %d Hz: %d bursts, a lag of %.0f samples, Lag %d", tier-Tier0, hz, n, worst, r.Lag())
			}
		}
		rates := []int{48000, 240000}
		switch tier {
		case Tier0:
			rates = append(rates, 24000, 9600, 12000)
		case Tier1:
			rates = append(rates, 24000)
		case Tier3:
			rates = rates[:1]
		case TierDefault, Tier2:
		}
		for _, rate := range rates {
			var y []complex64
			switch {
			case rate == 24000:
				y = make([]complex64, len(x)/2)
				for i := range y {
					y[i] = (x[2*i] + x[2*i+1]) / 2
				}
			case rate < 24000:
				y = lowIQ(x, rate)
			default:
				y = dgUpsample(x, rate)
			}
			in := cf32(y)
			fed, worst, n := 0, 0.0, 0
			r := mustNew(RxConfig{Rate: rate, Tier: tier, Format: IQFormatCF32, OnBursts: func(bs []Burst) {
				for _, b := range bs {
					worst, n = max(worst, float64(fed/8)-(b.Seconds+burstSeconds)*float64(rate)), n+1
				}
			}})
			step := 8 * max(chunk, rate/3000)
			for k := 0; k < len(in); k += step {
				fed = min(k+step, len(in))
				_, _ = r.Write(in[k:fed])
			}
			if tier == Tier3 {
				_ = r.Flush()
			}
			if n == 0 || worst > float64(r.Lag()+step/8) {
				t.Errorf("I/Q tier %d at %d Hz: %d bursts, a lag of %.0f samples, Lag %d", tier-Tier0, rate, n, worst, r.Lag())
			}
		}
	}
}

func TestRxLookbackOneCall(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 2)
	x := noisyBaseband(slots, 1400, 20, 3)
	x = x[int(0.9*basebandRate):]
	in := cf32(x)
	most := 0
	r := mustNew(RxConfig{Tier: Tier1, Rate: basebandRate, Format: IQFormatCF32, OnBursts: func(bs []Burst) { most = max(most, len(bs)) }})
	for k := 0; k < len(in); k += 8 * 64 {
		_, _ = r.Write(in[k:min(k+8*64, len(in))])
	}
	if most < 2 {
		t.Errorf("a late lock gave its bursts in calls of at most %d", most)
	}
}
