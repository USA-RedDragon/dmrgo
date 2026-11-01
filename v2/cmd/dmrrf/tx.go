package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

type station struct {
	region  *rf.Region
	service *rf.Service
	power   *int
}

func stationFlags(fs *flag.FlagSet) station {
	s := station{region: new(rf.Region), service: new(rf.Service)}
	services := [...]rf.Service{rf.ServiceUnknown, rf.ServiceAmateur, rf.ServiceLandMobile, rf.ServiceLicenseFree}
	var regions []string
	for r := rf.RegionWorld; r <= rf.RegionJapan; r++ {
		regions = append(regions, strings.ToLower(r.String()))
	}
	fs.Func("region", "regulatory region whose limits the transmission keeps: "+strings.Join(regions, ", ")+" (default world, the strictest of all)", func(v string) error {
		for r := rf.RegionWorld; r <= rf.RegionJapan; r++ {
			if strings.EqualFold(v, r.String()) {
				*s.region = r
				return nil
			}
		}
		return fmt.Errorf("unknown region %q", v)
	})
	names := make([]string, 0, len(services))
	for _, sv := range services {
		names = append(names, strings.ToLower(sv.String()))
	}
	fs.Func("service", "radio service of the station: "+strings.Join(names, ", ")+" (default unknown, the limits of land mobile and amateur at once)", func(v string) error {
		for _, sv := range services {
			if strings.EqualFold(v, sv.String()) {
				*s.service = sv
				return nil
			}
		}
		return fmt.Errorf("unknown service %q", v)
	})
	s.power = fs.Int("power", 0, "most power of the station in dBm, 1 to 63; 0 is the highest legal power of the service in the region")
	return s
}

func (s station) hint(err error) error {
	var re *rf.RegionError
	if errors.As(err, &re) {
		return fmt.Errorf("%w; the limits depend on -region, -service and -power, now %s, %s, %d dBm", err, strings.ToLower(s.region.String()), strings.ToLower(s.service.String()), *s.power)
	}
	return err
}

type txModulation struct {
	seq                 string
	cfg                 rftx.SeqConfig
	repeat, corr, level float64
	rate, clock         float64
	scale               bool
	station             station
}

const frameSeconds = 2 * rftx.SlotSeconds

func (m txModulation) run(w io.Writer, sink *txSamples) error {
	built, err := rftx.Sequence(m.seq, m.cfg)
	if err != nil {
		return err
	}
	return m.write(w, built, sink)
}

