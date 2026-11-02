//go:build !tinygo

package rf

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"
)

const (
	docRxFile = "../../docs/rx.md"
	docTxFile = "../../docs/tx.md"
)

func hzText(hz float64) string {
	switch {
	case hz >= 1e6:
		return strconv.FormatFloat(hz/1e6, 'f', -1, 64) + " MHz"
	case hz >= 1000:
		return strconv.FormatFloat(hz/1000, 'f', -1, 64) + " kHz"
	}
	return strconv.FormatFloat(hz, 'f', -1, 64) + " Hz"
}

func docSources() (*goSource, *goSource, error) {
	rf, err := loadGoSource(".")
	if err != nil {
		return nil, nil, err
	}
	root, err := loadGoSource("..")
	return rf, root, err
}

func docBullet(b *strings.Builder, src *goSource, name, key string) {
	if d := src.doc(key); d != "" {
		fmt.Fprintf(b, "- `%s`: %s\n", name, d)
	}
}

func firstSentence(d string) string {
	for i := 0; i+2 < len(d); i++ {
		if d[i] == '.' && d[i+1] == ' ' && d[i+2] >= 'A' && d[i+2] <= 'Z' {
			return d[:i+1]
		}
	}
	return d
}

func featureBullet(b *strings.Builder, src *goSource, indent, name string) {
	if d := src.doc(name); d != "" {
		fmt.Fprintf(b, "%s- `%s`: %s\n", indent, name, firstSentence(d))
	}
}

func docTable(b *strings.Builder, src *goSource, types ...string) {
	b.WriteString("| Field | Rule |\n|-------|------|\n")
	for _, t := range types {
		for _, f := range src.fields[t] {
			if d := src.doc(t + "." + f); d != "" {
				fmt.Fprintf(b, "| `%s.%s` | %s |\n", t, f, d)
			}
		}
	}
}

func pkgParas(src *goSource, tx bool) string {
	var out []string
	for i, p := range src.pkg {
		if i == 0 || tx && strings.Contains(p, "Transmitter") {
			out = append(out, p)
		}
	}
	return strings.Join(out, "\n\n") + "\n"
}

func rxBitsSpan() (lo, hi int) {
	lo, hi = -1, -1
	for b := 1; b <= 32; b++ {
		if _, err := NewAudioReceiver(&AudioRxConfig{Bits: b, Tier: Tier0}); err == nil {
			if lo < 0 {
				lo = b
			}
			hi = b
		}
	}
	return lo, hi
}

func rxOffsetBound() float64 {
	lo, hi := 0.0, 1e6
	for range 40 {
		m := (lo + hi) / 2
		if _, err := NewReceiver(&RxConfig{Rate: basebandRate, OffsetHz: m}); err == nil {
			lo = m
		} else {
			hi = m
		}
	}
	return math.Round(hi)
}

func renderRxIntro(src, _ *goSource) string { return pkgParas(src, false) }

func renderRxFeatures(src, _ *goSource) string {
	var b strings.Builder
	for _, n := range []string{"Receiver", "AudioReceiver", "Monitor"} {
		featureBullet(&b, src, "", n)
	}
	b.WriteString("- Tiers:\n")
	for _, c := range src.consts["Tier"] {
		featureBullet(&b, src, "  ", c)
	}
	b.WriteString("- I/Q formats:\n")
	for _, c := range src.consts["IQFormat"] {
		featureBullet(&b, src, "  ", c)
	}
	return b.String()
}

func renderRxLimits(src, _ *goSource) string {
	lo, hi := rxBitsSpan()
	var b strings.Builder
	docTable(&b, src, "RxConfig", "AudioRxConfig")
	fmt.Fprintf(&b, "\nProbes of the constructors: `AudioRxConfig.Bits` from %d to %d is accepted, and `RxConfig.OffsetHz` up to %s in size at a `Rate` of %s. A field out of range gets a `*dmrgo.ConfigError` with one of these values of `Field`: %s.\n",
		lo, hi, hzText(rxOffsetBound()), hzText(basebandRate), "`"+strings.Join(src.configErrors("RxConfig.", "AudioRxConfig."), "`, `")+"`")
	return b.String()
}

