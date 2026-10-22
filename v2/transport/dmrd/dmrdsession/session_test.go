package dmrdsession_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdsession"
)

func newLink(t *testing.T, d dmrd.Dialect, options string, peerPass, masterPass string) *link {
	t.Helper()
	cfg := &dmrdsession.PeerConfig{Dialect: d, ID: 3129100, Password: []byte(peerPass), Config: mmdvmConfig(3129100), Options: []byte(options)}
	if d == dmrd.DialectHBRP {
		cfg.Config = hbrpConfig(3129100)
	}
	p, err := dmrdsession.NewPeer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m, err := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: d})
	if err != nil {
		t.Fatal(err)
	}
	return &link{t: t, peer: p, master: m, salt: 0x0A7ED498, pass: []byte(masterPass), now: time.Second}
}

func eventKinds(l *link) []dmrdsession.OutputKind {
	return kinds(l.events)
}

func sameKinds(t *testing.T, got []dmrdsession.OutputKind, want ...dmrdsession.OutputKind) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("events %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("events %v, want %v", got, want)
		}
	}
}

func TestLoopback(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		d       dmrd.Dialect
		options string
	}{{dmrd.DialectMMDVM, ""}, {dmrd.DialectMMDVM, "TS1=91;TS2=3100;"}, {dmrd.DialectHBRP, ""}} {
		l := newLink(t, tc.d, tc.options, "s3cret", "s3cret")
		l.advance(l.now)
		want := []dmrdsession.OutputKind{dmrdsession.OutputLogin, dmrdsession.OutputConfig, dmrdsession.OutputUp, dmrdsession.OutputUp}
		if tc.options != "" {
			want = []dmrdsession.OutputKind{dmrdsession.OutputLogin, dmrdsession.OutputConfig, dmrdsession.OutputUp, dmrdsession.OutputOptions, dmrdsession.OutputUp}
		}
		sameKinds(t, eventKinds(l), want...)
		if l.peer.State() != dmrdsession.StateRunning || l.master.State() != dmrdsession.StateRunning || l.master.ID() != 3129100 {
			t.Fatalf("%v: states %v %v", tc.d, l.peer.State(), l.master.State())
		}
		if o, _ := find(l.events, dmrdsession.OutputOptions); string(o.Packet) != tc.options {
			t.Fatalf("options %q", o.Packet)
		}
		l.events = nil
		l.advance(l.now + 10*time.Minute)
		if len(l.events) != 60 || count(l.events, dmrdsession.OutputPong) != 60 || l.peer.State() != dmrdsession.StateRunning || l.master.State() != dmrdsession.StateRunning {
			t.Fatalf("%v: keepalive: %v", tc.d, eventKinds(l))
		}
		l.events = nil
		var d dmrd.Data
		d.Repeater, d.Slot, d.Source, d.Destination = 3129100, 1, 3191868, 91
		pkt, _ := d.AppendBinary(nil)
		if o := l.master.Packet(l.now, pkt, nil); len(o) != 1 || o[0].Kind != dmrdsession.OutputData || o[0].Command != dmrd.CommandDMRD {
			t.Fatalf("master data %v", kinds(o))
		}
		if o := l.peer.Packet(l.now, pkt, nil); len(o) != 1 || o[0].Kind != dmrdsession.OutputData {
			t.Fatalf("peer data %v", kinds(o))
		}
		l.fromPeer(l.peer.Close(nil))
		l.pump()
		sameKinds(t, eventKinds(l), dmrdsession.OutputDown, dmrdsession.OutputDown)
		if l.events[0].Reason != dmrdsession.ReasonStopped || l.events[1].Reason != dmrdsession.ReasonClosed || l.master.State() != dmrdsession.StateIdle {
			t.Fatalf("close %+v", l.events)
		}
		if _, ok := l.peer.Deadline(); ok {
			t.Fatal("deadline after Close")
		}
	}
}