func (m txModulation) write(w io.Writer, built rftx.Built, sink *txSamples) error {
	if !sink.f.iq() && len(built.Carriers) > 0 {
		return fmt.Errorf("the sequence %s has %d carriers; audio carries one, so it needs an I/Q format", m.seq, len(built.Carriers)+1)
	}
	if m.repeat > 0 && len(built.Carriers) > 0 {
		return fmt.Errorf("-repeat does not apply to the multi-channel sequence %s", m.seq)
	}
	if !(m.rate > 0 && m.rate <= 1e9 && m.clock > 0 && m.clock <= 1e9) {
		return fmt.Errorf("-rate %g and -stretch give a sample rate out of range", m.rate)
	}
	for _, l := range built.Info {
		fmt.Fprintln(w, l)
	}
	cfg := m.cfg
	var bursts []rf.Burst
	lead := 0.0
	if cfg.BSSourced {
		slots := built.Slots
		if m.repeat > 0 {
			n := max(1, int(m.repeat/(float64(len(slots))*rftx.SlotSeconds)))
			for range n - 1 {
				slots = append(slots, built.Slots...)
			}
			fmt.Fprintf(w, "repeated %d times in one continuous stream\n", n)
		}
		bursts = bsBursts(slots)
	} else {
		bursts = make([]rf.Burst, len(built.Bursts))
		for i, b := range built.Bursts {
			slot := int64(i)
			if cfg.Slot >= 0 {
				slot = 2 * int64(i)
			}
			bursts[i] = rf.Burst{Data: b, Stream: 1, Slot: slot}
		}
		if cfg.Slot == 1 {
			lead = rftx.SlotSeconds
		}
	}
	var err error
	if sink.f.iq() {
		err = m.iq(built, bursts, lead, sink)
	} else {
		err = m.audio(w, bursts, lead, sink)
	}
	if err != nil {
		return err
	}
	if m.repeat > 0 && !cfg.BSSourced {
		frame := frameSeconds * m.clock
		end := int(math.Round(math.Ceil(float64(sink.samples)/frame) * frame))
		sink.zeros(end - sink.samples)
	}
	if cfg.BSSourced {
		fmt.Fprintf(w, "sequence %s (BS-sourced): %d bursts on slot %d in %d slots, %.2f s, cc %d, %d -> %d, offset %+.0f Hz\n",
			m.seq, len(built.Bursts), cfg.Slot, len(bursts), float64(sink.samples)/m.clock, cfg.ColorCode, cfg.Source, cfg.Dest, m.corr)
	} else {
		fmt.Fprintf(w, "sequence %s: %d bursts, %.2f s, slot %d, cc %d, %d -> %d, offset %+.0f Hz\n",
			m.seq, len(built.Bursts), float64(sink.samples)/m.clock, cfg.Slot, cfg.ColorCode, cfg.Source, cfg.Dest, m.corr)
	}
	for _, c := range built.Carriers {
		fmt.Fprintf(w, "carrier at %+.0f Hz: %d slots from %.2f s\n", c.OffsetHz, len(c.Slots), float64(c.LeadFrames)*frameSeconds)
	}
	return nil
}

func bsBursts(slots []rftx.BSSlot) []rf.Burst {
	out := make([]rf.Burst, len(slots))
	for i, s := range slots {
		out[i] = rf.Burst{Data: s.Burst, CACH: s.CACH, HasCACH: true, BS: true, Stream: 1, Slot: int64(i)}
	}
	return out
}

func iqFormat(name string) (rf.IQFormat, bool) {
	switch name {
	case nameCU8:
		return rf.IQFormatCU8, true
	case nameCS8:
		return rf.IQFormatCS8, true
	case nameCS16:
		return rf.IQFormatCS16, true
	case nameCF32:
		return rf.IQFormatCF32, true
	}
	return rf.IQFormatDefault, false
}

func (m txModulation) native() (int, bool) {
	r := int(math.Round(m.rate))
	return r, float64(r) == m.rate && m.clock == m.rate
}

func (m txModulation) iq(built rftx.Built, bursts []rf.Burst, lead float64, sink *txSamples) error {
	rate, exact := m.native()
	format, ok := iqFormat(sink.f.name)
	if exact && ok && len(built.Carriers) == 0 && !m.scale {
		b, size, err := m.iqCarrier(bursts, m.corr, format)
		if err != nil {
			return err
		}
		sink.zeros(int(math.Round(lead * m.rate)))
		sink.enc = append(sink.enc, b...)
		sink.samples += len(b) / size
		return nil
	}
	type part struct {
		x     []complex128
		start int
	}
	parts := make([]part, 0, 1+len(built.Carriers))
	add := func(bursts []rf.Burst, offset, lead float64) error {
		b, _, err := m.iqCarrier(bursts, offset, rf.IQFormatCF32)
		if err != nil {
			return err
		}
		x := make([]complex128, len(b)/8)
		peak := 0.0
		for i := range x {
			re := math.Float32frombits(binary.LittleEndian.Uint32(b[8*i:]))
			im := math.Float32frombits(binary.LittleEndian.Uint32(b[8*i+4:]))
			x[i] = complex(float64(re), float64(im))
			peak = math.Max(peak, math.Hypot(float64(re), float64(im)))
		}
		g := m.level / float64(1+len(built.Carriers)) / math.Max(peak, 1e-9)
		for i := range x {
			x[i] *= complex(g, 0)
		}
		parts = append(parts, part{x, int(math.Round(lead * float64(rate)))})
		return nil
	}
	if err := add(bursts, m.corr, lead); err != nil {
		return err
	}
	for _, c := range built.Carriers {
		if err := add(bsBursts(c.Slots), m.corr+c.OffsetHz, float64(c.LeadFrames)*frameSeconds); err != nil {
			return err
		}
	}
	n := 0
	for _, p := range parts {
		n = max(n, p.start+len(p.x))
	}
	sum := make([]complex128, n)
	for _, p := range parts {
		for i, v := range p.x {
			sum[p.start+i] += v
		}
	}
	stretchTo(len(sum), float64(rate)/m.clock, func(k int, f float64) {
		v := sum[k]
		if f > 0 && k+1 < len(sum) {
			v += (sum[k+1] - v) * complex(f, 0)
		}
		sink.emit(real(v), imag(v))
	})
	return nil
}

