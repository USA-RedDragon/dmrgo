package ipsc_test

import (
	"bufio"
	"encoding/hex"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func readWeak(t *testing.T, tag string) [][]byte {
	t.Helper()
	f, err := os.Open("testdata/weak.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out [][]byte
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if fs[0] != tag {
			continue
		}
		p, err := hex.DecodeString(fs[1])
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func placeAll(t *testing.T, packets [][]byte) string {
	t.Helper()
	var tr layer2.PositionTracker
	var sb strings.Builder
	for _, p := range packets {
		u, err := ipsc.DecodeUser(p)
		if err != nil {
			t.Fatal(err)
		}
		h := u.Hint()
		raw, err := u.RawBurst(1, layer2.SyncSourceBS)
		if err != nil {
			tr.Place(nil, &h)
			sb.WriteByte('-')
			continue
		}
		tr.Place(&raw, &h)
		i, ok := h.VoiceBurst.Index()
		switch {
		case h.Kind != layer2.KindVoice || !ok:
			sb.WriteByte('-')
		default:
			sb.WriteByte("ABCDEF"[i])
		}
	}
	return sb.String()
}

func drop(packets [][]byte, idx ...int) [][]byte {
	var out [][]byte
	for i, p := range packets {
		if !slices.Contains(idx, i) {
			out = append(out, p)
		}
	}
	return out
}

func TestPositionTrackerWeakSignalCaptures(t *testing.T) {
	t.Parallel()

	sync := readWeak(t, "synclost")
	next := readWeak(t, "nextcall")
	late := readWeak(t, "lateentry")
	both := append(slices.Clone(sync), next...)
	withoutA := append(slices.Clone(sync[2:8]), sync[9:15]...)
	var xpr [][]byte
	for _, c := range readXPR(t) {
		if c.tag == "voicecall" && len(xpr) < 20 {
			xpr = append(xpr, c.p)
		}
	}
	for _, c := range []struct {
		name    string
		packets [][]byte
		want    string
	}{
		{"burst lost and superframe restarted", sync, "--ABCDEFABCDEFA-ABCDEA"},
		{"next call", both, "--ABCDEFABCDEFA-ABCDEA-ABCDEA"},
		{"weak-signal call", late, "ABCDEFABCDEFA"},
		{"late entry before burst E", late[7:], "---EFA"},
		{"late entry at burst A", late[6:], "ABCDEFA"},
		{"burst A lost in transit", withoutA, "ABCDEFBCDEFA"},
		{"first burst A after the voice header lost", drop(sync, 2), "--BCDEFABCDEFA-ABCDEA"},
		{"restarted superframe with its burst A lost", drop(sync, 16), "--ABCDEFABCDEFA-BCDEA"},
		{"XPR 8400 call", xpr, "-ABCDEFABCDEFABCDEF-"},
		{"XPR 8400 call with LCSS single fragment on B and C", noEMB(t, xpr, 331, 332), "-ABCDEFABCDEFABCDEF-"},
	} {
		if got := placeAll(t, c.packets); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
		if got := receiverPositions(t, c.packets); got != c.want {
			t.Errorf("%s: voice.Receiver gives %s, want %s", c.name, got, c.want)
		}
	}
}

func noEMB(t *testing.T, packets [][]byte, seqs ...uint16) [][]byte {
	t.Helper()
	out := make([][]byte, len(packets))
	for i, p := range packets {
		u, err := ipsc.DecodeUser(p)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = p
		if slices.Contains(seqs, u.RTP.Seq) {
			u.Voice.EMB = 0
			if out[i], err = u.AppendBinary(nil); err != nil {
				t.Fatal(err)
			}
		}
	}
	return out
}

func receiverPositions(t *testing.T, packets [][]byte) string {
	t.Helper()
	r, err := voice.NewReceiver(&voice.ReceiverConfig{Lost: -1, Hang: -1})
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	var ev []voice.Event
	now := time.Duration(0)
	for _, p := range packets {
		u, err := ipsc.DecodeUser(p)
		if err != nil {
			t.Fatal(err)
		}
		h := u.Hint()
		raw, err := u.RawBurst(1, layer2.SyncSourceBS)
		now += 60 * time.Millisecond
		if err != nil {
			if h.SyncLost {
				ev = r.Lost(now, ev[:0])
			}
			sb.WriteByte('-')
			continue
		}
		ev = r.Burst(now, &raw, h, ev[:0])
		letter := byte('-')
		for _, e := range ev {
			if i, ok := e.Position.Index(); ok && e.Kind == voice.EventVoice {
				letter = "ABCDEF"[i]
			}
		}
		sb.WriteByte(letter)
	}
	return sb.String()
}
