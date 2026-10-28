package rf

import (
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"
)

var openGate = &txGate{noSpur: true}

func errField(err error) (field string, region bool) {
	var ce *dmrgo.ConfigError
	if errors.As(err, &ce) {
		return ce.Field, false
	}
	var re *RegionError
	if errors.As(err, &re) {
		return re.Field, true
	}
	if err != nil {
		return err.Error(), false
	}
	return "", false
}

type refusal struct {
	name   string
	gate   *txGate
	iq     *TxConfig
	audio  *AudioTxConfig
	field  string
	region bool
}

func refusals() []refusal {
	us := func(c TxConfig) *TxConfig { c.Region, c.Service, c.Power = RegionUS, ServiceLandMobile, 40; return &c }
	ua := func(c AudioTxConfig) *AudioTxConfig {
		c.Region, c.Service, c.Power = RegionUS, ServiceLandMobile, 40
		return &c
	}
	return []refusal{
		{"zero I/Q", nil, &TxConfig{}, nil, "TxConfig.Power", true},
		{"zero audio", nil, nil, &AudioTxConfig{}, "AudioTxConfig.Power", true},
		{"US land mobile 40 dBm", nil, us(TxConfig{}), nil, "TxConfig.Region", true},
		{"Tier3", nil, &TxConfig{Tier: Tier3}, nil, "TxConfig.Tier", false},
		{"Tier 9", nil, &TxConfig{Tier: 9}, nil, "TxConfig.Tier", false},
		{"Tier -1", nil, nil, &AudioTxConfig{Tier: -1}, "AudioTxConfig.Tier", false},
		{"format 9", nil, &TxConfig{Format: 9}, nil, "TxConfig.Format", false},
		{"rate 47999", nil, &TxConfig{Rate: 47999}, nil, "TxConfig.Rate", false},
		{"rate 20000001", nil, &TxConfig{Rate: 20000001}, nil, "TxConfig.Rate", false},
		{"rate -1", nil, &TxConfig{Rate: -1}, nil, "TxConfig.Rate", false},
		{"Tier0 at 192 kHz", nil, &TxConfig{Tier: Tier0, Rate: 192000}, nil, "TxConfig.Rate", false},
		{"Tier0 at 2.4 MHz", nil, &TxConfig{Tier: Tier0}, nil, "TxConfig.Rate", false},
		{"offset Rate/4", nil, &TxConfig{OffsetHz: 600000}, nil, "TxConfig.OffsetHz", false},
		{"offset NaN", nil, &TxConfig{OffsetHz: math.NaN()}, nil, "TxConfig.OffsetHz", false},
		{"power -1", nil, &TxConfig{Power: -1}, nil, "TxConfig.Power", false},
		{"power 64", nil, &TxConfig{Power: 64}, nil, "TxConfig.Power", false},
		{"region 9", nil, &TxConfig{Region: 9}, nil, "TxConfig.Region", true},
		{"service 9", nil, &TxConfig{Service: 9}, nil, "TxConfig.Service", true},
		{"license-free US", nil, &TxConfig{Region: RegionUS, Service: ServiceLicenseFree}, nil, "TxConfig.Service", true},
		{"license-free Japan", nil, &TxConfig{Region: RegionJapan, Service: ServiceLicenseFree}, nil, "TxConfig.Service", true},
		{"license-free CEPT", openGate, &TxConfig{Region: RegionCEPT, Service: ServiceLicenseFree}, nil, "TxConfig.Service", true},
		{"license-free China", openGate, &TxConfig{Region: RegionChina, Service: ServiceLicenseFree}, nil, "TxConfig.Service", true},
		{"license-free World", openGate, &TxConfig{Service: ServiceLicenseFree}, nil, "TxConfig.Service", true},
		{"license-free 28 dBm", openGate, &TxConfig{Region: RegionCEPT, Service: ServiceLicenseFree, Power: 28}, nil, "TxConfig.Power", true},
		{"China land mobile 45 dBm", openGate, &TxConfig{Region: RegionChina, Service: ServiceLandMobile, Power: 45}, nil, "TxConfig.Power", true},
		{"US land mobile 61 dBm", openGate, &TxConfig{Region: RegionUS, Service: ServiceLandMobile, Power: 61}, nil, "TxConfig.Power", true},
		{"US amateur highest power", openGate, &TxConfig{Region: RegionUS, Service: ServiceAmateur}, nil, "TxConfig.Power", true},
		{"CEPT amateur 61 dBm", openGate, &TxConfig{Region: RegionCEPT, Service: ServiceAmateur, Power: 61}, nil, "TxConfig.Power", true},
		{"unknown service highest power", openGate, &TxConfig{Region: RegionUS}, nil, "TxConfig.Power", true},
		{"China amateur", openGate, &TxConfig{Region: RegionChina, Service: ServiceAmateur, Power: 40}, nil, "TxConfig.Service", true},
		{"Japan amateur", openGate, &TxConfig{Region: RegionJapan, Service: ServiceAmateur, Power: 40}, nil, "TxConfig.Service", true},
		{"image under 62.5 kHz", openGate, us(TxConfig{Rate: 50000}), nil, "TxConfig.Rate", true},
		{"Tier0 at 48 kHz", openGate, us(TxConfig{Tier: Tier0, Rate: 48000}), nil, "TxConfig.Rate", true},
		{"CS8 at 10 W", openGate, us(TxConfig{Format: IQFormatCS8}), nil, "TxConfig.Rate", true},
		{"CS8 at 1 W under 148651 Hz", openGate, &TxConfig{Region: RegionUS, Service: ServiceLandMobile, Power: 30, Format: IQFormatCS8, Rate: 100000}, nil, "TxConfig.Rate", true},
		{"audio rate 7999", nil, nil, &AudioTxConfig{Rate: 7999}, "AudioTxConfig.Rate", false},
		{"audio Tier0 at 44100", nil, nil, &AudioTxConfig{Tier: Tier0, Rate: 44100}, "AudioTxConfig.Rate", false},
		{"audio rate 22051", nil, nil, &AudioTxConfig{Rate: 22051}, "AudioTxConfig.Rate", false},
		{"audio bits 7", nil, nil, &AudioTxConfig{Bits: 7}, "AudioTxConfig.Bits", false},
		{"audio bits 17", nil, nil, &AudioTxConfig{Bits: 17}, "AudioTxConfig.Bits", false},
		{"audio offset 2047 at 12 bits", nil, nil, &AudioTxConfig{Bits: 12, Offset: 2047}, "AudioTxConfig.Offset", false},
		{"audio level 16001", nil, nil, &AudioTxConfig{Level: 16001}, "AudioTxConfig.Level", false},
		{"audio level -1", nil, nil, &AudioTxConfig{Level: -1}, "AudioTxConfig.Level", false},
		{"audio level 1100 at 12 bits", nil, nil, &AudioTxConfig{Bits: 12, Level: 1100}, "AudioTxConfig.Level", false},
		{"audio input low pass -1", nil, nil, &AudioTxConfig{InputLowpassHz: -1}, "AudioTxConfig.InputLowpassHz", false},
		{"audio pre-emphasis", openGate, nil, ua(AudioTxConfig{PreEmphasis: true}), "AudioTxConfig.PreEmphasis", true},
		{"audio level 1000 at 12 bits", openGate, nil, ua(AudioTxConfig{Tier: Tier0, Bits: 12, Level: 1000}), "AudioTxConfig.Level", true},
		{"audio level 60 at 8 bits", openGate, nil, ua(AudioTxConfig{Tier: Tier0, Bits: 8, Level: 60}), "AudioTxConfig.Level", true},
		{"audio at 100 W", openGate, nil, &AudioTxConfig{Region: RegionUS, Service: ServiceLandMobile, Power: 50}, "AudioTxConfig.Rate", true},
		{"audio at 192 kHz", openGate, nil, ua(AudioTxConfig{Rate: 192000}), "AudioTxConfig.Rate", true},
		{"audio at 20 kHz, 10 W", openGate, nil, ua(AudioTxConfig{Rate: 20000}), "AudioTxConfig.Rate", true},
		{"audio Tier0 at 10 W", openGate, nil, ua(AudioTxConfig{Tier: Tier0, Rate: 24000}), "AudioTxConfig.Rate", true},
		{"audio Tier1 12-bit at 48 kHz", openGate, nil, ua(AudioTxConfig{Bits: 12, Unsigned: true}), "AudioTxConfig.Level", true},
	}
}

