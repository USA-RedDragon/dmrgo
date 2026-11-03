//go:build !tinygo

package fsktx

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
)

var updateHighpass = flag.Bool("update-highpass", false, "measure the stated input high pass into testdata/highpass.json and rewrite its block of docs/tx-notes.md")

const (
	highpassFile    = "testdata/highpass.json"
	highpassBegin   = "<!-- txhighpass:begin -->"
	highpassEnd     = "<!-- txhighpass:end -->"
	hpSheetBegin    = "<!-- txhighpass:sheet:begin -->"
	hpSheetEnd      = "<!-- txhighpass:sheet:end -->"
	highpassStreams = 64
)

var (
	highpassCorners = []int{1, 10, 20}
	highpassRates   = []int{24000, 48000, 96000}
)

type highpassLimit struct {
	New     []string `json:"new_failures"`
	Point   string   `json:"point"`
	Key     string   `json:"key"`
	Base    float64  `json:"base_db"`
	Stated  float64  `json:"stated_db"`
	RoomHz  float64  `json:"room_hz"`
	Corner  int      `json:"corner_hz"`
	Lowpass int      `json:"lowpass_hz"`
}

type highpassEye struct {
	Tier     int     `json:"tier"`
	Corner   int     `json:"corner_hz"`
	Off      float64 `json:"off_mean"`
	On       float64 `json:"on_mean"`
	RRC      float64 `json:"rrc_mean"`
	DiffMean float64 `json:"diff_mean"`
	DiffSE   float64 `json:"diff_se"`
}

type highpassData struct {
	Worst []highpassLimit `json:"worst"`
	Eyes  []highpassEye   `json:"eyes"`
}

func highpassRoomOf(u txUnit, bits int) float64 {
	a, err := NewAudio(AudioConfig{Hz: u.rate, Level: 1, Tier: u.tier})
	if err != nil {
		return 0
	}
	pk, err := a.PeakAt(u.level)
	avail := float64(int(1)<<(bits-1) - 1)
	if err != nil || math.Ceil(pk) >= avail {
		return 0
	}
	return (avail - math.Ceil(pk)) * 3 * DeviationHz / float64(u.level)
}

func measureHighpassLimits(t *testing.T) []highpassLimit {
	type job struct {
		p       limPoint
		corner  int
		room    float64
		lowpass int
	}
	var jobs []job
	for _, tier := range []Tier{Tier1, Tier2} {
		c := limClass{tier: tier, bits: 16}
		for _, rate := range highpassRates {
			for _, lp := range inputLowpassGrid[:2] {
				for _, p := range c.points(rate, lp) {
					for _, hz := range highpassCorners {
						q := p
						q.u.hp, q.u.room = hz, highpassRoomOf(p.u, 16)
						jobs = append(jobs, job{p: q, corner: hz, room: q.u.room, lowpass: lp})
					}
					jobs = append(jobs, job{p: p, lowpass: lp})
				}
			}
		}
	}
	res := make([]map[string]float64, len(jobs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, testParallel()))
	for i := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			res[i] = jobs[i].p.measure(t, 8)
		}()
	}
	wg.Wait()
	base := map[string]map[string]float64{}
	for i, j := range jobs {
		if j.corner == 0 {
			base[j.p.String()] = res[i]
		}
	}
	worst := map[int]highpassLimit{}
	news := map[int][]string{}
	keys := func(m map[string]float64) []string {
		ks := make([]string, 0, len(m))
		for k := range m {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		return ks
	}
	for i, j := range jobs {
		if j.corner == 0 {
			continue
		}
		v := j.p
		v.u.hp, v.u.room = 0, 0
		b := base[v.String()]
		for _, k := range keys(res[i]) {
			m, need := res[i][k], limNeed(k)
			if m < need && b[k] >= need {
				news[j.corner] = append(news[j.corner], fmt.Sprintf("%s %s %+.2f", j.p, k, m-need))
			}
			if b[k] < need {
				continue
			}
			w, ok := worst[j.corner]
			if !ok || m-need < w.Stated-limNeed(w.Key) {
				worst[j.corner] = highpassLimit{Point: j.p.String(), Key: k, Base: b[k], Stated: m, RoomHz: j.room, Corner: j.corner, Lowpass: j.lowpass}
			}
		}
	}
	for c, w := range worst {
		w.New = news[c]
		worst[c] = w
	}
	out := make([]highpassLimit, 0, len(worst))
	for _, w := range worst {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Corner < out[j].Corner })
	return out
}

