package ipscsession_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

func readXPR(t testing.TB) [][]byte {
	t.Helper()
	f, err := os.Open("../testdata/xpr8400.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out [][]byte
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		p, err := hex.DecodeString(fs[1])
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func xprPacket(t *testing.T, lines [][]byte, op ipsc.Opcode, id uint32) []byte {
	t.Helper()
	for _, p := range lines {
		if r, err := ipsc.DecodeRaw(p); err == nil && r.Opcode == op && r.ID == id {
			return p
		}
	}
	t.Fatalf("no %v from %x", op, id)
	return nil
}

var (
	xprAddr    = ipscsession.Addr{IP: [4]byte{192, 168, 0, 231}, Port: 50000}
	masterAddr = ipscsession.Addr{IP: [4]byte{192, 168, 0, 10}, Port: 50000}
)

func TestMasterReplaysXPR8400(t *testing.T) {
	t.Parallel()

	lines := readXPR(t)
	m, err := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Flags: ipsc.Flags{0, 0, 0, 0x1d}, RegisterVersion: [4]byte{4, 2, 4, 1}, AliveVersion: [4]byte{4, 2, 4, 1}})
	if err != nil {
		t.Fatal(err)
	}
	const xpr = 0x0004ce2e
	for _, step := range []struct {
		in   ipsc.Opcode
		want ipsc.Opcode
	}{{ipsc.OpRegisterRequest, ipsc.OpRegisterReply}, {ipsc.OpPeerListRequest, ipsc.OpPeerListReply}, {ipsc.OpAliveRequest, ipsc.OpAliveReply}} {
		out := m.Packet(time.Second, xprAddr, xprPacket(t, lines, step.in, xpr), nil, nil)
		s := sends(out)
		if len(s) != 1 || out[0].Addr != xprAddr || !bytes.Equal(s[0], xprPacket(t, lines, step.want, 1)) {
			t.Fatalf("%v: %x, want %x", step.in, s, xprPacket(t, lines, step.want, 1))
		}
	}
	if m.Peers() != 1 || m.PeerAt(0).ID != xpr || m.PeerAt(0).Mode != 0x6a || m.PeerAt(5) != (ipscsession.PeerInfo{}) {
		t.Fatalf("peers %+v", m.PeerAt(0))
	}
	if out := m.Packet(time.Second, xprAddr, xprPacket(t, lines, ipsc.OpStatus, xpr), nil, nil); len(out) != 0 {
		t.Fatal("status answered")
	}
	calls := 0
	for _, p := range lines {
		if ipsc.Opcode(p[0]) == ipsc.OpGroupVoice || ipsc.Opcode(p[0]) == ipsc.OpGroupData || ipsc.Opcode(p[0]) == ipsc.OpPrivateData {
			out := m.Packet(2*time.Second, xprAddr, p, nil, nil)
			if len(out) != 1 || out[0].Kind != ipscsession.OutputData || !bytes.Equal(out[0].Packet, p) || out[0].ID != xpr {
				t.Fatalf("user packet %x: %v", p, kinds(out))
			}
			calls++
		}
	}
	if calls < 50 {
		t.Fatalf("%d user packets", calls)
	}
}

func TestMasterSendsXPR8400Versions(t *testing.T) {
	t.Parallel()

	lines := readXPR(t)
	m, err := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	const xpr = 0x0004ce2e
	reg, alive := xprPacket(t, lines, ipsc.OpRegisterRequest, xpr), xprPacket(t, lines, ipsc.OpAliveRequest, xpr)
	peerReg, _ := (&ipsc.Control{Opcode: ipsc.OpPeerRegisterRequest, ID: xpr, Version: [4]byte(reg[10:14])}).AppendBinary(nil)
	for _, step := range []struct {
		in   []byte
		at   int
		want []byte
	}{{reg, 12, reg[10:14]}, {alive, 10, reg[10:14]}, {peerReg, 5, reg[10:14]}} {
		s := sends(m.Packet(time.Second, xprAddr, step.in, nil, nil))
		if len(s) != 1 || len(s[0]) < step.at+4 || !bytes.Equal(s[0][step.at:step.at+4], step.want) {
			t.Fatalf("%x: %x, want version %x", step.in, s, step.want)
		}
	}
}

