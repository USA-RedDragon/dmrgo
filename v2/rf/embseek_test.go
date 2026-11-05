//go:build !tinygo

package rf

import (
	"strings"
	"testing"
)

func mxRowNamed(t *testing.T, name string) mxRow {
	t.Helper()
	var r mxRow
	toks := strings.Split(name, ".")
	k := 0
	for d := range mxDims {
		if k == len(toks) {
			break
		}
		for v, s := range mxValues[d] {
			if s == toks[k] && (d < mxBaseDims || v != 0) {
				r[d] = uint8(v)
				k++
				break
			}
		}
	}
	if r.name() != name {
		t.Fatalf("row %q parses as %q", name, r.name())
	}
	return r
}

// TestRxSeekSecondTalker checks that Tiers 1 to 3 receive the voice bursts
// of a second MS talker that has no SYNC in the input, from its EMB, with
// every bit right. In the first row, burst B of that talker comes before the
// first talker's lock and is not received yet. In the others, the seek takes
// burst B, which comes after the lock, on the EMB of burst C.
func TestRxSeekSecondTalker(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		row  string
		miss string
	}{
		{"mid.none.+0.ms2.voice.0ppm.nom.cs16-48k.inf", "#0 "},
		{"mid.none.-700.ms2.voice.-500ppm.nom.cs16-48k.40dB", ""},
		{"mid.none.-700.ms2.voice.-500ppm.nom.cs16-48k.40dB.dmr40", ""},
	} {
		row := mxRowNamed(t, c.row)
		var rxs []mxRx
		for _, x := range mxReceivers(row.val(mxFormat), true) {
			if x.tier != Tier0 {
				rxs = append(rxs, x)
			}
		}
		out, err := mxRun(t, row, rxs)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range rxs {
			res := out[x]
			var miss string
			for _, d := range res.detail {
				if m, ok := strings.CutPrefix(d, "missing: "); ok {
					miss = m
				}
			}
			if c.miss == "" && miss != "" || c.miss != "" && (!strings.HasPrefix(miss, "1 of ") || !strings.Contains(miss, c.miss)) || res.fail["bits"] || res.fail["extra"] {
				t.Errorf("%s/%s: %s", c.row, x.name(), strings.Join(res.detail, " | "))
			}
		}
	}
}
