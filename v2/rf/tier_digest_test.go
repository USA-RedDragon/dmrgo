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
		{name: "cu8x2048k", tiers: []Tier{Tier0, Tier1, Tier2}, format: IQFormatCU8, rate: 2048000},
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
	"audio12000/tier0":     "be0990b79dccfb3a25a1042dab0e0c881ef485f604db2b0f4cfff896bd616721",
	"audio12000/tier1":     "17d20d74f2306032acf4cc9433927ecc347a676cbe765cd535a3cf6435f634fc",
	"audio12000/tier2":     "484ab60b6fa1c10a2423ce90d0594e2d5a42408965148efa8813b7eb6c5e53fe",
	"audio12000/tier3":     "249315d002920a26da99edba32db0a03b227f95b076a6934b2e3aea0411c75e0",
	"audio24000/tier0":     "96b357d56f74da9780be5611b88d1a54ac35355e86226711887a7231282d38b9",
	"audio24000/tier1":     "bf45d2f585a241e238bfa0cd3713a00113b78ddfdad7e70f29c8efda761f20a7",
	"audio24000/tier2":     "4a666b97f311ecff2eb0b421f354d3bb5f5651cb14bf7a000d9482fbf4699930",
	"audio24000/tier3":     "f5bab1667623d28c0d6dd702a6ba2fcd6e9b82ea4aebcc60bf43e9494f40916a",
	"audio44100/tier0":     "045fd3bff7622cb68fa3b8ea502061c7fb08a2de6f8c5e47dd47c84d6fe983a1",
	"audio44100/tier1":     "de22b5044823109507c7c5e7a828d0189cbdb0c682884fe18ccf449eb5b9fa9c",
	"audio48000/tier0":     "18a95448926306f03b8e34922cd97c3d7c4a4a73db5b301a266564e11ed2c699",
	"audio48000/tier1":     "88a81d570b3449de2741e24e09fa880a8f0d05502f6b67d24caf6294d91cddbb",
	"audio48000/tier2":     "ad0267373c58ac1c74c229eea9de3cfddff46ebc5c1aa1e7893688acfac137da",
	"audio48000/tier3":     "68ce6077804a35194a06aa959bfe1700bcecce3384282d04ac7e6eb6e76441e3",
	"audio48000weak/tier2": "2b0789f40bdfd2ebe5dfa23f04de86a74acb9a445b3f44bbb945299eccca6ebf",
	"audio48000weak/tier3": "d1547b2bd51673f8f335b220f044c814ff84cf1dab95b27e5684a5ccd3ee9ba7",
	"audio9600/tier0":      "72b5859db4bc29ea5ceb5cf99728080da590a97b5bb43e3416e581d705a3ec05",
	"audio9600/tier1":      "1de18c783abf83e05c7bd367694836bae73df70973070251baa7aba5702d3026",
	"audio9600/tier2":      "fb7597645f946b02da8faf32b7ea16a07dd3de67a87ca92aab8f0490f1ff695c",
	"audio9600/tier3":      "0e1af861ddc063f8b822d16cc2a6cc636540613aadc271837fb6f7bbf61fcf4c",
	"cf32/tier0":           "42952a8f98c328648fa0f7f5338feec74201297f3023daf6fe829b8f1b81b1af",
	"cf32/tier1":           "a5b96f8d301a471e6edf1aeb2b789a00bbc0a434d36c8092b310fafca87b642f",
	"cf32/tier2":           "73e7051baae5fce1cc373154e7f24aa89a4ce40fe529f273981c93a581012462",
	"cf32/tier3":           "2c089b5967a34793d5ffe1e5e90159eacb739f1542bd1bf653c50572f3c67a96",
	"cf32x5/tier0":         "d5d9cbf2d98f1ca08e45ac67aa39fbe4a24cf97f598ac27dc6d6af8682fa9139",
	"cf32x5/tier1":         "749811b8117119e35abac353211c5b7ef052ecad80cca861c24c6ca61dd8cc4e",
	"cf32x5/tier2":         "4d7e53f895fda0f7836af907697813628148af9ee939c7d47c8e5d05df741b30",
	"cf32x5/tier3":         "67de56277da09e5187d6a593219acf27039380fa70334c2037271ef41ad2c221",
	"cs16/tier0":           "086a72a3a8ad49b3205c02ff06338213c707726ec9d097559b88f74a92210253",
	"cs16/tier1":           "49aefe835e70b1bcd2ba201cc72e564eecf5ad504743fad18cd6142914798f87",
	"cs16/tier2":           "c2e5cff554dbf15aad7e3ccf4621bd7f3ad0937092be9606caf60dbf400313a4",
	"cs16/tier3":           "5e71f623a3fac6ebffb0013ed11a4102b1cae669b2a381afcddffec583070ec5",
	"cs16low12/tier0":      "39dc8dc9d3f540c0aed524827700d51b2182e5f0c77e22e84ee34c429a83847c",
	"cs8/tier0":            "4403095158941496a9d8fbd487ceaca26f1186d069035a1f04e2508c20f2373e",
	"cs8/tier1":            "2edfc65e42c4c4c38f827eb0c770863aa2933954d8562e971c80c2bff8ea249a",
	"cs8/tier2":            "5e63cf9edc70acec943f17f989da38df8c2f76207f84726691b65cf5f2a917cf",
	"cs8/tier3":            "58a29513a7fdd855b9d8e902d45ece5f8f61133adedc98691b4a72a2ffe69ed8",
	"cu8/tier0":            "b1c15f7bf5e1d759bd2c1305a1b4a11bbb53f2b61f2b013a342747f91fab4db9",
	"cu8/tier1":            "5540674ba162b546313632f884f511f93afeaba7519d2208155029d22d662ba1",
	"cu8/tier2":            "d76282a35f889e7f7a5157a57a72addb4a3d07edbca75905dc272a32e40a8f03",
	"cu8/tier3":            "01e3fd283e602b8da8d049c6b25fbeade4cdd3d106a76a6c1ac01dc6ecc3617b",
	"cu8x2048k/tier0":      "5a33221b2d59b0e1da87d84997ae4c983ce463eb8aacb144a868bc0cfe5b5d21",
	"cu8x2048k/tier1":      "e328cc0cd263a7ab85db50c98c0da9591bd28815471b9813852360ddfc4a9395",
	"cu8x2048k/tier2":      "459c55f44420ed083e30baf7b8adfa5c7d341e0dca7b0666efd5abf415923160",
	"cu8half/tier0":        "5205b7851ef353085b8b932a61e2a911ca9066107045b9bdf5ae1f337e382efc",
	"cu8half/tier1":        "2413503aaffaec4c384ad462d18d6238233dc535e20ccb86146a586710e3bbd5",
	"cu8low/tier0":         "24aeacc3e18cffc648a3fbb4ca73b07ad1b8ac022c775c01afcc28653d479453",
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