// xprRig is the link setup between an XPR 8400 and a master that sent
// 04 08 04 00, from runC.ipsc.log lines 1 to 4, 7 and 8 of the rig run
// dmrgo-rfcap/runs/xpr-ipsc2hbrp-20261022, without the HMAC trailers.
var xprRig = func() map[string][]byte {
	m := map[string][]byte{}
	for k, v := range map[string]string{
		"reg":        "900004ce2e6a0002805c04080400",
		"regReply":   "91003010af6a0000001d000104080400",
		"list":       "920004ce2e",
		"listReply":  "93003010af000b0004ce2ec0a800e7c3506a",
		"alive":      "960004ce2e6a0002805c04080400",
		"aliveReply": "97003010af6a0000001d04080400",
	} {
		m[k], _ = hex.DecodeString(v)
	}
	return m
}()

func TestMasterReplaysXPR8400Rig(t *testing.T) {
	t.Parallel()

	m, err := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 0x3010af, Flags: ipsc.Flags{0, 0, 0, 0x1d}})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range [][2]string{{"reg", "regReply"}, {"list", "listReply"}, {"alive", "aliveReply"}} {
		s := sends(m.Packet(time.Second, xprAddr, xprRig[step[0]], nil, nil))
		if len(s) != 1 || !bytes.Equal(s[0], xprRig[step[1]]) {
			t.Fatalf("%s: %x, want %x", step[0], s, xprRig[step[1]])
		}
	}
}

func TestPeerReplaysXPR8400Rig(t *testing.T) {
	t.Parallel()

	p, err := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 0x0004ce2e, Flags: ipsc.Flags{0, 0x02, 0x80, 0x5c}, Master: masterAddr})
	if err != nil {
		t.Fatal(err)
	}
	if s := sends(p.Tick(0, nil)); len(s) != 1 || !bytes.Equal(s[0], xprRig["reg"]) {
		t.Fatalf("register %x, want %x", s, xprRig["reg"])
	}
	p.Packet(0, masterAddr, xprRig["regReply"], nil)
	if s := sends(p.Tick(5*time.Second, nil)); len(s) != 2 || !bytes.Equal(s[0], xprRig["alive"]) || !bytes.Equal(s[1], xprRig["list"]) {
		t.Fatalf("keepalive and list request %x, want %x %x", s, xprRig["alive"], xprRig["list"])
	}
}

func TestPeerReplaysXPR8400(t *testing.T) {
	t.Parallel()

	lines := readXPR(t)
	const xpr = 0x0004ce2e
	p, err := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: xpr, Flags: ipsc.Flags{0, 0, 0x80, 0xdc}, AliveVersion: [4]byte{4, 2, 4, 1}, Master: masterAddr})
	if err != nil {
		t.Fatal(err)
	}
	reg := sends(p.Tick(0, nil))
	var want []byte
	for _, pk := range lines {
		if len(reg) == 1 && ipsc.Opcode(pk[0]) == ipsc.OpRegisterRequest && bytes.Equal(reg[0], pk) {
			want = pk
		}
	}
	if want == nil {
		t.Fatalf("register %x, want %x", reg, want)
	}
	out := p.Packet(0, masterAddr, xprPacket(t, lines, ipsc.OpRegisterReply, 1), nil)
	if len(out) != 1 || out[0].Kind != ipscsession.OutputUp || out[0].ID != 1 {
		t.Fatalf("reply %+v", out)
	}
	s := sends(p.Tick(5*time.Second, nil))
	if len(s) != 2 || !bytes.Equal(s[0], xprPacket(t, lines, ipsc.OpAliveRequest, xpr)) || !bytes.Equal(s[1], xprPacket(t, lines, ipsc.OpPeerListRequest, xpr)) {
		t.Fatalf("keepalive and list request %x", s)
	}
	out = p.Packet(0, masterAddr, xprPacket(t, lines, ipsc.OpPeerListReply, 1), nil)
	if len(out) != 1 || out[0].Kind != ipscsession.OutputPeers || p.Peers() != 0 {
		t.Fatalf("list %v", kinds(out))
	}
	p.Packet(0, masterAddr, xprPacket(t, lines, ipsc.OpAliveReply, 1), nil)
	if s = sends(p.Tick(10*time.Second, nil)); len(s) != 1 || ipsc.Opcode(s[0][0]) != ipsc.OpAliveRequest {
		t.Fatalf("keepalive %x", s)
	}
	for _, pk := range lines {
		if ipsc.Opcode(pk[0]) == ipsc.OpGroupVoice {
			out = p.Packet(0, masterAddr, append([]byte{pk[0], 0, 0, 0, 1}, pk[5:]...), nil)
			if len(out) != 1 || out[0].Kind != ipscsession.OutputData {
				t.Fatalf("voice %v", kinds(out))
			}
		}
	}
	if id, ok := p.Master(); !ok || id != 1 {
		t.Fatal("master")
	}
}