func renderRxLevels(src, _ *goSource) string {
	var b strings.Builder
	for _, k := range []string{"AudioReceiver.Level", "AudioReceiver.Offset", "Receiver.OffsetHz", "Receiver.DeviationHz"} {
		docBullet(&b, src, k, k)
	}
	return b.String()
}

func renderRxEye(_, _ *goSource) string {
	return fmt.Sprintf("Each eye diagram shows the receiver input after the DMR matched filter (root raised cosine, roll-off %s), at %s, folded on `Burst.Seconds` of each burst that the tier recovered. The dotted lines are the four symbol levels.\n",
		strconv.FormatFloat(rrcAlpha, 'f', -1, 64), hzText(basebandRate))
}

func renderRxSweep(_, _ *goSource) string {
	const noiseBandHz = 12500
	var b strings.Builder
	b.WriteString("- The test stream is a BS outbound stream: a voice call on one slot and data, CSBKs and headers on the other.\n")
	fmt.Fprintf(&b, "- White Gaussian noise is added at each C/N in %s. Eb/N0 is C/N plus %.1f dB.\n", hzText(noiseBandHz), 10*math.Log10(float64(noiseBandHz)/(2*symbolRate)))
	fmt.Fprintf(&b, "- The audio tiers get the audio of an FM discriminator at %s. The sweep makes it from the same noisy signal with the IF filter of a %s radio, so their Eb/N0 is that of the signal at the antenna.\n", hzText(benchAudioHz), hzText(2*audioIFHz))
	fmt.Fprintf(&b, "- The I/Q tiers get the noisy samples at %s. Low-rate I/Q tier 0 gets them at %s, through the decimation filter of a radio chip: a %d-tap Kaiser-windowed sinc with its -6 dB point at %s.\n", hzText(basebandRate), hzText(iq0LowHz), lowIQTaps, hzText(lowIQCut*iq0LowHz))
	return b.String()
}

func txPowerClasses() []int {
	var tops []int
	prev, last := -1, 0
	for p := 1; p <= maxPowerDBm; p++ {
		r := txRegs{region: RegionUS, service: ServiceAmateur, power: p}
		c, err := r.check(&iqErrs, &txGate{noSpur: true})
		if err != nil {
			continue
		}
		if c != prev && prev >= 0 {
			tops = append(tops, p-1)
		}
		prev, last = c, p
	}
	return append(tops, last)
}

func wattsOf(dbm int) string {
	w := math.Pow(10, float64(dbm-30)/10)
	if w < 10 {
		return strconv.FormatFloat(math.Round(w*10)/10, 'f', -1, 64) + " W"
	}
	return strconv.FormatFloat(math.Round(w), 'f', -1, 64) + " W"
}

func tier0AudioRates() []int {
	return []int{fsktx.Table24k().Hz(), fsktx.Table48k().Hz()}
}

func joinRates(hz []int) string {
	parts := make([]string, len(hz))
	for i, v := range hz {
		parts[i] = strconv.FormatFloat(float64(v)/1000, 'f', -1, 64)
	}
	if len(parts) < 2 {
		return strings.Join(parts, "") + " kHz"
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1] + " kHz"
}

func kHzNum(hz float64) string {
	s := strconv.FormatFloat(hz/1000, 'f', -1, 64)
	if whole, frac, _ := strings.Cut(s, "."); len(whole) > 3 {
		s = whole[:len(whole)-3] + "," + whole[len(whole)-3:]
		if frac != "" {
			s += "." + frac
		}
	}
	return s
}

