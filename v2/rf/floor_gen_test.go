package rf

import (
	"encoding/json"
	"math"
	"os"
	"runtime"
	"strings"
	"testing"
)

// floorPath holds the floor curves that cmd/rxbench draws with the sweep.
// Regenerate with DMRRF_FLOOR=full go test -run TestRxFloor .
const floorPath = "testdata/rxbench_floor.json"

// floorBounds describes the bound of each payload in floorPath.
func floorBounds() map[string]string {
	const conv = "the converse bound for any code with as many values over the same symbols"
	return map[string]string{
		"ambe":      "the larger of maximum likelihood over the uncoded C2 and C3 bits, the genie also giving C0 and C1, maximum likelihood over the C0 and C1 values of c0c1Search, and " + conv,
		"ambe_c0c1": "the larger of maximum likelihood over the C0 and C1 values of c0c1Search, the genie also giving C2 and C3, and " + conv,
		"rate12":    "the larger of maximum likelihood over OSD candidates and " + conv,
		"rate34":    "the larger of maximum likelihood over the Viterbi candidate and the paths of rate34Search, and " + conv,
		"rate1":     "the larger of maximum likelihood over the Viterbi candidate and " + conv,
		"csbk":      "the larger of maximum likelihood over OSD candidates that pass the CRC, " + conv + ", and the slot type bound",
		"lc":        "the larger of maximum likelihood over OSD candidates that pass the Reed-Solomon check, " + conv + ", and the slot type bound",
		"header":    "the larger of maximum likelihood over OSD candidates that pass the CRC, " + conv + ", and the slot type bound",
		"elc":       "over the embedded LC of every superframe of the call so far, the larger of maximum likelihood over OSD candidates that pass the checksum and " + conv,
		"emb":       "the larger of maximum likelihood over the 8 EMB values of the stream's color code and " + conv,
		"slottype":  "the larger of maximum likelihood over the 16 slot types of the stream's color code and " + conv,
		"sync":      "the larger of maximum likelihood between the voice and the data SYNC and " + conv,
		"rate1_bit": "the larger of bitwise MAP detection of the rate 1 payload bits and Fano's inequality on the information density of the rate 1 block",
	}
}

// floorDeep lists the payloads of the graphs, which the generator runs
// with more seeds where their error rate is low.
func floorDeep() []string {
	return []string{"ambe", "rate12", "rate34", "csbk", "elc", "rate1_bit"}
}

// floorCurve is one payload of floorPath: counts per Eb/N0, with the error
// rate 1 - exact/truth.
type floorCurve struct {
	EbN0  []float64 `json:"ebn0"`
	Truth []int     `json:"truth"`
	Exact []int     `json:"exact"`
	Bound string    `json:"bound"`
}

// floorFile is the content of floorPath.
type floorFile struct {
	Source   string                 `json:"source"`
	Payloads map[string]*floorCurve `json:"payloads"`
}

// floorLimit returns the most items of payload k that the generator
// evaluates at one point.
func floorLimit(k string) int {
	switch k {
	case "rate1_bit":
		return 500000
	case "ambe":
		return 60000
	}
	return 100000
}

// TestRxFloor writes floorPath with DMRRF_FLOOR=full: the floor of every
// payload of the sweep stream from C/N -8 to 12 dB on the seeds of the
// README sweep, and more seeds for the graphed payloads until each point
// has 20 error events or floorLimit items.
func TestRxFloor(t *testing.T) {
	t.Parallel()
	if os.Getenv("DMRRF_FLOOR") != "full" {
		t.Skip("set DMRRF_FLOOR=full to regenerate " + floorPath)
		return
	}
	genCurves(t, false, floorPath, "rf floor_eval_test.go: a lower bound on the frame error rate of any receiver that knows less than a genie, which gives it the timing, the carrier, the noise level and every transmitted bit outside the field, on the stream and noise seeds of the sweep", floorBounds())
}

// TestRxAware writes awarePath with DMRRF_AWARE=full: the error rates of
// the code-aware receiver on the points and seeds of TestRxFloor.
func TestRxAware(t *testing.T) {
	t.Parallel()
	if os.Getenv("DMRRF_AWARE") != "full" {
		t.Skip("set DMRRF_AWARE=full to regenerate " + awarePath)
		return
	}
	genCurves(t, true, awarePath, "rf floor_aware_test.go: the error rate of a receiver with the knowledge of the genie of the floor and the decoders of the tiers, on the stream and noise seeds of the sweep", awareDescriptions())
}

// genCurves writes the floor, or the code-aware receiver when aware, of
// every payload of the sweep stream to path, with source and the
// description of each payload in bounds.
func genCurves(t *testing.T, aware bool, path, source string, bounds map[string]string) {
	slots, truth := benchStream(t, 30)
	workers := max(1, runtime.NumCPU()*3/4)
	var cases []benchCase
	for cn := -8.0; cn <= 12; cn += 0.5 {
		for seed := range uint64(4) {
			cases = append(cases, benchCase{cn, 100 + seed})
		}
	}
	pts := floorRun(t, slots, truth, cases, 1, true, nil, workers, aware)
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
			more := floorRun(t, slots, truth, deep, 1, true, only, workers, aware)
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
	out := floorFile{Source: source, Payloads: map[string]*floorCurve{}}
	for _, k := range floorPayloads() {
		c := &floorCurve{Bound: bounds[k]}
		for i := range pts {
			v, n, ok := pts[i].dmrFER(k)
			if !ok {
				continue
			}
			c.EbN0 = append(c.EbN0, math.Round((pts[i].CN+10*math.Log10(12500.0/9600))*1000)/1000)
			c.Truth = append(c.Truth, n)
			c.Exact = append(c.Exact, n-int(math.Round(v*float64(n))))
		}
		out.Payloads[k] = c
	}
	b, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