type node struct {
	addr ipscsession.Addr
	peer *ipscsession.Peer
}

type ipscNet struct {
	t       *testing.T
	master  *ipscsession.Master
	nodes   []*node
	now     time.Duration
	queue   []packet
	events  map[ipscsession.Addr][]ipscsession.Output
	deaf    map[ipscsession.Addr]bool
	silence map[ipscsession.Addr]bool
}

type packet struct {
	from, to ipscsession.Addr
	p        []byte
}

func (n *ipscNet) out(from ipscsession.Addr, out []ipscsession.Output) {
	for _, o := range out {
		if o.Kind == ipscsession.OutputSend {
			if !n.silence[from] {
				n.queue = append(n.queue, packet{from, o.Addr, append([]byte(nil), o.Packet...)})
			}
			continue
		}
		n.events[from] = append(n.events[from], o)
	}
}

func (n *ipscNet) pump() {
	for range 10000 {
		if len(n.queue) == 0 {
			return
		}
		q := n.queue[0]
		n.queue = n.queue[1:]
		if n.deaf[q.to] {
			continue
		}
		if q.to == masterAddr {
			n.out(masterAddr, n.master.Packet(n.now, q.from, q.p, nil, nil))
			continue
		}
		for _, nd := range n.nodes {
			if nd.addr == q.to {
				n.out(nd.addr, nd.peer.Packet(0, q.from, q.p, nil))
			}
		}
	}
	n.t.Fatal("network does not settle")
}

func (n *ipscNet) run(to time.Duration) {
	for n.now < to {
		next := to
		if at, ok := n.master.Deadline(); ok && at < next {
			next = at
		}
		for _, nd := range n.nodes {
			if at, ok := nd.peer.Deadline(); ok && at < next {
				next = at
			}
		}
		n.now = max(next, n.now)
		n.out(masterAddr, n.master.Tick(n.now, nil))
		for _, nd := range n.nodes {
			n.out(nd.addr, nd.peer.Tick(n.now, nil))
		}
		n.pump()
		if next == to {
			return
		}
	}
}

func count(out []ipscsession.Output, k ipscsession.OutputKind, r ipscsession.Reason) int {
	c := 0
	for _, o := range out {
		if o.Kind == k && o.Reason == r {
			c++
		}
	}
	return c
}

func newNet(t *testing.T) *ipscNet {
	t.Helper()
	key, _ := ipscKey()
	m, err := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: true, Key: key, Timeout: 60 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	n := &ipscNet{t: t, master: m, events: map[ipscsession.Addr][]ipscsession.Output{}, deaf: map[ipscsession.Addr]bool{}, silence: map[ipscsession.Addr]bool{}}
	for i := range 3 {
		addr := ipscsession.Addr{IP: [4]byte{10, 0, 0, byte(i + 2)}, Port: 50000}
		p, err := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: uint32(100 + i), Master: masterAddr, Auth: true, Key: key, MaxMissed: 3})
		if err != nil {
			t.Fatal(err)
		}
		n.nodes = append(n.nodes, &node{addr: addr, peer: p})
	}
	n.run(30 * time.Second)
	if m.Peers() != 3 || count(n.events[masterAddr], ipscsession.OutputUp, ipscsession.ReasonNone) != 3 {
		t.Fatalf("master peers %d %v", m.Peers(), kinds(n.events[masterAddr]))
	}
	for _, nd := range n.nodes {
		ev := n.events[nd.addr]
		if nd.peer.State() != ipscsession.StateRunning || nd.peer.Peers() != 2 || count(ev, ipscsession.OutputUp, ipscsession.ReasonNone) != 3 {
			t.Fatalf("%v: %v peers %d %v", nd.addr, nd.peer.State(), nd.peer.Peers(), kinds(ev))
		}
		for i := range nd.peer.Peers() {
			if !nd.peer.PeerAt(i).Up {
				t.Fatalf("%v: peer %+v down", nd.addr, nd.peer.PeerAt(i))
			}
		}
	}
	n.events = map[ipscsession.Addr][]ipscsession.Output{}
	return n
}

