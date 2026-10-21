//go:build !tinygo

package hytera_test

import (
	"bufio"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

func TestBuiltPackets(t *testing.T) {
	t.Parallel()

	f, err := os.Open("testdata/built.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var n captureCounts
	var positions []enums.VoiceBurstType
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		fs := strings.Fields(line)
		p, err := hex.DecodeString(fs[1])
		if err != nil {
			t.Fatal(err)
		}
		checkCapture(t, p, &n)
		if fr, err := hytera.DecodeFrame(p); err == nil && fr.HasBurst() {
			if h := fr.Hint(); h.VoiceBurst != enums.VoiceBurstUnknown {
				positions = append(positions, h.VoiceBurst)
			}
		}
	}
	if n.frames != 9 || n.voiceA != 1 || n.voice != 6 || n.data != 2 || n.noBurst != 1 || n.heartbeats != 3 || n.p2p != 4 {
		t.Fatalf("%+v", n)
	}
	for i, pos := range positions {
		if want, _ := enums.VoiceBurstAt(i); pos != want {
			t.Fatalf("voice burst %d is %v", i, pos)
		}
	}
}
