package main

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

const (
	extBegin = "<!-- extcmp:begin -->"
	extEnd   = "<!-- extcmp:end -->"
)

type extPoint struct {
	CN       float64           `json:"cn"`
	Seeds    int               `json:"seeds"`
	Payloads map[string]*score `json:"payloads"`
}

type extDecoder struct {
	Name    string     `json:"name"`
	CPUNs   float64    `json:"ns_per_second"`
	WorstNs float64    `json:"worst_ns_per_second"`
	Points  []extPoint `json:"points"`
}

type extPair struct {
	Ours, Ext, Payload string
	CN                 float64
	Sum, SumSq         int64
	N                  int
	OursTot, ExtTot    int
	TruthTot           int
}

func (p *extPair) UnmarshalJSON(b []byte) error {
	var a []json.RawMessage
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	if len(a) != 6 {
		return fmt.Errorf("pair of %d fields", len(a))
	}
	for i, v := range []any{&p.Ours, &p.Ext, &p.Payload, &p.CN, &p.Sum, &p.SumSq} {
		if err := json.Unmarshal(a[i], v); err != nil {
			return err
		}
	}
	return nil
}

type extStream struct {
	Name     string       `json:"name"`
	OffsetHz float64      `json:"offset_hz"`
	Decoders []extDecoder `json:"decoders"`
	Pairs    []extPair    `json:"pairs"`
}

type extData struct {
	Machine  string            `json:"machine"`
	Versions map[string]string `json:"versions"`
	Seeds    int               `json:"seeds"`
	IFTaps   int               `json:"if_taps"`
	Streams  []extStream       `json:"streams"`
}

func extPayloadNames() [][2]string {
	return [][2]string{
		{keyAMBE, "Voice"}, {"ambe_c0c1", "Voice C0 C1"}, {keyRate12, "Rate 1/2"}, {keyRate34, "Rate 3/4"},
		{"rate1", "Rate 1"}, {keyCSBK, "CSBK"}, {"lc", "LC header"}, {"header", "Data header"},
		{"elc", "Embedded LC"}, {"emb", "EMB"}, {"slottype", "Slot type"},
	}
}

func extDecoderName(n string) string {
	switch n {
	case "dsd-fme":
		return "dsd-fme"
	case "mmdvm":
		return "MMDVM firmware"
	case "mmdvm-fixed":
		return "MMDVM firmware, end of burst fixed"
	}
	for _, m := range []string{modeIQLow, modeAudio, modeIQ} {
		if t, ok := strings.CutPrefix(n, m); ok {
			return modeLabel(m) + " tier " + t
		}
	}
	return n
}

func extExternal(n string) bool { return n == "dsd-fme" || strings.HasPrefix(n, "mmdvm") }

func extStreamName(s extStream) string {
	src := "BS outbound, both slots"
	if s.Name == "ms" {
		src = "MS bursts, one slot"
	}
	return src + ", carrier " + strconv.FormatFloat(s.OffsetHz, 'f', 0, 64) + " Hz off"
}

func extCurve(d extDecoder, k string) (x, y []float64, top float64) {
	for _, p := range d.Points {
		top = max(top, cnToEbN0(p.CN))
		if s := p.Payloads[k]; s != nil && s.Truth > 0 {
			x = append(x, cnToEbN0(p.CN))
			y = append(y, float64(s.Exact)/float64(s.Truth))
		}
	}
	return x, y, top
}

func extCell(d extDecoder, k string) string {
	x, y, top := extCurve(d, k)
	if len(x) == 0 {
		return "-"
	}
	c := at90(x, y)
	switch {
	case !c.ok:
		return "> " + f1(top) + " dB"
	case c.below:
		return "< " + f1(c.ebn0) + " dB"
	}
	return f1(c.ebn0) + " dB"
}

type extVerdict int

const (
	extWin extVerdict = iota
	extTie
	extBehind
)