func TestWrongPassword(t *testing.T) {
	t.Parallel()

	for _, d := range []dmrd.Dialect{dmrd.DialectMMDVM, dmrd.DialectHBRP} {
		l := newLink(t, d, "", "wrong", "right")
		l.advance(l.now)
		sameKinds(t, eventKinds(l), dmrdsession.OutputLogin, dmrdsession.OutputDown, dmrdsession.OutputDown)
		if l.events[1].Reason != dmrdsession.ReasonRefused || l.events[2].Reason != dmrdsession.ReasonRefused || l.events[2].State != dmrdsession.StateAuth {
			t.Fatalf("%+v", l.events)
		}
		if l.peer.State() != dmrdsession.StateWait {
			t.Fatalf("peer %v", l.peer.State())
		}
	}
}

func TestPeerRetryTimeoutBackoff(t *testing.T) {
	t.Parallel()

	p, err := dmrdsession.NewPeer(&dmrdsession.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 1234, Config: mmdvmConfig(1234), Backoff: 40 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var at []time.Duration
	var downs []time.Duration
	now := time.Duration(0)
	for now < 400*time.Second {
		d, ok := p.Deadline()
		if !ok {
			t.Fatal("no deadline")
		}
		now = d
		for _, o := range p.Tick(now, nil) {
			switch o.Kind {
			case dmrdsession.OutputSend:
				if command(t, o.Packet, dmrd.DialectMMDVM) != dmrd.CommandRPTL {
					t.Fatalf("sent %q", o.Packet)
				}
				at = append(at, now)
			case dmrdsession.OutputDown:
				if o.Reason != dmrdsession.ReasonTimeout || o.State != dmrdsession.StateLogin {
					t.Fatalf("%+v", o)
				}
				downs = append(downs, now)
			default:
				t.Fatalf("%+v", o)
			}
		}
	}
	s := time.Second
	wantAt := []time.Duration{0, 10 * s, 20 * s, 30 * s, 40 * s, 50 * s, 70 * s, 80 * s, 90 * s, 100 * s, 110 * s, 120 * s, 150 * s}
	for i, w := range wantAt {
		if at[i] != w {
			t.Fatalf("RPTL at %v, want %v", at[:len(wantAt)], wantAt)
		}
	}
	wantDown := []time.Duration{60 * s, 130 * s, 210 * s, 310 * s}
	for i, w := range wantDown {
		if downs[i] != w {
			t.Fatalf("down at %v, want %v", downs, wantDown)
		}
	}
}

func mmdvmCtl(c dmrd.Command, id uint32) []byte {
	b, _ := dmrd.Control{Command: c, ID: id}.AppendBinary(nil, dmrd.DialectMMDVM)
	return b
}

func ack(id uint32) []byte {
	return mmdvmCtl(dmrd.CommandRPTACK, id)
}

func ctl(c dmrd.Command, id uint32) []byte {
	return mmdvmCtl(c, id)
}

func rptk(id uint32, tk [32]byte) []byte {
	b, _ := dmrd.Control{Command: dmrd.CommandRPTK, ID: id, Token: tk}.AppendBinary(nil, dmrd.DialectMMDVM)
	return b
}

func newPeer(t *testing.T) *dmrdsession.Peer {
	t.Helper()
	p, err := dmrdsession.NewPeer(&dmrdsession.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 1234, Password: []byte("pw"), Config: mmdvmConfig(1234), Options: []byte("X=1")})
	if err != nil {
		t.Fatal(err)
	}
	if out := p.Packet(0, ack(1), nil); len(out) != 0 {
		t.Fatal("packet before start")
	}
	p.Tick(0, nil)
	return p
}

func runningPeer(t *testing.T) *dmrdsession.Peer {
	t.Helper()
	p := newPeer(t)
	for range 4 {
		p.Packet(2*time.Second, ack(1234), nil)
	}
	if p.State() != dmrdsession.StateRunning {
		t.Fatal(p.State())
	}
	return p
}

