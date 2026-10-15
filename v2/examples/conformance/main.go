// Command conformance decodes every burst of the layer2 test captures,
// re-encodes it, and checks it byte for byte and against the CRC and parity
// of the codeword byte functions.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"text/tabwriter"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const burstSize = 33

type tally struct {
	file                    string
	bursts, voice, data     int
	reencoded, rebuilt      int
	sealed, embeddedLC, fec int
	failures                []string
}

func (t *tally) failf(i int, format string, args ...any) {
	t.failures = append(t.failures, fmt.Sprintf("%s burst %d: %s", t.file, i, fmt.Sprintf(format, args...)))
}

func checkCapture(name string, raw []byte) *tally {
	t := &tally{file: name}
	if len(raw)%burstSize != 0 {
		t.failf(0, "length %d is not a multiple of %d", len(raw), burstSize)
		return t
	}
	var rx layer2.VoiceReceiver
	var frags [4][4]byte
	for i := 0; i < len(raw)/burstSize; i++ {
		in := [burstSize]byte(raw[i*burstSize : (i+1)*burstSize])
		t.bursts++
		b, err := layer2.NewBurstFromBytes(in)
		if err != nil {
			t.failf(i, "decode: %v", err)
			continue
		}
		agg := b.FEC.Aggregate()
		if agg.Uncorrectable {
			t.failf(i, "uncorrectable FEC %+v", b.FEC)
		} else if agg.ErrorsCorrected > 0 {
			t.fec++
		}
		out, err := b.Encode()
		switch {
		case err != nil:
			t.failf(i, "encode: %v", err)
		case out != in && agg.ErrorsCorrected == 0:
			t.failf(i, "re-encode differs: %x", out)
		case out == in:
			t.reencoded++
		}

		rb := layer2.RawBurst(in)
		switch b.Kind {
		case layer2.KindData:
			t.data++
			checkData(t, i, b, rb)
			rx.Reset()
		case layer2.KindVoice:
			t.voice++
			checkVoice(t, i, b, rb, &rx, &frags)
		case layer2.KindReverseChannel, layer2.KindUnknown:
			t.failf(i, "unexpected kind %v", b.Kind)
		}
	}
	return t
}

func syncSource(p enums.SyncPattern) (layer2.SyncSource, bool) {
	for _, s := range []layer2.SyncSource{layer2.SyncSourceBS, layer2.SyncSourceMS, layer2.SyncSourceDirectTS1, layer2.SyncSourceDirectTS2} {
		if _, data, _ := s.SyncPatterns(); data == p {
			return s, true
		}
	}
	return 0, false
}

func checkData(t *tally, i int, b *layer2.Burst, rb layer2.RawBurst) {
	st := rb.SlotType()
	if st != b.SlotType {
		t.failf(i, "RawBurst slot type %v, Burst %v", st, b.SlotType)
	}
	src, ok := syncSource(b.SyncPattern)
	if !ok {
		t.failf(i, "no sync source for %v", b.SyncPattern)
		return
	}
	dt := st.DataType
	var rebuilt layer2.RawBurst
	var err error
	switch dt {
	case enums.DataTypeRate34:
		p, _ := rb.Rate34Payload()
		rebuilt, err = layer2.BuildRate34DataBurst(p, st.ColorCode, src)
	case enums.DataTypeRate1:
		p, _ := rb.Rate1Payload()
		rebuilt, err = layer2.BuildRate1DataBurst(p, st.ColorCode, src)
	case enums.DataTypeReserved:
		t.failf(i, "reserved data type")
		return
	case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC,
		enums.DataTypePIHeader, enums.DataTypeCSBK, enums.DataTypeMBCHeader,
		enums.DataTypeDataHeader, enums.DataTypeUnifiedSingleBlock,
		enums.DataTypeMBCContinuation, enums.DataTypeRate12, enums.DataTypeIdle:
		p, _ := rb.BPTCPayload()
		checkSeal(t, i, dt, p)
		rebuilt, err = layer2.BuildDataBurst(p, dt, st.ColorCode, src)
	}
	switch {
	case err != nil:
		t.failf(i, "build %v: %v", dt, err)
	case rebuilt == rb:
		t.rebuilt++
	case b.FEC.Aggregate().ErrorsCorrected == 0:
		t.failf(i, "rebuilt %v burst differs: %x", dt, rebuilt)
	}
}

