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

func TestDatasheetShards(t *testing.T) {
	t.Parallel()
	b, err := os.ReadFile("../../.github/workflows/datasheet.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	meta := slices.Sorted(maps.Keys(benchShards()))
	metaCase := false
	for line := range strings.SplitSeq(string(b), "\n") {
		metaCase = metaCase || strings.TrimSpace(line) == strings.Join(meta, "|")+")"
		if s, ok := strings.CutPrefix(strings.TrimSpace(line), "shard: ["); ok {
			for _, x := range strings.Split(strings.TrimSuffix(s, "]"), ",") {
				got = append(got, strings.TrimSpace(x))
			}
		}
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
	if !metaCase {
		t.Errorf("datasheet.yaml has no case %s) that runs the sweeps of the tiers of benchShards() without the deep sweep", strings.Join(meta, "|"))
	}
	slices.Sort(got)
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the shard matrix of datasheet.yaml is %v, want %v", got, want)
	}
}
