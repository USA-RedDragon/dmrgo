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
	"audio12000/tier0":     "7cebcad7df67669befd0f84638e2ab4062299c720849cf42a386089f14239324",
	"audio12000/tier1":     "f1c2ffff58e7a299382bb142fe428a2d7c2381e50432661a15d3e143ff059ae1",
	"audio12000/tier2":     "92787132c9f79442d8a787043cbe2bff6675d304232c2e233d2b706ca0e6c021",
	"audio12000/tier3":     "29155bb2f76399ac18d62765134cb88279c5b7a154620656e0a8fb36b58f65af",
	"audio24000/tier0":     "215cf931f12fe12de8b7d04541226138908f509ff2136153e1f8957e393a34bd",
	"audio24000/tier1":     "4d76390c013dd61741bcd442618c0f8d34da49798a078bcdc7b7e5e6106b62c4",
	"audio24000/tier2":     "6e1775e1aab8329292184d2a84adaf54ddbf1f65af051ebaef9b11cdce5b8459",
	"audio24000/tier3":     "98ce70a1b436f39815845c193c2feffa83a9902c9ef8d86d03de7c09c65f9ad0",
	"audio44100/tier0":     "d333d94f7822cacafddc655c918ec5b2c44aec7217031248ec133c6529d4b51d",
	"audio44100/tier1":     "e8ae71442210cc6405bd6e839a057cc6e3514db7251b6c6015d4175d10f9e00b",
	"audio48000/tier0":     "68fe9867f54309cf9e4efbc46889896873888ac3582bf799d9eb4e882d1810cf",
	"audio48000/tier1":     "2aa5c6d9e3e4b4b2d36cf3ef19f0b732ea7cbf193580f9774ea77c7ee7cb0699",
	"audio48000/tier2":     "cb0b35d935621f8a1e361da10d6d829f577a3826745800a849369a5976b7d4e3",
	"audio48000/tier3":     "e2ded248709fe17cfa314438afcfd653eb83a71cbc349ad93a44449ab6f505a6",
	"audio48000weak/tier2": "5f3fdc24121f6925394d5b6d3099773852082c29b7e03a22cd6453392c769c69",
	"audio48000weak/tier3": "d942cd2e1af4fc4982d000e24cd1e661e3541c6c9bc68e1002d31ff959366a05",
	"audio9600/tier0":      "0fb049c24fb0724af65f2be3d53a00c414091df37c289e8eef40d29edff3baa1",
	"audio9600/tier1":      "7df9bd4bafa832e6f8a45c713a5c9101d15831c6f3523d6bb1554c75854b88d2",
	"audio9600/tier2":      "b85b80966bff0f8b7ced91af15340657842646101c64872546cb79764e3f0d68",
	"audio9600/tier3":      "3b4ad40d0bbed78e762250a14472af865a7bd9fdb99a4bfbab3f2f5da17ec7f2",
	"cf32/tier0":           "164ad0c045f07ade2182bfdcb82c3cfccda20f95bd466a8491e8d09586adac93",
	"cf32/tier1":           "873d59199ebc3967351148a17c994ea4478eafb27bfa9145fffa089a16112414",
	"cf32/tier2":           "a746c999165d8fb4782b9cbd78eab5b58ec86b4337ca37caa1027fae8ab3b63d",
	"cf32/tier3":           "cf9175ed270e5b06c0cbfca0963820a0fe9632e7c0334ef0900fd8ba65fc3fa3",
	"cf32x5/tier0":         "4694fb26af53138c0fa089c17cbb441c62140a2fdb80c1494f910238a2cf06c5",
	"cf32x5/tier1":         "db651ef4887a65f10ad8d3e71637f26925de747bc285daaecd44731621a73ac6",
	"cf32x5/tier2":         "01970e559db6fd396cfc526053d0bce29fe847ef52c39ab38828cae570bf07a4",
	"cf32x5/tier3":         "3e3a09d59f1217fff3e882d5330ded3f868d32ea32e1babbf97ace0cfde70fbb",
	"cs16/tier0":           "7de09edc294a162c226d20d0a39f23e96c27448bdca78bc94bd31a55af2638dc",
	"cs16/tier1":           "6d8d4f4d69837662c81add754f2eeab5b31d13ad4dd3563421f3b6e08e99be5a",
	"cs16/tier2":           "2bd704f256777e555d2f23ad2c94501edfef67401c65ffa75c15b7354f808b05",
	"cs16/tier3":           "cb82cd4a2a604450206f3b515e9d11fcccae530fbddfd39b95c92182f3269d3e",
	"cs16low12/tier0":      "bf42c721778125c9a0d804dd8367fd0720593da2e0a5a6ae1ca479adbddce87e",
	"cs8/tier0":            "caf08f3eebd9caafd713a28de0f79241d2de9fe770fd834bcd22b77946521a2a",
	"cs8/tier1":            "5fac24851edc0373a50e496c202b4c978c970816ac2a487bf8d95f1ff439e21c",
	"cs8/tier2":            "eae7e8186bc7b4542b6eecc6cafcce439137db2741ccb33f0fb881bed87a1fa7",
	"cs8/tier3":            "70085f395fde5d12d022e07f9782946a213d61ccecb695e93ccbaa4d1dd72e80",
	"cu8/tier0":            "06f783a343bbf6d56bce64215824353d2d51fa1d59c65a0b1ccf884c8c5cf2ce",
	"cu8/tier1":            "502f4e67975f3aa0ae3f5bb090f2caa283ec7c05b6e9b6f2b53354e13f62ff43",
	"cu8/tier2":            "1266d86deb1451d8bf59aa244596893b4e2716afd4eb7bb8a85ae6d51f5b3124",
	"cu8/tier3":            "15121235c3b5deecc40bf15c2ea1a6b47a964228d064b69e14460332eadd6c2b",
	"cu8x2048k/tier0":      "ffc9a87c14b58fe04aa14badd022555d10dff52c68736cfa1b1f325904a4cc41",
	"cu8x2048k/tier1":      "b28d1a701ff639ad00442fcef2715c5f446c3dccc4536aad12094481f035475a",
	"cu8x2048k/tier2":      "2c2b60c0820d8a3162b4a0e9cde6a153f10aa5aef8c5896bbe905d4ff7e69972",
	"cu8half/tier0":        "c033433bdd37b902e1dc8869cae72c48a49b60586258b69eeb79ba94f4bb7c7a",
	"cu8half/tier1":        "400e7c5e235a934d75b5bd184b837c2ac6e0e971cf579d1d8f89f74e7b5dfe55",
	"cu8low/tier0":         "f6aa47206f057f25250ab3bc016d59defc7b8479db654e0047eec407f901d095",
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
