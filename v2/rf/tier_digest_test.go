package rf

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"runtime"
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
	cns    []float64
}

func tierDigestInputs() []tierDigestInput {
	all := []Tier{Tier0, Tier1, Tier2, Tier3}
	return []tierDigestInput{
		{name: "audio9600", tiers: all, rate: 9600, audio: true},
		{name: "audio12000", tiers: all, rate: 12000, audio: true},
		{name: "audio24000", tiers: all, rate: 24000, audio: true},
		{name: "audio44100", tiers: []Tier{Tier0, Tier1}, rate: 44100, audio: true},
		{name: "audio48000", tiers: all, rate: 48000, audio: true},
		{name: "audio48000weak", tiers: []Tier{Tier2, Tier3}, rate: 48000, audio: true, cns: []float64{2, 1, 0}},
		{name: "cu8", tiers: all, format: IQFormatCU8, rate: basebandRate},
		{name: "cs8", tiers: all, format: IQFormatCS8, rate: basebandRate},
		{name: "cs16", tiers: all, format: IQFormatCS16, rate: basebandRate},
		{name: "cf32", tiers: all, format: IQFormatCF32, rate: basebandRate},
		{name: "cf32x5", tiers: all, format: IQFormatCF32, rate: 5 * basebandRate},
		{name: "cu8low", tiers: []Tier{Tier0}, format: IQFormatCU8, rate: iq0LowHz},
		{name: "cs16low12", tiers: []Tier{Tier0}, format: IQFormatCS16, rate: discRate / 2},
		{name: "cu8half", tiers: []Tier{Tier0, Tier1}, format: IQFormatCU8, rate: discRate},
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

type tierDigestFeed struct {
	audio []int16
	iq    []byte
}

func tierDigestPrep(in tierDigestInput, x []complex64) tierDigestFeed {
	if in.audio {
		if basebandRate%in.rate == 0 {
			return tierDigestFeed{audio: discAudio(x, in.rate)}
		}
		return tierDigestFeed{audio: sincResample(discAudio(x, discRate), discRate, in.rate)}
	}
	y := x
	switch {
	case in.rate == iq0LowHz, in.rate == discRate/2, in.rate == discRate:
		y = lowIQ(x, in.rate)
	case in.rate != basebandRate:
		y = dgUpsample(x, in.rate)
	}
	return tierDigestFeed{iq: iqBytes(y, in.format)}
}

func tierDigestRun(in tierDigestInput, tier Tier, f tierDigestFeed) []Burst {
	if in.audio {
		r := mustAudio(AudioRxConfig{Rate: in.rate, Tier: tier})
		a := f.audio
		var out []Burst
		for k := 0; k < len(a); k += 1500 {
			out = r.AppendBursts(out, a[k:min(k+1500, len(a))])
		}
		return r.AppendFlush(out)
	}
	b := f.iq
	r := mustNew(RxConfig{Tier: tier, Rate: in.rate, Format: in.format})
	var out []Burst
	for k := 0; k < len(b); k += 3001 {
		out = r.AppendBursts(out, b[k:min(k+3001, len(b))])
	}
	return r.AppendFlush(out)
}

func tierDigestWrite(h io.Writer, b *Burst) {
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
	sigsAt := func(cns []float64) []sig {
		sigs := make([]sig, 0, 2*len(cns)*len(conds))
		for _, sn := range []string{"bs", "ms"} {
			for _, c := range conds {
				for _, cn := range cns {
					key := fmt.Sprintf("%s %s %g", sn, c.name, cn)
					sigs = append(sigs, sig{key, rxBuild(streams[sn], c, cn, false, rxSeed(key)).x})
				}
			}
		}
		return sigs
	}
	strong := sigsAt([]float64{40, 12, 6})
	ins := tierDigestInputs()
	parts := make([][][][]byte, len(ins))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for i, in := range ins {
		sigs := strong
		if in.cns != nil {
			sigs = sigsAt(in.cns)
		}
		parts[i] = make([][][]byte, len(sigs))
		for j, s := range sigs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				f := tierDigestPrep(in, s.x)
				parts[i][j] = make([][]byte, len(in.tiers))
				for k, tier := range in.tiers {
					var b bytes.Buffer
					bs := tierDigestRun(in, tier, f)
					fmt.Fprintf(&b, "%s %d\n", s.key, len(bs))
					for n := range bs {
						tierDigestWrite(&b, &bs[n])
					}
					parts[i][j][k] = b.Bytes()
				}
			}()
		}
	}
	wg.Wait()
	m := make(map[string]string)
	for i, in := range ins {
		for k, tier := range in.tiers {
			h := sha256.New()
			for j := range parts[i] {
				h.Write(parts[i][j][k])
			}
			m[fmt.Sprintf("%s/tier%d", in.name, tier-Tier0)] = hex.EncodeToString(h.Sum(nil))
		}
	}
	return m
}