func TestPeerSteps(t *testing.T) {
	t.Parallel()

	s := time.Second
	newPeer := func() *dmrdsession.Peer { return newPeer(t) }
	steps := []dmrd.Command{dmrd.CommandRPTK, dmrd.CommandRPTC, dmrd.CommandRPTO}
	states := []dmrdsession.State{dmrdsession.StateAuth, dmrdsession.StateConfig, dmrdsession.StateOptions}
	p := newPeer()
	for i, c := range steps {
		out := p.Packet(s, ack(99), nil)
		if len(out) != 1 || command(t, out[0].Packet, dmrd.DialectMMDVM) != c || p.State() != states[i] {
			t.Fatalf("step %d: %v %v", i, kinds(out), p.State())
		}
		again := p.Tick(11*s, nil)
		if len(again) != 1 || !bytes.Equal(again[0].Packet, sends(out)[0]) {
			t.Fatalf("step %d resend %v", i, kinds(again))
		}
		p.Tick(0, nil)
	}
	tok, _ := auth.Token(dmrd.DialectMMDVM, 99, []byte("pw"))
	p2 := newPeer()
	out := p2.Packet(s, ack(99), nil)
	if k, _ := dmrd.DecodeControl(out[0].Packet, dmrd.DialectMMDVM); k.Token != tok {
		t.Fatal("token")
	}
	out = p.Packet(2*s, ack(1234), nil)
	sameKinds(t, kinds(out), dmrdsession.OutputUp)
	if out = p.Packet(2*s, ack(1234), nil); len(out) != 0 {
		t.Fatal("ack while running")
	}
}

func TestPeerRunning(t *testing.T) {
	t.Parallel()

	s := time.Second
	p := runningPeer(t)
	var out []dmrdsession.Output
	if out = p.Tick(12*s, nil); len(out) != 1 || command(t, out[0].Packet, dmrd.DialectMMDVM) != dmrd.CommandRPTPING {
		t.Fatalf("ping %v", kinds(out))
	}
	p.Packet(12*s, ctl(dmrd.CommandMSTPONG, 1234), nil)
	for now := 22 * s; now < 70*s; now += 10 * s {
		p.Tick(now, nil)
	}
	if p.State() != dmrdsession.StateRunning {
		t.Fatal("timeout despite pong")
	}
	if out = p.Packet(70*s, ctl(dmrd.CommandRPTSBKN, 1234), nil); len(out) != 1 || out[0].Kind != dmrdsession.OutputMessage {
		t.Fatalf("beacon %v", kinds(out))
	}
	out = p.Packet(70*s, ctl(dmrd.CommandMSTNAK, 1234), nil)
	if len(out) != 1 || out[0].Reason != dmrdsession.ReasonRefused || out[0].State != dmrdsession.StateRunning || p.State() != dmrdsession.StateLogin {
		t.Fatalf("nak %+v %v", out, p.State())
	}
	if at, _ := p.Deadline(); at != 80*s {
		t.Fatalf("login again at %v", at)
	}
	out = p.Tick(80*s, nil)
	if len(out) != 1 || command(t, out[0].Packet, dmrd.DialectMMDVM) != dmrd.CommandRPTL {
		t.Fatalf("login again %v", kinds(out))
	}
	out = p.Packet(81*s, ctl(dmrd.CommandMSTCL, 1234), nil)
	if len(out) != 1 || out[0].Reason != dmrdsession.ReasonClosed || p.State() != dmrdsession.StateWait {
		t.Fatalf("mstcl %+v", out)
	}
	if out = p.Packet(81*s, ctl(dmrd.CommandMSTNAK, 1234), nil); len(out) != 0 {
		t.Fatal("nak while waiting")
	}
	out = p.Close(nil)
	if len(out) != 0 || p.State() != dmrdsession.StateIdle {
		t.Fatalf("close while waiting %v", kinds(out))
	}
	p.Reset()
	if at, ok := p.Deadline(); !ok || at != 0 {
		t.Fatal("reset")
	}
}

func TestPeerRefusedInAuth(t *testing.T) {
	t.Parallel()

	s := time.Second
	newPeer := func() *dmrdsession.Peer { return newPeer(t) }
	p3 := newPeer()
	p3.Packet(s, ack(5), nil)
	out := p3.Packet(s, ctl(dmrd.CommandMSTNAK, 1234), nil)
	if len(out) != 1 || out[0].State != dmrdsession.StateAuth || p3.State() != dmrdsession.StateWait {
		t.Fatalf("nak in auth %+v", out)
	}
}

