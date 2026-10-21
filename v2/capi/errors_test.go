package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func lockCodes(t *testing.T) map[string]int {
	t.Helper()
	f, err := os.Open("abi.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	codes := map[string]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		if fl := strings.Fields(sc.Text()); len(fl) == 3 && fl[0] == "code" {
			n, err := strconv.Atoi(fl[2])
			if err != nil {
				t.Fatal(err)
			}
			codes[fl[1]] = n
		}
	}
	return codes
}

func TestErrCodeChain(t *testing.T) {
	t.Parallel()

	codes := lockCodes(t)
	for _, c := range []struct {
		err  error
		want string
	}{
		{layer2.ErrVoiceTerminated, "DMRGO_ERR_LAYER2_VOICE_TERMINATED"},
		{fmt.Errorf("x: %w", voice.ErrBusy), "DMRGO_ERR_VOICE_BUSY"},
		{errors.Join(voice.ErrBusy, layer2.ErrVoiceTerminated), "DMRGO_ERR_VOICE_BUSY"},
		{errors.Join(layer2.ErrVoiceTerminated, voice.ErrBusy), "DMRGO_ERR_LAYER2_VOICE_TERMINATED"},
		{fmt.Errorf("x: %w", &dmrgo.ConfigError{Type: "voice", Field: "ColorCode"}), "DMRGO_ERR_CONFIG_ERROR"},
		{fmt.Errorf("%w: %w", pdu.ErrNoAddresses, &dmrgo.FieldRangeError{Type: "enums", Field: "ColorCode", Bits: 4}), "DMRGO_ERR_PDU_NO_ADDRESSES"},
	} {
		want, ok := codes[c.want]
		if !ok {
			t.Fatalf("%s not in abi.txt", c.want)
		}
		if got := int(errCode(c.err)); got != want {
			t.Errorf("errCode(%v) = %d, want %s (%d)", c.err, got, c.want, want)
		}
	}
	if got := int(errCode(errors.New("other"))); got != -8 {
		t.Errorf("errCode(other) = %d, want -8", got)
	}
}

func TestPartialResultsAsGo(t *testing.T) {
	t.Parallel()

	lc, err := pdu.NewVoiceLC(true, 9990, 3191868, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := layer2.NewVoiceTransmitter(&layer2.VoiceTransmitterOptions{LC: lc, ColorCode: 1, Source: layer2.SyncSourceMS})
	if err != nil {
		t.Fatal(err)
	}
	frames := make([]vocoder.AMBEFrame, 7)
	for i := range frames {
		frames[i] = vocoder.AMBESilence()
	}
	frames[6][6] |= 1
	got, gerr := tx.AppendFrames(nil, frames...)
	if gerr == nil {
		t.Fatal("Go AppendFrames took a bad frame")
	}
	n, rc, err := partialC()
	if err != nil {
		t.Fatal(err)
	}
	if n != len(got) || rc != int(errCode(gerr)) {
		t.Errorf("C wrote %d bursts with status %d; Go returned %d with %d (%v)", n, rc, len(got), int(errCode(gerr)), gerr)
	}
}

func TestLastMessageLength(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		msg               string
		capx, n, need, rc int
	}{
		{strings.Repeat("x", 600), 1024, 511, 512, -6},
		{"short", 1024, 5, 6, 0},
		{"short", 3, 2, 6, -6},
	} {
		n, need, rc := lastMessage(c.msg, c.capx)
		if n != c.n || need != c.need || rc != c.rc {
			t.Errorf("%d-byte message, cap %d: len %d need %d status %d, want %d %d %d", len(c.msg), c.capx, n, need, rc, c.n, c.need, c.rc)
		}
	}
}
