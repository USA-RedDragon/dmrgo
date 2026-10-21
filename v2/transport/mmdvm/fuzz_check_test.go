package mmdvm_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

func fuzzSeeds() [][]byte {
	seeds := make([][]byte, 0, 8)
	for _, h := range []string{
		"e00e01020000001c1c0000000000",
		"e01104008047a11900de391a59400ecf19",
		"e01a020202010066800100808083808080808004808005058005",
		"e028028202000a0080808080808080808000808000000004050500000007008000000000000000000000",
		"e0251a0053c25eaba8671dc7383bd9361e7dff57d75df5d47bf6e465171b0000000000000000000000",
		"e00c1c0339a0005330a55a50",
		"e0047004e0057f0204",
		"e00af3746578740001ffff",
	} {
		p, _ := hex.DecodeString(h)
		seeds = append(seeds, p)
	}
	return seeds
}

func checkStream(t *testing.T, p []byte) {
	t.Helper()
	for len(p) > 0 {
		f, n, st := mmdvm.Next(p)
		if st == mmdvm.StatusMore {
			return
		}
		if n <= 0 || n > len(p) {
			t.Fatalf("%x: n %d", p, n)
		}
		if st == mmdvm.StatusFrame {
			back, err := mmdvm.AppendFrame(nil, f.Command, f.Payload)
			g, m, gst := mmdvm.Next(back)
			if err != nil || gst != mmdvm.StatusFrame || m != len(back) || g.Command != f.Command || !bytes.Equal(g.Payload, f.Payload) {
				t.Fatalf("frame %x: %x %v", p[:n], back, err)
			}
			if p[1] != 0 && !bytes.Equal(back, p[:n]) {
				t.Fatalf("short frame %x: %x", p[:n], back)
			}
			checkPayload(t, f)
		}
		p = p[n:]
	}
}

func checkPayload(t *testing.T, f mmdvm.Frame) {
	t.Helper()
	if d, err := mmdvm.DecodeDMRData(f); err == nil {
		out, _ := d.AppendBinary(nil)
		if !bytes.Equal(out, f.Payload) {
			t.Fatalf("DMR data %x: %x", f.Payload, out)
		}
		_ = d.Hint()
	}
	if v, err := mmdvm.DecodeVersion(f.Payload); err == nil {
		out, _ := v.AppendBinary(nil)
		if !bytes.Equal(out, f.Payload) {
			t.Fatalf("version %x: %x", f.Payload, out)
		}
	}
	if d, err := mmdvm.DecodeDebug(f); err == nil {
		out, err := d.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, f.Payload) {
			t.Fatalf("debug %x: %x %v", f.Payload, out, err)
		}
	}
	if s, err := mmdvm.DecodeSetFreq(f.Payload); err == nil {
		out, err := s.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, f.Payload) {
			t.Fatalf("set freq %x: %x %v", f.Payload, out, err)
		}
	}
	_, _ = mmdvm.DecodeStatusV1(f.Payload)
	_, _ = mmdvm.DecodeStatusV2(f.Payload)
	_, _ = mmdvm.DecodeConfigV1(f.Payload)
	_, _ = mmdvm.DecodeConfigV2(f.Payload)
}
