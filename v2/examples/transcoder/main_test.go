package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/vocoder/codec2"
)

func TestRun(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := run(&buf, filepath.Join("..", "..", "layer2", "testdata")); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	want := []string{
		"capture: 79 bursts, 77 voice bursts, 231 AMBE frames, 13 embedded LCs, 3191868 -> 9990",
		"AMBE72 FEC: 3 bits corrected, 0 uncorrectable frames, 230 of 231 frames re-encode unchanged",
		"AMBE72 with 3 flipped bits: 3 corrected, frame restored true",
		"rebuilt: 80 bursts, 13 embedded LCs, AMBE frames equal true",
	}
	c, err := codec2.New(codec2.Config{})
	switch {
	case errors.Is(err, codec2.ErrUnavailable):
		want = append(want, "codec2 unavailable, skipping")
	case err != nil:
		t.Fatal(err)
	default:
		_ = c.Close()
		want = append(want,
			"codec2: 50 frames -> 20 bursts, rx FID 0xc2, 3191868 -> 9990, 3 embedded LCs, slots equal true",
			"codec2 decode: 8640 samples, audible true",
		)
	}
	for _, s := range want {
		if !strings.Contains(out, s) {
			t.Errorf("output lacks %q\noutput:\n%s", s, out)
		}
	}
}
