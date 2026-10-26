package dmrd_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

type captured struct {
	dir string
	p   []byte
}

func readCapture(t *testing.T, name string) []captured {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []captured
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 4096)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) != 4 {
			t.Fatalf("bad line %q", sc.Text())
		}
		p, err := hex.DecodeString(fs[3])
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, captured{fs[1], p})
	}
	return out
}

func TestDMRGatewayCapture(t *testing.T) {
	t.Parallel()

	seen := map[dmrd.Command]int{}
	for _, c := range readCapture(t, "dmrgateway-master.txt") {
		cmd := dmrd.Classify(c.p, dmrd.DialectMMDVM)
		seen[cmd]++
		checkGatewayPacket(t, cmd, c)
	}
	for _, c := range []dmrd.Command{dmrd.CommandRPTL, dmrd.CommandRPTACK, dmrd.CommandRPTK, dmrd.CommandRPTC, dmrd.CommandRPTO, dmrd.CommandRPTPING, dmrd.CommandMSTPONG, dmrd.CommandDMRD, dmrd.CommandRPTSBKN} {
		if seen[c] == 0 {
			t.Errorf("no %v in the capture", c)
		}
	}
}

func checkGatewayPacket(t *testing.T, cmd dmrd.Command, c captured) {
	t.Helper()
	switch cmd {
	case dmrd.CommandDMRD:
		d, err := dmrd.DecodeData(c.p)
		if err != nil || d.Size != 55 || d.Source != 3191868 || d.Destination != 91 || d.Slot != enums.Slot2 || d.Repeater != 3149999 {
			t.Fatalf("DMRD %x: %v %v", c.p, d.String(), err)
		}
		out, err := d.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, c.p) {
			t.Fatalf("DMRD %x: encode %x %v", c.p, out, err)
		}
		if h := d.Hint(0); h.Kind == layer2.KindUnknown {
			t.Fatalf("DMRD %x: hint %+v", c.p, h)
		}
	case dmrd.CommandRPTC:
		checkGatewayConfig(t, c.p)
	case dmrd.CommandUnknown:
		t.Fatalf("unknown packet %x", c.p)
	default:
		m, err := dmrd.DecodeControl(c.p, dmrd.DialectMMDVM)
		if err != nil {
			t.Fatalf("%v %x: %v", cmd, c.p, err)
		}
		out, err := m.AppendBinary(nil, dmrd.DialectMMDVM)
		if err != nil || !bytes.Equal(out, c.p) {
			t.Fatalf("%v %x: encode %x %v", cmd, c.p, out, err)
		}
		if cmd == dmrd.CommandRPTO {
			o, err := dmrd.ParseDMRPlusOptions(m.Text)
			if err != nil || o.StartRef.Value != 4013 || o.TS1[0].Value != 262 {
				t.Errorf("RPTO %q: %+v %v", m.Text, o, err)
			}
		}
	}
}

func checkGatewayConfig(t *testing.T, p []byte) {
	t.Helper()
	cfg, err := dmrd.MMDVMConfigOf(p)
	if err != nil {
		t.Fatal(err)
	}
	cc, _ := cfg.ColorCode()
	pw, _ := cfg.Power()
	h, _ := cfg.Height()
	rx, _ := cfg.RXFrequency()
	tx, _ := cfg.TXFrequency()
	s, _ := cfg.Slots()
	if cfg.ID() != 3149999 || cc != 9 || pw != 8 || h != 10 || rx != 447000000 || tx != 434500000 || s != 3 ||
		string(cfg.Callsign()) != "KI5VMF" || string(cfg.Location()) != "Anytown" || string(cfg.PackageID()) != "MMDVM" {
		t.Errorf("RPTC: id %d cc %d power %d height %d rx %d tx %d slots %d %q %q %q", cfg.ID(), cc, pw, h, rx, tx, s, cfg.Callsign(), cfg.Location(), cfg.PackageID())
	}
}

func TestMMDVMHostLinkCapture(t *testing.T) {
	t.Parallel()

	n := 0
	for _, c := range readCapture(t, "mmdvmhost-dmrgateway.txt") {
		switch cmd := dmrd.Classify(c.p, dmrd.DialectLocal); cmd {
		case dmrd.CommandDMRD:
			d, err := dmrd.DecodeData(c.p)
			if err != nil || d.Size != 55 {
				t.Fatalf("DMRD %x: %v", c.p, err)
			}
			if out, _ := d.AppendBinary(nil); !bytes.Equal(out, c.p) {
				t.Fatalf("DMRD %x: encode %x", c.p, out)
			}
			n++
		case dmrd.CommandDMRP, dmrd.CommandDMRB:
			m, err := dmrd.DecodeControl(c.p, dmrd.DialectLocal)
			if err != nil || m.Command != cmd {
				t.Fatalf("%x: %+v %v", c.p, m, err)
			}
		default:
			t.Fatalf("%s %v %x", c.dir, cmd, c.p)
		}
	}
	if n != 16 {
		t.Errorf("%d DMRD packets, want 16", n)
	}
}

func TestClassifyUnknownDialectCaptures(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		d    dmrd.Dialect
	}{
		{"dmrgateway-master.txt", dmrd.DialectMMDVM},
		{"mmdvmhost-dmrgateway.txt", dmrd.DialectLocal},
	} {
		n := 0
		for i, c := range readCapture(t, tt.name) {
			cmd := dmrd.Classify(c.p, tt.d)
			want := dmrd.CommandUnknown
			switch cmd {
			case dmrd.CommandDMRD, dmrd.CommandDMRE, dmrd.CommandBCKA, dmrd.CommandBCSQ, dmrd.CommandBCST, dmrd.CommandBCVE:
				want = cmd
				n++
			default:
			}
			if got := dmrd.Classify(c.p, dmrd.DialectUnknown); got != want {
				t.Errorf("%s packet %d: DialectUnknown %v, %v %v", tt.name, i, got, tt.d, cmd)
			}
		}
		if n == 0 {
			t.Errorf("%s: no DMRD or OpenBridge packet", tt.name)
		}
	}
}
