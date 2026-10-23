//go:build !nocodec2 && !tinygo

package codec2

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
)

func newCodec(t *testing.T, cfg Config) *Codec {
	t.Helper()
	c, err := New(&cfg)
	if err != nil {
		t.Fatalf("New(%+v): %v", cfg, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func voiced(frames int) []int16 {
	const f0 = 150.0
	pcm := make([]int16, frames*FrameSamples)
	for n := range pcm {
		var v float64
		for h := 1; float64(h)*f0 < 3800; h++ {
			f := float64(h) * f0
			d1, d2 := (f-700)/250, (f-1200)/300
			amp := math.Exp(-d1*d1) + 0.5*math.Exp(-d2*d2) + 0.1
			v += amp * math.Sin(2*math.Pi*f*float64(n)/8000)
		}
		pcm[n] = int16(2500 * v)
	}
	return pcm
}

func rms(pcm []int16) float64 {
	var sum float64
	for _, s := range pcm {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(len(pcm)))
}

func pitchLag(pcm []int16) int {
	best, bestLag := math.Inf(-1), 0
	for lag := 20; lag <= 160; lag++ {
		var r float64
		for n := lag; n < len(pcm); n++ {
			r += float64(pcm[n]) * float64(pcm[n-lag])
		}
		if r > best {
			best, bestLag = r, lag
		}
	}
	return bestLag
}

func encodeAll(t *testing.T, c *Codec, pcm []int16) [][9]byte {
	t.Helper()
	slots := make([][9]byte, 0, len(pcm)/FrameSamples)
	for off := 0; off < len(pcm); off += FrameSamples {
		slot, err := c.Encode(pcm[off : off+FrameSamples])
		if err != nil {
			t.Fatal(err)
		}
		slots = append(slots, slot)
	}
	return slots
}

func decodeAll(t *testing.T, c *Codec, slots [][9]byte) []int16 {
	t.Helper()
	pcm := make([]int16, 0, len(slots)*FrameSamples)
	for _, slot := range slots {
		out, err := c.Decode(nil, slot)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != FrameSamples {
			t.Fatalf("Decode returned %d samples", len(out))
		}
		pcm = append(pcm, out...)
	}
	return pcm
}

func TestNew(t *testing.T) {
	t.Parallel()

	c := newCodec(t, Config{})
	if c.Mode() != ModeAuto || c.DecodeMode() != ModeAuto || c.FID() != 0xC2 || c.FrameSamples() != 160 {
		t.Errorf("defaults: mode %d decode mode %d FID %#x samples %d", c.Mode(), c.DecodeMode(), c.FID(), c.FrameSamples())
	}
	if s := newCodec(t, Config{Mode: Mode2400}).Silence(); c.Silence() != s {
		t.Errorf("ModeAuto Silence %x, Mode2400 %x", c.Silence(), s)
	}
	c = newCodec(t, Config{Mode: Mode3200, FID: 0x81})
	if c.Mode() != Mode3200 || c.DecodeMode() != Mode3200 || c.FID() != 0x81 {
		t.Errorf("explicit: mode %d decode mode %d FID %#x", c.Mode(), c.DecodeMode(), c.FID())
	}
	if _, err := New(&Config{Mode: 1600}); !errors.Is(err, ErrUnsupportedMode) {
		t.Errorf("New(1600) error = %v, want ErrUnsupportedMode", err)
	}
}

func TestSeparateCodecsConcurrently(t *testing.T) {
	t.Parallel()

	pcm := voiced(20)
	done := make(chan error, 4)
	for range 4 {
		c := newCodec(t, Config{})
		go func() {
			for off := 0; off < len(pcm); off += FrameSamples {
				slot, err := c.Encode(pcm[off : off+FrameSamples])
				if err == nil {
					_, err = c.Decode(nil, slot)
				}
				if err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}()
	}
	for range 4 {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}

func TestEncodeFrameLength(t *testing.T) {
	t.Parallel()

	c := newCodec(t, Config{})
	for _, n := range []int{0, 1, 159, 161, 320} {
		if _, err := c.Encode(make([]int16, n)); !errors.Is(err, ErrFrameLength) {
			t.Errorf("Encode(%d samples) error = %v, want ErrFrameLength", n, err)
		}
	}
	if _, err := c.Encode(nil); !errors.Is(err, ErrFrameLength) {
		t.Errorf("Encode(nil) error = %v, want ErrFrameLength", err)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	c, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := c.Encode(make([]int16, FrameSamples)); !errors.Is(err, ErrClosed) {
		t.Errorf("Encode after Close error = %v", err)
	}
	if pcm, err := c.Decode(nil, [9]byte{}); pcm != nil || !errors.Is(err, ErrClosed) {
		t.Errorf("Decode after Close = %v, %v", pcm, err)
	}
	dst := []int16{1, 2}
	if got, err := c.Decode(dst, [9]byte{}); len(got) != len(dst) || !errors.Is(err, ErrClosed) {
		t.Errorf("Decode after Close = %v, %v", got, err)
	}
}

func TestDecode(t *testing.T) {
	t.Parallel()

	c := newCodec(t, Config{})
	slot, err := c.Encode(voiced(1))
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Decode([]int16{7, 8}, slot)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2+FrameSamples || got[0] != 7 || got[1] != 8 {
		t.Fatalf("Decode = %d samples, prefix %v", len(got), got[:2])
	}
	if !slices.ContainsFunc(got[2:], func(s int16) bool { return s != 0 }) {
		t.Error("Decode of a voiced frame is all zero")
	}
}

func TestEncodePadBitsZero(t *testing.T) {
	t.Parallel()

	pcm := voiced(10)
	for _, mode := range allModes() {
		c := newCodec(t, Config{Mode: mode})
		for i, slot := range encodeAll(t, c, pcm) {
			for b := mode.frameBytes() * 8; b < 72; b++ {
				if bits.Bit(slot[:], b) != 0 {
					t.Fatalf("mode %d frame %d: pad bit %d set", mode, i, b)
				}
			}
		}
	}
}

func TestDecodeIgnoresPadBits(t *testing.T) {
	t.Parallel()

	pcm := voiced(10)
	for _, mode := range allModes() {
		slots := encodeAll(t, newCodec(t, Config{Mode: mode}), pcm)
		padded := make([][9]byte, len(slots))
		for i, slot := range slots {
			for b := mode.frameBytes() * 8; b < 72; b++ {
				bits.SetBit(slot[:], b, 1)
			}
			padded[i] = slot
		}
		a := decodeAll(t, newCodec(t, Config{Mode: mode}), slots)
		b := decodeAll(t, newCodec(t, Config{Mode: mode}), padded)
		if ra, rb := rms(a[800:]), rms(b[800:]); math.Abs(ra-rb) > 0.05*ra {
			t.Errorf("mode %d: RMS %.0f with zero pad bits, %.0f with set pad bits", mode, ra, rb)
		}
	}
}

func TestRoundTripVoiced(t *testing.T) {
	t.Parallel()

	pcm := voiced(50)
	in := rms(pcm[800:])
	for _, mode := range allModes() {
		c := newCodec(t, Config{Mode: mode})
		out := decodeAll(t, c, encodeAll(t, c, pcm))[800:]
		if ratio := rms(out) / in; ratio < 0.5 || ratio > 2 {
			t.Errorf("mode %d: output/input RMS ratio %.2f", mode, ratio)
		}
		if lag := pitchLag(out); lag < 50 || lag > 57 {
			t.Errorf("mode %d: pitch lag %d samples, want about 53 (150 Hz)", mode, lag)
		}
	}
}

func TestSilence(t *testing.T) {
	t.Parallel()

	for _, mode := range allModes() {
		a := newCodec(t, Config{Mode: mode})
		b := newCodec(t, Config{Mode: mode})
		if a.Silence() != b.Silence() {
			t.Fatalf("mode %d: Silence differs between codecs", mode)
		}
		if a.Silence() == ([9]byte{}) {
			t.Errorf("mode %d: Silence is all zero bits", mode)
		}

		fresh := newCodec(t, Config{Mode: mode})
		want, err := fresh.Encode(make([]int16, FrameSamples))
		if err != nil {
			t.Fatal(err)
		}
		if a.Silence() != want {
			t.Errorf("mode %d: Silence is not a fresh encode of zeros", mode)
		}

		_, _ = a.Encode(voiced(1))
		if s := a.Silence(); s != want {
			t.Errorf("mode %d: Silence changed after Encode", mode)
		}

		dec := newCodec(t, Config{Mode: mode})
		slots := make([][9]byte, 10)
		for i := range slots {
			slots[i] = a.Silence()
		}
		if r := rms(decodeAll(t, dec, slots)); r > 50 {
			t.Errorf("mode %d: decoded silence RMS %.1f, want near zero", mode, r)
		}
	}
}

func writeRaw(t *testing.T, path string, pcm []int16) {
	t.Helper()
	buf := make([]byte, 2*len(pcm))
	for i, s := range pcm {
		binary.LittleEndian.PutUint16(buf[2*i:], uint16(s))
	}
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readRaw(t *testing.T, path string) []int16 {
	t.Helper()
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]int16, len(buf)/2)
	for i := range pcm {
		pcm[i] = int16(binary.LittleEndian.Uint16(buf[2*i:]))
	}
	return pcm
}

func encodeBytes(t *testing.T, mode Mode, pcm []int16) []byte {
	t.Helper()
	slots := encodeAll(t, newCodec(t, Config{Mode: mode}), pcm)
	out := make([]byte, 0, len(slots)*mode.frameBytes())
	frame := make([]byte, mode.frameBytes())
	for _, slot := range slots {
		copy(frame, slot[:])
		out = append(out, frame...)
	}
	return out
}

func TestEncodeMatchesLibcodec2(t *testing.T) {
	t.Parallel()

	want := map[Mode]string{
		Mode2400: "3c83d62e923ffa8f5ccf92d7da1afb47e407eccabf5f72c45c2e6cd429937f18",
		Mode3200: "eddc244569099b03255dc1bac01f0085ce917a09e8ab2255783f1fce149abe77",
	}
	pcm := voiced(50)
	for _, mode := range allModes() {
		sum := sha256.Sum256(encodeBytes(t, mode, pcm))
		if got := hex.EncodeToString(sum[:]); got != want[mode] {
			t.Errorf("mode %d: frames hash to %s, libcodec2 1.2.0 to %s", mode, got, want[mode])
		}
	}
}

var decodeTolerance = map[string]int{"native": 0, "wasm": 1}[Backend]

func lookTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"c2enc", "c2dec"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
}

func TestMatchesC2Enc(t *testing.T) {
	t.Parallel()

	lookTools(t)
	pcm := voiced(50)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.raw")
	writeRaw(t, in, pcm)
	for _, mode := range allModes() {
		out := filepath.Join(dir, "out-"+strconv.Itoa(int(mode))+".bin")
		if b, err := exec.CommandContext(t.Context(), "c2enc", strconv.Itoa(int(mode)), in, out).CombinedOutput(); err != nil {
			t.Fatalf("c2enc: %v: %s", err, b)
		}
		want, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		got := encodeBytes(t, mode, pcm)
		if string(got) != string(want) {
			t.Errorf("mode %d: Encode output differs from c2enc (%d vs %d bytes)", mode, len(got), len(want))
		}
	}
}

func TestMatchesC2Dec(t *testing.T) {
	t.Parallel()

	if os.Getenv("CODEC2_C2DEC_HELPER") != "" {
		c2decHelper(t)
		return
	}
	lookTools(t)
	pcm := voiced(50)
	dir := t.TempDir()
	for _, mode := range allModes() {
		frames := encodeBytes(t, mode, pcm)
		bits := filepath.Join(dir, "in-"+strconv.Itoa(int(mode))+".bin")
		if err := os.WriteFile(bits, frames, 0o600); err != nil {
			t.Fatal(err)
		}
		wantPath := filepath.Join(dir, "c2dec-"+strconv.Itoa(int(mode))+".raw")
		if b, err := exec.CommandContext(t.Context(), "c2dec", strconv.Itoa(int(mode)), bits, wantPath).CombinedOutput(); err != nil {
			t.Fatalf("c2dec: %v: %s", err, b)
		}
		want := readRaw(t, wantPath)
		for _, decMode := range []Mode{mode, ModeAuto} {
			gotPath := filepath.Join(dir, "go-"+strconv.Itoa(int(mode))+"-"+strconv.Itoa(int(decMode))+".raw")
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestMatchesC2Dec$")
			cmd.Env = append(os.Environ(),
				"CODEC2_C2DEC_HELPER=1",
				"CODEC2_C2DEC_MODE="+strconv.Itoa(int(mode)),
				"CODEC2_C2DEC_DECODE="+strconv.Itoa(int(decMode)),
				"CODEC2_C2DEC_IN="+bits,
				"CODEC2_C2DEC_OUT="+gotPath)
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("helper: %v: %s", err, b)
			}
			got := readRaw(t, gotPath)
			if len(got) != len(want) {
				t.Fatalf("mode %d, decoder %d: %d samples, c2dec %d", mode, decMode, len(got), len(want))
			}
			for i := range got {
				if d := int(got[i]) - int(want[i]); d > decodeTolerance || d < -decodeTolerance {
					t.Fatalf("mode %d, decoder %d: sample %d is %d, c2dec %d", mode, decMode, i, got[i], want[i])
				}
			}
		}
	}
}

func c2decHelper(t *testing.T) {
	mode, err := strconv.Atoi(os.Getenv("CODEC2_C2DEC_MODE"))
	if err != nil {
		t.Fatal(err)
	}
	frames, err := os.ReadFile(os.Getenv("CODEC2_C2DEC_IN"))
	if err != nil {
		t.Fatal(err)
	}
	m := Mode(mode)
	dec, err := strconv.Atoi(os.Getenv("CODEC2_C2DEC_DECODE"))
	if err != nil {
		t.Fatal(err)
	}
	c := newCodec(t, Config{Mode: Mode(dec)})
	var slots [][9]byte
	for off := 0; off+m.frameBytes() <= len(frames); off += m.frameBytes() {
		var slot [9]byte
		copy(slot[:], frames[off:off+m.frameBytes()])
		slots = append(slots, slot)
	}
	writeRaw(t, os.Getenv("CODEC2_C2DEC_OUT"), decodeAll(t, c, slots))
}

func TestAutoDecode(t *testing.T) {
	t.Parallel()

	pcm := append(make([]int16, 10*FrameSamples), voiced(40)...)
	for _, mode := range allModes() {
		slots := encodeAll(t, newCodec(t, Config{Mode: mode}), pcm)
		slots = append(slots, make([][9]byte, 18)...)
		c := newCodec(t, Config{})
		for call := range 2 {
			var out []int16
			for i, slot := range slots {
				var err error
				if out, err = c.Decode(out, slot); err != nil {
					t.Fatal(err)
				}
				want := mode
				if call == 0 && i+1 < detectDecide {
					want = ModeAuto
				}
				if got := c.DecodeMode(); got != want {
					t.Fatalf("mode %d call %d frame %d: DecodeMode %d, want %d", mode, call, i, got, want)
				}
			}
			if lag := pitchLag(out[20*FrameSamples : 50*FrameSamples]); lag < 50 || lag > 57 {
				t.Errorf("mode %d call %d: pitch lag %d samples, want about 53 (150 Hz)", mode, call, lag)
			}
		}
		c.Reset()
		if c.DecodeMode() != ModeAuto {
			t.Errorf("mode %d: DecodeMode after Reset %d", mode, c.DecodeMode())
		}
	}
}