func TestHBRPSpecToken(t *testing.T) {
	t.Parallel()

	m, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: dmrd.DialectHBRP})
	out := m.Packet(0, []byte("RPTL00040178"), nil)
	sameKinds(t, kinds(out), dmrdsession.OutputLogin)
	out = m.Admit(0x0A7ED498, []byte("DL5DI"), nil)
	if len(out) != 1 || string(out[0].Packet) != "MSTACK000401780A7ED498" {
		t.Fatalf("salt %q", sends(out))
	}
	out = m.Packet(time.Second, []byte("RPTK00040178cbf0e29abbd11c6573825d36a664e3064441f91fc815ac5dc5ca570d4c4a5f85"), nil)
	if len(out) != 1 || string(out[0].Packet) != "MSTACK00040178" || m.State() != dmrdsession.StateConfig {
		t.Fatalf("token %q %v", sends(out), m.State())
	}
	p, _ := dmrdsession.NewPeer(&dmrdsession.PeerConfig{Dialect: dmrd.DialectHBRP, ID: 0x00040178, Password: []byte("DL5DI"), Config: hbrpConfig(0x00040178)})
	p.Tick(0, nil)
	out = p.Packet(0, []byte("MSTACK000401780A7ED498"), nil)
	if len(out) != 1 || !strings.EqualFold(string(out[0].Packet), "RPTK00040178cbf0e29abbd11c6573825d36a664e3064441f91fc815ac5dc5ca570d4c4a5f85") {
		t.Fatalf("peer token %q", sends(out))
	}
	if out = p.Packet(0, []byte("MSTACK00040178"), nil); len(out) != 1 || !bytes.Equal(out[0].Packet, hbrpConfig(0x00040178)) {
		t.Fatalf("config %v", kinds(out))
	}
	sameKinds(t, kinds(p.Packet(0, []byte("MSTACK00040178"), nil)), dmrdsession.OutputUp)
	if out = p.Tick(10*time.Second, nil); string(out[0].Packet) != "MSTPING00040178" {
		t.Fatalf("ping %q", out[0].Packet)
	}
	lower, _ := dmrdsession.NewPeer(&dmrdsession.PeerConfig{Dialect: dmrd.DialectHBRP, ID: 0x00040178, Password: []byte("DL5DI"), Config: hbrpConfig(0x00040178)})
	lower.Tick(0, nil)
	out = lower.Packet(0, []byte("MSTACK000401780a7ed498"), nil)
	want := sha256.Sum256([]byte("0a7ed498DL5DI"))
	if k, err := dmrd.DecodeControl(out[0].Packet, dmrd.DialectHBRP); err != nil || k.Token != want {
		t.Fatalf("token of a lower case salt %q", out[0].Packet)
	}
	p2, _ := dmrdsession.NewPeer(&dmrdsession.PeerConfig{Dialect: dmrd.DialectHBRP, ID: 0x00040178, Config: hbrpConfig(0x00040178)})
	p2.Tick(0, nil)
	if out = p2.Packet(0, []byte("MSTACK00040178"), nil); len(out) != 0 || p2.State() != dmrdsession.StateLogin {
		t.Fatal("MSTACK without salt taken as the salt")
	}
}

func TestMasterBeforeLogin(t *testing.T) {
	t.Parallel()

	d := dmrd.DialectMMDVM
	tok, _ := auth.Token(d, 7, []byte("pw"))
	s := time.Second
	m, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: d, Timeout: 30 * s})
	nak := func(out []dmrdsession.Output) bool {
		return len(out) == 1 && command(t, out[0].Packet, d) == dmrd.CommandMSTNAK
	}
	for _, p := range [][]byte{ctl(dmrd.CommandRPTPING, 5), rptk(5, tok), mmdvmConfig(5), ctl(dmrd.CommandRPTO, 5)} {
		if !nak(m.Packet(0, p, nil)) {
			t.Fatalf("%q before login", p)
		}
	}
	if out := m.Packet(0, ctl(dmrd.CommandRPTCL, 5), nil); len(out) != 0 {
		t.Fatal("RPTCL before login")
	}
	if out := m.Admit(1, nil, nil); len(out) != 0 {
		t.Fatal("admit before login")
	}
	if out := m.Refuse(nil); len(out) != 0 {
		t.Fatal("refuse before login")
	}
}

