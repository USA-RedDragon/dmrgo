// Command dmrrf builds DMR transmissions as I/Q or FM modulator audio and
// decodes DMR from I/Q or FM discriminator audio. It reads and writes
// files and pipes in the sample formats of common software radios; it
// does not drive a radio.
package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type stdio struct {
	in       io.Reader
	out, msg io.Writer
	software func() (vocoder.Codec, error)
}

func main() {
	if err := runIO(os.Args[1:], stdio{in: os.Stdin, out: os.Stdout, msg: os.Stderr, software: newSoftware}); err != nil {
		fmt.Fprintln(os.Stderr, "dmrrf:", err)
		os.Exit(1)
	}
}

func run(args []string, w io.Writer) error {
	return runIO(args, stdio{in: os.Stdin, out: w, msg: w, software: newSoftware})
}

func runIO(args []string, std stdio) error {
	if len(args) == 0 {
		return errors.New("usage: dmrrf tx|rx|cal [flags]")
	}
	switch args[0] {
	case "tx":
		return runTX(args[1:], std)
	case "rx":
		return runRX(args[1:], std)
	case "cal":
		return runCal(args[1:], std)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func idFlag(fs *flag.FlagSet, name, usage string) *uint32 {
	id := new(uint32)
	fs.Func(name, usage+" (default 0)", func(s string) error {
		v, err := strconv.ParseUint(s, 10, 24)
		if err != nil {
			return err
		}
		*id = uint32(v)
		return nil
	})
	return id
}

type tuning struct {
	freq, ppm, offset *float64
}

func tuningFlags(fs *flag.FlagSet, freqUsage, offsetUsage string) tuning {
	return tuning{
		freq:   fs.Float64("freq", 0, freqUsage),
		ppm:    fs.Float64("ppm", 0, "frequency error of the radio in ppm, positive when it runs high, to correct (needs -freq)"),
		offset: fs.Float64("offset-hz", 0, offsetUsage),
	}
}

func (t tuning) errorHz() (float64, error) {
	if *t.ppm != 0 && *t.freq <= 0 {
		return 0, errors.New("-ppm needs -freq")
	}
	return *t.ppm * 1e-6 * *t.freq, nil
}

func (t tuning) set() bool { return *t.ppm != 0 || *t.offset != 0 }

func isSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

func runTX(args []string, std stdio) (err error) {
	fs := flag.NewFlagSet("tx", flag.ContinueOnError)
	out := fs.String("o", "", "output file, or - for standard output")
	format := fs.String("format", "", formatUsage)
	seq := fs.String("seq", "data12", strings.Join(rftx.Sequences(), ", "))
	cw := fs.Float64("cw", 0, "write an unmodulated carrier of this many seconds instead (I/Q only)")
	rate := fs.Float64("rate", 0, "sample rate in Hz (default 48000 for audio; required for I/Q)")
	tune := tuningFlags(fs, "carrier frequency in Hz, for -ppm and the example command", "frequency offset of the carrier from the tuned frequency in Hz (I/Q only)")
	level := fs.Float64("level", 0.9, "peak amplitude, 0 to 1")
	slot := fs.Int("slot", 0, "TDMA slot 0 or 1, or -1 for back-to-back bursts")
	cc := enums.ColorCode(1)
	fs.Func("cc", "color code, 0 to 15 (default 1)", func(s string) error {
		v, err := strconv.ParseUint(s, 10, 8)
		if err != nil {
			return err
		}
		cc, err = enums.NewColorCode(uint8(v))
		return err
	})
	src := idFlag(fs, "src", "source radio ID")
	dst := idFlag(fs, "dst", "destination radio or talkgroup ID")
	bs := idFlag(fs, "bs", "BS (repeater) address for the wakeup CSBK")
	group := fs.Bool("group", false, "send to a talkgroup instead of a radio")
	wakeups := fs.Int("wakeups", 3, "wakeup CSBKs before the sequence, or idle frames before a -bs-sourced stream")
	msg := fs.String("msg", "DMRGO TEST", "message text")
	repeat := fs.Float64("repeat", 0, "repeat the sequence, padded to whole 60 ms frames, for this many seconds")
	stretch := fs.Float64("stretch", 0, "slow the symbol clock by this many ppm so repeats sweep across the repeater's slot timing")
	superframes := fs.Int("superframes", 3, "voice superframes (bursts A to F) of the voice and pi sequences")
	silence := fs.Bool("ambe-silence", false, "send AMBE+2 silence instead of the AMBE test pattern")
	lateEntry := fs.Bool("late-entry", false, "leave out the voice LC and PI headers, so the call is known only from the embedded LC")
	preambles := fs.Int("preambles", 4, "preamble CSBKs of the preamble and sms sequences")
	dd := uint8(enums.DDUTF16LE)
	fs.Func("dd", "Defined Data format of the sms sequence, 0 to 63; the text is always UTF-16LE (default 21, UTF-16LE)", func(s string) error {
		v, err := strconv.ParseUint(s, 10, 6)
		dd = uint8(v)
		return err
	})
	bsSourced := fs.Bool("bs-sourced", false, "send a continuous BS outbound stream (CACH, Short LC, idle bursts on the other slot) like a repeater")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("tx needs -o <file>, or -o - for standard output")
	}
	f, err := formatFor(*format, *out)
	if err != nil {
		return err
	}
	w := std.out
	if *out == "-" {
		w = std.msg
	}
	if *rate == 0 && !f.iq() {
		*rate = 48000
	}
	if *rate <= 0 {
		return errors.New("-rate is required for I/Q: give the sample rate of the transmitter")
	}
	errHz, err := tune.errorHz()
	if err != nil {
		return err
	}
	if !f.iq() && (tune.set() || *cw > 0) {
		return errors.New("-cw, -ppm and -offset-hz apply to I/Q only")
	}
	corr := *tune.offset - errHz
	sink := &txSamples{f: f}
	if *cw > 0 {
		n := int(*cw * *rate)
		for i := range n {
			p := 2 * math.Pi * corr * float64(i) / *rate
			sink.emit(*level*math.Cos(p), *level*math.Sin(p))
		}
		fmt.Fprintf(w, "carrier: %.1f s, offset %+.0f Hz\n", *cw, corr)
	} else {
		cfg := rftx.SeqConfig{ColorCode: cc, Source: *src, Dest: *dst, BS: *bs, Group: *group, Wakeups: *wakeups, Message: *msg,
			Superframes: *superframes, AMBESilence: *silence, LateEntry: *lateEntry, Preambles: *preambles, DefinedData: dd, BSSourced: *bsSourced, Slot: *slot}
		m := txModulation{seq: *seq, cfg: cfg, repeat: *repeat, corr: corr, rate: *rate * (1 + *stretch*1e-6), level: *level}
		if err := m.run(w, sink); err != nil {
			return err
		}
	}
	reps := 1
	if *repeat > 0 && !*bsSourced && sink.samples > 0 {
		reps = max(1, int(*repeat**rate/float64(sink.samples)))
		fmt.Fprintf(w, "repeated %d times (%.1f s), stretch %.0f ppm\n", reps, float64(reps*sink.samples) / *rate, *stretch)
	}
	written, err := sink.write(*out, std.out, *rate, reps)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote %s: %s, %d samples at %.0f Hz (%d bytes)\n", *out, f.name, reps*sink.samples, *rate, written)
	freq := "<freq>"
	if *tune.freq > 0 {
		freq = fmt.Sprintf("%.0f", *tune.freq)
	}
	if h := txHint(f, *out, freq, *rate); h != "" {
		fmt.Fprintf(w, "for example: %s\n", h)
	}
	fmt.Fprintln(w, "transmit only as your license allows; start at the lowest gain, and use an attenuator or dummy load for loopback tests")
	return nil
}

type txSamples struct {
	f       sampleFormat
	enc     []byte
	samples int
}

func (t *txSamples) emit(vals ...float64) {
	for _, v := range vals {
		t.enc = appendSample(t.enc, t.f.enc, v)
	}
	t.samples++
}

func (t *txSamples) write(path string, stdout io.Writer, rate float64, reps int) (n int, err error) {
	var hdr []byte
	if t.f.wav {
		if hdr, err = wavHeader(rate, t.f.channels, reps*t.samples); err != nil {
			return 0, err
		}
	}
	sink := stdout
	if path != "-" {
		file, err := os.Create(path)
		if err != nil {
			return 0, err
		}
		defer func() { err = errors.Join(err, file.Close()) }()
		sink = file
	}
	bw := bufio.NewWriterSize(sink, 1<<16)
	if _, err := bw.Write(hdr); err != nil {
		return 0, err
	}
	for range reps {
		if _, err := bw.Write(t.enc); err != nil {
			return 0, err
		}
	}
	return len(hdr) + reps*len(t.enc), bw.Flush()
}

type txModulation struct {
	seq                       string
	cfg                       rftx.SeqConfig
	repeat, corr, rate, level float64
}

func (m txModulation) run(w io.Writer, sink *txSamples) error {
	built, err := rftx.Sequence(m.seq, m.cfg)
	if err != nil {
		return err
	}
	if !sink.f.iq() && len(built.Carriers) > 0 {
		return fmt.Errorf("the sequence %s has %d carriers; audio carries one, so it needs an I/Q format", m.seq, len(built.Carriers)+1)
	}
	if m.repeat > 0 && len(built.Carriers) > 0 {
		return fmt.Errorf("-repeat does not apply to the multi-channel sequence %s", m.seq)
	}
	for _, l := range built.Info {
		fmt.Fprintln(w, l)
	}
	cfg := m.cfg
	var fm, env []float64
	if cfg.BSSourced {
		slots := built.Slots
		if m.repeat > 0 {
			n := max(1, int(m.repeat/(float64(len(slots))*rftx.SlotSeconds)))
			for range n - 1 {
				slots = append(slots, built.Slots...)
			}
			fmt.Fprintf(w, "repeated %d times in one continuous stream\n", n)
		}
		fm, env = rftx.ModulateBS(slots)
		fmt.Fprintf(w, "sequence %s (BS-sourced): %d bursts on slot %d in %d slots, %.2f s, cc %d, %d -> %d, offset %+.0f Hz\n",
			m.seq, len(built.Bursts), cfg.Slot, len(slots), float64(len(fm))/basebandRate, cfg.ColorCode, cfg.Source, cfg.Dest, m.corr)
	} else {
		fm, env = rftx.Modulate(built.Bursts, rftx.Schedule{Slot: cfg.Slot})
		if m.repeat > 0 {
			fm, env = rftx.PadToFrames(fm, env)
		}
		fmt.Fprintf(w, "sequence %s: %d bursts, %.2f s, slot %d, cc %d, %d -> %d, offset %+.0f Hz\n",
			m.seq, len(built.Bursts), float64(len(fm))/basebandRate, cfg.Slot, cfg.ColorCode, cfg.Source, cfg.Dest, m.corr)
	}
	for _, c := range built.Carriers {
		fmt.Fprintf(w, "carrier at %+.0f Hz: %d slots from %.2f s\n", c.OffsetHz, len(c.Slots), float64(c.LeadFrames)*2*rftx.SlotSeconds)
	}
	if sink.f.iq() {
		rftx.Mix(built.Signals(fm, env, m.corr), m.rate, m.level, func(re, im float64) { sink.emit(re, im) })
		return nil
	}
	peak := 0.0
	for _, v := range fm {
		peak = math.Max(peak, math.Abs(v))
	}
	scale := m.level / math.Max(peak, 1)
	n := int(float64(len(fm)) * m.rate / basebandRate)
	for i := range n {
		pos := float64(i) * basebandRate / m.rate
		k := int(pos)
		v := fm[k]
		if k+1 < len(fm) {
			v += (fm[k+1] - fm[k]) * (pos - float64(k))
		}
		sink.emit(v * scale)
	}
	fmt.Fprintf(w, "audio: full scale is %.0f Hz of deviation, peak %.0f Hz\n", 1/scale, peak)
	return nil
}

type input struct {
	f    sampleFormat
	rate float64
	r    io.Reader
}

func openInput(path, format string, rate float64, std stdio) (input, func() error, error) {
	if path == "" {
		return input{}, nil, errors.New("-i is required: a file, or - for standard input")
	}
	closer := func() error { return nil }
	src := std.in
	if path != "-" {
		file, err := os.Open(path)
		if err != nil {
			return input{}, nil, err
		}
		src, closer = file, file.Close
	}
	br := bufio.NewReaderSize(src, 1<<16)
	in := input{rate: rate, r: br}
	var err error
	if format == "" && isWAV(br) {
		format = nameWAV
	}
	if in.f, err = formatFor(format, path); err != nil {
		_ = closer()
		return input{}, nil, err
	}
	if in.f.wav {
		var hdrRate float64
		if in.f, hdrRate, in.r, err = readWAVHeader(br); err != nil {
			_ = closer()
			return input{}, nil, err
		}
		if in.rate == 0 {
			in.rate = hdrRate
		}
	}
	if in.rate <= 0 {
		_ = closer()
		return input{}, nil, errors.New("-rate is required: give the sample rate of the input")
	}
	return in, closer, nil
}

func (in input) read(buf []byte) (int, error) {
	n, err := io.ReadFull(in.r, buf)
	n -= n % in.f.frame()
	if errors.Is(err, io.ErrUnexpectedEOF) {
		err = io.EOF
	}
	return n, err
}

func (in input) each(fn func(b []byte) error) error {
	buf := make([]byte, 1<<16-(1<<16)%in.f.frame())
	for {
		n, err := in.read(buf)
		if n > 0 {
			if ferr := fn(buf[:n]); ferr != nil {
				return ferr
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (in input) complexSamples(b []byte, fn func(complex128)) {
	sz := in.f.enc.size()
	if in.f.iq() {
		for i := 0; i+2*sz <= len(b); i += 2 * sz {
			fn(complex(normalize(in.f.enc, b[i:]), normalize(in.f.enc, b[i+sz:])))
		}
		return
	}
	for i := 0; i+sz <= len(b); i += sz {
		fn(complex(normalize(in.f.enc, b[i:]), 0))
	}
}

const basebandChunk = 1 << 14

func receiveIQ(in input, tier rf.Tier, offsetHz float64, burst func(rf.Burst)) (offset, dev float64, err error) {
	raw := (in.f.enc == encU8 || in.f.enc == encS8) && in.rate >= basebandRate && in.rate == math.Trunc(in.rate)
	cfg := rf.RxConfig{Tier: tier, OffsetHz: offsetHz, Rate: basebandRate, Format: rf.IQFormatCF32, OnBursts: emitAll(burst)}
	if raw {
		cfg.Rate, cfg.Format = int(in.rate), rf.IQFormatCU8
		if in.f.enc == encS8 {
			cfg.Format = rf.IQFormatCS8
		}
	}
	rx, err := rf.NewReceiver(&cfg)
	if err != nil {
		return 0, 0, err
	}
	if raw {
		err = in.each(func(b []byte) error {
			_, err := rx.Write(b)
			return err
		})
	} else {
		rs := newResampler(in.rate, basebandRate, 0, true)
		var wide []complex128
		x := make([]byte, 0, 8*basebandChunk)
		flush := func() error {
			for i, v := range wide {
				x = binary.LittleEndian.AppendUint32(x, math.Float32bits(float32(real(v))))
				x = binary.LittleEndian.AppendUint32(x, math.Float32bits(float32(imag(v))))
				if len(x) == cap(x) || i == len(wide)-1 {
					if _, err := rx.Write(x); err != nil {
						return err
					}
					x = x[:0]
				}
			}
			wide = wide[:0]
			return nil
		}
		err = in.each(func(b []byte) error {
			in.complexSamples(b, func(v complex128) { wide = rs.push(v, wide) })
			return flush()
		})
	}
	if err != nil {
		return 0, 0, err
	}
	if err := rx.Flush(); err != nil {
		return 0, 0, err
	}
	return rx.OffsetHz(), rx.DeviationHz(), nil
}

func emitAll(burst func(rf.Burst)) func([]rf.Burst) {
	return func(bs []rf.Burst) {
		for _, b := range bs {
			burst(b)
		}
	}
}

func receiveAudio(in input, tier rf.Tier, burst func(rf.Burst)) (offset, dev float64, err error) {
	hz := int(in.rate)
	rx, err := rf.NewAudioReceiver(&rf.AudioRxConfig{Rate: hz, Tier: tier, OnBursts: emitAll(burst)})
	if float64(hz) != in.rate || err != nil {
		hz = 24000
		if rx, err = rf.NewAudioReceiver(&rf.AudioRxConfig{Rate: hz, Tier: tier, OnBursts: emitAll(burst)}); err != nil {
			return 0, 0, err
		}
	}
	const chunk = 1 << 12
	pcm := make([]int16, 0, chunk)
	write := func(v float64) error {
		pcm = append(pcm, int16(math.Round(math.Max(-32767, math.Min(32767, v*32767)))))
		if len(pcm) < chunk {
			return nil
		}
		_, err := rx.Write(pcm)
		pcm = pcm[:0]
		return err
	}
	var rs *resampler
	if float64(hz) != in.rate {
		rs = newResampler(in.rate, float64(hz), 0, false)
	}
	var out []complex128
	err = in.each(func(b []byte) error {
		var werr error
		in.complexSamples(b, func(v complex128) {
			if rs == nil {
				werr = errors.Join(werr, write(real(v)))
				return
			}
			out = rs.push(v, out[:0])
			for _, y := range out {
				werr = errors.Join(werr, write(real(y)))
			}
		})
		return werr
	})
	if err != nil {
		return 0, 0, err
	}
	if len(pcm) > 0 {
		if _, err := rx.Write(pcm); err != nil {
			return 0, 0, err
		}
	}
	if err := rx.Flush(); err != nil {
		return 0, 0, err
	}
	return float64(rx.Offset()), float64(rx.Level()), nil
}

func runRX(args []string, std stdio) (err error) {
	fs := flag.NewFlagSet("rx", flag.ContinueOnError)
	in := fs.String("i", "", "input file, or - for standard input")
	format := fs.String("format", "", formatUsage)
	rate := fs.Float64("rate", 0, "sample rate in Hz (default from a WAV header)")
	tune := tuningFlags(fs, "tuned frequency in Hz, for -ppm and the ppm figure", "frequency offset of the carrier from the tuned frequency in Hz, as with an offset tune (I/Q only)")
	tier := fs.Int("tier", 3, "receiver tier 0 to 3: 0 discriminator, 1 with the soft stages, 2 coherent, 3 with the phase in the trellis state; audio defaults to the rf default")
	ambeSpec := fs.String("ambe", "", ambeUsage)
	pcmPath := fs.String("pcm", "", "write the decoded AMBE+2 voice as 8 kHz 16-bit little-endian PCM to this file, or - for standard output (needs -ambe)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pcmPath != "" && *ambeSpec == "" {
		return errors.New("-pcm needs -ambe")
	}
	if *tier < 0 || *tier > 3 {
		return fmt.Errorf("-tier %d, expected 0 to 3", *tier)
	}
	w := std.out
	if *pcmPath == "-" {
		w = std.msg
	}
	var audio *audioOut
	if *ambeSpec != "" {
		a, err := openAMBE(*ambeSpec, std.software)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, a.close()) }()
		out := io.Discard
		switch *pcmPath {
		case "":
		case "-":
			bw := bufio.NewWriter(std.out)
			defer func() { err = errors.Join(err, bw.Flush()) }()
			out = bw
		default:
			f, err := os.Create(*pcmPath)
			if err != nil {
				return err
			}
			defer func() { err = errors.Join(err, f.Close()) }()
			out = f
		}
		audio = &audioOut{codec: a.codec, w: out}
		fmt.Fprintf(w, "ambe: %s\n", a.desc)
	}
	src, closeIn, err := openInput(*in, *format, *rate, std)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeIn()) }()
	errHz, err := tune.errorHz()
	if err != nil {
		return err
	}
	rep := NewReporter(w)
	rep.audio = audio
	n := 0
	burst := func(b rf.Burst) {
		n++
		rep.Burst(b)
	}
	if src.f.iq() {
		off, dev, err := receiveIQ(src, rf.Tier0+rf.Tier(*tier), *tune.offset-errHz, burst)
		if err != nil {
			return err
		}
		off -= *tune.offset - errHz
		ppm := ""
		if *tune.freq > 0 {
			ppm = fmt.Sprintf(" (%+.2f ppm)", round(off / *tune.freq * 1e6, 2))
		}
		fmt.Fprintf(w, "carrier offset %+.0f Hz%s, outer deviation %.0f Hz, %d bursts\n", round(off, 0), ppm, dev, n)
	} else {
		if tune.set() {
			return errors.New("-ppm and -offset-hz apply to I/Q only")
		}
		t := rf.TierDefault
		if isSet(fs, "tier") {
			t = rf.Tier0 + rf.Tier(*tier)
		}
		dc, dev, err := receiveAudio(src, t, burst)
		if err != nil {
			return err
		}
		fmt.Fprintf(w, "audio offset %+.0f, outer deviation %.0f, in 16-bit sample units, %d bursts\n", dc, dev, n)
	}
	if audio != nil {
		if audio.err != nil {
			return audio.err
		}
		fmt.Fprintf(w, "audio: %d frames, %.2f s\n", audio.frames, float64(audio.frames)*0.02)
	}
	return nil
}

func runCal(args []string, std stdio) (err error) {
	fs := flag.NewFlagSet("cal", flag.ContinueOnError)
	in := fs.String("i", "", "input file of I/Q, or - for standard input")
	format := fs.String("format", "", formatUsage)
	rate := fs.Float64("rate", 0, "sample rate in Hz (default from a WAV header)")
	tune := tuningFlags(fs, "frequency the carrier should be on, in Hz", "frequency offset of the carrier from the tuned frequency in Hz, as with an offset tune")
	if err := fs.Parse(args); err != nil {
		return err
	}
	src, closeIn, err := openInput(*in, *format, *rate, std)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, closeIn()) }()
	if !src.f.iq() {
		return errors.New("cal needs I/Q input")
	}
	errHz, err := tune.errorHz()
	if err != nil {
		return err
	}
	rs := newResampler(src.rate, basebandRate, errHz-*tune.offset, false)
	var x []complex128
	if err := src.each(func(b []byte) error {
		src.complexSamples(b, func(v complex128) { x = rs.push(v, x) })
		return nil
	}); err != nil {
		return err
	}
	off := carrierOffset(x)
	fmt.Fprintf(std.out, "carrier offset %+.1f Hz", round(off, 1))
	if *tune.freq > 0 {
		fmt.Fprintf(std.out, " (%+.2f ppm)", round(off / *tune.freq * 1e6, 2))
	}
	fmt.Fprintln(std.out)
	return nil
}

func round(v float64, digits int) float64 {
	p := math.Pow(10, float64(digits))
	if r := math.Round(v*p) / p; r != 0 {
		return r
	}
	return 0
}
