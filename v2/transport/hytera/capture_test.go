//go:build !tinygo

package hytera_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/hytera"
)

// The captures of Hytera_Homebrew_Bridge are AGPL test data, so they are
// not in this repository. DMRGO_HYTERA_TSV names text exports of them,
// separated by commas, with the UDP payload in hex in the last column of
// each line (tshark -T fields -e data.data).
func TestCaptures(t *testing.T) {
	t.Parallel()

	paths := os.Getenv("DMRGO_HYTERA_TSV")
	if paths == "" {
		t.Skip("DMRGO_HYTERA_TSV is not set")
	}
	var n captureCounts
	for _, path := range strings.Split(paths, ",") {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 1<<16), 1<<20)
		for sc.Scan() {
			fs := strings.Split(sc.Text(), "\t")
			p, err := hex.DecodeString(fs[len(fs)-1])
			if err != nil || len(p) == 0 {
				continue
			}
			checkCapture(t, p, &n)
		}
		_ = f.Close()
	}
	t.Logf("%+v", n)
	if n.voiceA == 0 || n.data == 0 || n.heartbeats == 0 {
		t.Fatalf("%+v", n)
	}
}

type captureCounts struct {
	frames, voiceA, voice, data, noBurst, empty, heartbeats, p2p, other int
}

func checkCapture(t *testing.T, p []byte, n *captureCounts) {
	t.Helper()
	checkPacket(t, p)
	switch {
	case len(p) == hytera.FrameLen && p[0] != 0x7E:
		n.frames++
		f, err := hytera.DecodeFrame(p)
		if err != nil {
			t.Fatalf("%x: %v", p, err)
		}
		checkBurst(t, p, f, n)
	case len(p) == 1 || len(p) == hytera.HeartbeatLen && p[0] == 'Z':
		if _, err := hytera.DecodeHeartbeat(p); err != nil {
			t.Fatalf("%x: %v", p, err)
		}
		n.heartbeats++
	case bytes.HasPrefix(p, []byte("P2P")):
		if _, err := hytera.DecodeP2P(p); err != nil {
			t.Fatalf("%x: %v", p, err)
		}
		n.p2p++
	default:
		n.other++
	}
}

func checkBurst(t *testing.T, p []byte, f hytera.Frame, n *captureCounts) {
	t.Helper()
	if !f.HasBurst() {
		n.noBurst++
		return
	}
	if f.Burst == (layer2.RawBurst{}) {
		n.empty++
		return
	}
	h := f.Hint(0)
	sync, _ := f.Burst.Sync()
	switch h.Kind {
	case layer2.KindVoice:
		n.voice++
		if h.VoiceBurst == enums.VoiceBurstA {
			n.voiceA++
			if sync != enums.SyncBSSourcedVoice && sync != enums.SyncMSSourcedVoice {
				t.Errorf("%x: burst A has SYNC %v", p, sync)
			}
		}
	case layer2.KindData:
		n.data++
		st := f.Burst.SlotType()
		if st.FEC.Uncorrectable || st.DataType != h.DataType || st.ColorCode != f.ColorCode {
			t.Errorf("%x: slot type %+v, frame %v CC %d", p, st, f.SlotType, f.ColorCode)
		}
		if !h.DataType.IsBPTC() {
			return
		}
		pl, res := f.Burst.BPTCPayload()
		if res.Uncorrectable {
			t.Errorf("%x: BPTC uncorrectable", p)
		}
		switch h.DataType {
		case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC:
			if !layer2.MMDVMHostAccepts(h.DataType, pl) {
				t.Errorf("%x: %v fails its RS(12,9)", p, h.DataType)
			}
			dst := uint32(pl[3])<<16 | uint32(pl[4])<<8 | uint32(pl[5])
			src := uint32(pl[6])<<16 | uint32(pl[7])<<8 | uint32(pl[8])
			if dst != f.Destination || src != f.Source {
				t.Errorf("%x: LC %d to %d, frame %d to %d", p, src, dst, f.Source, f.Destination)
			}
		case enums.DataTypeCSBK, enums.DataTypeDataHeader:
			if !layer2.MMDVMHostAccepts(h.DataType, pl) {
				t.Errorf("%x: %v fails its CRC", p, h.DataType)
			}
		default:
		}
	default:
		t.Errorf("%x: slot type %v gives no kind", p, f.SlotType)
	}
}