func TestMasterLogin(t *testing.T) {
	t.Parallel()

	d := dmrd.DialectMMDVM
	tok, _ := auth.Token(d, 7, []byte("pw"))
	s := time.Second
	m, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: d, Timeout: 30 * s})
	nak := func(out []dmrdsession.Output) bool {
		return len(out) == 1 && command(t, out[0].Packet, d) == dmrd.CommandMSTNAK
	}
	sameKinds(t, kinds(m.Packet(0, ctl(dmrd.CommandRPTL, 5), nil)), dmrdsession.OutputLogin)
	if out := m.Packet(s, ctl(dmrd.CommandRPTL, 5), nil); len(out) != 0 {
		t.Fatal("second RPTL while waiting for Admit")
	}
	salt := sends(m.Admit(7, []byte("pw"), nil))
	if again := sends(m.Packet(2*s, ctl(dmrd.CommandRPTL, 5), nil)); len(again) != 1 || !bytes.Equal(again[0], salt[0]) {
		t.Fatal("RPTL again does not get the same salt")
	}
	if !nak(m.Packet(2*s, mmdvmConfig(5), nil)) || !nak(m.Packet(2*s, rptk(6, tok), nil)) {
		t.Fatal("RPTC before RPTK or RPTK of another ID")
	}
	for range 2 {
		out := m.Packet(3*s, rptk(5, tok), nil)
		if len(out) != 1 || command(t, out[0].Packet, d) != dmrd.CommandRPTACK || m.State() != dmrdsession.StateConfig {
			t.Fatalf("token %v", kinds(out))
		}
	}
	if !nak(m.Packet(3*s, mmdvmConfig(6), nil)) || !nak(m.Packet(3*s, ctl(dmrd.CommandRPTPING, 5), nil)) {
		t.Fatal("config of another ID or ping before config")
	}
	sameKinds(t, kinds(m.Packet(4*s, mmdvmConfig(5), nil)), dmrdsession.OutputSend, dmrdsession.OutputConfig, dmrdsession.OutputUp)
	sameKinds(t, kinds(m.Packet(4*s, mmdvmConfig(5), nil)), dmrdsession.OutputSend, dmrdsession.OutputConfig)
	pos := make([]byte, 0, 25)
	pos = append(pos, ctl(dmrd.CommandRPTG, 5)...)
	pos = append(pos, "+30.2672-097.7431"...)
	sameKinds(t, kinds(m.Packet(4*s, pos, nil)), dmrdsession.OutputMessage)
	if out := m.Packet(5*s, ctl(dmrd.CommandRPTPING, 5), nil); len(out) != 2 || command(t, out[0].Packet, d) != dmrd.CommandMSTPONG || out[1].Kind != dmrdsession.OutputPong || out[1].ID != 5 {
		t.Fatal("pong")
	}
	if at, _ := m.Deadline(); at != 35*s {
		t.Fatalf("deadline %v", at)
	}
	if out := m.Tick(34*s, nil); len(out) != 0 {
		t.Fatal("early timeout")
	}
	out := m.Tick(35*s, nil)
	if len(out) != 1 || out[0].Reason != dmrdsession.ReasonTimeout || out[0].ID != 5 || m.State() != dmrdsession.StateIdle {
		t.Fatalf("timeout %+v", out)
	}
}

func TestMasterRefusals(t *testing.T) {
	t.Parallel()

	d := dmrd.DialectMMDVM
	s := time.Second
	m, _ := dmrdsession.NewMaster(&dmrdsession.MasterConfig{Dialect: d, Timeout: 30 * s})
	nak := func(out []dmrdsession.Output) bool {
		return len(out) == 1 && command(t, out[0].Packet, d) == dmrd.CommandMSTNAK
	}
	m.Packet(40*s, ctl(dmrd.CommandRPTL, 5), nil)
	m.Admit(8, []byte("pw"), nil)
	bad, _ := auth.Token(d, 8, []byte("px"))
	out := m.Packet(41*s, rptk(5, bad), nil)
	if len(out) != 2 || out[1].Reason != dmrdsession.ReasonRefused || m.State() != dmrdsession.StateIdle {
		t.Fatalf("bad token %+v", out)
	}
	m.Packet(42*s, ctl(dmrd.CommandRPTL, 5), nil)
	out = m.Refuse(nil)
	if len(out) != 2 || !nak(out[:1]) || out[1].State != dmrdsession.StateLogin {
		t.Fatalf("refuse %+v", out)
	}
	m.Packet(43*s, ctl(dmrd.CommandRPTL, 5), nil)
	out = m.Packet(43*s, ctl(dmrd.CommandRPTL, 9), nil)
	if len(out) != 2 || out[0].Reason != dmrdsession.ReasonLogin || out[1].ID != 9 || m.ID() != 9 {
		t.Fatalf("new login %+v", out)
	}
	out = m.Close(nil)
	if len(out) != 2 || command(t, out[0].Packet, d) != dmrd.CommandMSTCL || out[1].Reason != dmrdsession.ReasonStopped {
		t.Fatalf("close %+v", out)
	}
	m.Reset()
	if _, ok := m.Deadline(); ok || m.ID() != 0 {
		t.Fatal("reset")
	}
}