func commonAudioRates() string {
	var ok []string
	for _, hz := range []int{8000, 11025, 16000, 22050, 32000, 44100, 48000, 96000} {
		if _, err := fsktx.NewAudio(fsktx.AudioConfig{Hz: hz, Tier: fsktx.Tier1}); err == nil {
			ok = append(ok, strconv.Itoa(hz))
		}
	}
	if len(ok) < 2 {
		return strings.Join(ok, "") + " Hz"
	}
	return strings.Join(ok[:len(ok)-1], ", ") + " and " + ok[len(ok)-1] + " Hz"
}

func renderTxIntro(src, _ *goSource) string { return pkgParas(src, true) }

func renderTxFeatures(src, _ *goSource) string {
	var b strings.Builder
	for _, n := range []string{"Transmitter", "AudioTransmitter", "PreEmphasis", "RegionError", "ErrBurstOrder"} {
		featureBullet(&b, src, "", n)
	}
	return b.String()
}

func renderTxFormats(src, _ *goSource) string {
	var b strings.Builder
	b.WriteString("| Tier | I/Q rate (kHz) | Audio rate (kHz) |\n|:---|---:|---:|\n")
	fmt.Fprintf(&b, "| Tier 0 | %s | %s |\n", kHzNum(tier0IQRate), strings.TrimSuffix(joinRates(tier0AudioRates()), " kHz"))
	fmt.Fprintf(&b, "| Tier 1, Tier 2 | %s to %s, %s by default | %s to %s, %s by default |\n\n",
		kHzNum(shaperIQRate), kHzNum(maxTxIQRate), kHzNum(defaultTxRate), kHzNum(minTxAudio), kHzNum(maxTxAudio), kHzNum(defaultTxAudio))
	fmt.Fprintf(&b, "Under %s, I/Q puts the image of the main lobe within two channels of the carrier, and the limits do not accept it. Tier 1 and Tier 2 make audio at %s, among other rates. The deviation of a run of symbol +3 is %d Hz.\n\n",
		hzText(minTxIQRate), commonAudioRates(), int(txDeviationHz))
	docTable(&b, src, "TxConfig", "AudioTxConfig")
	return b.String()
}

func txRegionErrors() []*RegionError {
	var out []*RegionError
	seen := map[string]bool{}
	for _, errs := range []*txErrs{&audioErrs, &iqErrs} {
		v := reflect.ValueOf(errs).Elem()
		for i := range v.NumField() {
			f := v.Field(i)
			if f.Type() != reflect.TypeFor[*RegionError]() || f.IsNil() {
				continue
			}
			e := (*RegionError)(f.UnsafePointer())
			name := strings.TrimPrefix(strings.TrimPrefix(e.Field, "AudioTxConfig."), "TxConfig.")
			if !seen[name+e.Reason] {
				seen[name+e.Reason] = true
				out = append(out, e)
			}
		}
	}
	return out
}

