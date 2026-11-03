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
	"audio12000/tier0":     "a25def3ab4855876f0bf3d70f63005f05b1b2d89ab5587f7dd9cf70c5aa68149",
	"audio12000/tier1":     "66928509cc3ea3977c239c036b483bbc235a9009bc0e693c1577f3eec6b40235",
	"audio12000/tier2":     "43f7262c10e6f88ec5a2cbbd1cf95474f203dd562e510350099b2866b1f603f4",
	"audio12000/tier3":     "54bfb8e9487488419ae5b521e3ebceb48dbef4a1c3a5d02099707e803f64596f",
	"audio24000/tier0":     "6dc32dd669087bd650c2c978ea8593e1b66123bbaf9f8fa92673a0c2cf59a9e9",
	"audio24000/tier1":     "34cadad37acff2259fab67db2cbfcefb057adcb21cafb6ab5ec6907dd0935a54",
	"audio24000/tier2":     "371b2e473a82e11d36161f7ece2197fd6ea2150831f68f02df5f82d1ddf99cf5",
	"audio24000/tier3":     "afa6e7543489ca46779e6b6767492cc75e9e42f033c4a916d4eafa4cd46816ab",
	"audio44100/tier0":     "7d8de9e515dafc1300324dbbe6c9dc6d99ad5a3ca732832c4142ca35573f9f0b",
	"audio44100/tier1":     "8beaacad0911dbeea4101895eec050e5f13db935cbc0d46deb29385665a546bc",
	"audio48000/tier0":     "3be83dd3866c4b5dd06118d1cc4a52158c60859de323aecb791c4504fb5378ab",
	"audio48000/tier1":     "18daa3b73df4f8a56c59d14aefff301e965b00395b758281d3bbc8854b5db910",
	"audio48000/tier2":     "42b7d77cfe4ba786fb501c44d42d8826f4b17b00565ddda44e33c9349c3c1958",
	"audio48000/tier3":     "55525fb2566500f436d5067c6a78ea339710de1f4b9f22bf46835cbbde717b76",
	"audio48000weak/tier2": "77617aa5d3e746e0799d2e0009414918089acc78c79a07079588b180398666d2",
	"audio48000weak/tier3": "751c9e25b74298de75bd3ed7c59769be8b0a23d9950994c40c5e9d15cf2a1e44",
	"audio9600/tier0":      "abfed9d4c4de90179c68c405c9ea82123f413c8bd958a53e4dfdc29e6d7b51f5",
	"audio9600/tier1":      "2c60ee532af834523cda8730df3e0783d17599312ae3b429d3c41a529b0470cc",
	"audio9600/tier2":      "f17b697d21f43d24ea6c9d3c02c776636357f72868f7583c44864b60750138b1",
	"audio9600/tier3":      "1c7f848980b8702f91a9278924f64429a966ba8e0cbceb1a34336fa86330d585",
	"cf32/tier0":           "0aac470a05dcc0a9b03debb995a41268d29437d110bb47cc5e64938c4b21f518",
	"cf32/tier1":           "af762c3b53af95b9a07f1da54e07962c8c8b09ecee07ab2a0b820598bd3729b6",
	"cf32/tier2":           "ab58e012e16eb88d42ade42a1d7a1e6f453b17864d7b14d82af158a6770eedf2",
	"cf32/tier3":           "ea88725f293da3e2736b8c48a53e09b4e247b6b1ec6eba6f98f8969ae94fe232",
	"cf32x5/tier0":         "40cf4ffcdb81cf136b0e36ef314efa8be366528c0982b92b4eebb1215b6eb226",
	"cf32x5/tier1":         "61bf699a02e6dadc94825681b59001a92c8f8267867d8225e855063fb4662dcc",
	"cf32x5/tier2":         "d7be07589145aaaf0b4d9c156ea9ff3e3b2c7e5453cf045428441661eaefa1e7",
	"cf32x5/tier3":         "6da841ef809e847e93f4bdf53e43c5ffa3a2c8bdf2c82343d2ca918dca39de4f",
	"cs16/tier0":           "0e4367d88a16b7c71af2bcc484f1309c97296db7f86b6c740452bb403a99695d",
	"cs16/tier1":           "61bc0191a9234002fb754a2a9f8c5303c8d4c048dfa159b8f3d63d0bc49dbbbb",
	"cs16/tier2":           "c07d21824f034808deb111682257a9bdcf15e886591343fb66afc8484a18aa80",
	"cs16/tier3":           "8a9e40238541350a909baeb00dccc19e9b39ed6abaf3f99897d993d10b879b3c",
	"cs16low12/tier0":      "5b12335af382e90bcf484b78a74c45db919310ade9151a25eeee376fd6b3e42e",
	"cs8/tier0":            "763303e975de59932934387299f2bed9026747135eb4834b17759f0e6fb5707c",
	"cs8/tier1":            "fa1536fd280b40b83bd1cfdb36553ef01348a81c32326327fce895c521234fbf",
	"cs8/tier2":            "8e95d7400d9b0b46f96984fd7bd36f401ebc91c60261fae66c5e1ece5722c30b",
	"cs8/tier3":            "d2afa73b47af440d2ddeaaa82f62c040ab041047f3603221d4c9cdeee52ddd6b",
	"cu8/tier0":            "fccbe2149ea13b2f601aea2610f5bf8c29f7ed3e7c1270b186092af66e5d3d6d",
	"cu8/tier1":            "7fea32b9f23d8a3acfb922db18ed6d80618b046eb7b78101d50dd951c4d68332",
	"cu8/tier2":            "922a41ada3d048e250f77aae510d21448d9c894797147f2b4a2184a867d64bf8",
	"cu8/tier3":            "5560db8ce05852b35b502f10117cb9187f548fdb1f415c639dfa8e2783d1ac7f",
	"cu8x2048k/tier0":      "9bc9c142fc25c90c0f12e8ba2930b109e88c4150565f1ce1b6533b00d35023f2",
	"cu8x2048k/tier1":      "faa4bae96a39f168bfde7ab28d3ee389bd2cece39812b29e275f8a74d671b6b7",
	"cu8x2048k/tier2":      "6c1288811ecc1b223c59707db4dc7744e1c46b8176a54d55a346a2b2b99c3844",
	"cu8half/tier0":        "343f0f35cd41f9a8c2fd61028077f8e87427b7db2b5f5ef37e7f0c3df0f75b23",
	"cu8half/tier1":        "fc9e1aa675373f8f59d9a4c795ba23ce88e6c22b8b3d1b2d3b969079ef6fbe4a",
	"cu8low/tier0":         "3956beee414f6b05f967d373a88891b1c5e5198824b143c36e5cf5f406bcfe1c",
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
