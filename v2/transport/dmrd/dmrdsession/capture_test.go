package dmrdsession_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func gatewayPeerConfig(t *testing.T, lines []capLine) *dmrdsession.PeerConfig {
	t.Helper()
	cfg := &dmrdsession.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 0x003010af, Password: []byte("passw0rd")}
	for _, l := range lines {
		if l.dir != "r>m" {
			continue
		}
		switch command(t, l.p, dmrd.DialectMMDVM) {
		case dmrd.CommandRPTC:
			cfg.Config = l.p
		case dmrd.CommandRPTO:
			cfg.Options = l.p[8:]
		default:
		}
	}
	return cfg
}

func TestPeerReplaysDMRGateway(t *testing.T) {
	t.Parallel()

	lines := readCapture(t, "dmrgateway-master.txt")
	p, err := dmrdsession.NewPeer(gatewayPeerConfig(t, lines))
	if err != nil {
		t.Fatal(err)
	}
	start := lines[0].at
	var sent [][]byte
	ups, data := 0, 0
	handle := func(out []dmrdsession.Output) {
		for _, o := range out {
			switch o.Kind {
			case dmrdsession.OutputSend:
				sent = append(sent, append([]byte(nil), o.Packet...))
			case dmrdsession.OutputUp:
				ups++
			case dmrdsession.OutputData:
				data++
			case dmrdsession.OutputDown:
				t.Fatalf("down: %+v", o)
			default:
			}
		}
	}
	handle(p.Tick(start, nil))
	var want [][]byte
	for _, l := range lines {
		if l.dir == "r>m" {
			if command(t, l.p, dmrd.DialectMMDVM) != dmrd.CommandDMRD {
				want = append(want, l.p)
			}
			continue
		}
		for {
			at, ok := p.Deadline()
			if !ok || at > l.at {
				break
			}
			handle(p.Tick(at, nil))
		}
		handle(p.Packet(l.at, l.p, nil))
	}
	if ups != 1 || data != 8 || p.State() != dmrdsession.StateRunning {
		t.Fatalf("ups %d data %d state %v", ups, data, p.State())
	}
	if len(sent) != len(want) {
		t.Fatalf("sent %d packets, the capture has %d", len(sent), len(want))
	}
	for i := range want {
		if !bytes.Equal(sent[i], want[i]) {
			t.Fatalf("packet %d: %x, want %x", i, sent[i], want[i])
		}
	}
}

func TestMasterReplaysDMRGateway(t *testing.T) {
	t.Parallel()

	lines := readCapture(t, "dmrgateway-master.txt")
	m, err := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM})
	if err != nil {
		t.Fatal(err)
	}
	var sent [][]byte
	var events []dmrdsession.OutputKind
	for _, l := range lines {
		if l.dir != "r>m" {
			continue
		}
		for _, o := range m.Packet(l.at, l.p, nil) {
			switch o.Kind {
			case dmrdsession.OutputSend:
				sent = append(sent, append([]byte(nil), o.Packet...))
			case dmrdsession.OutputLogin:
				if o.ID != 0x003010af {
					t.Fatalf("login of %x", o.ID)
				}
				sent = append(sent, sends(m.Admit(0, 0xfcb17c08, []byte("passw0rd"), nil))...)
				events = append(events, o.Kind)
			default:
				events = append(events, o.Kind)
			}
		}
	}
	var want [][]byte
	for _, l := range lines {
		if l.dir == "m>r" {
			switch command(t, l.p, dmrd.DialectMMDVM) {
			case dmrd.CommandDMRD, dmrd.CommandRPTSBKN:
			default:
				want = append(want, l.p)
			}
		}
	}
	if len(sent) != len(want) {
		t.Fatalf("sent %d packets, the capture has %d", len(sent), len(want))
	}
	for i := range want {
		if !bytes.Equal(sent[i], want[i]) {
			t.Fatalf("packet %d: %x, want %x", i, sent[i], want[i])
		}
	}
	wantEvents := make([]dmrdsession.OutputKind, 0, 14)
	wantEvents = append(wantEvents, dmrdsession.OutputLogin, dmrdsession.OutputConfig, dmrdsession.OutputUp, dmrdsession.OutputOptions)
	for range 8 {
		wantEvents = append(wantEvents, dmrdsession.OutputData)
	}
	wantEvents = append(wantEvents, dmrdsession.OutputPong, dmrdsession.OutputPong)
	if len(events) != len(wantEvents) {
		t.Fatalf("events %v", events)
	}
	for i := range events {
		if events[i] != wantEvents[i] {
			t.Fatalf("events %v, want %v", events, wantEvents)
		}
	}
}

func TestBridgeReplaysHBlink3(t *testing.T) {
	t.Parallel()

	for _, enhanced := range []bool{false, true} {
		b, err := dmrdsession.NewBridge(&dmrdsession.BridgeConfig{Key: []byte("passw0rd-longer-than-twenty"), Enhanced: enhanced})
		if err != nil {
			t.Fatal(err)
		}
		lines := readCapture(t, "hblink3-openbridge.txt")
		data := 0
		for _, l := range lines {
			for _, o := range b.Packet(0, l.p, nil) {
				if o.Kind == dmrdsession.OutputData && o.Command == dmrd.CommandDMRD && bytes.Equal(o.Packet, l.p) {
					data++
				}
			}
			bad := append([]byte(nil), l.p...)
			bad[60] ^= 1
			if out := b.Packet(0, bad, nil); len(out) != 0 {
				t.Fatalf("bad signature: %v", kinds(out))
			}
		}
		if data != len(lines) {
			t.Fatalf("enhanced %v: %d of %d packets", enhanced, data, len(lines))
		}
		other, _ := dmrdsession.NewBridge(&dmrdsession.BridgeConfig{Key: []byte("passw0rd")})
		if out := other.Packet(0, lines[0].p, nil); len(out) != 0 {
			t.Fatal("packet of another key taken")
		}
	}
}