func TestLoopback(t *testing.T) {
	t.Parallel()

	n := newNet(t)
	m := n.master
	n.events = map[ipscsession.Addr][]ipscsession.Output{}
	n.run(10 * time.Minute)
	for a, ev := range n.events {
		if count(ev, ipscsession.OutputDown, ipscsession.ReasonTimeout) != 0 {
			t.Fatalf("%v: %v", a, kinds(ev))
		}
	}
	a, b := n.nodes[0], n.nodes[1]
	voice := must(a.peer.Sign([]byte{byte(ipsc.OpGroupVoice), 0, 0, 0, 100, 1, 2, 3}))
	out := b.peer.Packet(0, a.addr, voice, nil)
	if len(out) != 1 || out[0].Kind != ipscsession.OutputData || out[0].ID != 100 || len(out[0].Packet) != 8 {
		t.Fatalf("voice %+v", out)
	}
	if out = b.peer.Packet(0, a.addr, voice[:8], nil); len(out) != 0 {
		t.Fatal("packet without trailer taken")
	}
	if out = m.Packet(n.now, a.addr, voice, nil, nil); len(out) != 1 || out[0].Kind != ipscsession.OutputData {
		t.Fatalf("master voice %v", kinds(out))
	}
}

func TestLoopbackSilentPeer(t *testing.T) {
	t.Parallel()

	n := newNet(t)
	m := n.master
	a, b, c := n.nodes[0], n.nodes[1], n.nodes[2]
	n.events = map[ipscsession.Addr][]ipscsession.Output{}
	n.deaf[c.addr], n.silence[c.addr] = true, true
	n.run(n.now + 2*time.Minute)
	if m.Peers() != 2 || count(n.events[masterAddr], ipscsession.OutputDown, ipscsession.ReasonTimeout) != 1 {
		t.Fatalf("master after silence %d %v", m.Peers(), kinds(n.events[masterAddr]))
	}
	for _, nd := range []*node{a, b} {
		ev := n.events[nd.addr]
		if count(ev, ipscsession.OutputDown, ipscsession.ReasonTimeout)+count(ev, ipscsession.OutputDown, ipscsession.ReasonList) != 1 || nd.peer.Peers() != 1 {
			t.Fatalf("%v after silence: %v peers %d", nd.addr, kinds(ev), nd.peer.Peers())
		}
	}
	ev := n.events[c.addr]
	if count(ev, ipscsession.OutputDown, ipscsession.ReasonTimeout) != 3 || c.peer.State() != ipscsession.StateRegistering || c.peer.Peers() != 2 {
		t.Fatalf("silent peer: %v %v", kinds(ev), c.peer.State())
	}
	n.deaf[c.addr], n.silence[c.addr] = false, false
	n.events = map[ipscsession.Addr][]ipscsession.Output{}
	n.run(n.now + 30*time.Second)
	if m.Peers() != 3 || c.peer.Peers() != 2 || a.peer.Peers() != 2 {
		t.Fatalf("back: %d %d %d", m.Peers(), c.peer.Peers(), a.peer.Peers())
	}
}

func TestLoopbackClose(t *testing.T) {
	t.Parallel()

	n := newNet(t)
	m := n.master
	a, b := n.nodes[0], n.nodes[1]
	n.events = map[ipscsession.Addr][]ipscsession.Output{}
	n.out(b.addr, b.peer.Close(0, nil))
	n.pump()
	if m.Peers() != 2 || count(n.events[masterAddr], ipscsession.OutputDown, ipscsession.ReasonClosed) != 1 || b.peer.State() != ipscsession.StateIdle {
		t.Fatalf("close: %d %v", m.Peers(), kinds(n.events[masterAddr]))
	}
	if count(n.events[a.addr], ipscsession.OutputDown, ipscsession.ReasonClosed) != 1 || a.peer.Peers() != 1 {
		t.Fatalf("a after close %v %d", kinds(n.events[a.addr]), a.peer.Peers())
	}
	if _, ok := b.peer.Deadline(); ok {
		t.Fatal("deadline after Close")
	}
	n.events = map[ipscsession.Addr][]ipscsession.Output{}
	n.out(masterAddr, m.Close(0, nil))
	n.pump()
	if m.State() != ipscsession.StateIdle || count(n.events[a.addr], ipscsession.OutputDown, ipscsession.ReasonClosed) != 1 || a.peer.State() != ipscsession.StateRegistering {
		t.Fatalf("master close %v %v", kinds(n.events[a.addr]), a.peer.State())
	}
	b.peer.Reset()
	m.Reset()
	if m.State() != ipscsession.StateRunning || b.peer.State() != ipscsession.StateRegistering {
		t.Fatal("reset")
	}
}