func TestBridgeKeepalive(t *testing.T) {
	t.Parallel()

	s := time.Second
	a, _ := dmrdsession.NewBridge(&dmrdsession.BridgeConfig{Key: []byte("k"), Enhanced: true})
	b, _ := dmrdsession.NewBridge(&dmrdsession.BridgeConfig{Key: []byte("k"), Enhanced: true, Timeout: 30 * s})
	out := a.Tick(0, nil)
	if len(out) != 1 || command(t, out[0].Packet, dmrd.DialectMMDVM) != dmrd.CommandBCKA {
		t.Fatalf("bcka %v", kinds(out))
	}
	sameKinds(t, kinds(b.Packet(0, out[0].Packet, nil)), dmrdsession.OutputUp)
	if b.State() != dmrdsession.StateRunning {
		t.Fatal("not running")
	}
	if at, _ := a.Deadline(); at != 10*s {
		t.Fatalf("next bcka %v", at)
	}
	b.Tick(0, nil)
	if at, _ := b.Deadline(); at != 10*s {
		t.Fatalf("b deadline %v", at)
	}
	b.Tick(10*s, nil)
	b.Tick(20*s, nil)
	out = b.Tick(30*s, nil)
	if len(out) != 2 || out[0].Kind != dmrdsession.OutputDown || out[0].Reason != dmrdsession.ReasonTimeout || out[1].Kind != dmrdsession.OutputSend {
		t.Fatalf("timeout %v", kinds(out))
	}
	sq := dmrd.BridgeControl{Command: dmrd.CommandBCSQ, TalkGroup: 91, Stream: 7}
	p, _ := sq.AppendBinary(nil)
	if err := a.Sign(p); err != nil {
		t.Fatal(err)
	}
	out = b.Packet(31*s, p, nil)
	sameKinds(t, kinds(out), dmrdsession.OutputUp, dmrdsession.OutputMessage)
	if out[1].Command != dmrd.CommandBCSQ {
		t.Fatal("command")
	}
	plain, _ := dmrdsession.NewBridge(&dmrdsession.BridgeConfig{Key: []byte("k")})
	if _, ok := plain.Deadline(); ok || len(plain.Tick(0, nil)) != 0 {
		t.Fatal("plain bridge has a keepalive")
	}
	if err := plain.Sign([]byte("RPTL")); err == nil {
		t.Fatal("signed a packet that is not OpenBridge")
	}
}

