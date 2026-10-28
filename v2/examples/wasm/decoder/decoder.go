// Command decoder decodes DMR in a web browser: the receivers of package rf,
// compiled to WebAssembly, take FM discriminator audio or I/Q from a file,
// a microphone or line-in, or an RTL-SDR over WebUSB, and the page shows the
// bursts, calls, data and text messages that they carry. Built for GOOS=js,
// it exports the decoder to JavaScript; built natively, it decodes a file
// and prints the same events, one JSON object per line.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

type encoding int

const (
	encU8 encoding = iota
	encS8
	encS16
	encF32
)

func (e encoding) size() int {
	switch e {
	case encU8, encS8:
		return 1
	case encS16:
		return 2
	case encF32:
		return 4
	}
	return 1
}

const (
	audioHz      = 24000
	basebandRate = 48000
	pcmChunk     = 1 << 12
	basebandMax  = 1 << 14
	wavHeaderMax = 1 << 16
)

type options struct {
	format    string
	rate      float64
	audioTier int
	iqTier    int
}

type decoder struct {
	opt      options
	wav      bool
	hdr      []byte
	ready    bool
	enc      encoding
	channels int
	rate     float64

	disc *rf.AudioReceiver
	iq   *rf.Receiver
	raw  bool
	rs   *resampler
	out  []rf.Burst
	cf32 []byte

	pend  [8]byte
	npend int
	pcm   []int16
	bb    []complex64
	wide  []complex64

	rep      *reporter
	samples  int64
	onBurst  func(*rf.Burst)
	maxWrite int
}

func newDecoder(opt options) (*decoder, error) {
	d := &decoder{opt: opt, rep: newReporter()}
	switch opt.format {
	case "wav":
		d.wav = true
		d.hdr = make([]byte, 0, 4096)
		return d, nil
	case "f32":
		d.enc, d.channels = encF32, 1
	case "s16":
		d.enc, d.channels = encS16, 1
	case "cu8":
		d.enc, d.channels = encU8, 2
	case "cs8":
		d.enc, d.channels = encS8, 2
	case "cs16":
		d.enc, d.channels = encS16, 2
	case "cf32":
		d.enc, d.channels = encF32, 2
	default:
		return nil, fmt.Errorf("format %q, want wav, f32, s16, cu8, cs8, cs16 or cf32", opt.format)
	}
	d.rate = opt.rate
	return d, d.setup()
}

func (d *decoder) audio() bool { return d.channels == 1 }

func (d *decoder) frame() int { return d.channels * d.enc.size() }

func (d *decoder) setup() error {
	if d.rate <= 0 {
		return errors.New("the sample rate is not set")
	}
	d.ready = true
	if d.audio() {
		tier := rf.TierDefault
		if d.opt.audioTier >= 0 {
			tier = rf.Tier0 + rf.Tier(d.opt.audioTier)
		}
		hz := audioHz
		switch d.rate {
		case 24000, 12000, 9600:
			hz = int(d.rate)
		}
		rx, err := rf.NewAudioReceiver(&rf.AudioRxConfig{Rate: hz, Tier: tier})
		var ce *dmrgo.ConfigError
		if errors.As(err, &ce) && ce.Field == "AudioRxConfig.Tier" {
			return fmt.Errorf("audio tier %d is not in this build; a TinyGo build has audio tier 0 only: %w", d.opt.audioTier, err)
		}
		if err != nil {
			return err
		}
		d.disc = rx
		if float64(hz) != d.rate {
			d.rs = newResampler(d.rate, float64(hz), false)
		}
		d.pcm = make([]int16, 0, pcmChunk)
		d.wide = make([]complex64, 0, 64)
		return nil
	}
	tier := rf.Tier2
	if d.opt.iqTier >= 0 {
		tier = rf.Tier0 + rf.Tier(d.opt.iqTier)
	}
	d.raw = (d.enc == encU8 || d.enc == encS8) && d.rate >= basebandRate
	cfg := rf.RxConfig{Tier: tier, Rate: basebandRate, Format: rf.IQFormatCF32}
	d.maxWrite = 2 * basebandMax
	if d.raw {
		cfg.Rate, cfg.Format, d.maxWrite = int(d.rate), rf.IQFormatCU8, 1<<16
		if d.enc == encS8 {
			cfg.Format = rf.IQFormatCS8
		}
	}
	rx, err := rf.NewReceiver(&cfg)
	if err != nil {
		return err
	}
	d.iq = rx
	if !d.raw {
		d.rs = newResampler(d.rate, basebandRate, true)
		d.bb = make([]complex64, 0, basebandMax)
		d.cf32 = make([]byte, 0, 8*basebandMax)
		d.wide = make([]complex64, 0, 64)
	}
	return nil
}

