package main

import (
	"bufio"
	"bytes"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestCaptureXPR8400GainSweep decodes a voice call from the weakest capture
// of the RTL-SDR gain sweep, about 7.3 dB C/N, and checks how many of its
// AMBE+2 frames each tier recovers exactly, in order.
func TestCaptureXPR8400GainSweep(t *testing.T) {
	t.Parallel()
	iq, err := os.ReadFile("testdata/xpr8400_gain_sweep.cu8")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("testdata/xpr8400_gain_sweep.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var want []uint64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "#") {
			continue
		}
		v, err := strconv.ParseUint(sc.Text(), 16, 64)
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, v)
	}
	ambe := regexp.MustCompile(`^ambe .* 0x([0-9a-f]+) fec=`)
	for _, c := range []struct {
		tier Tier
		min  int
	}{{TierDisc, 32}, {TierCoherent, 45}, {TierMax, 47}} {
		var buf bytes.Buffer
		rep := NewReporter(&buf)
		for _, r := range DemodulateBytes(iq, 256e3, false, c.tier).Bursts {
			rep.Burst(r)
		}
		n, next := 0, 0
		for _, line := range strings.Split(buf.String(), "\n") {
			m := ambe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			v, _ := strconv.ParseUint(m[1], 16, 64)
			for k := next; k < len(want); k++ {
				if want[k] == v {
					n, next = n+1, k+1
					break
				}
			}
		}
		t.Logf("tier %d: %d of %d frames exact", c.tier, n, len(want))
		if n < c.min {
			t.Errorf("tier %d: %d of %d frames exact, want at least %d", c.tier, n, len(want), c.min)
		}
	}
}
