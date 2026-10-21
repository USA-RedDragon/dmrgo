package main

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func TestRXCarrierOffset(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Superframes = 3
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	const freq, off = 450e6, 11000.0
	b := rftx.Channel(rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6, 2.048e6, off, 25, 5)
	in := filepath.Join(t.TempDir(), "in.cu8")
	if err := os.WriteFile(in, b, 0o600); err != nil {
		t.Fatal(err)
	}
	line := regexp.MustCompile(`carrier offset ([-+0-9]+) Hz.*, (\d+) bursts`)
	for _, c := range []struct {
		args []string
		set  float64
	}{
		{nil, 0},
		{[]string{"-freq", "450e6", "-ppm", strconv.FormatFloat(-off/freq*1e6, 'f', 3, 64)}, off},
		{[]string{"-offset-hz", "10800"}, 10800},
	} {
		var out bytes.Buffer
		if err := run(append([]string{"rx", "-i", in, "-format", "cu8", "-rate", "2.048e6", "-tier", "0"}, c.args...), &out); err != nil {
			t.Fatal(err)
		}
		m := line.FindStringSubmatch(out.String())
		if m == nil {
			t.Fatalf("%v: no summary line in %q", c.args, out.String())
		}
		hz, _ := strconv.ParseFloat(m[1], 64)
		n, _ := strconv.Atoi(m[2])
		switch {
		case c.set != 0 && (n < len(built.Bursts)-2 || hz < off-c.set-100 || hz > off-c.set+100):
			t.Errorf("%v: %d bursts of %d, residual offset %+.0f Hz", c.args, n, len(built.Bursts), hz)
		case c.set == 0 && n > 2:
			t.Errorf("%v: %d bursts without the offset; the test no longer shows the next channel", c.args, n)
		}
	}
}
