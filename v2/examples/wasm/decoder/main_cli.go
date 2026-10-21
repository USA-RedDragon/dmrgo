//go:build !(js && wasm)

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	format := flag.String("format", "", "wav, f32, s16, cu8, cs8, cs16 or cf32 (default from the file extension)")
	rate := flag.Float64("rate", 0, "sample rate in Hz (default from a WAV header)")
	tier := flag.Int("tier", -1, "receiver tier, -1 for the default of the input")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: decoder [-format f] [-rate hz] [-tier n] file")
		os.Exit(2)
	}
	if err := run(os.Stdout, os.Stderr, flag.Arg(0), options{format: *format, rate: *rate, audioTier: *tier, iqTier: *tier}); err != nil {
		fmt.Fprintln(os.Stderr, "decoder:", err)
		os.Exit(1)
	}
}

func formatOf(path string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
}

func run(w, msg io.Writer, path string, opt options) error {
	if opt.format == "" {
		opt.format = formatOf(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	d, err := newDecoder(opt)
	if err != nil {
		return err
	}
	start := time.Now()
	buf := make([]byte, 1<<20)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if werr := d.write(buf[:n]); werr != nil {
				return werr
			}
			if _, werr := w.Write(d.rep.take()); werr != nil {
				return werr
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	if err := d.flush(); err != nil {
		return err
	}
	d.rep.emit(d.rep.summary(d.seconds()))
	el := time.Since(start).Seconds()
	if _, err := w.Write(d.rep.take()); err != nil {
		return err
	}
	fmt.Fprintf(msg, "%.3f s of input in %.3f s, real-time factor %.4f\n", d.seconds(), el, el/max(d.seconds(), 1e-9))
	return nil
}
