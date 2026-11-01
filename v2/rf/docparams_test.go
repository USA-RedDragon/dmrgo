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
	docRxFile = "../../docs/rx/README.md"
	docTxFile = "../../docs/tx/README.md"
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

func renderRxLimits() string {
	lo, hi := rxBitsSpan()
	var b strings.Builder
	b.WriteString("| Field | Accepted | Refused |\n|-------|----------|---------|\n")
	b.WriteString("| `RxConfig.Tier`, `AudioRxConfig.Tier` | `TierDefault`, `Tier0` to `Tier3` | other values |\n")
	b.WriteString("| `RxConfig.Format` | `IQFormatDefault` (CS16), `IQFormatCU8`, `IQFormatCS8`, `IQFormatCS16`, `IQFormatCF32` | other values |\n")
	fmt.Fprintf(&b, "| `RxConfig.OffsetHz` | under %d Hz in size, or under `Rate`/2 for a `Rate` under %d | larger offsets |\n", basebandRate/2, basebandRate)
	fmt.Fprintf(&b, "| `AudioRxConfig.Bits` | 0 (the default) and %d to %d | other values |\n", lo, hi)
	b.WriteString("| `RxConfig.Rate`, `AudioRxConfig.Rate` | the rates of the table below | other rates |\n")
	return b.String()
}

func renderRxConditions() string {
	var b strings.Builder
	fmt.Fprintf(&b, "- Carrier: the I/Q receiver searches %s around `RxConfig.OffsetHz` for the carrier. [Operating characteristics](#operating-characteristics) gives the offset range of each tier.\n", hzText(iqTuneMaxHz))
	b.WriteString("- Sample clock: the rate of the samples must be the stated `Rate`. [Operating characteristics](#operating-characteristics) gives the clock error range of each tier.\n")
	b.WriteString("- Audio level: the audio receiver measures the gain and the DC offset itself. The audio must not clip.\n")
	b.WriteString("- Audio band: Tier 0 needs the audio of the discriminator with its low frequencies. Above Tier 0, `NewAudioReceiver` outside TinyGo also takes audio that a high-pass filter cut at its low end, as the speaker output of a scanner gives it.\n")
	return b.String()
}

func renderRxEye() string {
	return fmt.Sprintf("[Test conditions](#test-conditions) gives the method. Each eye diagram shows the receiver input after the DMR matched filter (root raised cosine, roll-off %s), at %s, folded on the symbol timing that the tier recovered for each burst. The dotted lines are the four symbol levels.\n",
		strconv.FormatFloat(rrcAlpha, 'f', -1, 64), hzText(basebandRate))
}

func renderRxSweep() string {
	var b strings.Builder
	b.WriteString("- The test stream is a BS outbound stream: a voice call on one slot and data, CSBKs and headers on the other.\n")
	fmt.Fprintf(&b, "- White Gaussian noise is added at each C/N in %s. Eb/N0 is C/N plus %.1f dB.\n", hzText(noiseBandHz), 10*math.Log10(float64(noiseBandHz)/(2*symbolRate)))
	fmt.Fprintf(&b, "- The audio tiers get the audio of an FM discriminator at %s. The sweep makes it from the same noisy signal with the IF filter of a %s radio, so their Eb/N0 is that of the signal at the antenna.\n", hzText(benchAudioHz), hzText(2*audioIFHz))
	fmt.Fprintf(&b, "- The I/Q tiers get the noisy samples at %s. Low-rate I/Q tier 0 gets them at %s, through the decimation filter of a radio chip: a %d-tap Kaiser-windowed sinc with its -6 dB point at %s.\n", hzText(basebandRate), hzText(iq0LowHz), lowIQTaps, hzText(lowIQCut*iq0LowHz))
	return b.String()
}

func txPowerClasses() []int {
	var tops []int
	prev := -1
	for p := 1; p <= modeledDBm; p++ {
		r := txRegs{region: RegionUS, service: ServiceLandMobile, power: p}
		c, err := r.check(&iqErrs, &txGate{noSpur: true})
		if err != nil {
			continue
		}
		if c != prev && prev >= 0 {
			tops = append(tops, p-1)
		}
		prev = c
	}
	return append(tops, modeledDBm)
}

