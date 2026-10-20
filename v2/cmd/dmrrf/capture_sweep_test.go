package main

import (
	"bufio"
	"bytes"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

const (
	captureStarts = 8
	captureStep   = 0.0073
)

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
	for _, c := range []struct {
		tier rf.Tier
		min  int
	}{{rf.Tier0, 181}, {rf.Tier1, 220}, {rf.Tier2, 339}, {rf.Tier3, 374}, {rf.Tier4, 374}} {
		var got [captureStarts]int
		var wg sync.WaitGroup
		for k := range got {
			wg.Add(1)
			go func() {
				defer wg.Done()
				skip := 2 * int(float64(k)*captureStep*256e3)
				var err error
				if got[k], err = exactInOrder(iq[skip:], c.tier, want); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		n := 0
		for _, v := range got {
			n += v
		}
		t.Logf("tier %d: %d of %d frames exact, %v", c.tier-rf.Tier0, n, captureStarts*len(want), got)
		if n < c.min {
			t.Errorf("tier %d: %d of %d frames exact, want at least %d", c.tier-rf.Tier0, n, captureStarts*len(want), c.min)
		}
	}
}

func exactInOrder(iq []byte, tier rf.Tier, want []uint64) (int, error) {
	d, err := DemodulateBytes(iq, 256e3, false, tier)
	if err != nil {
		return 0, err
	}
	ambe := regexp.MustCompile(`^ambe .* 0x([0-9a-f]+) fec=`)
	var buf bytes.Buffer
	rep := NewReporter(&buf)
	for _, r := range d.Bursts {
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
	return n, nil
}
