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
	"audio48000/tier2": "73556c9104d3967e5f6c1b2fdfe31e6095d7e9052d41e5e3f9e27dfe93c9fa7e",
	"audio48000/tier3": "73556c9104d3967e5f6c1b2fdfe31e6095d7e9052d41e5e3f9e27dfe93c9fa7e",
	"audio9600/tier0":  "f3ea669632acc70c691760246a1a33ab317466ce834f584a1a267aa5d617d161",
	"audio9600/tier1":  "b8e10e65b217aef6230366b406e221e19ee96b1a8cf4187b0332bb425b42c606",
	"audio9600/tier2":  "385336c2b7d6bc0b833889160b9bb402992f76adb89119f61638de0f5023994a",
	"audio9600/tier3":  "385336c2b7d6bc0b833889160b9bb402992f76adb89119f61638de0f5023994a",
	"cf32/tier0":       "238152fc1752acc873e7470db25ad8f77642a7c6b21256847c196b3780f5bca9",
	"cf32/tier1":       "9bcd2f3adbb251da3279737f00e6332be8df7bdd772e41c8769a45b3bcde791b",
	"cf32/tier2":       "91d3cabcb7b8c3072b8521aafded8ef9996df24c792aeae149bcf7bf6dcf429b",
	"cf32/tier3":       "2c43051d5a69784d42bf16cd2921d2a69174e98614b341240ea32bd190e69704",
	"cf32x5/tier0":     "937440068b1a5d0e114c376b954dd84d7dbdb327eeebb12f21ce6177f86201ab",
	"cf32x5/tier1":     "0982de1029c52a6b44ccc1559f30c270f72f7d858d37492a63858b1d41823279",
	"cf32x5/tier2":     "b35fe5bed548284947a6195fc57646b45ddf99a1e6a04be313446de5279b3c4d",
	"cf32x5/tier3":     "c5e0919e9811230441bd41b5be4ff6c966b825240a51f4e3f8221bf3c5adb6b3",
	"cs16/tier0":       "5a536df034e3fb6e6515f921c750c1bb4a387697ea46e35412dbec0128dd96d2",
	"cs16/tier1":       "66cfcf58fd19603a01cfa9f3efac3a9f4c109fbac2a3da1b1cb6d1dc39caeb07",
	"cs16/tier2":       "3e5a164a25e9c45330d78a22e729b666cbae08b39695ba739ec7b130d652de82",
	"cs16/tier3":       "5689b58340fefe862227a53e3bb2679e4932a4f66c54eea94b7640853e1321b6",
	"cs8/tier0":        "9412c5f8332e5d36b26585040eab737a11f63cacee834f4eaf56b837270ce072",
	"cs8/tier1":        "a300da444036e881ceae81c82e708fc96147e5b26753ec9e157b49c97e245f1c",
	"cs8/tier2":        "bdbdf6f695291b35955fc4ac1253533a454d682db8588b90c7ad39b27422c218",
	"cs8/tier3":        "a8b1f2854c1afbadd5fa8a58f6c571aefc8dbcfb884ace3903a1a428a1437a92",
	"cu8/tier0":        "4e74fc806f30eae7e2dc6ca98fd3fc7f539c711ade71e7e79d6268028fb10407",
	"cu8/tier1":        "028758aee95dbf2dbcfe702fbe5db47efaa7d9535dab06d651a3896c6c1f2676",
	"cu8/tier2":        "225c3e8fb5d213a8e52ce7b1d24c8d0fdf6e1465f5450f1a73d6c48f9ad3dcf6",
	"cu8/tier3":        "bb56a18a21da887e16818b20724bda1c14b5ebb9c1cd1c1dc61438c907209e3a",
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