func (m txModulation) iqCarrier(bursts []rf.Burst, offset float64, format rf.IQFormat) ([]byte, int, error) {
	rate, _ := m.native()
	var out []byte
	tx, err := rf.NewTransmitter(&rf.TxConfig{Tier: rf.Tier1, Rate: rate, Format: format, OffsetHz: offset,
		Region: *m.station.region, Service: *m.station.service, Power: *m.station.power,
		OnSamples: func(b []byte) { out = append(out, b...) }})
	if err != nil {
		return nil, 0, m.station.hint(err)
	}
	if _, err := tx.Write(bursts); err != nil {
		return nil, 0, err
	}
	if err := tx.Flush(); err != nil {
		return nil, 0, err
	}
	return out, tx.SampleBytes(), nil
}

func (m txModulation) audio(w io.Writer, bursts []rf.Burst, lead float64, sink *txSamples) error {
	rate, exact := m.native()
	cfg := rf.AudioTxConfig{Tier: rf.Tier1, Rate: rate, Region: *m.station.region, Service: *m.station.service, Power: *m.station.power}
	if m.scale {
		most, err := rf.NewAudioTransmitter(&cfg)
		if err != nil {
			return m.station.hint(err)
		}
		cfg.Level = max(1, int(math.Round(m.level*float64(most.Level()))))
	}
	var out []int16
	cfg.OnSamples = func(x []int16) { out = append(out, x...) }
	tx, err := rf.NewAudioTransmitter(&cfg)
	if err != nil {
		return m.station.hint(err)
	}
	if _, err := tx.Write(bursts); err != nil {
		return err
	}
	if err := tx.Flush(); err != nil {
		return err
	}
	const full, outerHz = 32767.0, 1944.0
	peak := 0
	for _, v := range out {
		peak = max(peak, int(v), -int(v))
	}
	level := float64(tx.Level())
	start := int(math.Round(lead * float64(rate)))
	if exact {
		sink.zeros(start)
		for _, v := range out {
			sink.emit(float64(v) / full)
		}
	} else {
		x := make([]float64, start+len(out))
		for i, v := range out {
			x[start+i] = float64(v) / full
		}
		stretchTo(len(x), float64(rate)/m.clock, func(k int, f float64) {
			v := x[k]
			if f > 0 && k+1 < len(x) {
				v += (x[k+1] - v) * f
			}
			sink.emit(v)
		})
	}
	fmt.Fprintf(w, "audio: full scale is %.0f Hz of deviation, peak %.0f Hz\n", outerHz*full/level, outerHz*float64(peak)/level)
	return nil
}

func stretchTo(n int, step float64, at func(k int, f float64)) {
	if step == 1 {
		for k := range n {
			at(k, 0)
		}
		return
	}
	m := int(float64(n) / step)
	for i := range m {
		pos := float64(i) * step
		k := int(pos)
		at(k, pos-float64(k))
	}
}

func (t *txSamples) zeros(n int) {
	z := make([]float64, t.f.channels)
	for range n {
		t.emit(z...)
	}
}
