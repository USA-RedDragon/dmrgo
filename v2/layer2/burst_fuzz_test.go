package layer2_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// ambiguousSync reports a decode whose SYNC classification can change on
// re-encode: the first center matched a SYNC pattern with bit errors, a
// voice burst B-F re-encoded with an embedded field within the SYNC match
// threshold, or a burst without a SYNC pattern whose EMB Encode writes back
// corrected, so that the EMB checks of a data burst with an EMB give another
// result. A damaged SYNC burst and such a voice burst look the same.
func ambiguousSync(first, second *layer2.Burst) bool {
	if first.SyncPattern != enums.SyncNone {
		return first.SyncDistance > 0
	}
	return first.Kind != second.Kind || second.SyncPattern != enums.SyncNone
}

// fuzzCorpus returns the 33-octet first values of the corpus files in
// testdata/fuzz/name.
func fuzzCorpus(tb testing.TB, name string) []layer2.RawBurst {
	tb.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", "fuzz", name, "*"))
	if err != nil {
		tb.Fatal(err)
	}
	var out []layer2.RawBurst
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			tb.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) < 2 || !strings.HasPrefix(lines[1], "[]byte(") {
			tb.Fatalf("%s: not a []byte corpus file", file)
		}
		v, err := strconv.Unquote(strings.TrimSuffix(strings.TrimPrefix(lines[1], "[]byte("), ")"))
		if err != nil {
			tb.Fatalf("%s: %v", file, err)
		}
		if len(v) == 33 {
			out = append(out, layer2.RawBurst([]byte(v)))
		}
	}
	return out
}

// checkBurstDecode checks that a burst decoded from raw and encoded again
// decodes to a burst that encodes to the same octets, unless its center is
// ambiguous as ambiguousSync describes.
func checkBurstDecode(t *testing.T, raw []byte) {
	if len(raw) != 33 {
		return
	}
	frame := layer2.RawBurst(raw)
	var b layer2.Burst
	err := b.DecodeFromBytes(frame)
	first, encErr := b.Encode()
	if err != nil || encErr != nil {
		return
	}
	var b2 layer2.Burst
	_ = b2.DecodeFromBytes(first)
	second, err := b2.Encode()
	if ambiguousSync(&b, &b2) {
		return
	}
	if err != nil {
		t.Fatalf("re-encode failed: %v", err)
	}
	if first != second {
		t.Fatalf("encode not stable\ninput  %x\nfirst  %x\nsecond %x", frame, first, second)
	}
}

// FuzzBurstDecode decodes any 33 octets and checks them with
// checkBurstDecode.
func FuzzBurstDecode(f *testing.F) {
	for _, r := range captureFrames(f) {
		f.Add(r[:])
	}
	f.Fuzz(checkBurstDecode)
}

// fuzzOptions returns the DecodeOptions of the fuzz values: any kind,
// voice position, data type, threshold and expected color code, valid or
// not.
func fuzzOptions(kind, pos, dt, thr, cc byte) *layer2.DecodeOptions {
	o := &layer2.DecodeOptions{
		Kind:          layer2.Kind(kind % 5),
		VoiceBurst:    enums.VoiceBurstType(pos % 8),
		SyncThreshold: int(int8(thr)) % 8,
	}
	if dt&0x80 != 0 {
		d := enums.DataType(dt & 0xF)
		o.DataType = &d
	}
	if cc&0x80 != 0 {
		c := enums.ColorCode(cc & 0xF)
		o.ColorCode = &c
	}
	return o
}

// decodeWithSeed is one seed of FuzzBurstDecodeWith.
type decodeWithSeed struct {
	raw                    []byte
	kind, pos, dt, thr, cc byte
}

// decodeWithSeeds returns the seeds of FuzzBurstDecodeWith: each burst of
// the captures with a spread of options.
func decodeWithSeeds(tb testing.TB) []decodeWithSeed {
	tb.Helper()
	frames := captureFrames(tb)
	seeds := make([]decodeWithSeed, len(frames))
	for i, r := range frames {
		n := byte(i)
		seeds[i] = decodeWithSeed{r[:], n, n / 5, n * 37, n % 3, n * 91}
	}
	return seeds
}

// checkBurstDecodeWith decodes raw with the options of the fuzz values. The
// error is ErrReservedDataType, ErrColorCodeMismatch exactly when the
// expected color code differs from the one of the burst, or an error for
// an invalid kind or voice position. A burst that encodes decodes, with the
// same options, to a burst that encodes to the same octets. The options fix
// the classification, except that a data burst can take its center as a
// SYNC or an EMB, and without a kind or voice position the center can be
// ambiguous as ambiguousSync describes.
func checkBurstDecodeWith(t *testing.T, raw []byte, kind, pos, dt, thr, cc byte) {
	if len(raw) != 33 {
		return
	}
	frame := layer2.RawBurst(raw)
	opts := fuzzOptions(kind, pos, dt, thr, cc)
	var b layer2.Burst
	err := b.DecodeFromBytesWith(frame, opts)
	badOpts := opts.Kind > layer2.KindReverseChannel || opts.Kind == layer2.KindVoice && opts.VoiceBurst > enums.VoiceBurstF
	if err != nil && !errors.Is(err, layer2.ErrColorCodeMismatch) && !errors.Is(err, layer2.ErrReservedDataType) && !badOpts {
		t.Fatalf("decode with %+v: %v", opts, err)
	}
	if opts.ColorCode != nil && !badOpts {
		got, has := b.SlotType.ColorCode, b.Kind == layer2.KindData
		if !has && b.HasEmbeddedSignalling() {
			got, has = b.EmbeddedSignalling.ColorCode, true
		}
		if errors.Is(err, layer2.ErrColorCodeMismatch) != (has && got != *opts.ColorCode) {
			t.Fatalf("decode with color code %d of a burst with %d (%v): %v", *opts.ColorCode, got, has, err)
		}
	}
	if err != nil {
		return
	}
	first, err := b.Encode()
	if err != nil {
		return
	}
	var b2 layer2.Burst
	_ = b2.DecodeFromBytesWith(first, opts)
	switch {
	case opts.Kind == layer2.KindData && b.SyncPattern != b2.SyncPattern:
		return
	case opts.Kind == layer2.KindUnknown || opts.Kind == layer2.KindVoice && opts.VoiceBurst == enums.VoiceBurstUnknown:
		if ambiguousSync(&b, &b2) {
			return
		}
	}
	second, err := b2.Encode()
	if err != nil || first != second {
		t.Fatalf("encode with %+v not stable: %v\ninput  %x\nfirst  %x\nsecond %x", opts, err, frame, first, second)
	}
}

// FuzzBurstDecodeWith decodes any 33 octets with any DecodeOptions and
// checks them with checkBurstDecodeWith.
func FuzzBurstDecodeWith(f *testing.F) {
	for _, s := range decodeWithSeeds(f) {
		f.Add(s.raw, s.kind, s.pos, s.dt, s.thr, s.cc)
	}
	f.Fuzz(checkBurstDecodeWith)
}