func TestTxRefusals(t *testing.T) {
	t.Parallel()
	for _, c := range refusals() {
		var err, ierr error
		var tier0, changed bool
		if c.iq != nil {
			_, err = newTransmitter(*c.iq, c.gate)
			ic := *c.iq
			if ic.Tier == TierDefault && ic.Rate == 0 {
				ic.Rate = tier0IQRate
			}
			var tx Transmitter
			tier0 = ic.Tier == TierDefault || ic.Tier == Tier0
			ierr = tx.init(ic, c.gate)
			changed = tx.set
		} else {
			_, err = newAudioTransmitter(*c.audio, c.gate)
			var tx AudioTransmitter
			tier0 = c.audio.Tier == TierDefault || c.audio.Tier == Tier0
			ierr = tx.init(*c.audio, c.gate)
			changed = tx.set
		}
		if f, region := errField(err); f != c.field || region != c.region {
			t.Errorf("%s: New: %v, want %s (region %v)", c.name, err, c.field, c.region)
		}
		if ierr != nil && changed {
			t.Errorf("%s: Init changed the transmitter on %v", c.name, ierr)
		}
		if !tier0 && ierr == nil {
			t.Errorf("%s: Init took a tier other than Tier0", c.name)
		}
		switch c.field[len(c.field)-4:] {
		case "Rate", "Tier", "evel":
		default:
			if f, _ := errField(ierr); tier0 && f != c.field {
				t.Errorf("%s: Init: %v, want %s", c.name, ierr, c.field)
			}
		}
	}
}