func (d *decoder) write(p []byte) error {
	if !d.ready {
		var err error
		if p, err = d.header(p); err != nil || !d.ready {
			return err
		}
	}
	if d.raw {
		d.writeRaw(p)
		return nil
	}
	fr := d.frame()
	if d.npend > 0 {
		k := copy(d.pend[d.npend:fr], p)
		d.npend += k
		p = p[k:]
		if d.npend < fr {
			return nil
		}
		d.sample(d.pend[:fr])
		d.npend = 0
	}
	for len(p) >= fr {
		d.sample(p[:fr])
		p = p[fr:]
	}
	d.npend = copy(d.pend[:], p)
	return nil
}

func (d *decoder) writeRaw(p []byte) {
	if d.npend > 0 && len(p) > 0 {
		d.pend[1] = p[0]
		d.emit(d.iq.AppendBursts(d.out[:0], d.pend[:2]))
		d.samples++
		d.npend = 0
		p = p[1:]
	}
	for len(p) >= 2 {
		n := min(len(p), d.maxWrite) &^ 1
		d.emit(d.iq.AppendBursts(d.out[:0], p[:n]))
		d.samples += int64(n / 2)
		p = p[n:]
	}
	if len(p) == 1 {
		d.pend[0], d.npend = p[0], 1
	}
}

func normalize(e encoding, b []byte) float32 {
	switch e {
	case encU8:
		return (float32(b[0]) - 127.5) / 127.5
	case encS8:
		v := int(b[0])
		return float32(v-v>>7<<8) / 127
	case encS16:
		v := int(binary.LittleEndian.Uint16(b))
		return float32(v-v>>15<<16) / 32767
	case encF32:
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(b))
}

func (d *decoder) sample(b []byte) {
	d.samples++
	sz := d.enc.size()
	if d.audio() {
		v := normalize(d.enc, b)
		if d.rs == nil {
			d.pushPCM(v)
			return
		}
		d.wide = d.rs.push(complex(v, 0), d.wide[:0])
		for _, y := range d.wide {
			d.pushPCM(real(y))
		}
		return
	}
	d.wide = d.rs.push(complex(normalize(d.enc, b), normalize(d.enc, b[sz:])), d.wide[:0])
	for _, y := range d.wide {
		d.bb = append(d.bb, y)
		if len(d.bb) == cap(d.bb) {
			d.emit(d.writeBaseband())
			d.bb = d.bb[:0]
		}
	}
}

func (d *decoder) pushPCM(v float32) {
	d.pcm = append(d.pcm, int16(math.Round(float64(max(-32767, min(32767, v*32767))))))
	if len(d.pcm) < cap(d.pcm) {
		return
	}
	d.emit(d.disc.AppendBursts(d.out[:0], d.pcm))
	d.pcm = d.pcm[:0]
}

func (d *decoder) writeBaseband() []rf.Burst {
	b := d.cf32[:0]
	for _, v := range d.bb {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(real(v)))
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(imag(v)))
	}
	d.cf32 = b
	return d.iq.AppendBursts(d.out[:0], b)
}

