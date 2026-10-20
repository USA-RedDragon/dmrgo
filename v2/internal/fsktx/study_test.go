//go:build !tinygo

package fsktx

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

type filterStage struct {
	in shaper
	q  biquad
}

func (f *filterStage) appendSymbol(dst []int16, s int8) []int16 {
	n := len(dst)
	dst = f.in.appendSymbol(dst, s)
	for i := n; i < len(dst); i++ {
		dst[i] = sat16(f.q.step(float64(dst[i])))
	}
	return dst
}

// ETSI EN 300 113 clause 7.9.3.1
type rampResult struct {
	Shape     string  `json:"shape"`
	Ms        float64 `json:"ms"`
	Transient float64 `json:"transient_dbc"`
	Modulated float64 `json:"modulated_dbc"`
}

type radioResult struct {
	Name      string    `json:"name"`
	FSK       fskResult `json:"fsk"`
	Peak      int       `json:"peak"`
	VoicePen  float64   `json:"voice_penalty_db"`
	DataPen   float64   `json:"data_penalty_db"`
	DiscVoice float64   `json:"disc_voice_penalty_db"`
	DiscData  float64   `json:"disc_data_penalty_db"`
}

func TestTxStudy(t *testing.T) {
	t.Parallel()
	dir := os.Getenv("DMRGO_TXSTUDY")
	if dir == "" {
		t.Skip("set DMRGO_TXSTUDY to an output directory")
	}
	ts := newTestStream(t, 6, 40)
	syms := make([]int8, 0, 400)
	syms = append(syms, make([]int8, 30)...)
	syms = append(syms, ts.syms[ts.lead+12+144:ts.lead+12+144+132]...)
	syms = append(syms, make([]int8, 30)...)
	freq := refDeviation(syms)
	sps := dacRate / SymbolRate
	first := (30+24)*sps - sps/2
	last := (30+24+132)*sps - sps/2
	var ramps []rampResult
	for _, shape := range []int{rampStep, rampLinear, rampCosine} {
		for _, ms := range []float64{0.1, 0.25, 0.5, 1.0, 1.25} {
			if shape == rampStep && ms > 0.1 {
				continue
			}
			n := int(ms * 1e-3 * dacRate)
			if shape == rampStep {
				n, ms = 0, 0
			}
			e := rampEnvelope(len(freq), first, last, n, shape)
			x := fm(freq)
			cw := make([]complex128, len(x))
			for i := range x {
				x[i] *= complex(e[i], 0)
				cw[i] = complex(e[i], 0)
			}
			r := rampResult{Shape: map[int]string{rampStep: "step", rampLinear: "linear", rampCosine: "cosine"}[shape], Ms: ms, Transient: transient(cw, dacRate, 1), Modulated: transient(x, dacRate, 1)}
			t.Logf("ramp %+v", r)
			ramps = append(ramps, r)
		}
	}
	n := int((2*rxPad + float64(len(ts.syms))/SymbolRate) * rxRate)
	run := newSweepRunner(ts, n)
	ref48 := resample(fm(refDeviation(ts.syms)), dacRate, rxRate)
	refFSK := fsk(ref48, ts.syms)
	ref := sweepResult{}
	ref.Points = run.curve(ref48, int(math.Round(refFSK.Delay)), -1.5, false)
	ref.VoiceCN90, ref.DataCN90 = cn90(ref.Points)
	ref.DiscPoints = run.curve(ref48, int(math.Round(refFSK.Delay)), 3, true)
	ref.DiscVoiceCN90, ref.DiscDataCN90 = cn90(ref.DiscPoints)
	base := spec{Kind: kindAudio, Algo: "poly64", Span: 8, Hz: 48000}
	type path struct {
		name string
		r    radio
		comp func(hz int) biquad
	}
	var paths []path
	for _, ac := range []float64{2, 10, 30} {
		paths = append(paths, path{name: "ac" + ftoa(ac), r: radio{acHz: ac}})
		paths = append(paths, path{name: "ac" + ftoa(ac) + "-comp", r: radio{acHz: ac, gain: 2}, comp: func(hz int) biquad { return half(hfUnity(shelf(ac, ac/10, float64(hz)))) }})
		paths = append(paths, path{name: "ac" + ftoa(ac) + "-comp100", r: radio{acHz: ac, gain: 2}, comp: func(hz int) biquad { return half(hfUnity(shelf(ac, ac/100, float64(hz)))) }})
	}
	paths = append(paths, path{name: "pre300", r: radio{preHz: 300, preTopHz: 3000}})
	paths = append(paths, path{name: "pre300-comp", r: radio{preHz: 300, preTopHz: 3000}, comp: func(hz int) biquad { return shelf(3000, 300, float64(hz)) }})
	radios := make([]radioResult, 0, len(paths))
	for _, p := range paths {
		k, hz := base.shaper()
		if p.comp != nil {
			k = &filterStage{in: k, q: p.comp(hz)}
		}
		a := make([]int16, 0, len(ts.syms)*hz/SymbolRate+16)
		for _, v := range ts.syms {
			a = k.appendSymbol(a, v)
		}
		peak := 0
		for _, v := range a {
			peak = max(peak, int(v), -int(v))
		}
		x48 := resample(dacFM(a, hz, p.r), dacRate, rxRate)
		res := sweepResult{FSK: fsk(x48, ts.syms)}
		shift := int(math.Round(res.FSK.Delay))
		res.Points = run.curve(x48, shift, math.Floor(min(ref.VoiceCN90, ref.DataCN90)*2)/2-1, false)
		res.VoiceCN90, res.DataCN90 = cn90(res.Points)
		res.DiscPoints = run.curve(x48, shift, math.Floor(min(ref.DiscVoiceCN90, ref.DiscDataCN90)*2)/2-1, true)
		res.DiscVoiceCN90, res.DiscDataCN90 = cn90(res.DiscPoints)
		r := radioResult{Name: p.name, FSK: res.FSK, Peak: peak, VoicePen: res.VoiceCN90 - ref.VoiceCN90, DataPen: res.DataCN90 - ref.DataCN90,
			DiscVoice: res.DiscVoiceCN90 - ref.DiscVoiceCN90, DiscData: res.DiscDataCN90 - ref.DiscDataCN90}
		t.Logf("radio %+v", r)
		radios = append(radios, r)
	}
	for i := range radios {
		for _, p := range []*float64{&radios[i].VoicePen, &radios[i].DataPen, &radios[i].DiscVoice, &radios[i].DiscData} {
			if math.IsNaN(*p) {
				*p = 99
			}
		}
	}
	b, err := json.MarshalIndent(map[string]any{"ramps": ramps, "radios": radios}, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "study.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func hfUnity(q biquad) biquad {
	g := (q.b0 - q.b1) / (1 - q.a1)
	q.b0 /= g
	q.b1 /= g
	return q
}

func half(q biquad) biquad {
	q.b0 /= 2
	q.b1 /= 2
	return q
}

func ftoa(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