func TestTxRefusalTable(t *testing.T) {
	t.Parallel()
	if !testing.Verbose() {
		t.Skip("prints the refusal table with -v")
	}
	for _, c := range refusals() {
		var err error
		if c.iq != nil {
			_, err = newTransmitter(*c.iq, c.gate)
		} else {
			_, err = newAudioTransmitter(*c.audio, c.gate)
		}
		g := "strict"
		if c.gate != nil {
			g = "no 10 dB gate"
		}
		t.Logf("| %s | %s | %v |", c.name, g, err)
	}
}

func TestTxAccepts(t *testing.T) {
	t.Parallel()
	for _, c := range []TxConfig{
		{Region: RegionUS, Service: ServiceLandMobile, Power: 50},
		{Region: RegionUS, Service: ServiceLandMobile, Power: 60, Tier: Tier2, Rate: 62500},
		{Region: RegionUS, Service: ServiceLandMobile, Power: 30, Format: IQFormatCS8},
		{Region: RegionUS, Service: ServiceLandMobile, Power: 30, Format: IQFormatCU8, Rate: 250000},
		{Region: RegionUS, Service: ServiceAmateur, Power: 60, Format: IQFormatCF32, Rate: 96000},
		{Region: RegionCEPT, Service: ServiceAmateur, Power: 50},
		{Region: RegionJapan, Service: ServiceLandMobile, Power: 60},
		{Region: RegionUS, Service: ServiceLandMobile, Tier: Tier0, Rate: 96000, OffsetHz: 12000.4},
		{Service: ServiceLandMobile, Power: 30},
		{Region: RegionCEPT, Service: ServiceLandMobile, Power: 30},
	} {
		tx, err := newTransmitter(c, openGate)
		if err != nil {
			t.Errorf("%+v: %v", c, err)
			continue
		}
		if c.Tier == Tier0 {
			var in Transmitter
			if err := in.init(c, openGate); err != nil || in.OffsetHz() != 12000 {
				t.Errorf("Init %+v: %v, offset %v", c, err, in.OffsetHz())
			}
		}
		if _, err := NewTransmitter(&c); err == nil {
			t.Errorf("%+v: accepted without the spurious measurement", c)
		}
		if tx.SampleBytes() != c.Format.bytes() || tx.DeviationHz() != 1944 {
			t.Errorf("%+v: %d bytes, %v Hz", c, tx.SampleBytes(), tx.DeviationHz())
		}
	}
	for _, c := range []AudioTxConfig{
		{Region: RegionUS, Service: ServiceLandMobile, Power: 40},
		{Service: ServiceLandMobile, Power: 30},
		{Region: RegionUS, Service: ServiceLandMobile, Power: 30, Bits: 13, Rate: 22500},
		{Region: RegionUS, Service: ServiceAmateur, Power: 50, Rate: 96000, Tier: Tier2},
		{Region: RegionUS, Service: ServiceLandMobile, Power: 40, Tier: Tier0},
		{Region: RegionUS, Service: ServiceLandMobile, Power: 30, Tier: Tier0, Rate: 24000, Bits: 12, Unsigned: true, Offset: 3},
	} {
		tx, err := newAudioTransmitter(c, openGate)
		if err != nil {
			t.Errorf("%+v: %v", c, err)
			continue
		}
		bits := max(c.Bits, 8)
		if c.Bits == 0 {
			bits = 16
		}
		if tx.Level() < measuredLevel(bits) {
			t.Errorf("%+v: level %d", c, tx.Level())
		}
		want := c.Offset
		if c.Unsigned {
			want += 1 << (bits - 1)
		}
		if tx.Offset() != want {
			t.Errorf("%+v: offset %d, want %d", c, tx.Offset(), want)
		}
		if c.Tier == Tier0 {
			var in AudioTransmitter
			if err := in.init(c, openGate); err != nil {
				t.Errorf("Init %+v: %v", c, err)
			}
		}
	}
}

