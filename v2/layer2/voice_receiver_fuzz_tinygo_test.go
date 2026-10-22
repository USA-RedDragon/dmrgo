//go:build tinygo

package layer2_test

import "testing"

func TestTinyGoVoiceReceiverSeeds(t *testing.T) {
	t.Parallel()
	for _, s := range voiceReceiverSeeds() {
		runVoiceReceiver(t, &s)
	}
}
