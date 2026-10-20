package rf

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type cmpFrame struct {
	sec  float64
	data layer2.RawBurst
}

func TestMMDVMCompare(t *testing.T) {
	t.Parallel()
	bin := os.Getenv("DMRGO_MMDVM")
	if bin == "" {
		t.Skip("DMRGO_MMDVM names the mmdvmrx host build")
	}
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9, Group: true, Superframes: 20}
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	truth := make([]float64, len(built.Bursts))
	for i := range truth {
		truth[i] = msPad + float64(2*i+1)*slotSeconds
	}
	dir := t.TempDir()
	seeds := 6
	fmt.Println("C/N dB, Eb/N0 dB, receiver: voice frames exact of n, headers and terminators exact of n")
	for cn := 2.0; cn <= 16; cn++ {
		tot := map[string][3]int{}
		for seed := range seeds {
			x := msBaseband(fm, env, 400, cn, uint64(seed+1))
			a24 := discAudio(x, 24000)
			path := filepath.Join(dir, "a.s16")
			if err := writeS16(path, a24, -8); err != nil {
				t.Fatal(err)
			}
			mm, err := runMMDVM(t.Context(), bin, path, int(cfg.ColorCode))
			if err != nil {
				t.Fatal(err)
			}
			for name, got := range map[string][]cmpFrame{
				"mmdvm 24k": mm,
				"ours 24k":  oursFrames(t, a24, 24000),
				"ours 9.6k": oursFrames(t, discAudio(x, 9600), 9600),
			} {
				v, h, c := scoreCompare(built.Bursts, truth, got)
				s := tot[name]
				tot[name] = [3]int{s[0] + v, s[1] + h, s[2] + c}
			}
		}
		nv, nh := countKinds(built.Bursts)
		names := make([]string, 0, len(tot))
		for k := range tot {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			fmt.Printf("%4.1f %5.2f %-10s voice %5.1f%%  ambe C0C1 %5.1f%%  hdr+term %3d/%3d\n", cn, cn+10*math.Log10(12500.0/9600), k, 100*float64(tot[k][0])/float64(nv*seeds), 100*float64(tot[k][2])/float64(3*nv*seeds), tot[k][1], nh*seeds)
		}
	}
}

const msPad = 0.3

func msBaseband(fm, env []float64, offsetHz, cn float64, seed uint64) []complex64 {
	p := int(msPad * basebandRate)
	x := make([]complex64, len(fm)+2*p)
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	var ph float64
	for i := range x {
		var re, im float64
		if k := i - p; k >= 0 && k < len(fm) {
			ph += 2 * math.Pi * (fm[k] + offsetHz) / basebandRate
			re, im = env[k]*math.Cos(ph), env[k]*math.Sin(ph)
		}
		x[i] = complex(float32(re+sigma*rng.NormFloat64()), float32(im+sigma*rng.NormFloat64()))
	}
	return x
}

func writeS16(path string, a []int16, gain int) error {
	b := make([]byte, 2*len(a))
	for i, v := range a {
		s := max(min(int(v)*gain, math.MaxInt16), math.MinInt16)
		b[2*i], b[2*i+1] = byte(s), byte(s>>8)
	}
	return os.WriteFile(path, b, 0o600)
}

func runMMDVM(ctx context.Context, bin, path string, cc int) ([]cmpFrame, error) {
	out, err := exec.CommandContext(ctx, bin, path, strconv.Itoa(cc)).Output()
	if err != nil {
		return nil, err
	}
	var fr []cmpFrame
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) != 3 {
			continue
		}
		n, _ := strconv.Atoi(f[0])
		b, err := hex.DecodeString(f[2])
		if err != nil || len(b) != 33 {
			continue
		}
		var r layer2.RawBurst
		copy(r[:], b)
		fr = append(fr, cmpFrame{sec: float64(n) / 24000, data: r})
	}
	return fr, nil
}

func oursFrames(t *testing.T, a []int16, hz int) []cmpFrame {
	t.Helper()
	var fr []cmpFrame
	r, err := NewDisc(DiscConfig{Hz: hz, OnBurst: func(b *Burst) {
		d, _ := b.Decoded()
		fr = append(fr, cmpFrame{sec: b.Seconds, data: d})
	}})
	if err != nil {
		t.Fatal(err)
	}
	for k := 0; k < len(a); k += 1024 {
		r.Write(a[k:min(k+1024, len(a))])
	}
	r.Flush()
	return fr
}

func burstKind(_ layer2.RawBurst, i, n int) (voice, hdr bool) {
	if i == 0 || i == n-1 {
		return false, true
	}
	return true, false
}

func countKinds(bursts []layer2.RawBurst) (nv, nh int) {
	for i, b := range bursts {
		v, h := burstKind(b, i, len(bursts))
		if v {
			nv++
		}
		if h {
			nh++
		}
	}
	return nv, nh
}

func scoreCompare(bursts []layer2.RawBurst, truth []float64, got []cmpFrame) (voice, hdr, c01 int) {
	bestN, bestOff := -1, 0.0
	for _, g := range got[:min(len(got), 20)] {
		for _, tt := range truth {
			off := g.sec - tt
			n := 0
			for _, h := range got {
				if i := nearest(truth, h.sec-off); i >= 0 && h.data == bursts[i] {
					n++
				}
			}
			if n > bestN {
				bestN, bestOff = n, off
			}
		}
	}
	byIdx := map[int]layer2.RawBurst{}
	for _, g := range got {
		if i := nearest(truth, g.sec-bestOff); i >= 0 {
			byIdx[i] = g.data
		}
	}
	for i, b := range bursts {
		d, ok := byIdx[i]
		if !ok {
			continue
		}
		v, h := burstKind(b, i, len(bursts))
		switch {
		case v:
			want, _ := b.AMBEFrames()
			gotF, _ := d.AMBEFrames()
			if want == gotF {
				voice++
			}
			for k := range want {
				if gotF[k][0] == want[k][0] && gotF[k][1] == want[k][1] && gotF[k][2] == want[k][2] {
					c01++
				}
			}
		case h:
			want, _ := b.BPTCPayload()
			gotP, _ := d.BPTCPayload()
			if want == gotP && d.SlotType().DataType == b.SlotType().DataType {
				hdr++
			}
		}
	}
	return voice, hdr, c01
}

func nearest(truth []float64, sec float64) int {
	i := int(math.Round((sec - truth[0]) / (2 * slotSeconds)))
	if i < 0 || i >= len(truth) || math.Abs(sec-truth[i]) > 0.005 {
		return -1
	}
	return i
}
