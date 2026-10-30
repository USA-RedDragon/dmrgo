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
	"audio48000/tier0": "4679b5868ccc8730ca53ef1546c7827cd2f99f8efed2d60a8986f9c0f5c16562",
	"audio48000/tier1": "5d2f67b6e333202ea2006b57f250f51901751fe137b6caca7ffe0a5e0aec858b",
	"audio48000/tier2": "73556c9104d3967e5f6c1b2fdfe31e6095d7e9052d41e5e3f9e27dfe93c9fa7e",
	"audio48000/tier3": "73556c9104d3967e5f6c1b2fdfe31e6095d7e9052d41e5e3f9e27dfe93c9fa7e",
	"audio9600/tier0":  "23bcd7c132e41de0dd2a7f09e4ff9f86efd61200db529808c8ba82fcf82df93f",
	"audio9600/tier1":  "615d0d64d8bb58d77b1e2e81d5afc3e11ae7f3eaa7134f29d389fbd0bf1d9ea2",
	"audio9600/tier2":  "83c629011520303334bc705bf025f45c9ca18c6416a93cf45cf5e50ecf5381a2",
	"audio9600/tier3":  "83c629011520303334bc705bf025f45c9ca18c6416a93cf45cf5e50ecf5381a2",
	"cf32/tier0":       "3c60025caedb34d140daf4e529bd98ef1a76217264242a9ccad2af6ee04f9e79",
	"cf32/tier1":       "629aead25f84a8d732d8cca93badba7b97eb910b1697b81f0391187f36f3a2e5",
	"cf32/tier2":       "7ae3886fdb918bf263ae544080c5cc42dd2548b35dfeecdf4cf11bd4ee9a9326",
	"cf32/tier3":       "2c43051d5a69784d42bf16cd2921d2a69174e98614b341240ea32bd190e69704",
	"cf32x5/tier0":     "937440068b1a5d0e114c376b954dd84d7dbdb327eeebb12f21ce6177f86201ab",
	"cf32x5/tier1":     "0982de1029c52a6b44ccc1559f30c270f72f7d858d37492a63858b1d41823279",
	"cf32x5/tier2":     "db941ded8e25694dcf50aed5589a5d539bf9ea29cd54264dc0bfce39f29335e5",
	"cf32x5/tier3":     "c5e0919e9811230441bd41b5be4ff6c966b825240a51f4e3f8221bf3c5adb6b3",
	"cs16/tier0":       "106b727d4e47bbe3c4976b38b7746d43d732b02728ac4474c8dea1875325c9f1",
	"cs16/tier1":       "63bb020e226972e601cb2d14102df39aee327be0cfaa56e08167e922bd010a6d",
	"cs16/tier2":       "9b71c1b26f6ffa1076d1636582916faabad42f8aba295b69e5e396ddab93d3c9",
	"cs16/tier3":       "667934bc73fe5dd1eb2086fdba9e60f5c05641e92e3c0717a57d1c6097d34cf9",
	"cs8/tier0":        "9412c5f8332e5d36b26585040eab737a11f63cacee834f4eaf56b837270ce072",
	"cs8/tier1":        "a300da444036e881ceae81c82e708fc96147e5b26753ec9e157b49c97e245f1c",
	"cs8/tier2":        "0b146ec26028c1f253c53dc3dfc90cd94a3047a19fa941079dd8328bc08b80c3",
	"cs8/tier3":        "86ce67cbde2a3d55d46b32819d65a367ff5aa3549a64b9515a3eed8a481a9ed7",
	"cu8/tier0":        "4e74fc806f30eae7e2dc6ca98fd3fc7f539c711ade71e7e79d6268028fb10407",
	"cu8/tier1":        "eb3318f3aa963791fa475fa08dd5492f66f1d04bfd3b7d16ec143c076d63953a",
	"cu8/tier2":        "8ff94d6cd048870a7774f60047a2e5912447e78655a6aebb593c013c132843b0",
	"cu8/tier3":        "bb56a18a21da887e16818b20724bda1c14b5ebb9c1cd1c1dc61438c907209e3a",
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
