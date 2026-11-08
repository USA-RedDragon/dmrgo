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
	"audio12000/tier0":     "460dc20990840d62635df4323aea655e30519c0d648446ae09b89f462765784d",
	"audio12000/tier1":     "5d6142b80d6a527cee309a5de195a9d0519ea9279e15866fd597f6f6eef32889",
	"audio12000/tier2":     "7ddd314601ca8c4a7f621cb37f09a03c7edd11bf3f9a9c4aba421ce47e746829",
	"audio12000/tier3":     "2d12002e97edeb0dc51fbac336e218406976ec4413d0a9fa38bded783b72c276",
	"audio24000/tier0":     "ab998d18a3d29ae8ffe35850b2e6a2a545a5fa6126055f07b146585f9484c9e7",
	"audio24000/tier1":     "33e413d0549fecfc807e3d424d347bcbf6fd625a649961325686f3b4eee156c8",
	"audio24000/tier2":     "016842493afde4cb0ebe81018a22f2f595fe25a1586bb303efeac0dc734b85d1",
	"audio24000/tier3":     "e037ce760d6369879f28dcacd628626f9ebf54036424940e14d1225ef91dc4d6",
	"audio44100/tier0":     "4be99ae3d5a831a4613f5c852db0e1981c1c881c892eb4fed1224771edc994ff",
	"audio44100/tier1":     "262eb40c9a4736e5cbad42e522ccb3957f44342d20f421da1a88ac8324686fc8",
	"audio48000/tier0":     "91b646284a156668b568c173ea3c6f2657f20937f06af2bef86d427f742e089a",
	"audio48000/tier1":     "27292c97231d1ac07bf1231a617f2d246780365b2000c74f80ce62430db7ffd5",
	"audio48000/tier2":     "25d17e2bfd421d888a52164c7cae257c1f7c2ca2ab40c51d4572a4a283de8832",
	"audio48000/tier3":     "70d684a7ca061b1cb19e3fc0b921b3a406dae7fb242f83ca84d5052feca4aeb2",
	"audio48000weak/tier2": "ea982762019bceb93a61d5f147dc122abdfc58a5b74e335b011d7217adc93bb0",
	"audio48000weak/tier3": "01cb44ec04ac3a5fbddbab5616454f839795096424940f2014d2210aed7910f0",
	"audio9600/tier0":      "d1b4e6176115fd41d0d81b3c5d604b527031f03e4d7eb40d015656fc836db780",
	"audio9600/tier1":      "cf3e72630822273b7201087a459de5cbf9f181616adebc987c1722b53b220c4e",
	"audio9600/tier2":      "9e97182ced735632ede7839af5502532b93f2acd41f003dc052ad8a37ddf9d12",
	"audio9600/tier3":      "1568b264ade765676c9f538e00e5caba6ce7bf3b7a80adf5b6459697909a4c07",
	"cf32/tier0":           "fadbeb19f4c531c3fb8fe0af5682233222d64faf5bbbb8b06bffaf07613b9d50",
	"cf32/tier1":           "6041436191008f9a4b757f85d0aa6b9bd9cd4a33a1b61e863b540e6cf438f1a7",
	"cf32/tier2":           "7a18dc32d644f695f0725ebb55f9c7b584afceb55e806b84ebb6fc14a2477792",
	"cf32/tier3":           "5b738fc93c5da1b5e86c6c71a29245e0f5202e93ea78619b5f84dafddbb8e867",
	"cf32x5/tier0":         "8d101c4209eff054246f6b75875491f80586c1a236d62e96c884d767f21806ac",
	"cf32x5/tier1":         "fce8e174f922acbcb181a3d2e3953b7e398a7898dedd1a79805ce5a8de608107",
	"cf32x5/tier2":         "3dd57cce4bc4c442f56ec886354aedad39f83a8b0212e68c0920bc9389dcd2ff",
	"cf32x5/tier3":         "fe5f620b129f37760a50cf06e4a37c683212bd1b830a3a37aa7e878231bdba70",
	"cs16/tier0":           "3681f7503c4c105a067864908051181bd47a7f65bc1dfb8d6363a5348412bcae",
	"cs16/tier1":           "dc7aa30b580d5d2d38f04695d883656911aa2c9938567055b0613ecbe8a72a92",
	"cs16/tier2":           "e0c61f08fb4a15dfc3d2a3cb89f9aa9cd9b10bcf9ad8c39ef52cd01c49adaba0",
	"cs16/tier3":           "ebe0947cd7d9b438df796d530eb9d20dfb25a3ad1d1497b7c4722667d6003ae3",
	"cs16low12/tier0":      "c3dba1e9b74c632f277a57fa5eaadea8c50865b97b94f5250215c0679ceb2192",
	"cs8/tier0":            "7d1acc1e9f1dd16bc0d238376da4a031c218cc413a90535049642efbbc528424",
	"cs8/tier1":            "c83b3a8a8c1ca05111721ae984f503b79309acaeeeb3c9e5b205f71d98c7257c",
	"cs8/tier2":            "3a21bcba1c8bfdc6b8ac2ba2bfa4117be330569af9db5f0374bd9e3cb5e1a03a",
	"cs8/tier3":            "e3287ce24c000a3721170f485929bdab1b1b71fdd5257a86f60fe4b4a210ba3c",
	"cu8/tier0":            "e116a248f8cc85643ad4c796cdf70686725cb63ba1ab4779ed185becb174749c",
	"cu8/tier1":            "66119faeeaf63b15f2dcf5bd402092f790c9dbaacc83572b1b56fb0a20fd1931",
	"cu8/tier2":            "d6df0d24377045c6412a031b953743fd882e84995ef6d8b8ec9ea88924d8596f",
	"cu8/tier3":            "2ffdd0cc8bdb708b31154731c67de6f482b219eecfee9fc1183f4595379607d0",
	"cu8x2048k/tier0":      "361f7e51d6baeb1aa7b0ae55ee47b4e0538dc6a2b7901d8f7ec3977d58764eee",
	"cu8x2048k/tier1":      "773a28b373fe4f002e6f52f7ed00a2a1fa3752e34a12e6d79b89cdbe66dc5c62",
	"cu8x2048k/tier2":      "651d9cad3cd98b1b3a868a2c830cbef2b2c729217d8ee49760d7e1db5065a3df",
	"cu8half/tier0":        "e577fba46f15422898bc9d6f7d763586563897dece9955fc88e1576fc61cb0f6",
	"cu8half/tier1":        "e5aa812e76d277a6ccee8dd468f0c9bf5e0f97b273afdac2cafb540b55fb087a",
	"cu8low/tier0":         "6e28bb70e27852eb100a9528086881ab14b91b9b9e78b78781b75b5851d75cf1",
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