func TestConfigErrors(t *testing.T) {
	t.Parallel()

	var ce *dmrgo.ConfigError
	long := make([]byte, dmrdsession.MaxOptions+1)
	for i, tc := range []struct {
		cfg   dmrdsession.PeerConfig
		field string
	}{
		{dmrdsession.PeerConfig{Dialect: dmrd.DialectLocal, ID: 1, Config: mmdvmConfig(1)}, "Dialect"},
		{dmrdsession.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 2, Config: mmdvmConfig(1)}, "Config"},
		{dmrdsession.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 1, Config: hbrpConfig(1)}, "Config"},
		{dmrdsession.PeerConfig{Dialect: dmrd.DialectHBRP, ID: 1, Config: mmdvmConfig(1)}, "Config"},
		{dmrdsession.PeerConfig{Dialect: dmrd.DialectHBRP, ID: 2, Config: hbrpConfig(1)}, "Config"},
		{dmrdsession.PeerConfig{Dialect: dmrd.DialectHBRP, ID: 1, Config: hbrpConfig(1), Options: []byte("a")}, "Options"},
		{dmrdsession.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 1, Config: mmdvmConfig(1), Options: long}, "Options"},
		{dmrdsession.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 1, Config: mmdvmConfig(1), Retry: -1}, "Retry"},
		{dmrdsession.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 1, Config: mmdvmConfig(1), Timeout: -1}, "Timeout"},
		{dmrdsession.PeerConfig{Dialect: dmrd.DialectMMDVM, ID: 1, Config: mmdvmConfig(1), Backoff: -1}, "Backoff"},
	} {
		if _, err := dmrdsession.NewPeer(&tc.cfg); !errors.As(err, &ce) || ce.Type != "dmrdsession" || ce.Field != tc.field {
			t.Errorf("peer %d: %v", i, err)
		}
	}
	for i, tc := range []struct {
		cfg   dmrdsession.MasterConfig
		field string
	}{
		{dmrdsession.MasterConfig{}, "Dialect"},
		{dmrdsession.MasterConfig{Dialect: dmrd.DialectLocal}, "Dialect"},
		{dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM, Timeout: -1}, "Timeout"},
		{dmrdsession.MasterConfig{Dialect: dmrd.DialectMMDVM, LoginTimeout: -1}, "LoginTimeout"},
	} {
		if _, err := dmrdsession.NewMaster(&tc.cfg); !errors.As(err, &ce) || ce.Field != tc.field {
			t.Errorf("master %d: %v", i, err)
		}
	}
	for i, tc := range []struct {
		cfg   dmrdsession.BridgeConfig
		field string
	}{{dmrdsession.BridgeConfig{Keepalive: -1}, "Keepalive"}, {dmrdsession.BridgeConfig{Timeout: -1}, "Timeout"}} {
		if _, err := dmrdsession.NewBridge(&tc.cfg); !errors.As(err, &ce) || ce.Field != tc.field {
			t.Errorf("bridge %d: %v", i, err)
		}
	}
	if _, err := dmrdsession.NewPeer(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil peer config: %v", err)
	}
	if _, err := dmrdsession.NewMaster(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil master config: %v", err)
	}
	if _, err := dmrdsession.NewBridge(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil bridge config: %v", err)
	}
}

func TestNames(t *testing.T) {
	t.Parallel()

	for k := dmrdsession.OutputKind(0); k <= dmrdsession.OutputPong+1; k++ {
		if k.String() == "" {
			t.Fatal(k)
		}
	}
	for r := dmrdsession.Reason(0); r <= dmrdsession.ReasonLogin+1; r++ {
		if r.String() == "" {
			t.Fatal(r)
		}
	}
	for s := dmrdsession.State(0); s <= dmrdsession.StateRunning+1; s++ {
		if s.String() == "" {
			t.Fatal(s)
		}
	}
	if dmrdsession.StateRunning.String() != "Running" || dmrdsession.OutputKind(99).String() != "Unknown" {
		t.Fatal("names")
	}
}

func TestPeerRepeatedChallenge(t *testing.T) {
	t.Parallel()

	p, _ := dmrdsession.NewPeer(&dmrdsession.PeerConfig{Dialect: dmrd.DialectHBRP, ID: 0x40178, Password: []byte("DL5DI"), Config: hbrpConfig(0x40178)})
	p.Tick(0, nil)
	p.Packet(0, []byte("MSTACK000401780A7ED498"), nil)
	if out := p.Packet(0, []byte("MSTACK000401780A7ED498"), nil); len(out) != 0 || p.State() != dmrdsession.StateAuth {
		t.Fatalf("HBRP challenge again taken as the ack: %v %v", kinds(out), p.State())
	}
	if out := p.Packet(0, []byte("MSTACK00040178"), nil); len(out) != 1 || p.State() != dmrdsession.StateConfig {
		t.Fatalf("ack after the challenge again: %v %v", kinds(out), p.State())
	}
}

func count(out []dmrdsession.Output, k dmrdsession.OutputKind) int {
	n := 0
	for _, o := range out {
		if o.Kind == k {
			n++
		}
	}
	return n
}
