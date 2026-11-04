package main

import (
	"slices"
	"strconv"
	"strings"
)

const acsBarDB = 60

type acsPoint struct {
	Tier   int     `json:"tier"`
	SideHz float64 `json:"side_hz"`
	DB     float64 `json:"acs_db"`
}

type acsData struct {
	MaxBER float64            `json:"max_ber"`
	Seeds  int                `json:"seeds"`
	StepDB float64            `json:"step_db"`
	CN     map[string]float64 `json:"wanted_cn"`
	Points []acsPoint         `json:"points"`
}

func renderACS(b *strings.Builder, d acsData) {
	if len(d.Points) == 0 {
		return
	}
	var tiers []int
	for _, p := range d.Points {
		if !slices.Contains(tiers, p.Tier) {
			tiers = append(tiers, p.Tier)
		}
	}
	slices.Sort(tiers)
	b.WriteString("\n### Adjacent channel selectivity\n\n")
	b.WriteString(tableHead("Tier", "Wanted C/N (dB)", "Channel below (dB)", "Channel above (dB)"))
	for _, t := range tiers {
		cells := []string{"I/Q tier " + strconv.Itoa(t), f1(d.CN[strconv.Itoa(t)])}
		for _, side := range []float64{-1, 1} {
			cell := "-"
			for _, p := range d.Points {
				if p.Tier == t && p.SideHz*side > 0 {
					cell = acsCell(p.DB)
				}
			}
			cells = append(cells, cell)
		}
		b.WriteString(tableRow(cells...))
	}
	b.WriteString("\n- EN 300 113 V2.2.1 clause 8.6: an A-M3 signal (FM, 400 Hz tone, 1.5 kHz deviation) 12.5 kHz above or below the wanted signal, which sits 3 dB above the sensitivity limit. The value is the highest A-M3 level over the wanted signal, in steps of " +
		strconv.FormatFloat(d.StepDB, 'f', -1, 64) + " dB, at which the bit error rate over " + strconv.Itoa(d.Seeds) + " noise draws stays at or under " +
		strconv.FormatFloat(d.MaxBER, 'g', -1, 64) + ". The standard asks for " + strconv.Itoa(acsBarDB) + " dB.\n")
}

func acsCell(db float64) string {
	if db < acsBarDB {
		return "< " + strconv.Itoa(acsBarDB)
	}
	return strconv.FormatFloat(db, 'f', -1, 64)
}
