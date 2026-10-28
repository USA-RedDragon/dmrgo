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
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
)

var updateTxDoc = flag.Bool("update-txdoc", false, "measure the margins into testdata/txmargins.json and rewrite the tables of docs/tx/README.md")

const (
	txDocFile     = "../../../docs/tx/README.md"
	txMarginsFile = "testdata/txmargins.json"
	txDocBegin    = "<!-- txdoc:begin -->"
	txDocEnd      = "<!-- txdoc:end -->"
)

func txDocRate(c limClass) int {
	switch {
	case !c.iq:
		return 48000
	case c.tier == Tier0:
		return 96000
	}
	return 2400000
}

func measureTxMargins(t *testing.T) map[string]map[string]float64 {
	out := map[string]map[string]float64{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, testParallel()))
	for _, c := range limClasses() {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			g := map[string]float64{}
			for _, p := range c.points(txDocRate(c), 0) {
				if _, err := p.u.mod(); err != nil {
					return
				}
				worstOf(g, p.measure(t, 1))
			}
			r := make(map[string]float64, len(g))
			for k, v := range g {
				r[k] = math.Round(v*1000) / 1000
			}
			mu.Lock()
			out[c.String()] = r
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}

type txLimitRow struct {
	name     string
	perPower bool
	keys     func(pw float64) []string
}

func txLimitRows() []txLimitRow {
	fixed := func(name string, keys ...string) txLimitRow {
		return txLimitRow{name: name, keys: func(float64) []string { return keys }}
	}
	power := func(name string, pre ...string) txLimitRow {
		return txLimitRow{name: name, perPower: true, keys: func(pw float64) []string {
			k := make([]string, 0, len(pre))
			for _, p := range pre {
				k = append(k, powKey(p, pw, "w_db"))
			}
			return k
		}}
	}
	return []txLimitRow{
		fixed("Deviation error, % under the 10% limit (TS 102 361-1 10.2.2.3)", "ts_dev_pct"),
		fixed("Adjacent channel power, dB (EN 300 113 7.4)", "en_acp_annexc_nominal_db", "en_acp_annexc_wide_db", "en_acp_bins_db"),
		fixed("Alternate channel power, dB (EN 300 113 7.4)", "en_alt_annexc_nominal_db", "en_alt_annexc_wide_db", "en_alt_bins_db"),
		fixed("Transient adjacent channel power, dB (EN 300 113 7.9)", "en_tacp_annexc_unmod_db", "en_tacp_sa_unmod_db"),
		fixed("Transient alternate channel power, dB (China)", "en_talt_annexc_unmod_db", "en_talt_sa_unmod_db"),
		fixed("Attack time t_p, ms over 0.1 ms (EN 300 113 7.9)", "en_tp_unmod_ms"),
		fixed("Release time t_d, ms over 0.1 ms (EN 300 113 7.9)", "en_td_unmod_ms"),
		fixed("Transient power slope, dB (EN 300 113 7.9)", "en_slope_unmod_db"),
		fixed("Transient frequency error, kHz (EN 300 113 7.9)", "en_df_unmod_khz"),
		power("Spurious emissions, dB (EN 300 113 7.5)", "en_spur_"),
		power("Emission Mask D, dB (47 CFR 90.210(d))", "fcc_maskd_", "fcc_maskd_far_"),
		power("Amateur spurious emissions, dB (47 CFR 97.307(e))", "am_us97e_"),
		power("Amateur spurious emissions, dB (ERC/REC 74-01)", "am_erc_"),
		fixed("Alternate channel power, dB (China)", "cn_alt_db"),
		fixed("Occupied bandwidth, kHz under 8.5 kHz (China)", "obw_cn_kr_khz"),
		fixed("Adjacent channel power, dB (Japan)", "jp_acp_db"),
		fixed("Occupied bandwidth, kHz under 11.5 kHz (Japan)", "obw_jp_khz"),
	}
}

func txWorst(g map[string]float64, keys []string) string {
	worst, ok := math.Inf(1), false
	for _, k := range keys {
		if v, has := g[k]; has {
			worst, ok = min(worst, v), true
		}
	}
	if !ok {
		return "-"
	}
	return fmt.Sprintf("%+.1f", worst)
}

func txClassName(c limClass) string {
	if c.iq {
		return [...]string{"", "I/Q 16-bit", "I/Q 8-bit"}[c.format]
	}
	return fmt.Sprintf("Audio %d-bit", c.bits)
}

func txTierClasses(tier Tier) []limClass {
	var out []limClass
	for _, c := range limClasses() {
		if c.tier == tier {
			out = append(out, c)
		}
	}
	return out
}

func khz(hz int) string {
	return strconv.FormatFloat(float64(hz)/1000, 'f', -1, 64)
}

type txRegime struct {
	name            string
	region, service int
}

var txRegimes = []txRegime{
	{"World, land mobile", limWorld, limLandMobile},
	{"World, amateur", limWorld, limAmateur},
	{"US, land mobile", limUS, limLandMobile},
	{"US, amateur", limUS, limAmateur},
	{"CEPT, land mobile", limCEPT, limLandMobile},
	{"CEPT, amateur", limCEPT, limAmateur},
	{"China, land mobile", limChina, limLandMobile},
	{"Japan, land mobile", limJapan, limLandMobile},
}

