//go:build tinygo

package vocoder_test

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range ambe72Seeds(t) {
		checkAMBE72(t, s.data, s.mask)
	}
	for _, s := range ambeVoiceSeeds() {
		checkAMBEVoice(t, s.f1, s.f2, s.f3, s.flags, s.payload)
	}
}
