package rf

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
)

const floorPath = "testdata/rxbench_floor.json"

const (
	dmrExact = "exact"
	dmrLower = "lower bound"
	dmrNone  = "not computed"
)

type floorDMR struct {
	bound, status string
}

func anyBounds() map[string]string {
	conv := func(bits int, over string) string {
		return fmt.Sprintf("the converse bound for any code with 2^%d values over %s", bits, over)
	}
	const syms = "the symbols of the payload"
	return map[string]string{
		"ambe":      conv(49, syms),
		"ambe_c0c1": conv(24, "the symbols of C0 and C1"),
		"rate12":    conv(96, syms),
		"rate34":    conv(144, syms),
		"rate1":     conv(192, syms),
		"csbk":      conv(80, syms),
		"lc":        conv(72, syms),
		"header":    conv(80, syms),
		"elc":       conv(72, "the embedded LC symbols of every superframe of the call so far"),
		"emb":       conv(3, syms),
		"slottype":  conv(4, syms),
		"sync":      conv(1, syms),
		"rate1_bit": "Fano's inequality on the information density of the rate 1 block: the bit error rate of any code with 2^192 values over its symbols",
	}
}

func dmrBounds() map[string]floorDMR {
	const none = "not computed: maximum likelihood over 2^%d values is out of reach, and no candidate of the ordered statistics search is more likely than the transmitted value where the 4FSK floor allows errors, so the tests add nothing to the 4FSK floor"
	return map[string]floorDMR{
		"ambe":      {"maximum likelihood over the uncoded C2 and C3 bits with the genie also giving C0 and C1, and the C0 and C1 search of the ambe_c0c1 payload", dmrLower},
		"ambe_c0c1": {"the search over C0 and C1 by the model metric of the genie trellis (floor_search_test.go), the genie also giving C2 and C3", dmrLower},
		"rate12":    {fmt.Sprintf(none, 96), dmrNone},
		"rate34":    {"the Viterbi path of the genie LLRs and the search over the paths that change one or two inputs at a time from the transmitted one (floor_search_test.go)", dmrLower},
		"rate1":     {"maximum likelihood detection of the 96 payload symbols by the genie trellis", dmrExact},
		"csbk":      {fmt.Sprintf(none, 80), dmrNone},
		"lc":        {fmt.Sprintf(none, 72), dmrNone},
		"header":    {fmt.Sprintf(none, 80), dmrNone},
		"elc":       {fmt.Sprintf(none, 72), dmrNone},
		"emb":       {"maximum likelihood over the 8 EMB values of the stream's color code", dmrExact},
		"slottype":  {"maximum likelihood over the 16 slot types of the stream's color code", dmrExact},
		"sync":      {"maximum likelihood between the voice and the data SYNC", dmrExact},
		"rate1_bit": {"bitwise MAP detection of the rate 1 payload bits", dmrExact},
	}
}

func floorDeep() []string {
	return []string{"ambe", "rate12", "rate34", "csbk", "elc", "rate1_bit"}
}

type floorCurve struct {
	EbN0  []float64 `json:"ebn0"`
	Truth []int     `json:"truth"`
	Exact []int     `json:"exact"`
	Bound string    `json:"bound"`
}

type floorPair struct {
	Any    *floorCurve `json:"any"`
	DMR    *floorCurve `json:"dmr"`
	Status string      `json:"status"`
}

type floorFile struct {
	Source   string                `json:"source"`
	Payloads map[string]*floorPair `json:"payloads"`
}

type awareFile struct {
	Source   string                 `json:"source"`
	Payloads map[string]*floorCurve `json:"payloads"`
}

func floorLimit(k string) int {
	switch k {
	case "rate1_bit":
		return 500000
	case "ambe":
		return 60000
	}
	return 100000
}

