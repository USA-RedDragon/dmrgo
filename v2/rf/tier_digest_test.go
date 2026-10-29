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
	"audio48000/tier0": "793093f996b0d01cf3613b6fbbcaab126a9aaa97395c7f09eb425287efcb8c66",
	"audio48000/tier1": "8d7fd4b498f2693c080fc552d2817934127dbc9af44862835b91acc6b3b269aa",
	"audio48000/tier2": "516f29a411e4a0b9a489fc084cd73d2e77d849d11de23075844f4c3e2404eeac",
	"audio48000/tier3": "516f29a411e4a0b9a489fc084cd73d2e77d849d11de23075844f4c3e2404eeac",
	"audio9600/tier0":  "f3ea669632acc70c691760246a1a33ab317466ce834f584a1a267aa5d617d161",
	"audio9600/tier1":  "b8e10e65b217aef6230366b406e221e19ee96b1a8cf4187b0332bb425b42c606",
	"audio9600/tier2":  "be8d90fe3253ebaef34c288f9dd808fab7d58433f049875b7741bf1fe1d2ba47",
	"audio9600/tier3":  "be8d90fe3253ebaef34c288f9dd808fab7d58433f049875b7741bf1fe1d2ba47",
	"cf32/tier0":       "9d40e59d77514c30ca5cc07a3a425d901b41444e7b59c524ed871ed389a8b2e0",
	"cf32/tier1":       "b8763f263eae1c2677a894747414665bb93c24a16fc3f26564ff823a68d4d2f5",
	"cf32/tier2":       "a63cf9399a4b8e4a35ea467c83d78cac9eb85cc591f28940ca6912c4eebfcd2b",
	"cf32/tier3":       "fcc47a260a30d596b72b9c9867b6ffde28fd1f7d6a5628a071757065af0545aa",
	"cf32x5/tier0":     "0d23b3c654725981c078df0a7beaa7d8931260344f9b314293c5c88542f1f7ed",
	"cf32x5/tier1":     "a35e3049004a7f03cc58f444ed6a0958c181475fc1d812b079938507dc4b23e3",
	"cf32x5/tier2":     "b8092c2b9c0537b420dc9e7e91f296dc09d991db33532ce27eec984ee5d06c48",
	"cf32x5/tier3":     "7cadae8fd8985e84a314f3888310127da8bf1c958c88f922788c5f33e7424f94",
	"cs16/tier0":       "dfe782a389da8efd792e090aaadf736465f01d7548f219a31742e329bb6bcef4",
	"cs16/tier1":       "fefa50cebdc9402741b32427ed89f4aa70975175e25ea3bd89c6dd92728cdfe6",
	"cs16/tier2":       "45edb8f9c9cf5596225afddb54ac8aa667ed616112562646870143dcd1e28f3f",
	"cs16/tier3":       "a8e83e11b5e9124963ccaa011ca157fb2580ef5018c064ad1a3d0c0ace28f3c4",
	"cs8/tier0":        "c6011e3ba73ee74ab6e486739bcf474745eb56fcadb9297a98c2b975c1d92e69",
	"cs8/tier1":        "d8556d978f0ad134bb335bae07ad245083f1676ebaf572cf33beba1043b1478b",
	"cs8/tier2":        "b546c461c43af3525efbbd8c5fff1546f0c08ada80e84d5a3d630552534e83fc",
	"cs8/tier3":        "1ff1777bce0c90d9b40baf62945a0e9b38e005e8f34eceac7597231e2229a16c",
	"cu8/tier0":        "debc7a4c2300667ec50b52949f38cd7e067699c168f75bef508166106da27723",
	"cu8/tier1":        "3c9fc437681c3c795f1eedabf6cc0be7521823d0e39998a118a9e9e4ce407bc7",
	"cu8/tier2":        "95db6f8cbd30792eceff2e316afa6d3bae2867a057616e051ae4772f59d4a6cb",
	"cu8/tier3":        "85270287279d5bfda8f69b00d057373a6f1608b7b419b4f29b743238eae2f716",
	"cu8low/tier0":     "a5df7c7062e2e25688facccf1003165683bd4b36eb9afb3786288e0180bbcaaa",
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