func TestMasterKeepaliveRegisters(t *testing.T) {
	t.Parallel()

	alive, _ := (&ipsc.Control{Opcode: ipsc.OpAliveRequest, ID: 7, Mode: 0x6a}).AppendBinary(nil)
	open, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1})
	if out := open.Packet(0, xprAddr, alive, nil, nil); !isDrop(out, 7, ipscsession.ReasonUnregistered) {
		t.Fatal("keepalive of a peer that is not registered taken without Auth")
	}
	key, _ := ipscKey()
	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: true, Key: key, MaxPeers: 1})
	out := m.Packet(0, xprAddr, must(m.Sign(append([]byte(nil), alive...))), nil, nil)
	if len(out) != 2 || out[1].Kind != ipscsession.OutputUp {
		t.Fatalf("authenticated keepalive %v", kinds(out))
	}
	other, _ := (&ipsc.Control{Opcode: ipsc.OpRegisterRequest, ID: 8}).AppendBinary(nil)
	if out = m.Packet(0, masterAddr, must(m.Sign(other)), nil, nil); !isDrop(out, 8, ipscsession.ReasonFull) {
		t.Fatal("registered beyond MaxPeers")
	}
	dereg, _ := (&ipsc.Control{Opcode: ipsc.OpDeregisterRequest, ID: 7}).AppendBinary(nil)
	if out = m.Packet(0, masterAddr, must(m.Sign(dereg)), nil, nil); !isDrop(out, 7, ipscsession.ReasonAddress) || m.Peers() != 1 {
		t.Fatal("de-registration from another address")
	}
	for _, op := range []ipsc.Opcode{ipsc.OpPeerRegisterRequest, ipsc.OpPeerAliveRequest} {
		c, _ := (&ipsc.Control{Opcode: op, ID: 7}).AppendBinary(nil)
		if out = m.Packet(time.Second, xprAddr, must(m.Sign(c)), nil, nil); len(out) != 1 || out[0].Kind != ipscsession.OutputSend {
			t.Fatalf("%v: %v", op, kinds(out))
		}
	}
	if at, _ := m.Deadline(); at != time.Second+ipscsession.TimeoutDefault {
		t.Fatalf("deadline %v", at)
	}
}

func TestConfigErrors(t *testing.T) {
	t.Parallel()

	var ce *dmrgo.ConfigError
	for i, tc := range []struct {
		cfg   *ipscsession.PeerConfig
		field string
	}{{&ipscsession.PeerConfig{Alive: -1}, "Alive"}, {&ipscsession.PeerConfig{MaxMissed: -1}, "MaxMissed"}, {&ipscsession.PeerConfig{MaxPeers: -1}, "MaxPeers"}, {&ipscsession.PeerConfig{MaxPeers: 6000}, "MaxPeers"}} {
		if _, err := ipscsession.NewPeer(tc.cfg); !errors.As(err, &ce) || ce.Type != "ipscsession" || ce.Field != tc.field {
			t.Errorf("peer %d: %v", i, err)
		}
	}
	for i, tc := range []struct {
		cfg   *ipscsession.MasterConfig
		field string
	}{{&ipscsession.MasterConfig{Timeout: -1}, "Timeout"}, {&ipscsession.MasterConfig{MaxPeers: 6000}, "MaxPeers"}} {
		if _, err := ipscsession.NewMaster(tc.cfg); !errors.As(err, &ce) || ce.Field != tc.field {
			t.Errorf("master %d: %v", i, err)
		}
	}
	if _, err := ipscsession.NewPeer(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil peer config: %v", err)
	}
	if _, err := ipscsession.NewMaster(nil); !errors.Is(err, dmrgo.ErrNilArgument) {
		t.Errorf("nil master config: %v", err)
	}
}

func TestNames(t *testing.T) {
	t.Parallel()

	if ipscsession.OutputData.String() != "Data" || ipscsession.OutputKind(0).String() != "Unknown" || ipscsession.ReasonList.String() != "List" ||
		ipscsession.Reason(99).String() != "Unknown" || ipscsession.StateRegistering.String() != "Registering" || ipscsession.State(9).String() != "Unknown" {
		t.Fatal("names")
	}
	for k := ipscsession.OutputKind(0); k <= ipscsession.OutputDropped; k++ {
		_ = k.String()
	}
	for r := ipscsession.Reason(0); r <= ipscsession.ReasonAddress; r++ {
		_ = r.String()
	}
	for s := ipscsession.State(0); s <= ipscsession.StateRunning; s++ {
		_ = s.String()
	}
}
