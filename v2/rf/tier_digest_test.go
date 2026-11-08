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
	"audio12000/tier0":     "8b2b3cfc026fb97e83d00b46ddea3af4c5bea6b0a124bc6f7471cc8ba81b3a21",
	"audio12000/tier1":     "9ecfdcaf426db20fef635a6908fe12966e4916149711e47513c00401997636a8",
	"audio12000/tier2":     "7ddd314601ca8c4a7f621cb37f09a03c7edd11bf3f9a9c4aba421ce47e746829",
	"audio12000/tier3":     "2d12002e97edeb0dc51fbac336e218406976ec4413d0a9fa38bded783b72c276",
	"audio24000/tier0":     "e3a76cdc28cfa56d5521ca2f308fe4aba6778f2467857700677ededab9e8bd71",
	"audio24000/tier1":     "501038db6b905b614b65c73c609c0419f1cec6aed1b2d6e4c018cc175f519fad",
	"audio24000/tier2":     "f440296c24936eced3e01ae2865fe3d189ed670ce08872ec9cdc56406be9d30e",
	"audio24000/tier3":     "6b3baf410341a1547d0aa890987dbf713b04cb26601d3ec021cd6f9ef19664e0",
	"audio44100/tier0":     "4be99ae3d5a831a4613f5c852db0e1981c1c881c892eb4fed1224771edc994ff",
	"audio44100/tier1":     "b40ffdcdd353d5c0d6fd5fa3755dc6577b511568dc5e2dce04a2162cf60c0f68",
	"audio48000/tier0":     "cbaa94c0b05af24ff4d10e15c3ea81ec9a3d3d6d9c3b09c79bed8643ee0b5ced",
	"audio48000/tier1":     "3d1f26d7b16f8a59816896629f78df649600422d5fbe02e0059db86221ec73d8",
	"audio48000/tier2":     "f8d59b2730fee8efe4712a98c82d2b7dda72bf7fa4de065f0d52831ed42a59b8",
	"audio48000/tier3":     "6476a920418ffaf98d1e6ba95309918aa2d7b0c2e72a5b349d3c64dca2d81b9e",
	"audio48000weak/tier2": "ea982762019bceb93a61d5f147dc122abdfc58a5b74e335b011d7217adc93bb0",
	"audio48000weak/tier3": "01cb44ec04ac3a5fbddbab5616454f839795096424940f2014d2210aed7910f0",
	"audio9600/tier0":      "d1b4e6176115fd41d0d81b3c5d604b527031f03e4d7eb40d015656fc836db780",
	"audio9600/tier1":      "cf3e72630822273b7201087a459de5cbf9f181616adebc987c1722b53b220c4e",
	"audio9600/tier2":      "9e97182ced735632ede7839af5502532b93f2acd41f003dc052ad8a37ddf9d12",
	"audio9600/tier3":      "1568b264ade765676c9f538e00e5caba6ce7bf3b7a80adf5b6459697909a4c07",
	"cf32/tier0":           "fadbeb19f4c531c3fb8fe0af5682233222d64faf5bbbb8b06bffaf07613b9d50",
	"cf32/tier1":           "c44e5b8aea8c5b7edea17b8a86042e5fc65ba54fa689856fdef478a2ac2ac4a2",
	"cf32/tier2":           "1c2df8d4b5894d8eed62ee9a77270a64a195430facdb0925d0979a502f819c44",
	"cf32/tier3":           "20815f4127be252959d7c30f5d235fd4a9f7058b048ac648ca309951ddd2fbdc",
	"cf32x5/tier0":         "8d101c4209eff054246f6b75875491f80586c1a236d62e96c884d767f21806ac",
	"cf32x5/tier1":         "fce8e174f922acbcb181a3d2e3953b7e398a7898dedd1a79805ce5a8de608107",
	"cf32x5/tier2":         "1c47035b75620bec441fa05f605bc7c12ed7659b4241c033c9fbb6618dee2b1e",
	"cf32x5/tier3":         "15121da7c81f7976603c8c96d193c7e8ff903929b7b234f219da6291fe41bc35",
	"cs16/tier0":           "3681f7503c4c105a067864908051181bd47a7f65bc1dfb8d6363a5348412bcae",
	"cs16/tier1":           "dc7aa30b580d5d2d38f04695d883656911aa2c9938567055b0613ecbe8a72a92",
	"cs16/tier2":           "506d71edcbddd887f10bbcbeb1940b88feec0c7aa346002736102ca705dec2a1",
	"cs16/tier3":           "2146c67e4e218dfd068f9edae928bbced3b19d3c1ba3e16345f52d162b08f8ea",
	"cs16low12/tier0":      "c3dba1e9b74c632f277a57fa5eaadea8c50865b97b94f5250215c0679ceb2192",
	"cs8/tier0":            "7d1acc1e9f1dd16bc0d238376da4a031c218cc413a90535049642efbbc528424",
	"cs8/tier1":            "07317c3c825c5074f67c1d0d46bfd98b8ef53f317f698dafd891b20e43f97787",
	"cs8/tier2":            "fd1e16d62f8b004d7e834d09b918ee3f09fec5c62203edc0b68f07153bdd575f",
	"cs8/tier3":            "987fb944c8fe39d93e95a5f3302cb316bd97bb3f62292a3d4fd64ec2fc7e621b",
	"cu8/tier0":            "e116a248f8cc85643ad4c796cdf70686725cb63ba1ab4779ed185becb174749c",
	"cu8/tier1":            "66119faeeaf63b15f2dcf5bd402092f790c9dbaacc83572b1b56fb0a20fd1931",
	"cu8/tier2":            "4e8a23d87eb928e53da62ccd4f01bc5c427e962f4c26556f155ad3ac4aad8e9d",
	"cu8/tier3":            "0811eb0d672fe36dfd65ea393e8136f4d72feb000d991433b6447d1dc4133707",
	"cu8x2048k/tier0":      "361f7e51d6baeb1aa7b0ae55ee47b4e0538dc6a2b7901d8f7ec3977d58764eee",
	"cu8x2048k/tier1":      "9e7cfd9fcc02d8490898c48d9bd6115ef39fefac6cbcd7b90791b1a46ff64c85",
	"cu8x2048k/tier2":      "2882e22eb791fc03f7760238a102019b154190f30088ceba2375b53e5a9abb21",
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