func measureHighpassEyes(t *testing.T) []highpassEye {
	base := newTestStream(t, 2, 40).syms
	_, rrc := pairConfigs(48000)
	out := make([]highpassEye, 0, 2*len(highpassCorners))
	for _, tier := range []Tier{Tier1, Tier2} {
		for _, hz := range highpassCorners {
			u := txUnit{format: fmtAudio, tier: tier, rate: 48000, level: tableUnits}
			on := AudioConfig{Hz: 48000, Level: tableUnits, Tier: tier}
			off := on
			if room := highpassRoomOf(u, 16); room > 0 {
				on.Droop = HighpassDroop(float64(hz), room)
			}
			d := make([][3]float64, highpassStreams)
			var wg sync.WaitGroup
			sem := make(chan struct{}, max(1, testParallel()))
			for i := range d {
				rng := rand.New(rand.NewPCG(uint64(i), 0x5ac))
				syms := make([]int8, len(base))
				for k := range syms {
					syms[k] = [4]int8{-3, -1, 1, 3}[rng.IntN(4)]
				}
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer func() { <-sem; wg.Done() }()
					r := radio{acHz: float64(hz)}
					d[i] = [3]float64{pairEye(t, off, syms, r), pairEye(t, on, syms, r), pairEye(t, rrc, syms, r)}
				}()
			}
			wg.Wait()
			e := highpassEye{Tier: int(tier - Tier0), Corner: hz}
			n := float64(highpassStreams)
			var s, s2 float64
			for _, v := range d {
				e.Off += v[0] / n
				e.On += v[1] / n
				e.RRC += v[2] / n
				x := v[1] - v[2]
				s += x
				s2 += x * x
			}
			e.DiffMean = s / n
			e.DiffSE = math.Sqrt((s2/n - e.DiffMean*e.DiffMean) / (n - 1))
			out = append(out, e)
		}
	}
	return out
}

func renderHighpass(h highpassData) string {
	var b strings.Builder
	b.WriteString(highpassBegin + "\n\n")
	b.WriteString("`AudioTxConfig.InputHighpassHz` states the AC coupling of the modulator input of the radio. Tier 1 and Tier 2 then add the low frequencies that the high pass takes away, held to the room of the converter above the peak at the measured level. `TestHighpass` measures the limits at 16 bits with the stated high pass in the radio and the correction on, against the same points with neither, and the eye at 48 kHz through a radio with that high pass, each stream sent by both.\n\n")
	b.WriteString("| Corner | Limits that the stated high pass fails and the same point keeps without it | Least margin over the need, stated (dB) | Without (dB) | Limit | Point | Room (Hz) |\n| ---: | ---: | ---: | ---: | --- | --- | ---: |\n")
	for _, w := range h.Worst {
		n := limNeed(w.Key)
		fmt.Fprintf(&b, "| %d Hz | %d | %+.2f | %+.2f | %s | %s | %.0f |\n", w.Corner, len(w.New), w.Stated-n, w.Base-n, w.Key, w.Point, w.RoomHz)
	}
	fmt.Fprintf(&b, "\n| Tier | Corner | Eye error, not stated | Eye error, stated | Root raised cosine | Stated less root raised cosine, %d streams |\n| ---: | ---: | ---: | ---: | ---: | ---: |\n", highpassStreams)
	for _, e := range h.Eyes {
		fmt.Fprintf(&b, "| %d | %d Hz | %.4f | %.4f | %.4f | %+.4f ± %.4f |\n", e.Tier, e.Corner, e.Off, e.On, e.RRC, e.DiffMean, e.DiffSE)
	}
	b.WriteString("\n`go test ./internal/fsktx -run TestHighpass -update-highpass` in v2 measures them into `v2/internal/fsktx/testdata/highpass.json` and writes this block.\n\n")
	b.WriteString(highpassEnd)
	return b.String()
}

