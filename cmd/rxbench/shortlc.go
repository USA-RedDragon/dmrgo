package main

import (
	"slices"
	"strconv"
	"strings"
)

const (
	shortLCBegin = "<!-- shortlc:begin -->"
	shortLCEnd   = "<!-- shortlc:end -->"
)

type shortLCTier struct {
	Mode         string    `json:"mode"`
	Tier         int       `json:"tier"`
	CN           []float64 `json:"cn"`
	Truth        []int     `json:"truth"`
	Hard         []int     `json:"hard"`
	LC           []int     `json:"lc"`
	HardWrong    int       `json:"hard_wrong"`
	LCWrong      int       `json:"lc_wrong"`
	Decoded      int       `json:"decoded"`
	NoiseSeconds float64   `json:"noise_seconds"`
	NoiseHard    int       `json:"noise_hard"`
	NoiseLC      int       `json:"noise_lc"`
}

type shortLCCapture struct {
	Sigma    float64 `json:"sigma"`
	Tier     int     `json:"tier"`
	Bursts   int     `json:"bursts"`
	Hard     int     `json:"hard"`
	LC       int     `json:"lc"`
	Soft     int     `json:"soft"`
	Distinct int     `json:"distinct"`
}

type shortLCData struct {
	Seeds    int              `json:"seeds"`
	Capture  string           `json:"capture"`
	Tiers    []shortLCTier    `json:"tiers"`
	Captures []shortLCCapture `json:"captures"`
}

func (t shortLCTier) at90(n []int) crossing {
	x := make([]float64, len(t.CN))
	y := make([]float64, len(t.CN))
	for i, cn := range t.CN {
		x[i] = cnToEbN0(cn)
		if i < len(t.Truth) && i < len(n) && t.Truth[i] > 0 {
			y[i] = float64(n[i]) / float64(t.Truth[i])
		}
	}
	return at90(x, y)
}

func sum(v []int) int {
	n := 0
	for _, x := range v {
		n += x
	}
	return n
}

func perHour(n int, s float64) string {
	if s <= 0 {
		return "-"
	}
	return f1(float64(n) / s * 3600)
}

func renderShortLC(d shortLCData) []byte {
	var b strings.Builder
	b.WriteString("\n| Tier | Input | Hard decode | `OnShortLC` | Wrong, hard decode | Wrong, `OnShortLC` | On noise, hard decode | On noise, `OnShortLC` |\n")
	b.WriteString("|------|-------|-------------|-------------|--------------------|--------------------|-----------------------|-----------------------|\n")
	for _, t := range d.Tiers {
		bt := tier{Mode: t.Mode, Tier: t.Tier}
		hard := sum(t.Hard) + t.HardWrong
		b.WriteString("| " + strings.Join([]string{shortName(bt), inputName(bt), dbCell(t.at90(t.Hard)), dbCell(t.at90(t.LC)),
			grouped(int64(t.HardWrong)) + " of " + grouped(int64(hard)), grouped(int64(t.LCWrong)) + " of " + grouped(int64(t.Decoded)),
			perHour(t.NoiseHard, t.NoiseSeconds) + " per hour", perHour(t.NoiseLC, t.NoiseSeconds) + " per hour"}, " | ") + " |\n")
	}
	b.WriteString("\n- Hard decode and `OnShortLC` are the Eb/N0 at which 90% of the Short LCs of the sweep stream are right, from " + strconv.Itoa(d.Seeds) + " streams per point. Hard decode is `layer2.ShortLCAssembler` on the `Decoded` CACH of the bursts.\n")
	b.WriteString("- Wrong counts the Short LCs that pass the CRC-8 with other bits than the stream sent, of all that pass. On noise gives the Short LCs that the receivers report on white Gaussian noise alone.\n")
	b.WriteString("\nThe Motorola XPR 8400 idle capture (`" + d.Capture + "`) with noise added to its CU8 codes. Short LCs, hard decode / `OnShortLC` (of them soft):\n\n")
	var tiers []int
	var sigmas []float64
	for _, c := range d.Captures {
		if !slices.Contains(tiers, c.Tier) {
			tiers = append(tiers, c.Tier)
		}
		if !slices.Contains(sigmas, c.Sigma) {
			sigmas = append(sigmas, c.Sigma)
		}
	}
	b.WriteString("| Noise sigma |")
	sep := "|-------------|"
	for _, t := range tiers {
		b.WriteString(" I/Q tier " + strconv.Itoa(t) + " |")
		sep += "------|"
	}
	b.WriteString("\n" + sep + "\n")
	for _, s := range sigmas {
		b.WriteString("| " + strconv.FormatFloat(s, 'f', -1, 64) + " |")
		for _, t := range tiers {
			for _, c := range d.Captures {
				if c.Sigma == s && c.Tier == t {
					b.WriteString(" " + strconv.Itoa(c.Hard) + " / " + strconv.Itoa(c.LC) + " (" + strconv.Itoa(c.Soft) + ") |")
				}
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	return []byte(b.String())
}
