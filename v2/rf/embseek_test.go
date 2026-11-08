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

// TestRxTier0SeekEMB checks that Tier 0 receives every burst of an MS talker
// whose first burst in the input is voice burst B, on the other slot of a
// talker that locked on voice burst A before any color code was voted. Tier 0
// keeps no slot back, so it takes burst B on its own EMB, which reads with no
// bit corrected as the first fragment of an embedded LC, and the seek runs
// at the end of the EMB field, where the hunt buffer still holds the burst
// up to there. The seek starts that talker's clock from the rate of the
// other one, and on these payloads burst C lost a bit at its start to the
// phase carried over the other slot until Tier 0 fitted the timing again on
// the EMB of burst C.
func TestRxTier0SeekEMB(t *testing.T) {
	t.Parallel()
	row := mxRowNamed(t, "mid.none.-700.ms2.voice.-500ppm.nom.cs16-48k.40dB")
	for _, run := range []uint64{2, 9} {
		res, err := mxRun(pinnedRun(t, run, "TestRxMatrix"), row, []mxRx{{row.val(mxFormat), Tier0}})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range res {
			if len(r.fail) > 0 {
				t.Errorf("run %d: %s", run, strings.Join(r.detail, " | "))
			}
		}
	}
}

// TestRxSeekBackfillRate checks that Tiers 2 and 3 get every bit of burst B
// of a second MS talker at -500 ppm that the seek takes on the EMB of burst
// C and decodes from the kept samples two slots back. The coherent grid of
// that burst stepped back from the newest strobe at the nominal symbol
// period, and on these payloads the head of burst B, about 420 symbols
// back, lay 0.2 symbol off the fitted clock and lost two or three bits.
func TestRxSeekBackfillRate(t *testing.T) {
	t.Parallel()
	row := mxRowNamed(t, "mid.none.-700.ms2.voice.-500ppm.nom.cs16-48k.40dB")
	for _, run := range []uint64{14559090507006385804, 11362867179671988559} {
		res, err := mxRun(pinnedRun(t, run, "TestRxSeekSecondTalker"), row, []mxRx{{row.val(mxFormat), Tier2}, {row.val(mxFormat), Tier3}})
		if err != nil {
			t.Fatal(err)
		}
		for x, r := range res {
			if len(r.fail) > 0 {
				t.Errorf("run %d, %s: %s", run, x.name(), strings.Join(r.detail, " | "))
			}
		}
	}
}

// TestRxReplaySeekPhase checks Tier 1 on I/Q on two MS talkers at +400 and
// -400 ppm whose first bursts come before the lock, on the payloads and
// noise of two draws. The replay sliced the slot of the talker with no
// anchor at the phase of the talker that locked, 48 us away at that point:
// on the first draw it lost the SYNC of that talker's first burst, and on
// the second it took the SYNC but carried the phase of the other talker
// into the next burst, which came out with 29 raw bit errors.
func TestRxReplaySeekPhase(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		run   uint64
		noise uint64
	}{{1015841, 2}, {16979474955301313960, 4}} {
		p := pinnedRun(t, c.run, "TestRxTwoTalkerClocks")
		f, e, tk := twoTalkers(p, [2]float64{400, -400}, 3)
		got := receiveBaseband(twoTalkerIQ(p, f, e, 1400, 12, c.noise), Tier1)
		for k, errs := range twoTalkerErrors(tk, got) {
			for j, n := range errs {
				if n > 10 && (j > 0 || n > 20) {
					t.Errorf("run %d: talker %d burst %d has %d raw bit errors", c.run, k+1, j, n)
				}
			}
		}
	}
}

// TestRxSeekEyeVertex checks that Tiers 1 to 3 take a second MS talker on
// the EMB of its burst C, a payload on which the five quarter symbol phases
// of the EMB seek all read the EMB with two or more bits wrong at no noise:
// the talker sat 0.13 symbol off the grid, half way between two of them.
func TestRxSeekEyeVertex(t *testing.T) {
	t.Parallel()
	row := mxRowNamed(t, "mid.none.+0.ms2.voice.0ppm.nom.cs16-48k.inf")
	var rxs []mxRx
	for _, x := range mxReceivers(row.val(mxFormat), true) {
		if x.tier != Tier0 {
			rxs = append(rxs, x)
		}
	}
	res, err := mxRun(pinnedRun(t, 2100744, "TestRxSeekSecondTalker"), row, rxs)
	if err != nil {
		t.Fatal(err)
	}
	for x, r := range res {
		var miss string
		for _, d := range r.detail {
			if m, ok := strings.CutPrefix(d, "missing: "); ok {
				miss = m
			}
		}
		if !strings.HasPrefix(miss, "1 of ") || r.fail["bits"] || r.fail["extra"] {
			t.Errorf("%s: %s", x.name(), strings.Join(r.detail, " | "))
		}
	}
}
