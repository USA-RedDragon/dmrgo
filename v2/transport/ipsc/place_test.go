package ipsc_test

import (
	"bufio"
	"encoding/hex"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
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
		h := u.Hint(0)
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
		h := u.Hint(0)
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

func renumber(t *testing.T, packets [][]byte, from int, by uint16) [][]byte {
	t.Helper()
	out := slices.Clone(packets)
	for i := from; i < len(out); i++ {
		u, err := ipsc.DecodeUser(out[i])
		if err != nil {
			t.Fatal(err)
		}
		u.RTP.Seq -= by
		if out[i], err = u.AppendBinary(nil); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestReceiverLostAfterXPRHeader(t *testing.T) {
	t.Parallel()

	sync := readWeak(t, "synclost")
	for _, c := range []struct {
		name    string
		packets [][]byte
		want    string
	}{
		{"call", sync[:8], "A0B0C0D0E0F0"},
		{"A lost", drop(sync[:8], 2), "B1C0D0E0F0"},
		{"late entry header of a relay, then E", renumber(t, drop(sync[:8], 2, 3, 4, 5), 2, 4), "E0F0"},
		{"A skipped", renumber(t, drop(sync[:8], 2), 2, 1), "B1C0D0E0F0"},
		{"six lost, then A skipped", renumber(t, drop(sync[:14], 2, 3, 4, 5, 6, 7, 8), 2, 1), "B7C0D0E0F0"},
	} {
		r, err := voice.NewReceiver(&voice.ReceiverConfig{Lost: -1, Hang: -1})
		if err != nil {
			t.Fatal(err)
		}
		var sb strings.Builder
		var ev []voice.Event
		now := time.Duration(0)
		for _, p := range c.packets {
			u, err := ipsc.DecodeUser(p)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := u.RawBurst(1, layer2.SyncSourceBS)
			if err != nil {
				t.Fatal(err)
			}
			now += 60 * time.Millisecond
			ev = r.Burst(now, &raw, u.Hint(0), ev[:0])
			for _, e := range ev {
				if i, ok := e.Position.Index(); ok && e.Kind == voice.EventVoice {
					sb.WriteByte("ABCDEF"[i])
					sb.WriteString(strconv.Itoa(int(e.Lost)))
				}
			}
		}
		if got := sb.String(); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestPositionTrackerXPRSyncLoss(t *testing.T) {
	t.Parallel()

	sync := readWeak(t, "synclost")
	for _, c := range []struct {
		name    string
		periods int
		jitter  time.Duration
		want    int
	}{
		{"no times", 0, 0, 1},
		{"one superframe", 6, 0, 5},
		{"two superframes", 12, 0, 11},
		{"two superframes, A 75 ms late", 12, 75 * time.Millisecond, 11},
		{"two superframes, A 75 ms early", 12, -75 * time.Millisecond, 11},
	} {
		var tr layer2.PositionTracker
		lost := -1
		for i, p := range sync[:17] {
			u, err := ipsc.DecodeUser(p)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Second + time.Duration(i)*60*time.Millisecond
			if i == 16 {
				now = time.Second + time.Duration(14+c.periods)*60*time.Millisecond + c.jitter
			}
			if c.periods == 0 {
				now = 0
			}
			h := u.Hint(now)
			raw, err := u.RawBurst(1, layer2.SyncSourceBS)
			if err != nil {
				tr.Place(nil, &h)
				continue
			}
			n, _ := tr.Place(&raw, &h)
			if i == 16 {
				lost = n
				if h.VoiceBurst != enums.VoiceBurstA {
					t.Fatalf("%s: %v after the sync loss", c.name, h.VoiceBurst)
				}
			}
		}
		if lost != c.want {
			t.Errorf("%s: lost %d, want %d", c.name, lost, c.want)
		}
	}
}