func renderHighpassSheet(h highpassData) string {
	var b strings.Builder
	b.WriteString(hpSheetBegin + "\n")
	room := 0.0
	for _, w := range h.Worst {
		room = max(room, w.RoomHz)
	}
	fmt.Fprintf(&b, "State the AC coupling of the modulator input of the radio, a first-order high pass, in `AudioTxConfig.InputHighpassHz`, from 1 to %d Hz. Tier 1 and Tier 2 then add back the low frequencies that it takes away, within the room of the converter above the peak at the measured level: up to %.0f Hz of deviation at 16 bits, and none at fewer bits, where the statement has no effect.", HighpassMaxHz, room)
	for _, e := range h.Eyes {
		if e.Tier == 1 && e.Corner == 10 {
			fmt.Fprintf(&b, " Through a radio with a 10 Hz high pass, the eye error of Tier 1 at 48 kHz falls from %.3f to %.3f.", e.Off, e.On)
		}
	}
	b.WriteString(" The stated high pass keeps every limit at the measured points; [Input high pass](tx-notes.md#input-high-pass) gives the measurement.\n")
	b.WriteString(hpSheetEnd)
	return b.String()
}

func replaceBlock(t *testing.T, path, begin, end, want string, update bool) {
	t.Helper()
	doc, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	i, j := bytes.Index(doc, []byte(begin)), bytes.Index(doc, []byte(end))
	if i < 0 || j < i {
		t.Fatalf("%s has no %s ... %s block", path, begin, end)
	}
	if update {
		out := append(append(append([]byte{}, doc[:i]...), want...), doc[j+len(end):]...)
		if err := os.WriteFile(path, out, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if got := string(doc[i : j+len(end)]); got != want {
		t.Errorf("the block %s of %s differs from %s; run go test ./internal/fsktx -run TestHighpass -update-highpass in v2", begin, path, highpassFile)
	}
}

func TestHighpass(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat(txNotesFile); errors.Is(err, fs.ErrNotExist) {
		t.Skip("docs/tx-notes.md is not in this tree")
	}
	var h highpassData
	if *updateHighpass {
		h = highpassData{Worst: measureHighpassLimits(t), Eyes: measureHighpassEyes(t)}
		js, err := json.MarshalIndent(h, "", "\t")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(highpassFile, append(js, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		js, err := os.ReadFile(highpassFile)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(js, &h); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range h.Worst {
		for _, f := range w.New {
			t.Errorf("a stated high pass of %d Hz fails %s", w.Corner, f)
		}
	}
	for _, e := range h.Eyes {
		if e.DiffMean > -2*e.DiffSE || e.On >= e.Off {
			t.Errorf("tier %d at %d Hz: the stated eye %.4f does not beat the root raised cosine by two standard errors", e.Tier, e.Corner, e.On)
		}
	}
	if c := highpassCorners[len(highpassCorners)-1]; c != HighpassMaxHz {
		t.Errorf("the corners are measured up to %d Hz, HighpassMaxHz is %d", c, HighpassMaxHz)
	}
	replaceBlock(t, txNotesFile, highpassBegin, highpassEnd, renderHighpass(h), *updateHighpass)
	replaceBlock(t, txDocFile, hpSheetBegin, hpSheetEnd, renderHighpassSheet(h), *updateHighpass)
}