func (p extPair) mean() float64 { return float64(p.Sum) / float64(p.N) }

func (p extPair) se() float64 {
	n := float64(p.N)
	v := (float64(p.SumSq) - float64(p.Sum)*float64(p.Sum)/n) / max(n-1, 1)
	return math.Sqrt(max(v, 0) / n)
}

func (p extPair) verdict() extVerdict {
	switch m, e := p.mean(), 2*p.se(); {
	case m > e:
		return extWin
	case m < -e:
		return extBehind
	}
	return extTie
}

func renderExt(e extData) []byte {
	var b strings.Builder
	b.WriteString("\n")
	var exts []string
	for _, s := range e.Streams {
		for _, d := range s.Decoders {
			if extExternal(d.Name) && !slices.Contains(exts, d.Name) {
				exts = append(exts, d.Name)
			}
		}
	}
	b.WriteString("The tiers and other receivers decode the same noisy signals. Each cell is the Eb/N0 at which the receiver recovers 90% of the frames exactly. Lower is better.\n\n")
	b.WriteString("- Receivers: ")
	var vs []string
	for _, n := range exts {
		v := e.Versions[strings.TrimSuffix(n, "-fixed")]
		vs = append(vs, extDecoderName(n)+" (commit "+v+")")
	}
	b.WriteString(strings.Join(vs, ", ") + ".\n")
	b.WriteString("- Each point decodes " + strconv.Itoa(e.Seeds) + " noise seeds at each C/N from 0 to 20 dB in steps of 1 dB. Every receiver gets the same seeds.\n")
	b.WriteString("- The FM audio comes from the discriminator model of the sweep with a " + strconv.Itoa(e.IFTaps) + "-tap IF filter: a Kaiser-windowed sinc with its -6 dB point at 6 kHz and at least 84 dB of attenuation from 7.5 kHz, as the IF of a 12.5 kHz radio. The audio tiers get it at 9.6 kHz, dsd-fme at 48 kHz and the MMDVM firmware at 24 kHz, with the DC offset of the carrier and no other.\n")
	b.WriteString("- dsd-fme decodes from `-fs -i - -o null -Z`. Its voice frames come from its AMBE+2 printout and its data blocks from one print line added after its FEC, which changes no decision. A frame counts when it equals the next frame of the truth in order. The MMDVM firmware is its DMO receiver (`DMRDMORX.cpp`) on the host with the 0.2 RRC filter of `IO.cpp`, and its burst bits go through the FEC of dmrgo. The tiers and the MMDVM firmware count a frame only at its own slot.\n")
	b.WriteString("- The MMDVM DMO receiver takes each burst one sample before its last symbol arrives. `DMRDMORX.cpp` sets the end at `m_endPtr = m_dataPtr + DMR_SLOT_TYPE_LENGTH_SAMPLES / 2U + DMR_INFO_LENGTH_SAMPLES / 2U - 1U` (lines 327 and 358), 269 samples after the last SYNC symbol, but the last symbol of the burst is 270 samples after it, and `samplesToBits` reads it from `m_endPtr + DMO_BUFFER_LENGTH_SAMPLES - DMR_FRAME_LENGTH_SAMPLES + DMR_RADIO_SYMBOL_LENGTH + 1U` (line 126) on, which reaches the sample of 60 ms before. So the last two bits of every burst are random, and its voice and rate 3/4 cells do not reach 90% at any C/N. The second MMDVM row drops the `- 1U` and the `+ 1U`. Only MS bursts carry the SYNC patterns that the MMDVM DMO receiver looks for.\n\n")
	for _, s := range e.Streams {
		b.WriteString("#### " + extStreamName(s) + "\n\n")
		b.WriteString("| Receiver |")
		for _, p := range extPayloadNames() {
			b.WriteString(" " + p[1] + " |")
		}
		b.WriteString(" CPU at C/N 6 dB |\n|---|")
		for range extPayloadNames() {
			b.WriteString("---|")
		}
		b.WriteString("---|\n")
		for _, d := range s.Decoders {
			b.WriteString("| " + extDecoderName(d.Name) + " |")
			for _, p := range extPayloadNames() {
				b.WriteString(" " + extCell(d, p[0]) + " |")
			}
			cpu := "-"
			if extExternal(d.Name) && d.CPUNs > 0 {
				cpu = msCell(d.CPUNs)
			}
			b.WriteString(" " + cpu + " |\n")
		}
		b.WriteString("\n")
	}
	renderExtCells(&b, e)
	return []byte(b.String())
}

