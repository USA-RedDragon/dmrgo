// Command dmrrf builds DMR bursts as HackRF I/Q for transmission and decodes
// DMR from rtl_sdr I/Q captures. It writes and reads files only; transmitting
// is done separately with hackrf_transfer by a licensed operator.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "dmrrf:", err)
		os.Exit(1)
	}
}

func run(args []string, w io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: dmrrf tx|rx|cal [flags]")
	}
	switch args[0] {
	case "tx":
		return runTX(args[1:], w)
	case "rx":
		return runRX(args[1:], w)
	case "cal":
		return runCal(args[1:], w)
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

func runTX(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("tx", flag.ContinueOnError)
	out := fs.String("o", "dmr.iq", "output file (signed 8-bit I/Q for hackrf_transfer -t)")
	seq := fs.String("seq", "data12", strings.Join(rftx.Sequences(), ", "))
	cw := fs.Float64("cw", 0, "write an unmodulated carrier of this many seconds instead")
	freq := fs.Float64("freq", 0, "carrier frequency in Hz, printed in the hackrf_transfer command")
	rate := fs.Float64("rate", 2e6, "sample rate in Hz (HackRF minimum is 2e6)")
	ppm := fs.Float64("ppm", 0, "measured HackRF error in ppm to correct (positive if it transmits high)")
	offset := fs.Float64("offset-hz", 0, "extra frequency offset in Hz to add")
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
	dd := uint8(7)
	fs.Func("dd", "Defined Data format of the sms sequence, 0 to 63 (default 7, UTF-16)", func(s string) error {
		v, err := strconv.ParseUint(s, 10, 6)
		dd = uint8(v)
		return err
	})
	bsSourced := fs.Bool("bs-sourced", false, "send a continuous BS outbound stream (CACH, Short LC, idle bursts on the other slot) like a repeater")
	if err := fs.Parse(args); err != nil {
		return err
	}
	corr := *offset
	if *freq > 0 {
		corr -= *ppm * 1e-6 * *freq
	}
	var iq []byte
	if *cw > 0 {
		iq = rftx.CW(*cw, *rate, corr, *level)
		fmt.Fprintf(w, "carrier: %.1f s, offset %+.0f Hz\n", *cw, corr)
	} else {
		cfg := rftx.SeqConfig{ColorCode: cc, Source: *src, Dest: *dst, BS: *bs, Group: *group, Wakeups: *wakeups, Message: *msg,
			Superframes: *superframes, AMBESilence: *silence, LateEntry: *lateEntry, Preambles: *preambles, DefinedData: dd, BSSourced: *bsSourced, Slot: *slot}
		built, err := rftx.Sequence(*seq, cfg)
		if err != nil {
			return err
		}
		for _, l := range built.Info {
			fmt.Fprintln(w, l)
		}
		var fm, env []float64
		if *bsSourced {
			slots := built.Slots
			if *repeat > 0 {
				n := max(1, int(*repeat/(float64(len(slots))*rftx.SlotSeconds)))
				for range n - 1 {
					slots = append(slots, built.Slots...)
				}
				fmt.Fprintf(w, "repeated %d times in one continuous stream\n", n)
			}
			fm, env = rftx.ModulateBS(slots)
			fmt.Fprintf(w, "sequence %s (BS-sourced): %d bursts on slot %d in %d slots, %.2f s, cc %d, %d -> %d, offset %+.0f Hz\n",
				*seq, len(built.Bursts), *slot, len(slots), float64(len(fm))/basebandRate, cc, *src, *dst, corr)
		} else {
			fm, env = rftx.Modulate(built.Bursts, rftx.Schedule{Slot: *slot})
			if *repeat > 0 {
				fm, env = rftx.PadToFrames(fm, env)
			}
			fmt.Fprintf(w, "sequence %s: %d bursts, %.2f s, slot %d, cc %d, %d -> %d, offset %+.0f Hz\n",
				*seq, len(built.Bursts), float64(len(fm))/basebandRate, *slot, cc, *src, *dst, corr)
		}
		if *repeat > 0 && len(built.Carriers) > 0 {
			return fmt.Errorf("-repeat does not apply to the multi-channel sequence %s", *seq)
		}
		for _, c := range built.Carriers {
			fmt.Fprintf(w, "carrier at %+.0f Hz: %d slots from %.2f s\n", c.OffsetHz, len(c.Slots), float64(c.LeadFrames)*2*rftx.SlotSeconds)
		}
		iq = rftx.MixIQ(built.Signals(fm, env, corr), *rate*(1+*stretch*1e-6), *level)
		if *repeat > 0 && !*bsSourced {
			n := max(1, int(*repeat**rate*2/float64(len(iq))))
			iq = bytes.Repeat(iq, n)
			fmt.Fprintf(w, "repeated %d times (%.1f s), stretch %.0f ppm\n", n, float64(len(iq))/2 / *rate, *stretch)
		}
	}
	if err := os.WriteFile(*out, iq, 0o600); err != nil {
		return err
	}
	f := "<freq>"
	if *freq > 0 {
		f = fmt.Sprintf("%.0f", *freq)
	}
	fmt.Fprintf(w, "wrote %s (%d bytes)\ntransmit: hackrf_transfer -t %s -f %s -s %.0f -a 0 -x 0\n", *out, len(iq), *out, f, *rate)
	fmt.Fprintln(w, "start with -x 0 and raise TX VGA gain slowly; use an attenuator or dummy load for loopback tests")
	return nil
}

func readIQ(path string, signed bool) ([]complex128, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if signed {
		return rftx.FromHackRF(b), nil
	}
	return rftx.FromRTL(b), nil
}

func runRX(args []string, w io.Writer) (err error) {
	fs := flag.NewFlagSet("rx", flag.ContinueOnError)
	in := fs.String("i", "", "input file (rtl_sdr unsigned 8-bit I/Q)")
	rate := fs.Float64("rate", 2.048e6, "sample rate in Hz")
	freq := fs.Float64("freq", 0, "tuned frequency in Hz, for the ppm figure")
	signed := fs.Bool("signed", false, "input is signed 8-bit (HackRF) instead of rtl_sdr unsigned")
	tier := fs.Int("tier", 3, "receiver tier 0 to 4: 0 discriminator, 1 with the soft stages, 2 coherent, 3 with the phase in the trellis state, 4 as 3 over a longer window")
	ambeSpec := fs.String("ambe", "", ambeUsage)
	pcmPath := fs.String("pcm", "", "write the decoded AMBE+2 voice as 8 kHz 16-bit little-endian PCM to this file (needs -ambe)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *pcmPath != "" && *ambeSpec == "" {
		return errors.New("-pcm needs -ambe")
	}
	var audio *audioOut
	if *ambeSpec != "" {
		a, err := openAMBE(*ambeSpec)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, a.close()) }()
		out := io.Discard
		if *pcmPath != "" {
			f, err := os.Create(*pcmPath)
			if err != nil {
				return err
			}
			defer f.Close()
			out = f
		}
		audio = &audioOut{codec: a.codec, w: out}
		fmt.Fprintf(w, "ambe: %s\n", a.desc)
	}
	b, err := os.ReadFile(*in)
	if err != nil {
		return err
	}
	if *tier < 0 || *tier > 4 {
		return fmt.Errorf("-tier %d, want 0 to 4", *tier)
	}
	d, err := DemodulateBytes(b, *rate, *signed, rf.Tier0+rf.Tier(*tier))
	if err != nil {
		return err
	}
	ppm := ""
	if *freq > 0 {
		ppm = fmt.Sprintf(" (%+.2f ppm)", d.OffsetHz / *freq * 1e6)
	}
	fmt.Fprintf(w, "carrier offset %+.0f Hz%s, outer deviation %.0f Hz, %d bursts\n", d.OffsetHz, ppm, d.DeviationHz, len(d.Bursts))
	rep := NewReporter(w)
	rep.audio = audio
	for _, r := range d.Bursts {
		rep.Burst(r)
	}
	if audio != nil {
		if audio.err != nil {
			return audio.err
		}
		fmt.Fprintf(w, "audio: %d frames, %.2f s\n", audio.frames, float64(audio.frames)*0.02)
	}
	return nil
}

func runCal(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("cal", flag.ContinueOnError)
	in := fs.String("i", "", "input file (rtl_sdr unsigned 8-bit I/Q)")
	rate := fs.Float64("rate", 2.048e6, "sample rate in Hz")
	freq := fs.Float64("freq", 0, "frequency the signal should be on, in Hz")
	signed := fs.Bool("signed", false, "input is signed 8-bit (HackRF) instead of rtl_sdr unsigned")
	if err := fs.Parse(args); err != nil {
		return err
	}
	x, err := readIQ(*in, *signed)
	if err != nil {
		return err
	}
	off := carrierOffset(decimate(x, *rate))
	fmt.Fprintf(w, "carrier offset %+.1f Hz", off)
	if *freq > 0 {
		fmt.Fprintf(w, " (%+.2f ppm)", off / *freq * 1e6)
	}
	fmt.Fprintln(w)
	return nil
}
