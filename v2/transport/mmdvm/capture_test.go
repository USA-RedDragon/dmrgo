package mmdvm_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/mmdvm"
)

type capFrame struct {
	dir string
	p   []byte
}

func readFrames(t *testing.T, name string) []capFrame {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []capFrame
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		p, err := hex.DecodeString(fs[1])
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, capFrame{fs[0], p})
	}
	return out
}

func TestCapturedFramesRoundTrip(t *testing.T) {
	t.Parallel()

	for _, file := range []struct {
		name     string
		protocol uint8
	}{{"pistar.txt", 1}, {"mmdvmhost.txt", 2}} {
		var stream []byte
		frames := readFrames(t, file.name)
		for _, c := range frames {
			stream = append(stream, c.p...)
		}
		for i, c := range frames {
			f, n, st := mmdvm.Next(stream)
			if st != mmdvm.StatusFrame || n != len(c.p) {
				t.Fatalf("%s %d: %v %d", file.name, i, st, n)
			}
			stream = stream[n:]
			back, err := mmdvm.AppendFrame(nil, f.Command, f.Payload)
			if err != nil || !bytes.Equal(back, c.p) {
				t.Fatalf("%x: frame %x %v", c.p, back, err)
			}
			checkMessage(t, c, f, file.protocol)
		}
		if len(stream) != 0 {
			t.Fatalf("%s: %x left", file.name, stream)
		}
	}
}

func checkMessage(t *testing.T, c capFrame, f mmdvm.Frame, protocol uint8) {
	t.Helper()
	out, err := appendMessage(f, protocol, c.dir)
	if err != nil {
		t.Fatalf("%x: %v", c.p, err)
	}
	if out != nil && !bytes.Equal(out, f.Payload) {
		t.Fatalf("%x: payload %x, want %x", c.p, out, f.Payload)
	}
}

func appendMessage(f mmdvm.Frame, protocol uint8, dir string) ([]byte, error) {
	switch {
	case f.Command == mmdvm.CommandGetVersion && dir == "m>h":
		v, err := mmdvm.DecodeVersion(f.Payload)
		if err != nil {
			return nil, err
		}
		return v.AppendBinary(nil)
	case f.Command == mmdvm.CommandGetStatus && dir == "m>h" && protocol == 1:
		s, err := mmdvm.DecodeStatusV1(f.Payload)
		if err != nil {
			return nil, err
		}
		return s.AppendBinary(nil)
	case f.Command == mmdvm.CommandSetConfig && protocol == 1:
		s, err := mmdvm.DecodeConfigV1(f.Payload)
		if err != nil {
			return nil, err
		}
		return s.AppendBinary(nil)
	case f.Command == mmdvm.CommandSetConfig:
		s, err := mmdvm.DecodeConfigV2(f.Payload)
		if err != nil {
			return nil, err
		}
		return s.AppendBinary(nil)
	case f.Command == mmdvm.CommandSetFreq:
		s, err := mmdvm.DecodeSetFreq(f.Payload)
		if err != nil {
			return nil, err
		}
		return s.AppendBinary(nil)
	case f.Command == mmdvm.CommandSetMode:
		s, err := mmdvm.DecodeSetMode(f.Payload)
		if err != nil {
			return nil, err
		}
		return s.AppendBinary(nil)
	case f.Command == mmdvm.CommandAck:
		s, err := mmdvm.DecodeAck(f.Payload)
		if err != nil {
			return nil, err
		}
		return s.AppendBinary(nil)
	case f.Command == mmdvm.CommandDMRShortLC:
		s, err := mmdvm.DecodeDMRShortLC(f.Payload)
		if err != nil {
			return nil, err
		}
		return s.AppendBinary(nil)
	case f.Command == mmdvm.CommandDMRData1 || f.Command == mmdvm.CommandDMRData2:
		d, err := mmdvm.DecodeDMRData(f)
		if err != nil {
			return nil, err
		}
		return d.AppendBinary(nil)
	}
	return nil, nil
}

func TestPiStarStart(t *testing.T) {
	t.Parallel()

	frames := readFrames(t, "pistar.txt")
	f, _, _ := mmdvm.Next(frames[1].p)
	v, err := mmdvm.DecodeVersion(f.Payload)
	if err != nil || v.Protocol != 1 || v.Capabilities != mmdvm.CapabilitiesV1 || !bytes.HasPrefix(v.Description, []byte("MMDVM_HS_Dual_Hat-v1.6.0 ")) {
		t.Fatalf("%+v %v", v, err)
	}
	f, _, _ = mmdvm.Next(frames[2].p)
	sf, err := mmdvm.DecodeSetFreq(f.Payload)
	if err != nil || sf.RX != 430000000 || sf.TX != 440000000 || sf.POCSAG != 433000000 || sf.Size != 14 || sf.RFLevel != 0x59 {
		t.Fatalf("%+v %v", sf, err)
	}
	f, _, _ = mmdvm.Next(frames[4].p)
	cfg, err := mmdvm.DecodeConfigV1(f.Payload)
	if err != nil || !cfg.TXInvert || cfg.RXInvert || !cfg.DMR || cfg.DStar || cfg.TXDelay != 1 || cfg.RXLevel != 102 || cfg.DMRColorCode != 1 || cfg.OscOffset != 128 || cfg.Simplex {
		t.Fatalf("%+v %v", cfg, err)
	}
	f, _, _ = mmdvm.Next(frames[9].p)
	st, err := mmdvm.DecodeStatusV1(f.Payload)
	if err != nil || !st.DMR || st.DStar || st.State != mmdvm.ModeIdle || st.TX || st.DMRSpace1 != 28 || st.DMRSpace2 != 28 {
		t.Fatalf("%+v %v", st, err)
	}
}

func TestMMDVMHostCall(t *testing.T) {
	t.Parallel()

	var bursts int
	for _, c := range readFrames(t, "mmdvmhost.txt") {
		f, _, _ := mmdvm.Next(c.p)
		switch f.Command {
		case mmdvm.CommandDMRData2:
			d, err := mmdvm.DecodeDMRData(f)
			if err != nil || d.Slot != enums.Slot2 || d.Control != 0 || d.HasRSSI {
				t.Fatalf("%x: %+v %v", c.p, d, err)
			}
			if h := d.Hint(); h.Kind != layer2.KindUnknown || h.Slot != enums.Slot2 {
				t.Errorf("%x: %+v", c.p, h)
			}
			if sync, _ := d.Burst.Sync(); sync == enums.SyncBSSourcedData {
				if st := d.Burst.SlotType(); st.FEC.Uncorrectable || st.ColorCode != 7 {
					t.Errorf("%x: slot type %+v", c.p, st)
				}
			}
			bursts++
		case mmdvm.CommandSetConfig:
			cfg, err := mmdvm.DecodeConfigV2(f.Payload)
			if err != nil || !cfg.DMR || cfg.DMRColorCode != 7 || !cfg.Simplex || cfg.OscOffset != 128 {
				t.Fatalf("%+v %v", cfg, err)
			}
		default:
		}
	}
	if bursts == 0 {
		t.Fatal("no bursts")
	}
}