func TestTxNil(t *testing.T) {
	t.Parallel()
	var p *Transmitter
	var a *AudioTransmitter
	if _, err := p.Write(nil); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Error(err)
	}
	if !errors.Is(p.Flush(), dmrgo.ErrNilReceiver) || !errors.Is(p.Init(TxConfig{}), dmrgo.ErrNilReceiver) {
		t.Error("nil Transmitter")
	}
	if _, err := a.Write(nil); !errors.Is(err, dmrgo.ErrNilReceiver) {
		t.Error(err)
	}
	if !errors.Is(a.Flush(), dmrgo.ErrNilReceiver) || !errors.Is(a.Init(AudioTxConfig{}), dmrgo.ErrNilReceiver) {
		t.Error("nil AudioTransmitter")
	}
	if p.AppendSamples(nil, make([]Burst, 1)) != nil || p.AppendFlush(nil) != nil || p.Active() || p.Lag() != 0 || p.MaxOutput(1) != 0 ||
		p.SampleBytes() != 0 || p.OffsetHz() != 0 || p.DeviationHz() != 0 {
		t.Error("nil Transmitter methods")
	}
	if a.AppendSamples(nil, make([]Burst, 1)) != nil || a.AppendFlush(nil) != nil || a.Active() || a.Lag() != 0 || a.MaxOutput(1) != 0 ||
		a.Level() != 0 || a.Offset() != 0 {
		t.Error("nil AudioTransmitter methods")
	}
	var z Transmitter
	var za AudioTransmitter
	if _, err := z.Write(nil); !errors.Is(err, ErrNotSetUp) {
		t.Error(err)
	}
	if !errors.Is(z.Flush(), ErrNotSetUp) || !errors.Is(za.Flush(), ErrNotSetUp) {
		t.Error("zero Flush")
	}
	if _, err := za.Write(nil); !errors.Is(err, ErrNotSetUp) {
		t.Error(err)
	}
	if z.AppendSamples(nil, make([]Burst, 1)) != nil || z.MaxOutput(1) != 0 || za.AppendFlush(nil) != nil || za.Lag() != 0 {
		t.Error("zero methods")
	}
}