func checkSeal(t *tally, i int, dt enums.DataType, p [12]byte) {
	var want [12]byte
	var err error
	_, width, _ := pdu.CRCMask(dt)
	switch width {
	case 24:
		if !pdu.FullLCParityOK(p, dt) {
			t.failf(i, "%v RS(12,9) parity fails", dt)
			return
		}
		want, err = pdu.FullLCCodeword([9]byte(p[:9]), dt)
	case 16:
		if !pdu.BlockCRCOK(p, dt) {
			t.failf(i, "%v CRC-CCITT fails", dt)
			return
		}
		want = p
		err = pdu.SealBlock(&want, dt)
	default:
		return
	}
	switch {
	case err != nil:
		t.failf(i, "seal %v: %v", dt, err)
	case want != p:
		t.failf(i, "resealed %v payload %x, on air %x", dt, want, p)
	default:
		t.sealed++
	}
}

func checkVoice(t *tally, i int, b *layer2.Burst, rb layer2.RawBurst, rx *layer2.VoiceReceiver, frags *[4][4]byte) {
	if b.HasEmbeddedSignalling() {
		if emb := rb.EMB(); emb != b.EmbeddedSignalling {
			t.failf(i, "RawBurst EMB %v, Burst %v", emb, b.EmbeddedSignalling)
		}
	}
	lc, ok, err := rx.Receive(b)
	if err != nil {
		t.failf(i, "voice receiver: %v", err)
		return
	}
	if b.VoiceBurst >= enums.VoiceBurstB && b.VoiceBurst <= enums.VoiceBurstE {
		frags[b.VoiceBurst-enums.VoiceBurstB] = rb.EmbeddedData()
	}
	if !ok {
		return
	}
	enc, err := layer2.EncodeEmbeddedFullLinkControl(&lc)
	switch {
	case err != nil:
		t.failf(i, "encode embedded LC: %v", err)
	case enc != *frags:
		t.failf(i, "embedded LC re-encodes to %x, on air %x", enc, *frags)
	default:
		t.embeddedLC++
	}
}

func run(w io.Writer, dataDir string) error {
	files := []string{"voice.bin", "parrot_kerchunk.bin", "d-sms.bin", "h-sms.bin", "m-sms.bin"}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "file\tbursts\tvoice\tdata\tre-encoded\trebuilt\tsealed\temb LC\tFEC fixed\tresult\t")
	var failures []string
	for _, f := range files {
		raw, err := os.ReadFile(filepath.Join(dataDir, f))
		if err != nil {
			return err
		}
		t := checkCapture(f, raw)
		result := "PASS"
		if len(t.failures) > 0 {
			result = "FAIL"
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%s\t\n",
			t.file, t.bursts, t.voice, t.data, t.reencoded, t.rebuilt, t.sealed, t.embeddedLC, t.fec, result)
		failures = append(failures, t.failures...)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	for _, f := range failures {
		fmt.Fprintln(w, "FAIL", f)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d conformance failures", len(failures))
	}
	fmt.Fprintln(w, "all captures conform")
	return nil
}

func defaultDataDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "layer2", "testdata")
}

func main() {
	dataDir := flag.String("data", defaultDataDir(), "directory with the capture files")
	flag.Parse()
	if err := run(os.Stdout, *dataDir); err != nil {
		fmt.Fprintln(os.Stderr, "conformance:", err)
		os.Exit(1)
	}
}
