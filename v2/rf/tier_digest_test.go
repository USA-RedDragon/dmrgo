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
	"audio12000/tier1":     "8d639f72bccdaa5bf36700bcaf5fc3255980269f782ad4e175b950baf5ef56b0",
	"audio12000/tier2":     "92787132c9f79442d8a787043cbe2bff6675d304232c2e233d2b706ca0e6c021",
	"audio12000/tier3":     "29155bb2f76399ac18d62765134cb88279c5b7a154620656e0a8fb36b58f65af",
	"audio24000/tier0":     "215cf931f12fe12de8b7d04541226138908f509ff2136153e1f8957e393a34bd",
	"audio24000/tier1":     "fb77b54311fef0639e9b8ade170ddd9b273990f6e95f123a08846efcb93c8575",
	"audio24000/tier2":     "3c2f7176b9c3cdfa9cb6800fd058f150a40df7c44f9bebe859aa8344c4d47306",
	"audio24000/tier3":     "dc292ee07f4463859793d41ad4fab83d5e194964453eb2825bc1cc819aef8148",
	"audio44100/tier0":     "d333d94f7822cacafddc655c918ec5b2c44aec7217031248ec133c6529d4b51d",
	"audio44100/tier1":     "d5874119a901f8f8339adbbeda90b7fcea35d43e28e1e242d94611985a1a0aa0",
	"audio48000/tier0":     "68fe9867f54309cf9e4efbc46889896873888ac3582bf799d9eb4e882d1810cf",
	"audio48000/tier1":     "6c580d008dea479cf3a7f4f584a034b906724ad7f7ae1caa5d6e27ee7a0454f1",
	"audio48000/tier2":     "cb0b35d935621f8a1e361da10d6d829f577a3826745800a849369a5976b7d4e3",
	"audio48000/tier3":     "e2ded248709fe17cfa314438afcfd653eb83a71cbc349ad93a44449ab6f505a6",
	"audio48000weak/tier2": "5eea14690e31070b1252f210b899de72eb0b2770b88cb37a7a6bd87bee257483",
	"audio48000weak/tier3": "672d8a553305e901ff737b255b0f8f65494f3fae50d004f942f6fbf226915561",
	"audio9600/tier0":      "0fb049c24fb0724af65f2be3d53a00c414091df37c289e8eef40d29edff3baa1",
	"audio9600/tier1":      "2f46764505eccad42e1af91fd47551022b756b8b17941997cb6666d805dbea08",
	"audio9600/tier2":      "b85b80966bff0f8b7ced91af15340657842646101c64872546cb79764e3f0d68",
	"audio9600/tier3":      "3b4ad40d0bbed78e762250a14472af865a7bd9fdb99a4bfbab3f2f5da17ec7f2",
	"cf32/tier0":           "e4c17f0d0f098525da31bc5550d6ed3c9a184c0b2b9030a03bcdedd14dd45c8d",
	"cf32/tier1":           "2096d2b2bab5831142031141bc598704c436adbfdc02a0cc625161943e3224d1",
	"cf32/tier2":           "971becf3a08e332f695c167941283160addbeea3005a1250386532b6e8af709d",
	"cf32/tier3":           "a0c04bf0acd50b9817a4d67df14a6214157eaf64bc9aa1305ee237b0a355bec1",
	"cf32x5/tier0":         "f0fab1528615fbe1da5e6c706a21e8c6724fae4875e8e4ed7d7725d09dfeddcc",
	"cf32x5/tier1":         "d840eb283a6ae5cf43ab397e11c76c8ae528d986ac0f631abbcf10d6edaab0fd",
	"cf32x5/tier2":         "f80e719608c0dc60bd08b6078bfcbcba836a8e9c95a9bd4fd1e081723b9129fa",
	"cf32x5/tier3":         "15ab0ea266485619e5f2afe50755e90e40749544c0f87118d2d439ea6f7bb97a",
	"cs16/tier0":           "e563315e48517d9919a84a42a327d09e932df8e50297e8b9d2df692169bcb65c",
	"cs16/tier1":           "57bda430bb14e1fc0cb118364cebf3dce2f3da88015234feda178d73cb96124c",
	"cs16/tier2":           "17235f8b51af8aee16f3e84a9922dea47aa474e53846366d616985cf067376be",
	"cs16/tier3":           "97bf4cd1928727f4221cc8b73ffbcb50a108a72ca548a26028aa6e8dd2f17bda",
	"cs16low12/tier0":      "bf42c721778125c9a0d804dd8367fd0720593da2e0a5a6ae1ca479adbddce87e",
	"cs8/tier0":            "3041c397dfd7446fcf088caea25541f5ef2835b93d450f0a9999f495ada7494b",
	"cs8/tier1":            "641e3552a624dd48dd33cc33d97e8263822c1a17bb0e5a44cc060a8839630449",
	"cs8/tier2":            "4c4737631787844feaf0c1806291521ca3f0a3e6cc1fb3361439c9a531ef31d6",
	"cs8/tier3":            "51d966e83cd3cfc271a6ba9e5e1e4c4707fc745094a5259ddd596b4f591229e6",
	"cu8/tier0":            "1f5e926563bd98a85cba37b0274dc8bda89e529d75ee70bdd0087d31d16b99aa",
	"cu8/tier1":            "2df2192712b75159a171b0e3bbae117e63802825cd393dde0bf15acb2ed6a6ec",
	"cu8/tier2":            "aaada87381e434b408539271e5f6ecec339130690e845209aed34c0ce75d4857",
	"cu8/tier3":            "5f0fce2c1c2f1e326fee6da6826bb518fd75a31130734d9d6a79fd5d5b311dcd",
	"cu8x2048k/tier0":      "8fc847d90847e107e50add68af5b374f0358c8a52a9595fccb244fbc07cbf934",
	"cu8x2048k/tier1":      "65e15fe119cf7d19f1f9dc0f7b0f2a6019693b9d57ba3246f001c3071675ec5d",
	"cu8x2048k/tier2":      "e3babb55b9c657df7cadbc8f55e00b3e58c86842de182a54b2807a27977d420f",
	"cu8half/tier0":        "c033433bdd37b902e1dc8869cae72c48a49b60586258b69eeb79ba94f4bb7c7a",
	"cu8half/tier1":        "1c7656d4af2a5197a9fda2ee90f859e089e9b746a098f6bf9d5d0b324cbb030a",
	"cu8low/tier0":         "8caf3dfe94ebcdb05559928ac4c929e1c54aa50185b08929d864858964094d2c",
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
