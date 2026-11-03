//go:build !tinygo

package rf

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestRxBenchMerge(t *testing.T) {
	t.Parallel()
	dir := os.Getenv("DMRRF_RXBENCH_MERGE")
	if dir == "" {
		t.Skip("set DMRRF_RXBENCH_MERGE to a directory of <tier>.json sweeps, and <tier>-<k>.json deep shards of the tiers of benchShards(), to merge them into " + rxbenchPath)
	}
	var base benchData
	if err := readBench(rxbenchPath, &base); err != nil {
		t.Fatal(err)
	}
	for i, bt := range base.Tiers {
		e := benchEntry{mode: bt.Mode, tier: Tier0 + Tier(bt.Tier)}
		var shard benchData
		err := readBench(filepath.Join(dir, e.name()+".json"), &shard)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		got, ok := shard.find(e)
		if !ok {
			t.Fatalf("%s.json has no %s", e.name(), e.name())
		}
		full, sharded, err := benchMergeShards(dir, e)
		if err != nil {
			t.Fatal(err)
		}
		if sharded {
			got.Full = full
		}
		got.NsPerSecond, got.WorstNs, got.WorstCN = bt.NsPerSecond, bt.WorstNs, bt.WorstCN
		base.Tiers[i] = got
	}
	b, err := json.MarshalIndent(base, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rxbenchPath, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readBench(path string, d *benchData) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, d)
}
