package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"slices"
	"sync"
)

// The files of an output directory.
const (
	variantsFile = "variants.jsonl"
	resultsFile  = "results.jsonl"
	costFile     = "cost.jsonl"
	m4File       = "m4.jsonl"
)

// score counts the items of one payload type.
type score struct {
	Truth int `json:"truth"`
	Exact int `json:"exact"`
}

// caseResult is one line of resultsFile: one stream of one variant.
type caseResult struct {
	ID         string            `json:"id"`
	CN         float64           `json:"cn"`
	Seed       uint64            `json:"seed"`
	Payloads   map[string]*score `json:"payloads"`
	CPUSeconds float64           `json:"cpu_seconds"`
	Seconds    float64           `json:"seconds"`
}

// variantRecord is one line of variantsFile.
type variantRecord struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Tree    string  `json:"tree"`
	Commit  string  `json:"commit"`
	Variant variant `json:"variant"`
}

// costRecord is one line of costFile, measured with the variant alone on
// one core.
type costRecord struct {
	ID                string  `json:"id"`
	RAMBytes          int     `json:"ram_bytes"`
	Allocs            float64 `json:"allocs"`
	FalsePerHour      float64 `json:"false_per_hour"`
	SearchNsPerSecond float64 `json:"search_ns_per_second"`
	// LockedNsPerSecond holds the CPU time per second of a stream at
	// C/N 10 dB and 5 dB.
	LockedNsPerSecond map[string]float64 `json:"locked_ns_per_second"`
}

// m4Record is one line of m4File: what the -m4 command printed.
type m4Record struct {
	ID     string                     `json:"id"`
	Result map[string]json.RawMessage `json:"result"`
}

// appender appends JSON lines to the files of an output directory.
type appender struct {
	mu sync.Mutex
}

func (a *appender) add(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// readLines decodes each line of path into a new T. A missing file has no
// lines.
func readLines[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		var v T
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

// point is the sum of the cases of a variant at one C/N.
type point struct {
	CN       float64
	Seeds    int
	Payloads map[string]*score
	CPU, Sec float64
}

// curveOf sums the cases of a variant by C/N, in increasing C/N.
func curveOf(cases []caseResult) []point {
	by := map[float64]*point{}
	for _, c := range cases {
		p := by[c.CN]
		if p == nil {
			p = &point{CN: c.CN, Payloads: map[string]*score{}}
			by[c.CN] = p
		}
		p.Seeds++
		p.CPU += c.CPUSeconds
		p.Sec += c.Seconds
		for k, s := range c.Payloads {
			a := p.Payloads[k]
			if a == nil {
				a = &score{}
				p.Payloads[k] = a
			}
			a.Truth += s.Truth
			a.Exact += s.Exact
		}
	}
	out := make([]point, 0, len(by))
	for _, p := range by {
		out = append(out, *p)
	}
	slices.SortFunc(out, func(a, b point) int { return cmpFloat(a.CN, b.CN) })
	return out
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// cnToEbN0 converts C/N in 12.5 kHz to Eb/N0 per channel bit at 9600 bit/s.
func cnToEbN0(cn float64) float64 { return cn + 10*math.Log10(12500.0/9600) }

// crossing is where a curve reaches a recovery level. below marks a curve
// that is there at its first point, and ok one that gets there at all.
type crossing struct {
	X         float64
	OK, Below bool
}

// at90 returns the x where the recovery y first reaches 90%, by linear
// interpolation.
func at90(x, y []float64) crossing {
	return atLevel(x, y, 0.9)
}

// atLevel returns the x at which y first reaches lvl, as at90.
func atLevel(x, y []float64, lvl float64) crossing {
	if len(y) > 0 && y[0] >= lvl {
		return crossing{x[0], true, true}
	}
	for i := 1; i < len(x); i++ {
		if y[i-1] < lvl && y[i] >= lvl {
			return crossing{x[i-1] + (lvl-y[i-1])/(y[i]-y[i-1])*(x[i]-x[i-1]), true, false}
		}
	}
	return crossing{}
}

// recovery returns the C/N and recovery of payload k over pts.
func recovery(pts []point, k string) (cn, y []float64) {
	for _, p := range pts {
		if s := p.Payloads[k]; s != nil && s.Truth > 0 {
			cn = append(cn, p.CN)
			y = append(y, float64(s.Exact)/float64(s.Truth))
		}
	}
	return cn, y
}