func renderTxDoc(margins map[string]map[string]float64) string {
	var b strings.Builder
	b.WriteString(txDocBegin + "\n\n")
	b.WriteString("## Least sample rate\n\n")
	b.WriteString("Each cell gives the least sample rate in kHz for the power classes 1 W, 10 W, 100 W and 1000 W, in that order. A dash is a setup that no rate of the grid keeps. `TestMinRatesGenerated` measures the grid, and these tables come from its output, `minrate_gen.go` in `v2/internal/fsktx`. Tier 0 makes audio at 24 and 48 kHz and I/Q at 96 kHz only.\n\n")
	head := "| Input |"
	sep := "| --- |"
	for _, r := range txRegimes {
		head += " " + r.name + " |"
		sep += " --- |"
	}
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		fmt.Fprintf(&b, "### Tier %d\n\n%s\n%s\n", tier-Tier0, head, sep)
		for _, c := range txTierClasses(tier) {
			lps := c.lowpasses()
			for li, lp := range lps {
				name := txClassName(c)
				if lp > 0 {
					name += fmt.Sprintf(", %d Hz low pass", lp)
				}
				b.WriteString("| " + name + " |")
				for _, r := range txRegimes {
					cells := make([]string, len(limPowerClasses))
					for p := range limPowerClasses {
						var hz int
						if c.iq {
							hz = int(minRateIQ[tier-Tier0][c.class][r.region][r.service][p])
						} else {
							hz = int(minRateAudio[tier-Tier0][c.class][li][r.region][r.service][p]) * minRateUnit
						}
						cells[p] = "-"
						if hz > 0 {
							cells[p] = khz(hz)
						}
					}
					b.WriteString(" " + strings.Join(cells, " / ") + " |")
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("## Measured margins\n\n")
	b.WriteString("Each cell is the margin of the signal to the limit, the worst over the measured points. A limit that changes with power has one value for each power class, 1 W, 10 W, 100 W and 1000 W, in that order. A positive value is inside the limit. A setup is accepted only with at least 3 dB on each limit in dB, except -0.01 dB on the power slope, and at least 0 on the others. Audio is at 48 kHz, I/Q at 96 kHz for Tier 0 and at 2.4 MHz for Tier 1 and Tier 2, with no input low pass. A dash is a limit that does not apply to the input. `go test ./internal/fsktx -run TestTxDoc -update-txdoc` in v2 measures them into `v2/internal/fsktx/testdata/txmargins.json` and writes these tables.\n\n")
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		cs := txTierClasses(tier)
		fmt.Fprintf(&b, "### Tier %d margins\n\n| Limit |", tier-Tier0)
		for _, c := range cs {
			b.WriteString(" " + txClassName(c) + " |")
		}
		b.WriteString("\n| --- |" + strings.Repeat(" ---: |", len(cs)) + "\n")
		for _, row := range txLimitRows() {
			b.WriteString("| " + row.name + " |")
			for _, c := range cs {
				g := margins[c.String()]
				if !row.perPower {
					b.WriteString(" " + txWorst(g, row.keys(0)) + " |")
					continue
				}
				cells := make([]string, len(limPowers))
				for i, pw := range limPowers {
					cells[i] = txWorst(g, row.keys(pw))
				}
				all := strings.Join(cells, " / ")
				if strings.Trim(all, "-/ ") == "" {
					all = "-"
				}
				b.WriteString(" " + all + " |")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString(txDocEnd)
	return b.String()
}

func TestTxDocRowsCoverLimits(t *testing.T) {
	t.Parallel()
	shown := map[string]bool{}
	for _, row := range txLimitRows() {
		for _, pw := range limPowers {
			for _, k := range row.keys(pw) {
				shown[k] = true
			}
		}
	}
	for r := range limRegions {
		for s := range limServices {
			for _, pw := range limPowers {
				for _, k := range regimeKeys(r, s, pw) {
					if !shown[k] {
						t.Errorf("limit %s of %s %s %g W has no row in docs/tx/README.md", k, limRegionNames[r], limServiceNames[s], pw)
					}
				}
			}
		}
	}
}

func TestTxDoc(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(txDocFile)
	if errors.Is(err, fs.ErrNotExist) && !*updateTxDoc {
		t.Skip("docs/tx/README.md is not in this tree")
	}
	if err != nil {
		t.Fatal(err)
	}
	var margins map[string]map[string]float64
	if *updateTxDoc {
		margins = measureTxMargins(t)
		js, err := json.MarshalIndent(margins, "", "\t")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(txMarginsFile, append(js, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	} else {
		js, err := os.ReadFile(txMarginsFile)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(js, &margins); err != nil {
			t.Fatal(err)
		}
		if longRun() {
			for cls, g := range measureTxMargins(t) {
				for k, v := range g {
					if w, ok := margins[cls][k]; !ok || math.Abs(w-v) > 0.01 {
						t.Errorf("%s %s: measured %.3f, %s has %.3f; run go test ./internal/fsktx -run TestTxDoc -update-txdoc", cls, k, v, txMarginsFile, w)
					}
				}
			}
		}
	}
	i, j := bytes.Index(doc, []byte(txDocBegin)), bytes.Index(doc, []byte(txDocEnd))
	if i < 0 || j < i {
		t.Fatalf("%s has no %s ... %s block", txDocFile, txDocBegin, txDocEnd)
	}
	want := renderTxDoc(margins)
	if *updateTxDoc {
		out := append(append(append([]byte{}, doc[:i]...), want...), doc[j+len(txDocEnd):]...)
		if err := os.WriteFile(txDocFile, out, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if got := string(doc[i : j+len(txDocEnd)]); got != want {
		t.Errorf("the tables of %s differ from minrate_gen.go and %s; run go test ./internal/fsktx -run TestTxDoc -update-txdoc in v2", txDocFile, txMarginsFile)
	}
}