func wattsOf(dbm int) string {
	return strconv.FormatFloat(math.Pow(10, float64(dbm-30)/10), 'f', -1, 64) + " W"
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

func renderTxFormats() string {
	var b strings.Builder
	b.WriteString("| Tier | Modulator | I/Q rates | Audio rates |\n| --- | --- | --- | --- |\n")
	fmt.Fprintf(&b, "| Tier 0 | Fixed-point multiply and accumulate over tables in flash, for a microcontroller. The default of `Init` and of TinyGo builds. | %s | %s |\n", hzText(tier0IQRate), joinRates(tier0AudioRates()))
	fmt.Fprintf(&b, "| Tier 1 | Pulse shaping in float32, I/Q from an interpolated cosine table. The default of `NewTransmitter` and `NewAudioTransmitter`. | %s to %s, %s by default | %s to %s, %s by default |\n",
		hzText(shaperIQRate), hzText(maxTxIQRate), hzText(defaultTxRate), hzText(minTxAudio), hzText(maxTxAudio), hzText(defaultTxAudio))
	b.WriteString("| Tier 2 | Pulse shaping in float64 over a longer span than Tier 1, I/Q from float64 sine and cosine. | as Tier 1 | as Tier 1 |\n\n")
	b.WriteString("Tier 3 has no modulator of its own, and both constructors return a `*dmrgo.ConfigError` for it.\n\n")
	fmt.Fprintf(&b, "- `TxConfig.Rate` 0 is %s. Tier 0 makes %s only, to stay in the CPU budget of the tier.\n", hzText(defaultTxRate), hzText(tier0IQRate))
	fmt.Fprintf(&b, "- I/Q under %s puts the image of the main lobe within two channels of the carrier, where no limit can count on a filter of the radio. The limits do not accept it.\n", hzText(minTxIQRate))
	b.WriteString("- `TxConfig.OffsetHz` moves the carrier from the center of the I/Q. It must be under `Rate`/4 in size. Tier 0 rounds it to whole Hz.\n")
	fmt.Fprintf(&b, "- `AudioTxConfig.Rate` 0 is %s. Tier 1 and Tier 2 make most rates from %s to %s, such as %s, and return a `*dmrgo.ConfigError` for a rate that their polyphase filter cannot make.\n",
		hzText(defaultTxAudio), hzText(minTxAudio), hzText(maxTxAudio), commonAudioRates())
	b.WriteString("- `AudioTxConfig.Bits` 0 is 16. Each sample holds one code in its low bits.\n")
	fmt.Fprintf(&b, "- `AudioTxConfig.Level` is the code of a run of symbol +3 (%d Hz of deviation) from the center, from 1 to %d. 0 is the most that fits in `Bits` with `Offset` over every run of symbols. `Offset` moves the center code.\n", int(txDeviationHz), maxTxLevel)
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

func renderTxLimits() string {
	var b strings.Builder
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
	b.WriteString("Each `RegionError` has one of these values of `Field`, after the name of the config, and `Reason`:\n\n| Field | `Reason` |\n| --- | --- |\n")
	for _, e := range txRegionErrors() {
		fmt.Fprintf(&b, "| `%s` | %s |\n", strings.TrimPrefix(strings.TrimPrefix(e.Field, "AudioTxConfig."), "TxConfig."), e.Reason)
	}
	return b.String()
}

type docBlock struct {
	file, name string
	render     func() string
}

func docBlocks() []docBlock {
	return []docBlock{
		{docRxFile, "rx-limits", renderRxLimits},
		{docRxFile, "rx-conditions", renderRxConditions},
		{docRxFile, "rx-eye", renderRxEye},
		{docRxFile, "rx-sweep", renderRxSweep},
		{docTxFile, "tx-formats", renderTxFormats},
		{docTxFile, "tx-limits", renderTxLimits},
	}
}

func TestDocParams(t *testing.T) {
	t.Parallel()
	write := os.Getenv("DMRGO_DOCPARAMS") == "write"
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
		want := begin + "\n" + blk.render() + end
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
