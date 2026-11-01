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
	"audio12000/tier0":     "59f3254755cd2bf3e8335b450f1354ae9b314a517894c92ca13b2ae1362f296c",
	"audio12000/tier1":     "7506dd4f0b2e50a185de1bed12e5d414c6b2a3a15675ec3beca07c4afa3fbb99",
	"audio12000/tier2":     "996603401b2be13c57aae16a9ad7af183e5040581211ca3a6a431b980edcda65",
	"audio12000/tier3":     "bff949674bbab588fcfc855de852d345f03855de2872a33cc9c4eed513377220",
	"audio24000/tier0":     "6d9bd1a61749385cb4649032a39d5d28d20d2d4c0d41cc1c911928a125f0c576",
	"audio24000/tier1":     "63701a39e353a14b42bc9839c83ef984f30f155d8f11c3e0eb5dd7f46a530f49",
	"audio24000/tier2":     "b1e44357d59864d936de41903d07b6c4fab3767d7ac74ab4fa59e77d81a05b06",
	"audio24000/tier3":     "d47e1d4281aa6d96c1ffc0fd872baab024bccbfa834968cf17ad45bfa9415a57",
	"audio44100/tier0":     "e480953a2988e1067e9ce4e971780be4b8a5420e093ad86c8fe4274844d53126",
	"audio44100/tier1":     "ba5a649d5921d86107b7bf5c65abee389434e5c7d08cd03741a769e40e60402b",
	"audio48000/tier0":     "b8650fe0b544db8e55c46f2611d8f24724c4b96820075f992fb8db81f7d051bd",
	"audio48000/tier1":     "2938e4ee35f3400715d714a2a3a83637e7ddef0b598f785bc8dc4f53cc2d564a",
	"audio48000/tier2":     "67afa1ee8993dddcbcc850e93edf08478d1196b2dedf5de6cd0ca477f7cb179b",
	"audio48000/tier3":     "bbd9c44dcf2eb82aab5f4193b446d86db78e4b5ece9e30fb095dd48ef0b6c5e5",
	"audio48000weak/tier2": "aecfe1c70195ec6558072ba0158f3d444f0d24fe79ab7902c14520f983d9d172",
	"audio48000weak/tier3": "26e075809b258170cd18862ba6a88ad5438caf509341f2cc4b3422d51c6351ad",
	"audio9600/tier0":      "5ca07dd0ab076f65eaf832fe70532ef81418b9ed89afeb97e6abca12c18078f0",
	"audio9600/tier1":      "9a030f6846d0137de70c1bbab31f67d00d78be637143202566550c009acffcc9",
	"audio9600/tier2":      "83d09823b4df334535434a31ab977a8b7fdc7fa53d9cddea429670dc7e2b91e9",
	"audio9600/tier3":      "010a23264e6d135157ae97ffd75d5cbe4ccf549c5ff96d46b23a9d32fd067e5f",
	"cf32/tier0":           "42952a8f98c328648fa0f7f5338feec74201297f3023daf6fe829b8f1b81b1af",
	"cf32/tier1":           "a5b96f8d301a471e6edf1aeb2b789a00bbc0a434d36c8092b310fafca87b642f",
	"cf32/tier2":           "c90bcc6f2c93a2ba717ac4dbe6342e2f643652af8159c18dd6007a0110add280",
	"cf32/tier3":           "e6580ad54d1698e737c7700ee4b2a846fde7ef63e4590d026a32d52f9ca4ba8e",
	"cf32x5/tier0":         "d5d9cbf2d98f1ca08e45ac67aa39fbe4a24cf97f598ac27dc6d6af8682fa9139",
	"cf32x5/tier1":         "749811b8117119e35abac353211c5b7ef052ecad80cca861c24c6ca61dd8cc4e",
	"cf32x5/tier2":         "33e48469697b7fd8038891961efa1c61dadd8711350d4c55331ee46b15b369c4",
	"cf32x5/tier3":         "319ee7bfe79671534b1f8fcfd211e199ba0953e31d216e61956437b71b894128",
	"cs16/tier0":           "086a72a3a8ad49b3205c02ff06338213c707726ec9d097559b88f74a92210253",
	"cs16/tier1":           "49aefe835e70b1bcd2ba201cc72e564eecf5ad504743fad18cd6142914798f87",
	"cs16/tier2":           "ded0bee507fbf6e3bba89645cc431b0218e4a82668ac185f4e708daa3d148e51",
	"cs16/tier3":           "503055f429bc985587c4a47822eab64573c977f60bbea9be9f6801bd0e77c920",
	"cs16low12/tier0":      "39dc8dc9d3f540c0aed524827700d51b2182e5f0c77e22e84ee34c429a83847c",
	"cs8/tier0":            "4403095158941496a9d8fbd487ceaca26f1186d069035a1f04e2508c20f2373e",
	"cs8/tier1":            "2edfc65e42c4c4c38f827eb0c770863aa2933954d8562e971c80c2bff8ea249a",
	"cs8/tier2":            "22aa4b17b2dbe7004b5c07305a7223645dec16285054f69d4101bdf27c6958dc",
	"cs8/tier3":            "43b95152688b7ce3e942f2506d02986b06e95e17f436404ad942a50cbe6a6680",
	"cu8/tier0":            "b1c15f7bf5e1d759bd2c1305a1b4a11bbb53f2b61f2b013a342747f91fab4db9",
	"cu8/tier1":            "5540674ba162b546313632f884f511f93afeaba7519d2208155029d22d662ba1",
	"cu8/tier2":            "1831ed0b3f389561394added5057df2a830bcd49d3bdc615857c578d4eff88f2",
	"cu8/tier3":            "4bdcae48d762baba9cb36192c4ce5c1e12b02a1d7571d06ad30721e42636e09e",
	"cu8x2048k/tier0":      "5a33221b2d59b0e1da87d84997ae4c983ce463eb8aacb144a868bc0cfe5b5d21",
	"cu8x2048k/tier1":      "e328cc0cd263a7ab85db50c98c0da9591bd28815471b9813852360ddfc4a9395",
	"cu8x2048k/tier2":      "8f98668c48821163914f0c5b4234792cf8db4584fb3d06910f0055d27fd843ea",
	"cu8half/tier0":        "5205b7851ef353085b8b932a61e2a911ca9066107045b9bdf5ae1f337e382efc",
	"cu8half/tier1":        "2413503aaffaec4c384ad462d18d6238233dc535e20ccb86146a586710e3bbd5",
	"cu8low/tier0":         "5936ddca31e7e62e7e0f0a7cffe1c704c223edb483d00061ad260b2915282c6e",
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
