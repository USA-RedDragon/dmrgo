package dvsi_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/vocoder/dvsi"
)

func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func ramp() []int16 {
	pcm := make([]int16, 160)
	for i := range pcm {
		pcm[i] = int16(i)
	}
	return pcm
}

func rampHex() string {
	var b strings.Builder
	for i := range 160 {
		b.WriteString(hex.EncodeToString([]byte{byte(i >> 8), byte(i)}))
	}
	return b.String()
}

func TestBuildManualPackets(t *testing.T) {
	t.Parallel()

	data := unhex(t, "00112233445566778899")
	ch80, err := dvsi.AppendChannel(nil, 0, data, 80, false)
	if err != nil {
		t.Fatal(err)
	}
	ch80p2, err := dvsi.AppendChannel(nil, 2, data, 80, true)
	if err != nil {
		t.Fatal(err)
	}
	rate, err := dvsi.AppendRateDMR(nil, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	rate1, err := dvsi.AppendRateDMR(nil, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	speech, err := dvsi.AppendSpeech(nil, 0, ramp(), true)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		got  []byte
		want string
	}{
		{"reset with parity, xlxd ambed", dvsi.AppendReset(nil, true), "610003 00 33 2f 1f"},
		{"reset, AMBEserver", dvsi.AppendReset(nil, false), "610001 00 33"},
		{"query with parity, xlxd ambed", dvsi.AppendQuery(nil, true), "610004 00 30 31 2f 2a"},
		{"query, serialDV and AMBEserver", dvsi.AppendQuery(nil, false), "610002 00 30 31"},
		{"parity off, AMBE-3003 manual Table 104", dvsi.AppendParityMode(nil, false, true), "610004 00 3f 00 2f 14"},
		{"parity on", dvsi.AppendParityMode(nil, true, false), "610002 00 3f 01"},
		{"DMR rate, serialDV 3600x2450 RATEP and INIT", rate, "61000f 00 0a 0431 0754 2400 0000 0000 6f48 0b 03"},
		{"DMR rate on channel 1", rate1, "610012 00 41 0a 0431 0754 2400 0000 0000 6f48 0b 03 2f 1b"},
		{"channel, AMBE-3000R manual Table 112", ch80, "61000c 01 01 50 00112233445566778899"},
		{"channel 2 with parity, AMBE-3003 manual Table 101 with LENGTH and parity counting its channel field", ch80p2, "61000f 01 42 01 50 00112233445566778899 2f 23"},
		{"speech with parity", speech, "610144 02 00 a0" + rampHex() + "2f c8"},
	}
	for _, tt := range tests {
		if want := unhex(t, tt.want); !bytes.Equal(tt.got, want) {
			t.Errorf("%s:\n got %x\nwant %x", tt.name, tt.got, want)
		}
	}
}

func TestBuildAppends(t *testing.T) {
	t.Parallel()

	dst := []byte{1, 2, 3}
	got := dvsi.AppendReset(dst, false)
	if !bytes.Equal(got, unhex(t, "010203 610001 00 33")) {
		t.Errorf("AppendReset to a non-empty dst = %x", got)
	}
	got, err := dvsi.AppendChannel([]byte{9}, 0, make([]byte, 9), 72, true)
	if err != nil {
		t.Fatal(err)
	}
	p, n, err := dvsi.Parse(got[1:], true)
	if err != nil || n != len(got)-1 || p.Type != dvsi.TypeChannel {
		t.Errorf("Parse of a channel packet after other bytes = %v, %d, %v", p, n, err)
	}
}

func TestBuildErrors(t *testing.T) {
	t.Parallel()

	dst := []byte{7}
	check := func(name string, got []byte, err, want error) {
		t.Helper()
		if !errors.Is(err, want) || !bytes.Equal(got, dst) {
			t.Errorf("%s = %x, %v; want dst unchanged and %v", name, got, err, want)
		}
	}
	got, err := dvsi.AppendRateDMR(dst, 3, false)
	check("AppendRateDMR channel 3", got, err, dvsi.ErrChannel)
	got, err = dvsi.AppendRateDMR(dst, -1, false)
	check("AppendRateDMR channel -1", got, err, dvsi.ErrChannel)
	got, err = dvsi.AppendChannel(dst, 3, make([]byte, 9), 72, false)
	check("AppendChannel channel 3", got, err, dvsi.ErrChannel)
	got, err = dvsi.AppendChannel(dst, 0, make([]byte, 8), 71, false)
	check("AppendChannel 71 bits in 8 bytes", got, err, dvsi.ErrBits)
	got, err = dvsi.AppendChannel(dst, 0, make([]byte, 4), 32, false)
	check("AppendChannel 32 bits", got, err, dvsi.ErrBits)
	got, err = dvsi.AppendChannel(dst, 0, make([]byte, 25), 200, false)
	check("AppendChannel 200 bits", got, err, dvsi.ErrBits)
	got, err = dvsi.AppendSpeech(dst, 0, make([]int16, 155), false)
	check("AppendSpeech 155 samples", got, err, dvsi.ErrSamples)
	got, err = dvsi.AppendSpeech(dst, 0, make([]int16, 165), false)
	check("AppendSpeech 165 samples", got, err, dvsi.ErrSamples)
	got, err = dvsi.AppendSpeech(dst, 5, make([]int16, 160), false)
	check("AppendSpeech channel 5", got, err, dvsi.ErrChannel)
	for _, n := range []int{dvsi.MinSamples, dvsi.MaxSamples} {
		b, err := dvsi.AppendSpeech(nil, 2, make([]int16, n), true)
		if err != nil {
			t.Fatal(err)
		}
		p, _, err := dvsi.Parse(b, true)
		if err != nil {
			t.Fatal(err)
		}
		pcm, ch, err := p.AppendSamples(nil)
		if err != nil || ch != 2 || len(pcm) != n {
			t.Errorf("%d samples on channel 2 parse as %d samples on channel %d, %v", n, len(pcm), ch, err)
		}
	}
}

func TestParseManualPackets(t *testing.T) {
	t.Parallel()

	ready := unhex(t, "610003 00 39 2f 15")
	p, n, err := dvsi.Parse(ready, true)
	if err != nil || n != len(ready) || p.Type != dvsi.TypeControl || !bytes.Equal(p.Fields, []byte{0x39}) {
		t.Errorf("PKT_READY of AMBE-3003 manual Table 103 = %v, %d, %v", p, n, err)
	}
	if d, ok := p.Response(dvsi.FieldReady); !ok || len(d) != 0 {
		t.Errorf("Response(FieldReady) = %x, %v", d, ok)
	}

	resp := unhex(t, "610002 00 3f 00")
	p, _, err = dvsi.Parse(resp, false)
	if err != nil {
		t.Fatal(err)
	}
	if d, ok := p.Response(dvsi.FieldParityMode); !ok || !bytes.Equal(d, []byte{0}) {
		t.Errorf("PARITYMODE response of AMBE-3003 manual Table 105 = %x, %v", d, ok)
	}

	noChannel := unhex(t, "61000e 01 01 50 00112233445566778899 2f 60")
	if _, _, err := dvsi.Parse(noChannel, true); err != nil {
		t.Errorf("AMBE-3003 manual Table 101 without its channel field, as its LENGTH and parity count it: %v", err)
	}

	speech := unhex(t, "610145 02 40 00 a0"+rampHex()+"2f 89")
	p, n, err = dvsi.Parse(speech, true)
	if err != nil || n != len(speech) {
		t.Fatalf("speech packet of AMBE-3003 manual Table 99: %d, %v", n, err)
	}
	pcm, ch, err := p.AppendSamples(nil)
	if err != nil || ch != 0 || !slices.Equal(pcm, ramp()) {
		t.Errorf("samples of Table 99 = %v on channel %d, %v", pcm, ch, err)
	}

	speech3000 := unhex(t, "610143 02 40 00 a0"+rampHex())
	p, _, err = dvsi.Parse(speech3000, false)
	if err != nil {
		t.Fatal(err)
	}
	if pcm, _, err := p.AppendSamples(nil); err != nil || !slices.Equal(pcm, ramp()) {
		t.Errorf("samples of AMBE-3000R manual Table 110 = %v, %v", pcm, err)
	}

	chan2 := unhex(t, "61000f 01 42 01 38 00112233445566 03 a1 02 0000")
	p, _, err = dvsi.Parse(chan2, false)
	if err != nil {
		t.Fatal(err)
	}
	data, bits, ch, err := p.AppendChannelData(nil)
	if err != nil || bits != 56 || ch != 2 || !bytes.Equal(data, unhex(t, "00112233445566")) {
		t.Errorf("channel packet of AMBE-3003 manual Table 102 = %x, %d bits, channel %d, %v", data, bits, ch, err)
	}

	xlxd := unhex(t, "61000b 01 01 48 b9e881526173002a6b")
	p, _, err = dvsi.Parse(xlxd, false)
	if err != nil {
		t.Fatal(err)
	}
	if data, bits, ch, err := p.AppendChannelData(nil); err != nil || bits != 72 || ch != 0 || len(data) != 9 {
		t.Errorf("72-bit channel packet as xlxd ambed reads it = %x, %d, %d, %v", data, bits, ch, err)
	}
}

func TestParseQueryResponse(t *testing.T) {
	t.Parallel()

	b := make([]byte, 0, 64)
	b = append(b, 0x61, 0, 0, 0, 0x30)
	b = append(b, "AMBE3000R\x00"...)
	b = append(b, 0x31)
	b = append(b, "V120.E100.XXXX.C106.G514.R009.B0010411.C0020208\x00"...)
	n := len(b) - 4
	b[1], b[2] = byte(n>>8), byte(n)
	p, _, err := dvsi.Parse(b, false)
	if err != nil {
		t.Fatal(err)
	}
	if d, ok := p.Response(dvsi.FieldProdID); !ok || string(d) != "AMBE3000R" {
		t.Errorf("product = %q, %v", d, ok)
	}
	if d, ok := p.Response(dvsi.FieldVerString); !ok || !strings.HasPrefix(string(d), "V120.") {
		t.Errorf("version = %q, %v", d, ok)
	}
	if _, ok := p.Response(dvsi.FieldReset); ok {
		t.Error("Response found a field the packet does not have")
	}
	p.Fields = p.Fields[:5]
	if _, ok := p.Response(dvsi.FieldProdID); ok {
		t.Error("Response read a product name without its zero byte")
	}
	p.Type = dvsi.TypeSpeech
	if _, ok := p.Response(dvsi.FieldProdID); ok {
		t.Error("Response read a speech packet")
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	good := unhex(t, "610003 00 39 2f 15")
	for i := range len(good) {
		if _, _, err := dvsi.Parse(good[:i], true); !errors.Is(err, dvsi.ErrIncomplete) {
			t.Errorf("Parse of %d of %d bytes = %v, want ErrIncomplete", i, len(good), err)
		}
	}
	tests := []struct {
		name   string
		b      string
		parity bool
		want   error
	}{
		{"no start byte", "620003 00 39 2f 15", true, dvsi.ErrStart},
		{"packet type 3", "610001 03 39", false, dvsi.ErrType},
		{"LENGTH over MaxLength", "610181 00", false, dvsi.ErrLength},
		{"too short for parity", "610001 00 39", true, dvsi.ErrLength},
		{"wrong parity byte", "610003 00 39 2f 16", true, dvsi.ErrParity},
		{"no parity field", "610003 00 39 30 15", true, dvsi.ErrParity},
	}
	for _, tt := range tests {
		if _, n, err := dvsi.Parse(unhex(t, tt.b), tt.parity); !errors.Is(err, tt.want) || n != 0 {
			t.Errorf("%s: Parse = %d, %v, want 0, %v", tt.name, n, err, tt.want)
		}
	}
}

func TestFieldErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		b    string
		want error
	}{
		{"speech: control packet", "610002 00 3f 00", dvsi.ErrField},
		{"speech: no SPEECHD", "610001 02 40", dvsi.ErrField},
		{"speech: cut SPEECHD", "610004 02 00 a0 0000", dvsi.ErrField},
		{"speech: unknown field", "610001 02 55", dvsi.ErrField},
		{"speech: channel after SPEECHD", "61" + "0143 02 00 a0" + strings.Repeat("00", 320) + "41", dvsi.ErrField},
		{"speech: 155 samples", "61" + "0138 02 00 9b" + strings.Repeat("00", 310), dvsi.ErrSamples},
		{"speech: cut CMODE", "61" + "0144 02 00 a0" + strings.Repeat("00", 320) + "02 00", dvsi.ErrField},
		{"channel: two CHAND", "610016 01 01 48 000000000000000000 01 48 000000000000000000", dvsi.ErrField},
		{"channel: 32 bits", "610006 01 01 20 00000000", dvsi.ErrBits},
		{"channel: cut CHAND4", "61000c 01 01 48 000000000000000000 17", dvsi.ErrField},
	}
	for _, tt := range tests {
		b := unhex(t, tt.b)
		p, _, err := dvsi.Parse(b, false)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		var got error
		if strings.HasPrefix(tt.name, "channel") {
			_, _, _, got = p.AppendChannelData(nil)
		} else {
			_, _, got = p.AppendSamples(nil)
		}
		if !errors.Is(got, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, got, tt.want)
		}
	}
	ok := unhex(t, "610013 01 40 01 48 000000000000000000 03a0 020000 1748")
	p, _, _ := dvsi.Parse(ok, false)
	if _, _, _, err := p.AppendChannelData(nil); !errors.Is(err, dvsi.ErrField) {
		t.Errorf("CHAND4 cut short = %v", err)
	}
	ok = unhex(t, "610015 01 40 01 48 000000000000000000 03a0 020000 080300 17")
	p, _, _ = dvsi.Parse(ok, false)
	if _, _, _, err := p.AppendChannelData(nil); !errors.Is(err, dvsi.ErrField) {
		t.Errorf("CHAND4 without a bit count = %v", err)
	}
	ok = unhex(t, "61001a 01 40 01 48 000000000000000000 03a0 020000 080300 1708 00112233")
	p, _, _ = dvsi.Parse(ok, false)
	if d, bits, _, err := p.AppendChannelData(nil); err != nil || bits != 72 || len(d) != 9 {
		t.Errorf("CHAND with SAMPLES, CMODE, TONE and CHAND4 fields = %d bits, %v", bits, err)
	}
}
