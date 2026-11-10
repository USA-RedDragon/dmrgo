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
	"audio12000/tier0":     "b68d01230bc611f272742642e13ce73a5fb670c22053ee14f40a79f8bc387a02",
	"audio12000/tier1":     "be58c8a7080eb37c7f85be48f3eddfb6d56b744e5339e162cd5f14db7c8ce7f4",
	"audio12000/tier2":     "1d21c522a2c334f0d9c0f804db28b9c1e8bfd2d82d68e476a3e5962055afa748",
	"audio12000/tier3":     "49b21319161761093a33ea51d0904d41cecc9b70a2fe49acc7979b9d858ee7fa",
	"audio24000/tier0":     "129ccb8b29da62e4356504def3791493e7768edb03115061506402231ab06a14",
	"audio24000/tier1":     "ea5f1a751dcb192842995f5e56a1c2905a26e5f2ad0b6e128ba815e2be19b124",
	"audio24000/tier2":     "6de3e5146da1b3733ddc25307003329e964954152444ff3db75847e13c01c08f",
	"audio24000/tier3":     "1854674f297b5e874fce04d3394a10a242773ee665da20bc17a7118ecc0424a1",
	"audio44100/tier0":     "6342b07dd5115be43b0a4da079200dcac38226f1d619dce8829f09d9928f61f2",
	"audio44100/tier1":     "b0feba74442acd17f71a2126e2022ce46c7d9127620a038f78ed6863845f5b43",
	"audio48000/tier0":     "118cc692b6c2e1a1e2c6d915afaf1e56b3cb08bfcb94bc9317dde243237e984f",
	"audio48000/tier1":     "3d1f26d7b16f8a59816896629f78df649600422d5fbe02e0059db86221ec73d8",
	"audio48000/tier2":     "9608066ac303c687a7d33c07d6193c012c47422548a6d84b53d043fcc1d34dc1",
	"audio48000/tier3":     "4a8a1eb76ec2316053269962340db7337022db0ab228d776d349449636ca1ce5",
	"audio48000weak/tier2": "42bf3ce5ed1c6fc6c68271bcdc9250efa8bd5e96548a3b2b0c6409f109e56099",
	"audio48000weak/tier3": "a252a1a22a3514e35496834bda0c8765734fa89d697cc9e86d431e27687847db",
	"audio9600/tier0":      "8e50fd97a689470dcc7fcf3849bbbfdf2ab1ec8086aa785b604b5a1b74040132",
	"audio9600/tier1":      "a0d096dada293e016939971c949993560840f08009c9ce9e18f288520c0af84c",
	"audio9600/tier2":      "b79a005a56442f734a0d0c63ff6873f8e7712f71096c52ad387a72d1275cba6c",
	"audio9600/tier3":      "008438279a8b43efc4dbe182f6b6adb55151e3273f32d3e82fc6f041f74aaf93",
	"cf32/tier0":           "ef86f3a9244c16d7bd7016d81b405fab47db98e006d381c2b69d03018d5aac7a",
	"cf32/tier1":           "7129f2939b2a9055b02017b1d552feb955387fc4e0c68e0085f0396ad6f0c572",
	"cf32/tier2":           "f93189d751d8253c228cc733a5348846ba5940e1c46224960f5c82de0f7c5a04",
	"cf32/tier3":           "20815f4127be252959d7c30f5d235fd4a9f7058b048ac648ca309951ddd2fbdc",
	"cf32x5/tier0":         "39395e5999c70540bb9c4abed01f824833fc854661d74dfeb980d469872e3035",
	"cf32x5/tier1":         "7a09373058fd9a26d72bcaa17fc57f8bf70b0bbb19824cc419a0b5561b26d8fd",
	"cf32x5/tier2":         "1c47035b75620bec441fa05f605bc7c12ed7659b4241c033c9fbb6618dee2b1e",
	"cf32x5/tier3":         "15121da7c81f7976603c8c96d193c7e8ff903929b7b234f219da6291fe41bc35",
	"cs16/tier0":           "fc6d388ef4f53ef7538f2fc9f87370f335a557ffb8617856c73b46d5244b469b",
	"cs16/tier1":           "241345810757e8f6d73f508158eadd5d8101a8d0f42b41c1a6f9d180a11fb24d",
	"cs16/tier2":           "47071c7a65f8140369a917711d36bcd88db4658270ebc8fa699d42e7c89d60f8",
	"cs16/tier3":           "2146c67e4e218dfd068f9edae928bbced3b19d3c1ba3e16345f52d162b08f8ea",
	"cs16low12/tier0":      "c906f9631692a05826dcccbb01aad50111837a3265c8adb76bfd6a4c16d5c4bd",
	"cs8/tier0":            "abce205d10f24a07bab496af458912c4a30a038bdba89f6e3ce66ed08a469507",
	"cs8/tier1":            "23d20f9aeb61a0345c3951fc39d3188de7bef08c2984ff6ce5774a60ef5efb55",
	"cs8/tier2":            "fd1e16d62f8b004d7e834d09b918ee3f09fec5c62203edc0b68f07153bdd575f",
	"cs8/tier3":            "987fb944c8fe39d93e95a5f3302cb316bd97bb3f62292a3d4fd64ec2fc7e621b",
	"cu8/tier0":            "2f0d0073bbfbe807b9c3860f4c7d571a3c65ab1a5bc66a2a91482465399184a9",
	"cu8/tier1":            "cabf89e483b56fe234474a4f1e5b66e35d5a30ec0e5b5e268d417e4fe787b022",
	"cu8/tier2":            "4e8a23d87eb928e53da62ccd4f01bc5c427e962f4c26556f155ad3ac4aad8e9d",
	"cu8/tier3":            "0811eb0d672fe36dfd65ea393e8136f4d72feb000d991433b6447d1dc4133707",
	"cu8x2048k/tier0":      "8d2be923acc1f9c8ff1a3d1c1952f7da8c185f65f2fe0b7fc7af4d192006629d",
	"cu8x2048k/tier1":      "ae07ac6df3f1a7b6561a363994c9c330ee9968508dc7e1463ca849cb8629e75e",
	"cu8x2048k/tier2":      "2882e22eb791fc03f7760238a102019b154190f30088ceba2375b53e5a9abb21",
	"cu8half/tier0":        "3c5278e18d06bfadbf494ee197694d4644d24d5f21dcb58e2e813890bc8a4b82",
	"cu8half/tier1":        "abf45459882205e0899d2712ddf9cbcd2ee58d1f5a2ee7d8fa4e817fa3936e66",
	"cu8low/tier0":         "136de71114dddd5b4f169203cb809a6d9ea4d35c45ba6c2eea2b3ac9d7610284",
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
