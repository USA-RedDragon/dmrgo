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

func runTX(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("tx", flag.ContinueOnError)
	out := fs.String("o", "dmr.iq", "output file (signed 8-bit I/Q for hackrf_transfer -t)")
	seq := fs.String("seq", "data12", "wakeup, data12, data34 or data1")
	cw := fs.Float64("cw", 0, "write an unmodulated carrier of this many seconds instead")
	freq := fs.Float64("freq", 0, "carrier frequency in Hz, printed in the hackrf_transfer command")
	rate := fs.Float64("rate", 2e6, "sample rate in Hz (HackRF minimum is 2e6)")
	ppm := fs.Float64("ppm", 0, "measured HackRF error in ppm to correct (positive if it transmits high)")
	offset := fs.Float64("offset-hz", 0, "extra frequency offset in Hz to add")
	level := fs.Float64("level", 0.9, "peak amplitude, 0 to 1")
	slot := fs.Int("slot", 0, "TDMA slot 0 or 1, or -1 for back-to-back bursts")
	cc := fs.Uint("cc", 1, "color code")
	src := fs.Uint("src", 0, "source radio ID")
	dst := fs.Uint("dst", 0, "destination radio or talkgroup ID")
	bs := fs.Uint("bs", 0, "BS (repeater) address for the wakeup CSBK")
	group := fs.Bool("group", false, "send to a talkgroup instead of a radio")
	wakeups := fs.Int("wakeups", 3, "wakeup CSBKs before the data")
	msg := fs.String("msg", "DMRGO TEST", "message text")
	repeat := fs.Float64("repeat", 0, "repeat the sequence, padded to whole 60 ms frames, for this many seconds")
	stretch := fs.Float64("stretch", 0, "slow the symbol clock by this many ppm so repeats sweep across the repeater's slot timing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	corr := *offset
	if *freq > 0 {
		corr -= *ppm * 1e-6 * *freq
	}
	var iq []byte
	if *cw > 0 {
		iq = CW(*cw, *rate, corr, *level)
		fmt.Fprintf(w, "carrier: %.1f s, offset %+.0f Hz\n", *cw, corr)
	} else {
		cfg := SeqConfig{ColorCode: uint8(*cc), Source: uint32(*src), Dest: uint32(*dst), BS: uint32(*bs), Group: *group, Wakeups: *wakeups, Message: *msg} //nolint:gosec // flag ranges are checked by the encoders
		bursts, err := Sequence(*seq, cfg)
		if err != nil {
			return err
		}
		fm, env := Modulate(bursts, Schedule{Slot: *slot})
		if *repeat > 0 {
			fm, env = padToFrames(fm, env)
		}
		iq = ToIQ(fm, env, *rate*(1+*stretch*1e-6), corr, *level)
		fmt.Fprintf(w, "sequence %s: %d bursts, %.2f s, slot %d, cc %d, %d -> %d, offset %+.0f Hz\n",
			*seq, len(bursts), float64(len(fm))/basebandRate, *slot, *cc, *src, *dst, corr)
		if *repeat > 0 {
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
		return FromHackRF(b), nil
	}
	return FromRTL(b), nil
}

func runRX(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("rx", flag.ContinueOnError)
	in := fs.String("i", "", "input file (rtl_sdr unsigned 8-bit I/Q)")
	rate := fs.Float64("rate", 2.048e6, "sample rate in Hz")
	freq := fs.Float64("freq", 0, "tuned frequency in Hz, for the ppm figure")
	signed := fs.Bool("signed", false, "input is signed 8-bit (HackRF) instead of rtl_sdr unsigned")
	if err := fs.Parse(args); err != nil {
		return err
	}
	x, err := readIQ(*in, *signed)
	if err != nil {
		return err
	}
	d := Demodulate(x, *rate)
	ppm := ""
	if *freq > 0 {
		ppm = fmt.Sprintf(" (%+.2f ppm)", d.OffsetHz / *freq * 1e6)
	}
	fmt.Fprintf(w, "carrier offset %+.0f Hz%s, outer deviation %.0f Hz, %d bursts\n", d.OffsetHz, ppm, d.DeviationHz, len(d.Bursts))
	rep := NewReporter(w)
	for _, r := range d.Bursts {
		rep.Burst(r)
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
