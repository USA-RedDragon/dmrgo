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

var updateTxDoc = flag.Bool("update-txdoc", false, "measure the margins into testdata/txmargins.json and rewrite the tables of docs/tx.md")

const (
	txDocFile     = "../../../docs/tx.md"
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
	keys     func(pw float64) ([]string, error)
}

func (r txLimitRow) at(t testing.TB, pw float64) []string {
	t.Helper()
	k, err := r.keys(pw)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func txLimitRows() []txLimitRow {
	fixed := func(name string, keys ...string) txLimitRow {
		return txLimitRow{name: name, keys: func(float64) ([]string, error) { return keys, nil }}
	}
	power := func(name string, pre ...string) txLimitRow {
		return txLimitRow{name: name, perPower: true, keys: func(pw float64) ([]string, error) {
			if err := checkPower(pw); err != nil {
				return nil, err
			}
			k := make([]string, 0, len(pre))
			for _, p := range pre {
				k = append(k, powKey(p, pw, "_db"))
			}
			return k, nil
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
		power("Spurious emissions, dB (EN 300 113 7.5, EN 303 405 7.5)", "en_spur_"),
		power("Emission Mask D, dB (47 CFR 90.210(d))", "fcc_maskd_"),
		power("Emission Mask D far out, dB (47 CFR 90.210(d)(3))", "fcc_maskd_far_"),
		power("Emission Mask H, dB (47 CFR 90.210(h))", "fcc_maskh_"),
		power("Emission Mask H far out, dB (47 CFR 90.210(h)(5))", "fcc_maskh_far_"),
		power("800 MHz EA mask, dB (47 CFR 90.691(a))", "fcc_ea_"),
		power("800 MHz EA mask far out, dB (47 CFR 90.691(a)(2))", "fcc_ea_far_"),
		power("Amateur spurious emissions, dB (47 CFR 97.307(e))", "am_us97e_"),
		power("Amateur spurious emissions, dB (ERC/REC 74-01)", "am_erc_"),
		fixed("Adjacent channel power, dB (EN 303 405 7.4)", "pmr_acp_db"),
		fixed("Alternate channel power, dB (EN 303 405 7.4)", "pmr_alt_db"),
		fixed("Alternate channel power, dB (China)", "cn_alt_db"),
		fixed("Occupied bandwidth, kHz under 8.5 kHz (China)", "obw_cn_kr_khz"),
		power("Spurious emissions, dB (China, positive peak)", "cn_spur_"),
		power("Amateur spurious emissions, dB (China)", "am_cn_"),
		fixed("Adjacent channel power, dB (Japan)", "jp_acp_db"),
		fixed("Occupied bandwidth, kHz under 11.5 kHz (Japan)", "obw_jp_khz"),
		power("Spurious emissions, dB (Japan)", "jp_spur_"),
		power("Amateur spurious emissions, dB (Japan)", "jp_am_spur_"),
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
	if c.pe {
		return fmt.Sprintf("Audio %d-bit, pre-emphasis", c.bits)
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
	{"World, license-free", limWorld, limLicenseFree},
	{"US, land mobile", limUS, limLandMobile},
	{"US, amateur", limUS, limAmateur},
	{"CEPT, land mobile", limCEPT, limLandMobile},
	{"CEPT, amateur", limCEPT, limAmateur},
	{"CEPT, PMR446", limCEPT, limLicenseFree},
	{"China, land mobile", limChina, limLandMobile},
	{"China, amateur", limChina, limAmateur},
	{"China, 409 MHz license-free", limChina, limLicenseFree},
	{"Japan, land mobile", limJapan, limLandMobile},
	{"Japan, amateur", limJapan, limAmateur},
}

func renderTxDoc(t testing.TB, margins map[string]map[string]float64) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(txDocBegin + "\n\n")
	b.WriteString("## Least sample rate\n\n")
	b.WriteString("Each cell gives the least sample rate in kHz for the power breakpoints 27, 30, 40, 44, 47, 50, 54, 57, 60 and 62 dBm, in that order. A power takes the next breakpoint at or above it. A dash is a setup that no rate of the grid keeps. `TestMinRatesGenerated` measures the grid, and these tables come from its output, `minrate_gen.go` in `v2/internal/fsktx`. Tier 0 makes audio at 24 and 48 kHz and I/Q at 96 kHz only, and audio with pre-emphasis at 48 kHz only.\n\n")
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
			for _, lp := range lps {
				name := txClassName(c)
				if lp > 0 {
					name += fmt.Sprintf(", %d Hz low pass", lp)
				}
				b.WriteString("| " + name + " |")
				for _, r := range txRegimes {
					cells := make([]string, len(limPowerClasses))
					for p := range limPowerClasses {
						hz := c.setup(r.region, r.service, p, lp).MinRate()
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
	writeMethods(&b)
	writeUnmeasured(&b)
	b.WriteString("## Measured margins\n\n")
	b.WriteString("Each cell is the margin of the signal to the limit, the worst over the measured points. A limit that changes with power has one value for each power breakpoint, 27, 30, 40, 44, 47, 50, 54, 57, 60 and 62 dBm, in that order. A positive value is inside the limit. A setup is accepted only with at least 10 dB on each spurious domain limit (the spurious emissions rows, the far out rows of the FCC masks and the amateur rows), where the amplifier shares the limit, at least 3 dB on each other limit in dB, except -0.01 dB on the power slope, and at least 0 on the others. China gives its spurious limit in positive peak: its row takes the mean power in 100 kHz plus 11.4 dB, the peak of Gaussian noise over a million samples. Audio is at 48 kHz, I/Q at 96 kHz for Tier 0 and at 2.4 MHz for Tier 1 and Tier 2, with no input low pass. A dash is a limit that does not apply to the input. `go test ./internal/fsktx -run TestTxDoc -update-txdoc` in v2 measures them into `v2/internal/fsktx/testdata/txmargins.json` and writes these tables.\n\n")
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
					b.WriteString(" " + txWorst(g, row.at(t, 0)) + " |")
					continue
				}
				cells := make([]string, len(limPowers))
				for i, pw := range limPowers {
					cells[i] = txWorst(g, row.at(t, pw))
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
			for _, k := range row.at(t, pw) {
				shown[k] = true
			}
		}
	}
	for r := range limRegions {
		for s := range limServices {
			for _, pw := range limPowers {
				keys, err := regimeKeys(r, s, pw)
				if err != nil {
					t.Fatal(err)
				}
				for _, k := range keys {
					if !shown[k] {
						t.Errorf("limit %s of %s %s %g dBm has no row in docs/tx.md", k, limRegionNames[r], limServiceNames[s], pw)
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
		t.Skip("docs/tx.md is not in this tree")
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
	doc = txSheetDoc(t, doc, margins)
	i, j := bytes.Index(doc, []byte(txDocBegin)), bytes.Index(doc, []byte(txDocEnd))
	if i < 0 || j < i {
		t.Fatalf("%s has no %s ... %s block", txDocFile, txDocBegin, txDocEnd)
	}
	want := renderTxDoc(t, margins)
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

func writeMethods(b *strings.Builder) {
	b.WriteString("## Methods of the limits\n\n")
	var bands []string
	for i, x := range enSpurBands {
		from := khz(int(x.from)) + " kHz"
		switch {
		case i == 0:
			bands = append(bands, fmt.Sprintf("in %s kHz from %s from the carrier (250%% of the 12.5 kHz channel separation)", khz(int(x.rbw)), from))
		case math.IsInf(x.to, 1):
			bands = append(bands, fmt.Sprintf("in %s kHz beyond %s", khz(int(x.rbw)), from))
		default:
			bands = append(bands, fmt.Sprintf("in %s kHz from %s", khz(int(x.rbw)), from))
		}
	}
	fmt.Fprintf(b, "- Spurious emissions of EN 300 113 and EN 303 405: the power %s, the reference bandwidths of EN 300 113 V2.2.1 tables 6a and 6b and EN 303 405 V1.1.1 tables 5b and 5c. The other spurious domain rows take the mean power in 100 kHz.\n", strings.Join(bands, ", "))
	fmt.Fprintf(b, "- Emission Mask H and the EA mask: 47 CFR 90.210 table note 6 puts digital emissions at 806-809/851-854 MHz under Mask H, and table note 3 puts 809-824/854-869 MHz under 90.691. Neither section states a method, so both take that of Mask D in 90.210(d)(4): a reference in 30 kHz, and each emission in a 100 Hz resolution bandwidth with peak hold, from %s kHz for Mask H and %s kHz for the EA mask out to %s kHz, then the mean power in 100 kHz, after 90.210(o). The 90.691(a)(1) limit holds out to %d kHz from the carrier: 37.5 kHz past the edge of the block, which is 12.5 kHz from the carrier of a 25 kHz channel, the stricter case.\n", khz(maskhFromHz), khz(eaFromHz), khz(maskNearHz), eaEdgeKHz)
	us := Setup{Tier: Tier0, Bits: 16, Region: LimitsUS, Service: LimitsLandMobile, Power: PowerClass(30)}
	lp := us
	lp.InputLowpassHz = lowpass8k
	fmt.Fprintf(b, "  With Mask H and the EA mask, US land mobile audio at 30 dBm needs an input low pass for 24 kHz: Tier 0 at 16 bits takes %s kHz with no `InputLowpassHz` and %s kHz with %d Hz.\n", khz(us.MinRate()), khz(lp.MinRate()), lowpass8k)
	b.WriteString("\n")
}

func writeUnmeasured(b *strings.Builder) {
	pw := powerBreakpoints()
	b.WriteString("## Setups without a measurement\n\n")
	b.WriteString("No measurement covers these setups, so the transmitter refuses them:\n\n")
	fmt.Fprintf(b, "- Audio over %s kHz and I/Q over %s MHz, the tops of the measured grids.\n", khz(audioRateTop), strconv.FormatFloat(float64(iqRateTop)/1e6, 'f', -1, 64))
	fmt.Fprintf(b, "- A `Power` over %d dBm, the top breakpoint.\n", pw[len(pw)-1])
	b.WriteString("- `AudioTxConfig.PreEmphasis` at Tier 1 and Tier 2, and at Tier 0 at a rate other than 48 kHz. The grid measures pre-emphasis for Tier 0 at 48 kHz only.\n")
	b.WriteString("- An `AudioTxConfig.Level` under the level that the grid measures for `Bits`.\n")
	b.WriteString("- Tier 3, which has no modulator.\n\n")
	join := func(v []int) string {
		s := make([]string, len(v))
		for i, x := range v {
			s[i] = strconv.Itoa(x)
		}
		return strings.Join(s[:len(s)-1], ", ") + " and " + s[len(s)-1]
	}
	fmt.Fprintf(b, "A setup between measured ones takes the nearest measured setup that is not looser: a power takes the next breakpoint up, a depth of `Bits` between %s takes the next lower depth, and an input low pass between the cutoffs %s Hz takes the next looser cutoff, or none.\n\n", join(audioBitClasses), join(inputLowpassGrid[1:]))
}