func renderTxLimits(src, root *goSource) string {
	var b strings.Builder
	for _, k := range []string{"TxConfig", "AudioTxConfig"} {
		docBullet(&b, src, k, k)
	}
	b.WriteString("\n")
	b.WriteString("| `Region` | Name | Limits |\n| --- | --- | --- |\n")
	for i, c := range src.consts["Region"] {
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", c, Region(i).String(), src.doc(c))
	}
	b.WriteString("\n| `Service` | Meaning |\n| --- | --- |\n")
	for _, c := range root.consts["Service"] {
		fmt.Fprintf(&b, "| `%s` | %s |\n", c, root.doc(c))
	}
	b.WriteString("\n")
	tops := txPowerClasses()
	classes := make([]string, 0, len(tops))
	for i, top := range tops {
		c := wattsOf(top) + " (" + strconv.Itoa(top) + " dBm)"
		if i == 0 {
			c = wattsOf(top) + " (up to " + strconv.Itoa(top) + " dBm)"
		}
		classes = append(classes, c)
	}
	b.WriteString("- `Power` is the most power of the station in dBm, from 1 to " + strconv.Itoa(maxPowerDBm) + ". 0 is the highest legal power of the service in the region. The limits use the power classes " + strings.Join(classes[:len(classes)-1], ", ") + " and " + classes[len(classes)-1] + ".\n\n")
	b.WriteString("Highest legal power, the `Power` of 0, in dBm:\n\n| `Region` | Land mobile | Amateur |\n| --- | ---: | ---: |\n")
	for g := RegionUS; g <= RegionJapan; g++ {
		fmt.Fprintf(&b, "| `Region%s` | %d | %d |\n", g, ceilingDBm[g][0], ceilingDBm[g][1])
	}
	b.WriteString("\n")
	b.WriteString("Each `RegionError` has one of these values of `Field`, after the name of the config, and `Reason`:\n\n| Field | `Reason` |\n| --- | --- |\n")
	for _, e := range txRegionErrors() {
		fmt.Fprintf(&b, "| `%s` | %s |\n", strings.TrimPrefix(strings.TrimPrefix(e.Field, "AudioTxConfig."), "TxConfig."), e.Reason)
	}
	fmt.Fprintf(&b, "\nA field out of range gets a `*dmrgo.ConfigError` with one of these values of `Field`: %s.\n", "`"+strings.Join(src.configErrors("TxConfig.", "AudioTxConfig."), "`, `")+"`")
	return b.String()
}

type docBlock struct {
	file, name string
	render     func(src, root *goSource) string
}

func docBlocks() []docBlock {
	return []docBlock{
		{docRxFile, "rx-intro", renderRxIntro},
		{docRxFile, "rx-features", renderRxFeatures},
		{docRxFile, "rx-limits", renderRxLimits},
		{docRxFile, "rx-eye", renderRxEye},
		{docRxFile, "rx-levels", renderRxLevels},
		{docRxFile, "rx-sweep", renderRxSweep},
		{docTxFile, "tx-intro", renderTxIntro},
		{docTxFile, "tx-features", renderTxFeatures},
		{docTxFile, "tx-formats", renderTxFormats},
		{docTxFile, "tx-limits", renderTxLimits},
		{docTxFile, "tx-keying", renderTxKeying},
	}
}

func renderTxKeying(src, _ *goSource) string {
	var b strings.Builder
	for _, k := range []string{"Transmitter.Write", "Transmitter.Active", "AudioTransmitter.Active"} {
		docBullet(&b, src, k, k)
	}
	return b.String()
}

func TestDocParams(t *testing.T) {
	t.Parallel()
	write := os.Getenv("DMRGO_DOCPARAMS") == "write"
	src, root, err := docSources()
	if err != nil {
		t.Fatal(err)
	}
	docs := map[string][]byte{}
	for _, blk := range docBlocks() {
		doc, ok := docs[blk.file]
		if !ok {
			var err error
			doc, err = os.ReadFile(blk.file)
			if errors.Is(err, fs.ErrNotExist) {
				t.Skipf("%s is not in this tree", blk.file)
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		begin, end := "<!-- rfparams:"+blk.name+":begin -->", "<!-- rfparams:"+blk.name+":end -->"
		i, j := bytes.Index(doc, []byte(begin)), bytes.Index(doc, []byte(end))
		if i < 0 || j < i {
			t.Fatalf("%s has no %s ... %s block", blk.file, begin, end)
		}
		want := begin + "\n" + blk.render(src, root) + end
		if got := string(doc[i : j+len(end)]); got != want && !write {
			t.Errorf("the %s block of %s differs from the code; run DMRGO_DOCPARAMS=write go test ./rf -run TestDocParams in v2", blk.name, blk.file)
		}
		docs[blk.file] = append(append(append([]byte{}, doc[:i]...), want...), doc[j+len(end):]...)
	}
	if !write {
		return
	}
	for f, doc := range docs {
		if err := os.WriteFile(f, doc, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