var tierWantDigests = map[string]string{
	"audio12000/tier0":     "03025a452b2cee14e05a1c9d9fd1f474223f5c6632c66453913947e40307ea0d",
	"audio12000/tier1":     "4c19c2083bed6d107c8c49f2627aa580a80f9cb52dc63cd48eeb34056ef8e005",
	"audio12000/tier2":     "eb09d16af58183b6a7aa949850c80ae8e979dc9afbd74e175ef0a21adbccc246",
	"audio12000/tier3":     "eb09d16af58183b6a7aa949850c80ae8e979dc9afbd74e175ef0a21adbccc246",
	"audio24000/tier0":     "7394b11c8f5fb198c62d881d64094dd5d129a4b292144b0430a07077ea5ef778",
	"audio24000/tier1":     "5fa3e0bf03e9444fecd31b051f0b8315add611042c83d67e3167a19bcb9aebe5",
	"audio24000/tier2":     "335c1dd8236495a2dc83187b3034c9f32ca0b724e9e4c126cf6616d205108a65",
	"audio24000/tier3":     "335c1dd8236495a2dc83187b3034c9f32ca0b724e9e4c126cf6616d205108a65",
	"audio44100/tier0":     "97b787f207c5ee71e2bda19ac673227431d4f25c80a91bbc94888ac5bb65117f",
	"audio44100/tier1":     "fc96d36f03c140b6f4320307db8cfc310cde4c7c10f727b3f0554c31cb29db10",
	"audio48000/tier0":     "fea78872e1fed9e7f6f93686ac40bdcad857917cc523322cd4eff7d57c14e669",
	"audio48000/tier1":     "c7728ae53f8b3cae69e009657e33f01b198e4abf83fd94ac934d5cecefd43924",
	"audio48000/tier2":     "6b8b8f8e1e38407661d59c7bd8c022a1d547579dd6f009016d02d2ff12f8a2af",
	"audio48000/tier3":     "6b8b8f8e1e38407661d59c7bd8c022a1d547579dd6f009016d02d2ff12f8a2af",
	"audio48000weak/tier2": "fa5984c5d9f4670c22360e6b36daefb78082e0498684eb3482d0a03e270b28d0",
	"audio48000weak/tier3": "a346df4bfd3396cfda48aa81bc8143e319d9fb9cacc58b348b17fb8e7ff95220",
	"audio9600/tier0":      "833026564df12d4d710fdbd8f08a35fe3181b64c362cd81e449f2abfc14cd995",
	"audio9600/tier1":      "7f2dfc627fe88352184697dc7135a5e7941325acdc73ba0f6d85f7937d358f04",
	"audio9600/tier2":      "ab1e3d4a05b26dd25c475883fed9d3aacb59716ccfd6a7b6aca67974dce35382",
	"audio9600/tier3":      "ab1e3d4a05b26dd25c475883fed9d3aacb59716ccfd6a7b6aca67974dce35382",
	"cf32/tier0":           "3c60025caedb34d140daf4e529bd98ef1a76217264242a9ccad2af6ee04f9e79",
	"cf32/tier1":           "af24c117b76630c18828226a5078bf138cecca15dfc82c95ca558930a8d53395",
	"cf32/tier2":           "87336a9a33c8c608d815251182c90e6fdda79fed58cc4fa6f34f29ad8694cf2b",
	"cf32/tier3":           "aa0a592fdbf934dda074c586375a5a1900be0cf1e17b5ee774d96a8e41cc5e78",
	"cf32x5/tier0":         "937440068b1a5d0e114c376b954dd84d7dbdb327eeebb12f21ce6177f86201ab",
	"cf32x5/tier1":         "8cf220570e53a4955dfdc61c57732dd42daf99d7b7453aa9a4d68da78aca7ba3",
	"cf32x5/tier2":         "91c4619d1733076512c70946234c98ec440cea09ff638562d6fda97e25618f26",
	"cf32x5/tier3":         "354561bf7a2456e02dad1519b15f7ac8d44b680cc376c50453c3b1a253f4c879",
	"cs16/tier0":           "106b727d4e47bbe3c4976b38b7746d43d732b02728ac4474c8dea1875325c9f1",
	"cs16/tier1":           "bbc36b3a971fd360bfb3f726f7cbab94b6c4e7b15243626e0d6ed9cca206624a",
	"cs16/tier2":           "e5a40bcbb55b013cca27276813056129bbe07e184af30bc2c949735775f1dae0",
	"cs16/tier3":           "5f43ca483fb10af893a5ea7163d07ed93d931d506a6ba7c371ff0a76b5476fa3",
	"cs16low12/tier0":      "6596640d0950ad83c580909f907a66b457892ecca247ad099916907271b083e0",
	"cs8/tier0":            "9412c5f8332e5d36b26585040eab737a11f63cacee834f4eaf56b837270ce072",
	"cs8/tier1":            "644c6a07a005b6332bee1d122b0cf069d74daf7bbec78006d56cc8a55ba1f667",
	"cs8/tier2":            "3b5f15b207839cffefb3e38d1c8fdd8df088870b443f08b4c5e3edbe30a09dcc",
	"cs8/tier3":            "12c84fafc27211d5a5be100286edfd9f199580dd5e42cfeb6bf58359ee2f3ded",
	"cu8/tier0":            "4e2c718014038a7dcf4a90142c075b43069d1c11ab6523cf716b19c1f62f3855",
	"cu8/tier1":            "e9d0b60588ba4e83efa0c3bfc184b260ef48aa1736caf3169501c07c4a0fe0c3",
	"cu8/tier2":            "9d17c6925e9f78959c7f86e76ec7a1f2cd03d9150eb3624b001a1c0a51748bca",
	"cu8/tier3":            "de6b9c2de38e734093c3ad07df087b19ca955fc09717e6ba3df0130731c2dd41",
	"cu8half/tier0":        "cf564b9c0cefa7c94fe5c432e25027193492b0b06dc2e3328f2bd8c2b9db14d0",
	"cu8half/tier1":        "7e9365660c21da99834a4a9ea5e4a0f9706f1230f9a61f17bf85f4f3dbd28959",
	"cu8low/tier0":         "d20d0e413f1ee1cfd71745425f5248a90ae300ac93a3baf8e02c4c9d940d9ae2",
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
