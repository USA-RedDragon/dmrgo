package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runText(t *testing.T, std stdio, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	if std.out == nil {
		std.out = &buf
	}
	if std.msg == nil {
		std.msg = &buf
	}
	if err := runIO(args, std); err != nil {
		t.Fatalf("dmrrf %s: %v\n%s", strings.Join(args, " "), err, buf.String())
	}
	return buf.String()
}

func wantPacket(t *testing.T, out string, bursts string) {
	t.Helper()
	if !strings.Contains(out, `packet complete: "DMRGO TEST"`) {
		t.Errorf("no complete packet in\n%s", out)
	}
	if !strings.Contains(out, bursts) {
		t.Errorf("no %q in\n%s", bursts, out)
	}
}

func TestSampleRoundTrip(t *testing.T) {
	t.Parallel()
	for _, e := range []encoding{encU8, encS8, encS16, encF32} {
		step := map[encoding]float64{encU8: 1 / 127.5, encS8: 1.0 / 127, encS16: 1.0 / 32767, encF32: 1e-7}[e]
		for _, v := range []float64{-1, -0.7071, -0.25, 0, 0.1, 0.5, 0.9, 1} {
			b := appendSample(nil, e, v)
			if len(b) != e.size() {
				t.Fatalf("encoding %d: %d bytes, want %d", e, len(b), e.size())
			}
			if got := normalize(e, b); math.Abs(got-v) > step/2+1e-12 {
				t.Errorf("encoding %d: %v came back as %v", e, v, got)
			}
		}
	}
}

func TestFormatRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct {
		name, file, format, rate, bursts string
		rxRate                           bool
	}{
		{"cu8", "a.cu8", "", "250000", "6 bursts", true},
		{"cs8", "a.cs8", "", "2000000", "6 bursts", true},
		{"cs16le", "a.cs16", "", "1000000", "6 bursts", true},
		{"cf32le", "a.cfile", "", "96000", "6 bursts", true},
		{"cs8 under 48 kHz", "low.cs8", "", "30000", "6 bursts", true},
		{"cu8 by flag", "a.bin", "cu8", "250000", "6 bursts", true},
		{"s16le", "a.raw", "s16le", "24000", "bursts", true},
		{"s16le resampled", "b.raw", "s16le", "44100", "bursts", true},
		{"wav", "a.wav", "", "48000", "bursts", false},
	} {
		path := filepath.Join(dir, c.file)
		tx := []string{"tx", "-seq", "data12", "-o", path, "-rate", c.rate}
		rx := []string{"rx", "-i", path}
		if c.format != "" {
			tx = append(tx, "-format", c.format)
			rx = append(rx, "-format", c.format)
		}
		if c.rxRate {
			rx = append(rx, "-rate", c.rate)
		}
		runText(t, stdio{}, tx...)
		wantPacket(t, runText(t, stdio{}, rx...), c.bursts)
	}
}

func TestFormatOffsetAndPPM(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "off.cf32")
	runText(t, stdio{}, "tx", "-seq", "data12", "-o", path, "-rate", "500000", "-freq", "446000000", "-ppm", "10", "-offset-hz", "20000")
	out := runText(t, stdio{}, "rx", "-i", path, "-rate", "500000", "-offset-hz", "20000")
	wantPacket(t, out, "carrier offset -4460 Hz")
	out = runText(t, stdio{}, "rx", "-i", path, "-rate", "500000", "-offset-hz", "20000", "-freq", "446000000", "-ppm", "10")
	wantPacket(t, out, "carrier offset +0 Hz")
}

func TestFormatInference(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	wav := filepath.Join(dir, "a.wav")
	runText(t, stdio{}, "tx", "-seq", "data12", "-o", wav, "-rate", "24000")
	b, err := os.ReadFile(wav)
	if err != nil {
		t.Fatal(err)
	}
	if string(b[:4]) != "RIFF" || len(b) != wavHeaderSize+2*7920 {
		t.Fatalf("WAV of %d bytes starting %q", len(b), b[:4])
	}
	odd := filepath.Join(dir, "capture.dat")
	if err := os.WriteFile(odd, b, 0o600); err != nil {
		t.Fatal(err)
	}
	wantPacket(t, runText(t, stdio{}, "rx", "-i", odd), "bursts")
	if err := os.WriteFile(filepath.Join(dir, "x.iq"), make([]byte, 64), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"rx", "-i", filepath.Join(dir, "x.iq"), "-rate", "1e6"}, "give -format"},
		{[]string{"rx", "-i", "-", "-rate", "1e6"}, "give -format"},
		{[]string{"rx", "-i", wav, "-format", "cs12"}, "want cu8"},
		{[]string{"rx", "-i", wav, "-offset-hz", "100"}, "I/Q only"},
		{[]string{"tx", "-o", filepath.Join(dir, "x.cs8")}, "-rate is required"},
		{[]string{"tx", "-o", filepath.Join(dir, "x.cs8"), "-rate", "1e6", "-ppm", "2"}, "-ppm needs -freq"},
		{[]string{"tx", "-o", filepath.Join(dir, "x.wav"), "-cw", "1"}, "I/Q only"},
		{[]string{"tx", "-o", filepath.Join(dir, "x.wav"), "-seq", "t3site", "-bs-sourced"}, "needs an I/Q format"},
		{[]string{"tx", "-seq", "data12"}, "tx needs -o"},
		{[]string{"cal", "-i", wav}, "cal needs I/Q"},
	} {
		if err := runIO(c.args, stdio{in: bytes.NewReader(nil), out: io.Discard, msg: io.Discard}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("dmrrf %s: %v, want %q", strings.Join(c.args, " "), err, c.want)
		}
	}
}