func usIQ(tier Tier, rate int, f IQFormat) TxConfig {
	return TxConfig{Tier: tier, Rate: rate, Format: f, Region: RegionUS, Service: ServiceLandMobile, Power: 30}
}

func usAudio(tier Tier, rate, bits int) AudioTxConfig {
	return AudioTxConfig{Tier: tier, Rate: rate, Bits: bits, Region: RegionUS, Service: ServiceLandMobile, Power: 30}
}

func TestTxModes(t *testing.T) {
	t.Parallel()
	bs := msBursts(t, "voice")[:4]
	tx, err := newTransmitter(usIQ(Tier1, 96000, IQFormatCS16), openGate)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Write(bs); err == nil || tx.Flush() == nil {
		t.Error("Write in append mode")
	}
	out := tx.AppendFlush(tx.AppendSamples(nil, bs))
	c := usIQ(Tier1, 96000, IQFormatCS16)
	var got []byte
	most := 0
	c.OnSamples = func(b []byte) { got = append(got, b...); most = max(most, len(b)) }
	st, err := newTransmitter(c, openGate)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := st.Write(bs); n != len(bs) || err != nil {
		t.Fatal(n, err)
	}
	if !st.Active() {
		t.Error("not active")
	}
	if st.Flush() != nil || st.Active() {
		t.Error("Flush")
	}
	if string(got) != string(out) || len(out) == 0 {
		t.Errorf("stream %d bytes, append %d", len(got), len(out))
	}
	if most > st.MaxOutput(0)*st.SampleBytes() || len(out) > tx.MaxOutput(len(bs))*tx.SampleBytes()+tx.MaxOutput(0)*tx.SampleBytes() {
		t.Errorf("chunk %d over %d, out %d over %d", most, st.MaxOutput(0)*4, len(out), tx.MaxOutput(len(bs))*4)
	}
	if st.AppendSamples(nil, bs) != nil || st.AppendFlush(nil) != nil {
		t.Error("Append in stream mode")
	}
	ac := usAudio(Tier0, 48000, 16)
	var ag []int16
	amost := 0
	ac.OnSamples = func(x []int16) { ag = append(ag, x...); amost = max(amost, len(x)) }
	var at AudioTransmitter
	if err := at.init(ac, openGate); err != nil {
		t.Fatal(err)
	}
	if n, err := at.Write(bs); n != len(bs) || err != nil || at.Flush() != nil {
		t.Fatal(n, err)
	}
	ac.OnSamples = nil
	var aa AudioTransmitter
	if err := aa.init(ac, openGate); err != nil {
		t.Fatal(err)
	}
	ao := aa.AppendFlush(aa.AppendSamples(nil, bs))
	if len(ao) != len(ag) || amost > at.MaxOutput(0) || len(ao) == 0 {
		t.Errorf("audio stream %d, append %d, chunk %d of %d", len(ag), len(ao), amost, at.MaxOutput(0))
	}
	for i := range ao {
		if ao[i] != ag[i] {
			t.Fatalf("audio sample %d differs", i)
		}
	}
	if _, err := aa.Write(bs); err == nil {
		t.Error("audio Write in append mode")
	}
	if at.Lag() != 8*10 && at.Lag() <= 0 {
		t.Errorf("lag %d", at.Lag())
	}
}

func TestTxBurstOrder(t *testing.T) {
	t.Parallel()
	bs := msBursts(t, "voice")[:3]
	bs[2].Slot = bs[1].Slot
	tx, err := newTransmitter(usIQ(Tier0, 96000, IQFormatCS16), openGate)
	if err != nil {
		t.Fatal(err)
	}
	tx.on = func([]byte) {}
	tx.chunk = tx.small[:int(tx.sps)*4]
	if n, err := tx.Write(bs); n != 2 || !errors.Is(err, ErrBurstOrder) {
		t.Errorf("%d, %v", n, err)
	}
	bs[2].Stream = 2
	if n, err := tx.Write(bs[2:]); n != 1 || err != nil {
		t.Errorf("new stream: %d, %v", n, err)
	}
}