func renderExtCells(b *strings.Builder, e extData) {
	type key struct{ ours, ext string }
	count := map[key]*[3]int{}
	behind := map[key][]string{}
	var ours, exts []string
	for _, s := range e.Streams {
		for _, p := range s.Pairs {
			p.N = e.Seeds
			p.OursTot, p.TruthTot = extTotal(s, p.Ours, p.Payload, p.CN)
			p.ExtTot, _ = extTotal(s, p.Ext, p.Payload, p.CN)
			k := key{p.Ours, p.Ext}
			if !slices.Contains(ours, p.Ours) {
				ours = append(ours, p.Ours)
			}
			if !slices.Contains(exts, p.Ext) {
				exts = append(exts, p.Ext)
			}
			if count[k] == nil {
				count[k] = &[3]int{}
			}
			v := p.verdict()
			count[k][v]++
			if v == extBehind {
				behind[k] = append(behind[k], fmt.Sprintf("%s, %s, C/N %s dB: %d against %d of %d, %+.3f ± %.3f a seed", extStreamName(s), payloadLabel(p.Payload), f1(p.CN), p.OursTot, p.ExtTot, p.TruthTot, p.mean(), p.se()))
			}
		}
	}
	b.WriteString("#### Cells won and lost\n\n")
	b.WriteString("A cell is one payload of one stream at one C/N. On each seed the tier and the other receiver decode the same signal, and the difference of their exact frames is taken. The tier wins the cell when the mean difference over the seeds is more than two standard errors above 0, and is behind when it is more than two standard errors below 0. The cells between are ties, which count as lost. Cells where both recover every frame, or both none, are left out.\n\n")
	b.WriteString("| Tier |")
	for _, x := range exts {
		b.WriteString(" " + extDecoderName(x) + ": won, tied, behind |")
	}
	b.WriteString("\n|---|")
	for range exts {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, o := range ours {
		b.WriteString("| " + extDecoderName(o) + " |")
		for _, x := range exts {
			c := count[key{o, x}]
			if c == nil {
				c = &[3]int{}
			}
			b.WriteString(" " + strconv.Itoa(c[extWin]) + ", " + strconv.Itoa(c[extTie]) + ", " + strconv.Itoa(c[extBehind]) + " |")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n<details><summary>Cells where a tier is behind</summary>\n\n")
	b.WriteString("Frames recovered by the tier against the other receiver, and the mean difference a seed with its standard error.\n\n")
	for _, o := range ours {
		for _, x := range exts {
			l := behind[key{o, x}]
			if len(l) == 0 {
				continue
			}
			b.WriteString("- " + extDecoderName(o) + " against " + extDecoderName(x) + ":\n")
			for _, s := range l {
				b.WriteString("  - " + s + "\n")
			}
		}
	}
	b.WriteString("\n</details>\n\n")
}

func extTotal(s extStream, dec, k string, cn float64) (exact, truth int) {
	for _, d := range s.Decoders {
		if d.Name != dec {
			continue
		}
		for _, p := range d.Points {
			if sc := p.Payloads[k]; p.CN == cn && sc != nil {
				return sc.Exact, sc.Truth
			}
		}
	}
	return 0, 0
}

func payloadLabel(k string) string {
	for _, p := range extPayloadNames() {
		if p[0] == k {
			return p[1]
		}
	}
	return k
}
