package rf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"math"
	"slices"
	"sync"
	"testing"
)

type tierDigestInput struct {
	name   string
	tiers  []Tier
	format IQFormat
	rate   int
	audio  bool
}

func tierDigestInputs() []tierDigestInput {
	all := []Tier{Tier0, Tier1, Tier2, Tier3}
	return []tierDigestInput{
		{name: "audio9600", tiers: all, rate: 9600, audio: true},
		{name: "audio48000", tiers: all, rate: 48000, audio: true},
		{name: "cu8", tiers: all, format: IQFormatCU8, rate: basebandRate},
		{name: "cs8", tiers: all, format: IQFormatCS8, rate: basebandRate},
		{name: "cs16", tiers: all, format: IQFormatCS16, rate: basebandRate},
		{name: "cf32", tiers: all, format: IQFormatCF32, rate: basebandRate},
		{name: "cf32x5", tiers: all, format: IQFormatCF32, rate: 5 * basebandRate},
		{name: "cu8low", tiers: []Tier{Tier0}, format: IQFormatCU8, rate: iq0LowHz},
	}
}

func tierDigestConds() []rxCond {
	want := []string{"clean", "offset +3000 Hz", "drift 100 Hz/s", "fade -20 dB", "rekey 0.50 s", "late C"}
	var out []rxCond
	for _, c := range rxConds() {
		if slices.Contains(want, c.name) {
			out = append(out, c)
		}
	}
	return out
}

func tierDigestRun(in tierDigestInput, tier Tier, x []complex64) []Burst {
	if in.audio {
		r := mustAudio(AudioRxConfig{Rate: in.rate, Tier: tier})
		a := discAudio(x, in.rate)
		var out []Burst
		for k := 0; k < len(a); k += 1500 {
			out = r.AppendBursts(out, a[k:min(k+1500, len(a))])
		}
		return r.AppendFlush(out)
	}
	y := x
	switch {
	case in.rate == iq0LowHz:
		y = lowIQ(x, iq0LowHz)
	case in.rate != basebandRate:
		y = dgUpsample(x, in.rate)
	}
	b := iqBytes(y, in.format)
	r := mustNew(RxConfig{Tier: tier, Rate: in.rate, Format: in.format})
	var out []Burst
	for k := 0; k < len(b); k += 3001 {
		out = r.AppendBursts(out, b[k:min(k+3001, len(b))])
	}
	return r.AppendFlush(out)
}

func tierDigestWrite(h hash.Hash, b *Burst) {
	fmt.Fprintf(h, "%x %x %d %v %v %x %v %v %d %d %v %x %x\n", b.Data, b.Fixed, b.Distance, b.Sync, b.Voice,
		b.CACH, b.HasCACH, b.BS, b.Stream, b.Slot, b.HasFixed, b.FixedCACH, math.Float64bits(b.Seconds))
}

func tierDigests(t testing.TB) map[string]string {
	streams := rxStreams(t)
	type sig struct {
		key string
		x   []complex64
	}
	conds := tierDigestConds()
	sigs := make([]sig, 0, 2*3*len(conds))
	for _, sn := range []string{"bs", "ms"} {
		for _, c := range conds {
			for _, cn := range []float64{40, 12, 6} {
				key := fmt.Sprintf("%s %s %g", sn, c.name, cn)
				sigs = append(sigs, sig{key, rxBuild(streams[sn], c, cn, false, rxSeed(key)).x})
			}
		}
	}
	type job struct {
		in   tierDigestInput
		tier Tier
	}
	var jobs []job
	for _, in := range tierDigestInputs() {
		for _, tier := range in.tiers {
			jobs = append(jobs, job{in, tier})
		}
	}
	out := make([]string, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h := sha256.New()
			for _, s := range sigs {
				bs := tierDigestRun(j.in, j.tier, s.x)
				fmt.Fprintf(h, "%s %d\n", s.key, len(bs))
				for k := range bs {
					tierDigestWrite(h, &bs[k])
				}
			}
			out[i] = hex.EncodeToString(h.Sum(nil))
		}()
	}
	wg.Wait()
	m := make(map[string]string, len(jobs))
	for i, j := range jobs {
		m[fmt.Sprintf("%s/tier%d", j.in.name, j.tier-Tier0)] = out[i]
	}
	return m
}

