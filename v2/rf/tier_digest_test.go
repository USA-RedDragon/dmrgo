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
	"audio12000/tier0":     "2376198486a05e6694ec5d6769a4c38ea81dd6a7c354e31daf4ec74ed4024661",
	"audio12000/tier1":     "be58c8a7080eb37c7f85be48f3eddfb6d56b744e5339e162cd5f14db7c8ce7f4",
	"audio12000/tier2":     "7ddd314601ca8c4a7f621cb37f09a03c7edd11bf3f9a9c4aba421ce47e746829",
	"audio12000/tier3":     "2d12002e97edeb0dc51fbac336e218406976ec4413d0a9fa38bded783b72c276",
	"audio24000/tier0":     "d1205ec5fdcbcc4de3005f0c0a7011ed3cd235305bfea7abbf9b1a027708e01c",
	"audio24000/tier1":     "ea5f1a751dcb192842995f5e56a1c2905a26e5f2ad0b6e128ba815e2be19b124",
	"audio24000/tier2":     "f440296c24936eced3e01ae2865fe3d189ed670ce08872ec9cdc56406be9d30e",
	"audio24000/tier3":     "6b3baf410341a1547d0aa890987dbf713b04cb26601d3ec021cd6f9ef19664e0",
	"audio44100/tier0":     "920d2c222e21ab9aa4f229c30b72840bf478d62489086ebd5160daa05f468690",
	"audio44100/tier1":     "b40ffdcdd353d5c0d6fd5fa3755dc6577b511568dc5e2dce04a2162cf60c0f68",
	"audio48000/tier0":     "58c925fe7eff9c1cc24d5882ecf03c12533160ee7ebcfee8005a5aa14bc8f279",
	"audio48000/tier1":     "3d1f26d7b16f8a59816896629f78df649600422d5fbe02e0059db86221ec73d8",
	"audio48000/tier2":     "f8d59b2730fee8efe4712a98c82d2b7dda72bf7fa4de065f0d52831ed42a59b8",
	"audio48000/tier3":     "6476a920418ffaf98d1e6ba95309918aa2d7b0c2e72a5b349d3c64dca2d81b9e",
	"audio48000weak/tier2": "39a0ab0dd75a3c19f62f8092c12255f468857033543925580e48f6a2a61fc018",
	"audio48000weak/tier3": "ecad5931c378392c54c08182d6676ea4d3641537fd00b03e38776248605a53c3",
	"audio9600/tier0":      "2300dbb2f00d479e613860a2d9ac39d342288c2da516d56282e161d2cb58d832",
	"audio9600/tier1":      "a0d096dada293e016939971c949993560840f08009c9ce9e18f288520c0af84c",
	"audio9600/tier2":      "9e97182ced735632ede7839af5502532b93f2acd41f003dc052ad8a37ddf9d12",
	"audio9600/tier3":      "ebdbb102a20df5597805da4f4b0d15cf0c4137a96c082e4dec582b2b5b319ded",
	"cf32/tier0":           "52a00dff2ae7859ea60e9edb69aa32f8a167ffa2e8993e269f0081b17a7b503e",
	"cf32/tier1":           "c44e5b8aea8c5b7edea17b8a86042e5fc65ba54fa689856fdef478a2ac2ac4a2",
	"cf32/tier2":           "1c2df8d4b5894d8eed62ee9a77270a64a195430facdb0925d0979a502f819c44",
	"cf32/tier3":           "20815f4127be252959d7c30f5d235fd4a9f7058b048ac648ca309951ddd2fbdc",
	"cf32x5/tier0":         "44987d841f3f4e3ee3c0754e78e4cee650af044c3ac89aa0ae07fb1c1927ccce",
	"cf32x5/tier1":         "7a09373058fd9a26d72bcaa17fc57f8bf70b0bbb19824cc419a0b5561b26d8fd",
	"cf32x5/tier2":         "1c47035b75620bec441fa05f605bc7c12ed7659b4241c033c9fbb6618dee2b1e",
	"cf32x5/tier3":         "15121da7c81f7976603c8c96d193c7e8ff903929b7b234f219da6291fe41bc35",
	"cs16/tier0":           "1a9bfb117400aa5ce49a3d6f289a2dc36ddf20d93a6fd198ccb184881d352394",
	"cs16/tier1":           "1023d37073b32b4c02f78d97f8e1604b4fb2a90fa5dbd7c7fdcd8b781fa7fc98",
	"cs16/tier2":           "506d71edcbddd887f10bbcbeb1940b88feec0c7aa346002736102ca705dec2a1",
	"cs16/tier3":           "2146c67e4e218dfd068f9edae928bbced3b19d3c1ba3e16345f52d162b08f8ea",
	"cs16low12/tier0":      "0d32665c0e6671b26b5d62aad7bbd5c8e670abbe245c957b4198823a06a03ee1",
	"cs8/tier0":            "3e31319618c48fb0578b033c52b41c30773274f4db420444da492dc9814de715",
	"cs8/tier1":            "23d20f9aeb61a0345c3951fc39d3188de7bef08c2984ff6ce5774a60ef5efb55",
	"cs8/tier2":            "fd1e16d62f8b004d7e834d09b918ee3f09fec5c62203edc0b68f07153bdd575f",
	"cs8/tier3":            "987fb944c8fe39d93e95a5f3302cb316bd97bb3f62292a3d4fd64ec2fc7e621b",
	"cu8/tier0":            "84cb14fb01ed488c408258b4aa677b1e855bb45fa2eabe98e2da4e9e9fb9079b",
	"cu8/tier1":            "cabf89e483b56fe234474a4f1e5b66e35d5a30ec0e5b5e268d417e4fe787b022",
	"cu8/tier2":            "4e8a23d87eb928e53da62ccd4f01bc5c427e962f4c26556f155ad3ac4aad8e9d",
	"cu8/tier3":            "0811eb0d672fe36dfd65ea393e8136f4d72feb000d991433b6447d1dc4133707",
	"cu8x2048k/tier0":      "8e9b0604a85d5bb37804c92f5cfa25f6e71ab9ce01407fd0229fc3133830cb68",
	"cu8x2048k/tier1":      "9e7cfd9fcc02d8490898c48d9bd6115ef39fefac6cbcd7b90791b1a46ff64c85",
	"cu8x2048k/tier2":      "2882e22eb791fc03f7760238a102019b154190f30088ceba2375b53e5a9abb21",
	"cu8half/tier0":        "7a93a1f143a058123aa968a01357516b6700582d9ea2a8e4e3890c274b6fc29c",
	"cu8half/tier1":        "0e928770a889b2a2af5b40e8e4032373fc9d7ff5b79ce2e2e7b1bd286403afab",
	"cu8low/tier0":         "b77f984d8fe72e3d8db096b3f33b07532861255b04c922dd46758fb637b70fe6",
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