func (d *decoder) emit(bs []rf.Burst) {
	if cap(bs) > cap(d.out) {
		d.out = bs[:0]
	}
	for i := range bs {
		if d.onBurst != nil {
			d.onBurst(&bs[i])
		}
		d.rep.burst(&bs[i])
	}
}

func (d *decoder) flush() {
	if !d.ready {
		return
	}
	switch {
	case d.disc != nil:
		if len(d.pcm) > 0 {
			d.emit(d.disc.AppendBursts(d.out[:0], d.pcm))
			d.pcm = d.pcm[:0]
		}
		d.emit(d.disc.AppendFlush(d.out[:0]))
		d.rep.offset, d.rep.deviation = float64(d.disc.Offset()), float64(d.disc.Level())
	case d.iq != nil:
		if len(d.bb) > 0 {
			d.emit(d.writeBaseband())
			d.bb = d.bb[:0]
		}
		d.emit(d.iq.AppendFlush(d.out[:0]))
		d.rep.offset, d.rep.deviation = d.iq.OffsetHz(), d.iq.DeviationHz()
	}
}

func (d *decoder) seconds() float64 {
	if d.rate <= 0 {
		return 0
	}
	return float64(d.samples) / d.rate
}

func (d *decoder) header(p []byte) ([]byte, error) {
	start := len(d.hdr)
	d.hdr = append(d.hdr, p...)
	f, rate, off, ok, err := parseWAV(d.hdr)
	if err != nil {
		return nil, err
	}
	if !ok {
		if len(d.hdr) > wavHeaderMax {
			return nil, errors.New("WAV header: no data chunk")
		}
		return nil, nil
	}
	d.enc, d.channels, d.rate = f.enc, f.channels, rate
	if d.opt.rate > 0 {
		d.rate = d.opt.rate
	}
	if err := d.setup(); err != nil {
		return nil, err
	}
	rest := p[off-start:]
	d.hdr = nil
	return rest, nil
}

type wavFormat struct {
	enc      encoding
	channels int
}

func parseWAV(b []byte) (f wavFormat, rate float64, dataOff int, ok bool, err error) {
	if len(b) < 12 {
		return f, 0, 0, false, nil
	}
	if string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return f, 0, 0, false, errors.New("not a WAV file")
	}
	off := 12
	haveFmt := false
	for {
		if len(b) < off+8 {
			return f, 0, 0, false, nil
		}
		id := string(b[off : off+4])
		size := int(binary.LittleEndian.Uint32(b[off+4:]))
		body := off + 8
		if id == "data" {
			if !haveFmt {
				return f, 0, 0, false, errors.New("WAV data before its fmt chunk")
			}
			return f, rate, body, true, nil
		}
		if len(b) < body+size {
			return f, 0, 0, false, nil
		}
		if id == "fmt " {
			if size < 16 {
				return f, 0, 0, false, fmt.Errorf("WAV fmt chunk of %d bytes", size)
			}
			c := b[body:]
			tag := binary.LittleEndian.Uint16(c)
			if tag == 0xFFFE && size >= 26 {
				tag = binary.LittleEndian.Uint16(c[24:])
			}
			f.channels = int(binary.LittleEndian.Uint16(c[2:]))
			rate = float64(binary.LittleEndian.Uint32(c[4:]))
			bits := binary.LittleEndian.Uint16(c[14:])
			switch {
			case tag == 1 && bits == 8:
				f.enc = encU8
			case tag == 1 && bits == 16:
				f.enc = encS16
			case tag == 3 && bits == 32:
				f.enc = encF32
			default:
				return f, 0, 0, false, fmt.Errorf("WAV format %d with %d bits, want 8-bit or 16-bit PCM or 32-bit float", tag, bits)
			}
			if f.channels != 1 && f.channels != 2 {
				return f, 0, 0, false, fmt.Errorf("WAV with %d channels, want 1 (audio) or 2 (I/Q)", f.channels)
			}
			haveFmt = true
		}
		off = body + size + size&1
	}
}