func TestRxFloor(t *testing.T) {
	t.Parallel()
	if os.Getenv("DMRRF_FLOOR") != "full" {
		t.Skip("set DMRRF_FLOOR=full to regenerate " + floorPath)
		return
	}
	pts := genPoints(t, false)
	out := floorFile{
		Source:   "rf floor_eval_test.go: lower bounds on the frame error rate of any receiver that knows less than a genie, which gives it the timing, the carrier, the noise level and every transmitted bit outside the field, on the stream and noise seeds of the sweep: the 4FSK floor for any code of the size of the field, and the DMR floor for its DMR code",
		Payloads: map[string]*floorPair{},
	}
	anyB, dmrB := anyBounds(), dmrBounds()
	for _, k := range floorPayloads() {
		out.Payloads[k] = &floorPair{
			Any:    pointCurve(pts, anyB[k], (*floorPoint).anyFER, k),
			DMR:    pointCurve(pts, dmrB[k].bound, (*floorPoint).dmrFER, k),
			Status: dmrB[k].status,
		}
	}
	b, err := json.MarshalIndent(out, "", " ")
	writeCurves(t, floorPath, b, err)
}

func TestRxAware(t *testing.T) {
	t.Parallel()
	if os.Getenv("DMRRF_AWARE") != "full" {
		t.Skip("set DMRRF_AWARE=full to regenerate " + awarePath)
		return
	}
	pts := genPoints(t, true)
	out := awareFile{
		Source:   "rf floor_aware_test.go: the error rate of a receiver with the knowledge of the genie of the floor and the decoders of the tiers, on the stream and noise seeds of the sweep",
		Payloads: map[string]*floorCurve{},
	}
	desc := awareDescriptions()
	for _, k := range floorPayloads() {
		out.Payloads[k] = pointCurve(pts, desc[k], (*floorPoint).dmrFER, k)
	}
	b, err := json.MarshalIndent(out, "", " ")
	writeCurves(t, awarePath, b, err)
}

func pointCurve(pts []floorPoint, bound string, rate func(*floorPoint, string) (float64, int, bool), k string) *floorCurve {
	c := &floorCurve{Bound: bound}
	for i := range pts {
		v, n, ok := rate(&pts[i], k)
		if !ok {
			continue
		}
		c.EbN0 = append(c.EbN0, math.Round((pts[i].CN+10*math.Log10(12500.0/9600))*1000)/1000)
		c.Truth = append(c.Truth, n)
		c.Exact = append(c.Exact, n-int(math.Round(v*float64(n))))
	}
	return c
}

func writeCurves(t *testing.T, path string, b []byte, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func genPoints(t *testing.T, aware bool) []floorPoint {
	slots, truth := benchStream(t, 30)
	workers := max(1, runtime.NumCPU()*3/4)
	var cases []benchCase
	for cn := -8.0; cn <= 12; cn += 0.5 {
		for seed := range uint64(4) {
			cases = append(cases, benchCase{cn, 100 + seed})
		}
	}
	pts := floorRun(t, slots, truth, cases, 1, true, nil, workers, aware, 0)
	done := map[string]bool{}
	for pi := range pts {
		p := &pts[pi]
		for batch := 0; ; batch++ {
			only := map[string]bool{}
			for _, k := range floorDeep() {
				v, n, ok := p.dmrFER(k)
				if !ok || done[k] || v == 0 && n >= floorLimit(k)/4 || v > 0.2 || v*float64(n) >= 20 || n >= floorLimit(k) {
					continue
				}
				only[k] = true
			}
			if len(only) == 0 {
				break
			}
			deep := make([]benchCase, workers)
			for j := range deep {
				deep[j] = benchCase{p.CN, 1000 + uint64(pi)<<20 + uint64(batch*workers+j)}
			}
			more := floorRun(t, slots, truth, deep, 1, true, only, workers, aware, 0)
			for k, v := range more[0].Counts {
				if !only[k] && !only[strings.TrimSuffix(k, "/st")] {
					continue
				}
				a := p.Counts[k]
				a.Truth += v.Truth
				a.Exact += v.Exact
			}
			for k, v := range more[0].Conv {
				if only[k] {
					p.Conv[k] = append(p.Conv[k], v...)
				}
			}
		}
		for _, k := range floorDeep() {
			if v, n, ok := p.dmrFER(k); ok && v == 0 && n >= floorLimit(k)/4 {
				done[k] = true
			}
		}
		t.Logf("C/N %.1f dB done", p.CN)
	}
	return pts
}