func TestTxLicenseFree(t *testing.T) {
	t.Parallel()
	c := usIQ(Tier0, 96000, IQFormatCS16)
	var tx Transmitter
	if err := tx.init(c, openGate); err != nil {
		t.Fatal(err)
	}
	tx.core.lf = true
	tx.on = func([]byte) {}
	tx.chunk = tx.small[:int(tx.sps)*4]
	b := []Burst{{BS: true, Stream: 1}}
	if n, err := tx.Write(b); n != 0 || !errors.Is(err, ErrBurstService) {
		t.Errorf("BS burst: %d, %v", n, err)
	}
	one := make([]Burst, 1)
	one[0].Stream = 1
	for i := range txLFSlots {
		one[0].Slot = int64(i)
		if _, err := tx.Write(one); err != nil {
			t.Fatalf("slot %d: %v", i, err)
		}
	}
	one[0].Slot = txLFSlots
	if _, err := tx.Write(one); !errors.Is(err, ErrTxTimeout) || tx.Active() {
		t.Fatalf("timeout: %v, active %v", err, tx.Active())
	}
	one[0].Slot++
	if _, err := tx.Write(one); !errors.Is(err, ErrTxTimeout) {
		t.Fatalf("rest of the stream: %v", err)
	}
	one[0].Stream = 2
	if _, err := tx.Write(one); err != nil || !tx.Active() {
		t.Fatalf("new stream: %v", err)
	}
}

func envelope(b []byte, sps int) []float64 {
	n := len(b) / 4 / sps
	out := make([]float64, n)
	for k := range n {
		var e float64
		for j := range sps {
			o := 4 * (k*sps + j)
			i, q := float64(int16(binary.LittleEndian.Uint16(b[o:]))), float64(int16(binary.LittleEndian.Uint16(b[o+2:])))
			e += i*i + q*q
		}
		out[k] = math.Sqrt(e/float64(sps)) / 32768
	}
	return out
}

func TestTxInputLowpass(t *testing.T) {
	t.Parallel()
	for _, tier := range []Tier{Tier1, Tier2} {
		for _, bits := range []int{16} {
			s := fsktx.Setup{Tier: fsktx.Tier(tier), Bits: bits, Region: fsktx.LimitsUS, Service: fsktx.LimitsLandMobile}
			r := txRegs{region: RegionUS, service: ServiceLandMobile, power: 30}
			class, err := r.check(&audioErrs, openGate)
			if err != nil {
				t.Fatal(err)
			}
			s.Power = class
			none := s.MinRate()
			lowered := false
			for _, hz := range []int{8000, 6000, 5000, 3000, 1000} {
				s.InputLowpassHz = hz
				least := s.MinRate()
				if least == 0 || none != 0 && least > none {
					t.Errorf("tier %v, %d bits, %d Hz: least %d, none %d", tier, bits, hz, least, none)
					continue
				}
				if least == none {
					continue
				}
				lowered = true
				c := usAudio(tier, least, bits)
				c.InputLowpassHz = hz
				if _, err := newAudioTransmitter(c, openGate); err != nil {
					t.Errorf("tier %v, %d bits, %d Hz at %d: %v", tier, bits, hz, least, err)
				}
				c.InputLowpassHz = 0
				if f, region := errField(func() error { _, err := newAudioTransmitter(c, openGate); return err }()); f != "AudioTxConfig.Rate" || !region {
					t.Errorf("tier %v, %d bits, no low pass at %d: %s", tier, bits, least, f)
				}
			}
			if !lowered {
				t.Errorf("tier %v, %d bits: no low pass lowers the least rate %d", tier, bits, none)
			}
		}
	}
}
