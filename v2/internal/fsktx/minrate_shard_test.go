//go:build !tinygo

package fsktx

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

type minRateShardFile struct {
	Shard   int                          `json:"shard"`
	Shards  int                          `json:"shards"`
	Inputs  string                       `json:"inputs"`
	Results map[string]map[string]string `json:"results"`
}

func parseMinRateShard(s string) (k, n int, err error) {
	if _, err := fmt.Sscanf(s, "%d/%d", &k, &n); err != nil {
		return 0, 0, fmt.Errorf("shard %q is not <k>/<n>: %w", s, err)
	}
	if n < 1 || k < 0 || k >= n || fmt.Sprintf("%d/%d", k, n) != s {
		return 0, 0, fmt.Errorf("shard %q is not <k>/<n> with 0 <= k < n", s)
	}
	return k, n, nil
}

func (it minRateItem) cost() float64 {
	if !it.c.iq {
		return audioRa(it.rate) / 38400
	}
	return float64(it.c.superframes()) / 8 * (10 + 2.2*float64(it.rate)/1e6)
}

func minRateShards(n int) [][]minRateItem {
	items := minRateItems()
	order := make([]int, len(items))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(a, b int) int { return cmp.Compare(items[b].cost(), items[a].cost()) })
	load := make([]float64, n)
	pick := make([][]int, n)
	for _, i := range order {
		k := 0
		for j := range load {
			if load[j] < load[k] {
				k = j
			}
		}
		load[k] += items[i].cost()
		pick[k] = append(pick[k], i)
	}
	out := make([][]minRateItem, n)
	for k, idx := range pick {
		slices.Sort(idx)
		for _, i := range idx {
			out[k] = append(out[k], items[i])
		}
	}
	return out
}

func encodeMinRates(res map[string]map[string]float64) map[string]map[string]string {
	out := make(map[string]map[string]string, len(res))
	for key, g := range res {
		if g == nil {
			out[key] = nil
			continue
		}
		e := make(map[string]string, len(g))
		for k, v := range g {
			e[k] = strconv.FormatFloat(v, 'x', -1, 64)
		}
		out[key] = e
	}
	return out
}

func decodeMinRates(enc map[string]map[string]string) (map[string]map[string]float64, error) {
	out := make(map[string]map[string]float64, len(enc))
	for key, e := range enc {
		if e == nil {
			out[key] = nil
			continue
		}
		g := make(map[string]float64, len(e))
		for k, s := range e {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", key, k, err)
			}
			g[k] = v
		}
		out[key] = g
	}
	return out, nil
}

func TestMinRatesShard(t *testing.T) {
	t.Parallel()
	spec, path := os.Getenv("DMRGO_MINRATE_SHARD"), os.Getenv("DMRGO_MINRATE_SHARD_OUT")
	if spec == "" || path == "" {
		t.Skip("set DMRGO_MINRATE_SHARD=<k>/<n> and DMRGO_MINRATE_SHARD_OUT=<file> to measure shard k of n of the rate grid of TestMinRatesGenerated")
	}
	k, n, err := parseMinRateShard(spec)
	if err != nil {
		t.Fatal(err)
	}
	items := minRateShards(n)[k]
	f := minRateShardFile{Shard: k, Shards: n, Inputs: minRateInputsDigest(t), Results: encodeMinRates(measureMinRateItems(t, items))}
	b, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readMinRateShards(dir, inputs string) (map[string]map[string]float64, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("%s has no shard files", dir)
	}
	res := map[string]map[string]float64{}
	var seen []bool
	n := 0
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var f minRateShardFile
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if n == 0 {
			n = f.Shards
			if n < 1 {
				return nil, fmt.Errorf("%s: %d shards", p, n)
			}
			seen = make([]bool, n)
		}
		switch {
		case f.Shards != n:
			return nil, fmt.Errorf("%s is shard %d of %d, other files are of %d", p, f.Shard, f.Shards, n)
		case f.Shard < 0 || f.Shard >= n:
			return nil, fmt.Errorf("%s is shard %d of %d", p, f.Shard, n)
		case seen[f.Shard]:
			return nil, fmt.Errorf("%s repeats shard %d", p, f.Shard)
		case f.Inputs != inputs:
			return nil, fmt.Errorf("%s was measured on inputs %s, this tree gives %s", p, f.Inputs, inputs)
		}
		seen[f.Shard] = true
		g, err := decodeMinRates(f.Results)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		want := map[string]bool{}
		for _, it := range minRateShards(n)[f.Shard] {
			want[it.key()] = true
		}
		for key, v := range g {
			if !want[key] {
				return nil, fmt.Errorf("%s has %s, which is not in shard %d of %d", p, key, f.Shard, n)
			}
			res[key] = v
		}
		if len(g) != len(want) {
			return nil, fmt.Errorf("%s has %d of the %d points of shard %d of %d", p, len(g), len(want), f.Shard, n)
		}
	}
	for k, ok := range seen {
		if !ok {
			return nil, fmt.Errorf("%s has no shard %d of %d", dir, k, n)
		}
	}
	return res, nil
}

func TestMinRatesMerge(t *testing.T) {
	t.Parallel()
	dir := os.Getenv("DMRGO_MINRATE_MERGE")
	if dir == "" {
		t.Skip("set DMRGO_MINRATE_MERGE to a directory of TestMinRatesShard files to merge them as TestMinRatesGenerated does")
	}
	res, err := readMinRateShards(dir, minRateInputsDigest(t))
	if err != nil {
		t.Fatal(err)
	}
	writeMinRates(t, res)
}

func TestMinRatesShardPlan(t *testing.T) {
	t.Parallel()
	items := minRateItems()
	keys := map[string]bool{}
	for _, it := range items {
		if keys[it.key()] {
			t.Fatalf("two points have the key %s", it.key())
		}
		keys[it.key()] = true
	}
	for _, n := range []int{1, 3, 24} {
		got := 0
		for _, s := range minRateShards(n) {
			for _, it := range s {
				if !keys[it.key()] {
					t.Fatalf("%d shards: %s is not on the grid or is in two shards", n, it.key())
				}
				delete(keys, it.key())
				got++
			}
		}
		if got != len(items) || len(keys) != 0 {
			t.Fatalf("%d shards hold %d of %d points", n, got, len(items))
		}
		for _, it := range items {
			keys[it.key()] = true
		}
	}
}

func TestMinRatesShardCodec(t *testing.T) {
	t.Parallel()
	in := map[string]map[string]float64{
		"a": {"x": 0.1, "y": -3.25e-310, "z": 1e300, "inf": math.Inf(1), "ninf": -math.Inf(1)},
		"b": nil,
	}
	b, err := json.Marshal(encodeMinRates(in))
	if err != nil {
		t.Fatal(err)
	}
	var enc map[string]map[string]string
	if err := json.Unmarshal(b, &enc); err != nil {
		t.Fatal(err)
	}
	out, err := decodeMinRates(enc)
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := out["b"]; !ok || v != nil {
		t.Fatalf("a point that did not build came back as %v, %v", v, ok)
	}
	for k, v := range in["a"] {
		if out["a"][k] != v {
			t.Errorf("%s: %v came back as %v", k, v, out["a"][k])
		}
	}
	for _, s := range []string{"", "1", "3/3", "-1/2", "1/0", "01/2", "1/2x"} {
		if _, _, err := parseMinRateShard(s); err == nil {
			t.Errorf("shard %q parses", s)
		}
	}
}
