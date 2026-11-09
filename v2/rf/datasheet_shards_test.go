//go:build !tinygo

package rf

import (
	"maps"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func datasheetMatrices(t *testing.T, yaml string) (map[string][]string, string) {
	t.Helper()
	matrices := map[string][]string{}
	minrateShards := ""
	inJobs := false
	job := ""
	for line := range strings.SplitSeq(yaml, "\n") {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "#"):
			continue
		case indent == 0:
			inJobs = trimmed == "jobs:"
			job = ""
		case indent == 2 && inJobs && strings.HasSuffix(trimmed, ":"):
			job = strings.TrimSuffix(trimmed, ":")
		case indent == 2 && !inJobs:
			if v, ok := strings.CutPrefix(trimmed, "MINRATE_SHARDS:"); ok {
				minrateShards = strings.TrimSpace(v)
			}
		}
		s, ok := strings.CutPrefix(trimmed, "shard: [")
		if !ok {
			continue
		}
		if job == "" {
			t.Fatalf("datasheet.yaml has a shard matrix outside a job: %q", line)
		}
		if _, dup := matrices[job]; dup {
			t.Fatalf("job %s of datasheet.yaml has more than one shard matrix", job)
		}
		matrices[job] = []string{}
		for x := range strings.SplitSeq(strings.TrimSuffix(s, "]"), ",") {
			matrices[job] = append(matrices[job], strings.TrimSpace(x))
		}
	}
	return matrices, minrateShards
}

func TestDatasheetShards(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("../../.github/workflows/datasheet.yaml")
	if err != nil {
		t.Fatal(err)
	}
	meta := slices.Sorted(maps.Keys(benchShards()))
	metaCase := false
	for line := range strings.SplitSeq(string(b), "\n") {
		metaCase = metaCase || strings.TrimSpace(line) == strings.Join(meta, "|")+")"
	}
	if !metaCase {
		t.Errorf("datasheet.yaml has no case %s) that runs the sweeps of the tiers of benchShards() without the deep sweep", strings.Join(meta, "|"))
	}
	matrices, minrateShards := datasheetMatrices(t, string(b))
	if jobs := slices.Sorted(maps.Keys(matrices)); !reflect.DeepEqual(jobs, []string{"minrate", "sweep"}) {
		t.Fatalf("datasheet.yaml has shard matrices in jobs %v, want [minrate sweep]", jobs)
	}

	var want []string
	for _, e := range benchEntries() {
		want = append(want, e.name())
		for k := range benchShards()[e.name()] {
			want = append(want, e.name()+"-"+strconv.Itoa(k))
		}
	}
	for _, kind := range []string{"floor", "aware"} {
		for k := range floorShards()[kind] {
			want = append(want, kind+"-"+strconv.Itoa(k))
		}
	}
	got := slices.Sorted(slices.Values(matrices["sweep"]))
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the sweep shard matrix of datasheet.yaml is %v, want %v", got, want)
	}

	n, err := strconv.Atoi(minrateShards)
	if err != nil || n < 1 {
		t.Fatalf("datasheet.yaml sets MINRATE_SHARDS to %q, want a positive integer", minrateShards)
	}
	wantMinrate := make([]string, n)
	for k := range n {
		wantMinrate[k] = strconv.Itoa(k)
	}
	if !reflect.DeepEqual(matrices["minrate"], wantMinrate) {
		t.Errorf("the minrate shard matrix of datasheet.yaml is %v, want %v for MINRATE_SHARDS %d", matrices["minrate"], wantMinrate, n)
	}
}
