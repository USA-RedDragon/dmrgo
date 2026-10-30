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
	"audio48000/tier0": "825dd8f2e7674c26f0286af79f38c26af024c39b05bda85ec957bcefd913717f",
	"audio48000/tier1": "71bd191dcc951716b9211a4e2af861f0146b9cbd167ab10e905f3ead8b7c1457",
	"audio48000/tier2": "18950137d664a128da0cc143756161cd86c6cacd892bed85b57a5c7384c7931a",
	"audio48000/tier3": "18950137d664a128da0cc143756161cd86c6cacd892bed85b57a5c7384c7931a",
	"audio9600/tier0":  "f3ea669632acc70c691760246a1a33ab317466ce834f584a1a267aa5d617d161",
	"audio9600/tier1":  "b8e10e65b217aef6230366b406e221e19ee96b1a8cf4187b0332bb425b42c606",
	"audio9600/tier2":  "be8d90fe3253ebaef34c288f9dd808fab7d58433f049875b7741bf1fe1d2ba47",
	"audio9600/tier3":  "be8d90fe3253ebaef34c288f9dd808fab7d58433f049875b7741bf1fe1d2ba47",
	"cf32/tier0":       "238152fc1752acc873e7470db25ad8f77642a7c6b21256847c196b3780f5bca9",
	"cf32/tier1":       "b8763f263eae1c2677a894747414665bb93c24a16fc3f26564ff823a68d4d2f5",
	"cf32/tier2":       "a63cf9399a4b8e4a35ea467c83d78cac9eb85cc591f28940ca6912c4eebfcd2b",
	"cf32/tier3":       "2d399384191e5c1d53931c706cd8541ab5aeb162cae7e3a862acd0c6bd853fdd",
	"cf32x5/tier0":     "937440068b1a5d0e114c376b954dd84d7dbdb327eeebb12f21ce6177f86201ab",
	"cf32x5/tier1":     "a35e3049004a7f03cc58f444ed6a0958c181475fc1d812b079938507dc4b23e3",
	"cf32x5/tier2":     "b8092c2b9c0537b420dc9e7e91f296dc09d991db33532ce27eec984ee5d06c48",
	"cf32x5/tier3":     "5e86c588a4a651e9df079e2a76f32eb969f5554bb78bb13ec55db017c9dae361",
	"cs16/tier0":       "5a536df034e3fb6e6515f921c750c1bb4a387697ea46e35412dbec0128dd96d2",
	"cs16/tier1":       "fefa50cebdc9402741b32427ed89f4aa70975175e25ea3bd89c6dd92728cdfe6",
	"cs16/tier2":       "45edb8f9c9cf5596225afddb54ac8aa667ed616112562646870143dcd1e28f3f",
	"cs16/tier3":       "599d18016df8a4936c3a9b703332e1efb03257498815ef9cf1219252ee6e8573",
	"cs8/tier0":        "9412c5f8332e5d36b26585040eab737a11f63cacee834f4eaf56b837270ce072",
	"cs8/tier1":        "d8556d978f0ad134bb335bae07ad245083f1676ebaf572cf33beba1043b1478b",
	"cs8/tier2":        "ca51cabf2ce40aabdc6a74419cf41c7bce80aed61af9f1b4363164228dceccc3",
	"cs8/tier3":        "1ff1777bce0c90d9b40baf62945a0e9b38e005e8f34eceac7597231e2229a16c",
	"cu8/tier0":        "4e74fc806f30eae7e2dc6ca98fd3fc7f539c711ade71e7e79d6268028fb10407",
	"cu8/tier1":        "3c9fc437681c3c795f1eedabf6cc0be7521823d0e39998a118a9e9e4ce407bc7",
	"cu8/tier2":        "bb076b49ec686025ef6571fa11c7d17505af08aa1fc7f8463b30e0ab8da6d228",
	"cu8/tier3":        "85270287279d5bfda8f69b00d057373a6f1608b7b419b4f29b743238eae2f716",
	"cu8low/tier0":     "17e3c8f85e1b81f71f1e6ad803940af7b05cfd5df51e1f52991b2b31fd986f5a",
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