var tierWantDigests = map[string]string{
	"audio48000/tier0": "fea78872e1fed9e7f6f93686ac40bdcad857917cc523322cd4eff7d57c14e669",
	"audio48000/tier1": "c7728ae53f8b3cae69e009657e33f01b198e4abf83fd94ac934d5cecefd43924",
	"audio48000/tier2": "6b8b8f8e1e38407661d59c7bd8c022a1d547579dd6f009016d02d2ff12f8a2af",
	"audio48000/tier3": "6b8b8f8e1e38407661d59c7bd8c022a1d547579dd6f009016d02d2ff12f8a2af",
	"audio9600/tier0":  "833026564df12d4d710fdbd8f08a35fe3181b64c362cd81e449f2abfc14cd995",
	"audio9600/tier1":  "7f2dfc627fe88352184697dc7135a5e7941325acdc73ba0f6d85f7937d358f04",
	"audio9600/tier2":  "ab1e3d4a05b26dd25c475883fed9d3aacb59716ccfd6a7b6aca67974dce35382",
	"audio9600/tier3":  "ab1e3d4a05b26dd25c475883fed9d3aacb59716ccfd6a7b6aca67974dce35382",
	"cf32/tier0":       "3c60025caedb34d140daf4e529bd98ef1a76217264242a9ccad2af6ee04f9e79",
	"cf32/tier1":       "af24c117b76630c18828226a5078bf138cecca15dfc82c95ca558930a8d53395",
	"cf32/tier2":       "87336a9a33c8c608d815251182c90e6fdda79fed58cc4fa6f34f29ad8694cf2b",
	"cf32/tier3":       "aa0a592fdbf934dda074c586375a5a1900be0cf1e17b5ee774d96a8e41cc5e78",
	"cf32x5/tier0":     "937440068b1a5d0e114c376b954dd84d7dbdb327eeebb12f21ce6177f86201ab",
	"cf32x5/tier1":     "8cf220570e53a4955dfdc61c57732dd42daf99d7b7453aa9a4d68da78aca7ba3",
	"cf32x5/tier2":     "91c4619d1733076512c70946234c98ec440cea09ff638562d6fda97e25618f26",
	"cf32x5/tier3":     "354561bf7a2456e02dad1519b15f7ac8d44b680cc376c50453c3b1a253f4c879",
	"cs16/tier0":       "106b727d4e47bbe3c4976b38b7746d43d732b02728ac4474c8dea1875325c9f1",
	"cs16/tier1":       "bbc36b3a971fd360bfb3f726f7cbab94b6c4e7b15243626e0d6ed9cca206624a",
	"cs16/tier2":       "e5a40bcbb55b013cca27276813056129bbe07e184af30bc2c949735775f1dae0",
	"cs16/tier3":       "5f43ca483fb10af893a5ea7163d07ed93d931d506a6ba7c371ff0a76b5476fa3",
	"cs8/tier0":        "9412c5f8332e5d36b26585040eab737a11f63cacee834f4eaf56b837270ce072",
	"cs8/tier1":        "644c6a07a005b6332bee1d122b0cf069d74daf7bbec78006d56cc8a55ba1f667",
	"cs8/tier2":        "3b5f15b207839cffefb3e38d1c8fdd8df088870b443f08b4c5e3edbe30a09dcc",
	"cs8/tier3":        "12c84fafc27211d5a5be100286edfd9f199580dd5e42cfeb6bf58359ee2f3ded",
	"cu8/tier0":        "4e2c718014038a7dcf4a90142c075b43069d1c11ab6523cf716b19c1f62f3855",
	"cu8/tier1":        "e9d0b60588ba4e83efa0c3bfc184b260ef48aa1736caf3169501c07c4a0fe0c3",
	"cu8/tier2":        "9d17c6925e9f78959c7f86e76ec7a1f2cd03d9150eb3624b001a1c0a51748bca",
	"cu8/tier3":        "de6b9c2de38e734093c3ad07df087b19ca955fc09717e6ba3df0130731c2dd41",
	"cu8low/tier0":     "d20d0e413f1ee1cfd71745425f5248a90ae300ac93a3baf8e02c4c9d940d9ae2",
}

func TestTierDigests(t *testing.T) {
	t.Parallel()
	got := tierDigests(t)
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if want := tierWantDigests[k]; got[k] != want {
			t.Errorf("%q: %q, want %q", k, got[k], want)
		}
	}
	if len(tierWantDigests) != len(got) {
		t.Errorf("%d digests, want %d", len(got), len(tierWantDigests))
	}
}
