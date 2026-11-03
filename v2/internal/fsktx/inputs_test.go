//go:build !tinygo

package fsktx

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"slices"
	"sync"
	"testing"
)

const inputProbeSymbols = 480

type inputProbe struct {
	c    limClass
	rate int
}

var inputDigests sync.Map

func writeInts[T int8 | int16 | int](h hash.Hash, v []T) {
	_ = binary.Write(h, binary.LittleEndian, int64(len(v)))
	for _, x := range v {
		_ = binary.Write(h, binary.LittleEndian, int64(x))
	}
}

func probeDigest(t testing.TB, p inputProbe) []byte {
	if v, ok := inputDigests.Load(p); ok {
		b, _ := v.([]byte)
		return b
	}
	h := sha256.New()
	h.Write([]byte(p.c.String()))
	seen := map[txUnit]bool{}
	for _, lp := range p.c.lowpasses() {
		for _, pt := range p.c.points(p.rate, lp) {
			h.Write([]byte(pt.String()))
			if seen[pt.u] {
				continue
			}
			seen[pt.u] = true
			if _, err := pt.u.mod(); err != nil {
				h.Write([]byte("no modulator"))
				continue
			}
			writeInts(h, []int{pt.u.lag(), p.c.superframes()})
			st := limStreamsOf(t, pt.u, p.c.superframes())
			writeInts(h, st.cont)
			writeInts(h, pt.u.pattern(patETSI, 4000))
			keys := []keying(nil)
			if !pt.u.audio() {
				keys = keyedOn()
				writeInts(h, st.ms.syms)
				for _, k := range st.keys {
					writeInts(h, []int{k.at, map[bool]int{false: 0, true: 1}[k.on]})
				}
			}
			writeInts(h, pt.u.run(t, st.cont[:min(len(st.cont), inputProbeSymbols)], keys))
		}
	}
	b := h.Sum(nil)
	inputDigests.Store(p, b)
	return b
}

func inputsDigest(t testing.TB, probes []inputProbe) string {
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, testParallel()))
	for _, p := range probes {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			probeDigest(t, p)
		}()
	}
	wg.Wait()
	h := sha256.New()
	for _, p := range probes {
		h.Write(probeDigest(t, p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func minRateProbes() []inputProbe {
	var out []inputProbe
	for _, c := range limClasses() {
		rates := iqGrid(c.tier)
		if !c.iq {
			rates = audioGrid(c.tier)
		}
		for _, r := range []int{rates[0], 24000, 48000, 96000, 2400000} {
			if slices.Contains(rates, r) && !slices.Contains(out, inputProbe{c, r}) {
				out = append(out, inputProbe{c, r})
			}
		}
	}
	return out
}

func txDocProbes() []inputProbe {
	cs := limClasses()
	out := make([]inputProbe, 0, len(cs))
	for _, c := range cs {
		out = append(out, inputProbe{c, txDocRate(c)})
	}
	return out
}

func minRateInputsDigest(t testing.TB) string { return inputsDigest(t, minRateProbes()) }

func txDocInputsDigest(t testing.TB) string { return inputsDigest(t, txDocProbes()) }

func TestTxDataCurrent(t *testing.T) {
	t.Parallel()
	if got := minRateInputsDigest(t); got != minRateInputs {
		t.Errorf("%s was measured on other inputs: its digest is %s, the test stream and modulators of this tree give %s; run DMRGO_LONG=1 go test ./internal/fsktx -run TestMinRatesGenerated -update-minrate in v2", minRateFile, minRateInputs, got)
	}
	var d txSheetData
	if err := readJSON(txSheetFile, &d); err != nil {
		t.Fatal(err)
	}
	if got := txDocInputsDigest(t); got != d.Inputs {
		t.Errorf("%s and %s were measured on other inputs: their digest is %s, the test stream and modulators of this tree give %s; run go test ./internal/fsktx -run TestTxDoc -update-txdoc in v2", txSheetFile, txMarginsFile, d.Inputs, got)
	}
}