func TestTXHint(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, c := range []struct{ file, want string }{
		{"a.cs8", "for example: hackrf_transfer -t "},
		{"a.cs16", "for example: tx_sdr -F CS16 -f 446000000 -s 1000000 "},
		{"a.cf32", ""},
		{"a.cu8", ""},
	} {
		out := runText(t, stdio{}, "tx", "-cw", "0.01", "-rate", "1e6", "-freq", "446000000", "-o", filepath.Join(dir, c.file))
		if c.want == "" && strings.Contains(out, "for example") || c.want != "" && !strings.Contains(out, c.want) {
			t.Errorf("%s: want hint %q in\n%s", c.file, c.want, out)
		}
	}
}

func TestPipe(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		tx   []string
		rx   []string
		want string
	}{
		{"cs16le", []string{"-format", "cs16le", "-rate", "250000"}, []string{"-format", "cs16le", "-rate", "250000"}, "6 bursts"},
		{"cf32le", []string{"-format", "cf32le", "-rate", "48000"}, []string{"-format", "cf32le", "-rate", "48000"}, "6 bursts"},
		{"wav", []string{"-format", "wav", "-rate", "48000"}, nil, "bursts"},
	} {
		pr, pw := io.Pipe()
		var txMsg bytes.Buffer
		done := make(chan error, 1)
		go func() {
			err := runIO(append([]string{"tx", "-seq", "data12", "-o", "-"}, c.tx...), stdio{out: pw, msg: &txMsg})
			pw.CloseWithError(err)
			done <- err
		}()
		out := runText(t, stdio{in: bufio.NewReader(pr)}, append([]string{"rx", "-i", "-"}, c.rx...)...)
		if err := <-done; err != nil {
			t.Fatalf("%s: tx: %v", c.name, err)
		}
		if !strings.Contains(txMsg.String(), "wrote -:") {
			t.Errorf("%s: tx messages did not go to the message stream:\n%s", c.name, txMsg.String())
		}
		wantPacket(t, out, c.want)
	}
}

func TestOSPipe(t *testing.T) {
	t.Parallel()
	if runtime.GOARCH == "wasm" {
		t.Skip("wasm has no pipe")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		err := runIO([]string{"tx", "-seq", "data12", "-o", "-", "-format", "cu8", "-rate", "2048000"}, stdio{out: w, msg: io.Discard})
		done <- errors.Join(err, w.Close())
	}()
	out := runText(t, stdio{in: r}, "rx", "-i", "-", "-format", "cu8", "-rate", "2048000")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	wantPacket(t, out, "6 bursts")
}

func TestFloatPathMatchesRaw(t *testing.T) {
	t.Parallel()
	raw := runText(t, stdio{}, "rx", "-i", weakCapture, "-rate", "256000")
	float := runText(t, stdio{}, "rx", "-i", weakCapture, "-rate", "256000", "-offset-hz", "0.001")
	for name, out := range map[string]string{"raw": raw, "float": float} {
		if n := strings.Count(out, "\nambe "); n != 18 {
			t.Errorf("%s path: %d ambe lines, want 18", name, n)
		}
	}
}

func TestWAVHeaderBounds(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ channels, samples int }{{2, 1 << 30}, {1, -1}, {0, 10}, {1 << 15, 1}} {
		if _, err := wavHeader(48000, c.channels, c.samples); err == nil {
			t.Errorf("wavHeader(48000, %d, %d) has no error", c.channels, c.samples)
		}
	}
	if b, err := wavHeader(48000, 2, 1<<29); err != nil || len(b) != wavHeaderSize {
		t.Errorf("wavHeader of 2 GiB of data: %d bytes, %v", len(b), err)
	}
}
