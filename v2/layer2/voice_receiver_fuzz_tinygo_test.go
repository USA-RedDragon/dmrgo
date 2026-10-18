//go:build tinygo

package layer2_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestVoiceReceiverFuzzCorpus runs the seeds and the testdata/fuzz inputs
// of FuzzVoiceReceiver, because TinyGo does not run fuzz tests.
func TestVoiceReceiverFuzzCorpus(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("testdata/fuzz/FuzzVoiceReceiver/*")
	if err != nil {
		t.Fatal(err)
	}
	inputs := voiceReceiverSeeds()
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) != 2 || lines[0] != "go test fuzz v1" || !strings.HasPrefix(lines[1], "[]byte(") {
			t.Fatalf("%s: not a []byte fuzz input", f)
		}
		s, err := strconv.Unquote(strings.TrimSuffix(strings.TrimPrefix(lines[1], "[]byte("), ")"))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		inputs = append(inputs, []byte(s))
	}
	for _, in := range inputs {
		runVoiceReceiver(t, in)
	}
}
