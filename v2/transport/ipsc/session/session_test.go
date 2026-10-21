package session_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/session"
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
	xprAddr    = session.Addr{IP: [4]byte{192, 168, 0, 231}, Port: 50000}
	masterAddr = session.Addr{IP: [4]byte{192, 168, 0, 10}, Port: 50000}
)

func TestMasterReplaysXPR8400(t *testing.T) {
	t.Parallel()

	lines := readXPR(t)
	m, err := session.NewMaster(&session.MasterConfig{ID: 1, Flags: ipsc.Flags{0, 0, 0, 0x1d}})
	if err != nil {
		t.Fatal(err)
	}
	const xpr = 0x0004ce2e
	for _, step := range []struct {
		in   ipsc.Opcode
		want ipsc.Opcode
	}{{ipsc.OpRegisterRequest, ipsc.OpRegisterReply}, {ipsc.OpPeerListRequest, ipsc.OpPeerListReply}, {ipsc.OpAliveRequest, ipsc.OpAliveReply}} {
		out := m.Packet(time.Second, xprAddr, xprPacket(t, lines, step.in, xpr), nil)
		s := sends(out)
		if len(s) != 1 || out[0].Addr != xprAddr || !bytes.Equal(s[0], xprPacket(t, lines, step.want, 1)) {
			t.Fatalf("%v: %x, want %x", step.in, s, xprPacket(t, lines, step.want, 1))
		}
	}
	if m.Peers() != 1 || m.PeerAt(0).ID != xpr || m.PeerAt(0).Mode != 0x6a || m.PeerAt(5) != (session.PeerInfo{}) {
		t.Fatalf("peers %+v", m.PeerAt(0))
	}
	if out := m.Packet(time.Second, xprAddr, xprPacket(t, lines, ipsc.OpStatus, xpr), nil); len(out) != 0 {
		t.Fatal("status answered")
	}
	calls := 0
	for _, p := range lines {
		if ipsc.Opcode(p[0]) == ipsc.OpGroupVoice || ipsc.Opcode(p[0]) == ipsc.OpGroupData || ipsc.Opcode(p[0]) == ipsc.OpPrivateData {
			out := m.Packet(2*time.Second, xprAddr, p, nil)
			if len(out) != 1 || out[0].Kind != session.OutputData || !bytes.Equal(out[0].Packet, p) || out[0].ID != xpr {
				t.Fatalf("user packet %x: %v", p, kinds(out))
			}
			calls++
		}
	}
	if calls < 50 {
		t.Fatalf("%d user packets", calls)
	}
}

func TestPeerReplaysXPR8400(t *testing.T) {
	t.Parallel()

	lines := readXPR(t)
	const xpr = 0x0004ce2e
	p, err := session.NewPeer(&session.PeerConfig{ID: xpr, Flags: ipsc.Flags{0, 0, 0x80, 0xdc}, Version: [4]byte{4, 2, 4, 1}, Master: masterAddr})
	if err != nil {
		t.Fatal(err)
	}
	reg := sends(p.Tick(0, nil))
	var want []byte
	for _, pk := range lines {
		if len(reg) == 1 && ipsc.Opcode(pk[0]) == ipsc.OpRegisterRequest && bytes.Equal(reg[0][:10], pk[:10]) {
			want = pk
		}
	}
	if want == nil {
		t.Fatalf("register %x, want %x", reg, want)
	}
	out := p.Packet(masterAddr, xprPacket(t, lines, ipsc.OpRegisterReply, 1), nil)
	if len(out) != 1 || out[0].Kind != session.OutputUp || out[0].ID != 1 {
		t.Fatalf("reply %+v", out)
	}
	s := sends(p.Tick(5*time.Second, nil))
	if len(s) != 2 || !bytes.Equal(s[0], xprPacket(t, lines, ipsc.OpAliveRequest, xpr)) || !bytes.Equal(s[1], xprPacket(t, lines, ipsc.OpPeerListRequest, xpr)) {
		t.Fatalf("keepalive and list request %x", s)
	}
	out = p.Packet(masterAddr, xprPacket(t, lines, ipsc.OpPeerListReply, 1), nil)
	if len(out) != 1 || out[0].Kind != session.OutputPeers || p.Peers() != 0 {
		t.Fatalf("list %v", kinds(out))
	}
	p.Packet(masterAddr, xprPacket(t, lines, ipsc.OpAliveReply, 1), nil)
	if s = sends(p.Tick(10*time.Second, nil)); len(s) != 1 || ipsc.Opcode(s[0][0]) != ipsc.OpAliveRequest {
		t.Fatalf("keepalive %x", s)
	}
	for _, pk := range lines {
		if ipsc.Opcode(pk[0]) == ipsc.OpGroupVoice {
			out = p.Packet(masterAddr, append([]byte{pk[0], 0, 0, 0, 1}, pk[5:]...), nil)
			if len(out) != 1 || out[0].Kind != session.OutputData {
				t.Fatalf("voice %v", kinds(out))
			}
		}
	}
	if id, ok := p.Master(); !ok || id != 1 {
		t.Fatal("master")
	}
}

type node struct {
	addr session.Addr
	peer *session.Peer
}

type net struct {
	t       *testing.T
	master  *session.Master
	nodes   []*node
	now     time.Duration
	queue   []packet
	events  map[session.Addr][]session.Output
	deaf    map[session.Addr]bool
	silence map[session.Addr]bool
}

type packet struct {
	from, to session.Addr
	p        []byte
}

func (n *net) out(from session.Addr, out []session.Output) {
	for _, o := range out {
		if o.Kind == session.OutputSend {
			if !n.silence[from] {
				n.queue = append(n.queue, packet{from, o.Addr, append([]byte(nil), o.Packet...)})
			}
			continue
		}
		n.events[from] = append(n.events[from], o)
	}
}

func (n *net) pump() {
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
			n.out(masterAddr, n.master.Packet(n.now, q.from, q.p, nil))
			continue
		}
		for _, nd := range n.nodes {
			if nd.addr == q.to {
				n.out(nd.addr, nd.peer.Packet(q.from, q.p, nil))
			}
		}
	}
	n.t.Fatal("network does not settle")
}

func (n *net) run(to time.Duration) {
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

func count(out []session.Output, k session.OutputKind, r session.Reason) int {
	c := 0
	for _, o := range out {
		if o.Kind == k && o.Reason == r {
			c++
		}
	}
	return c
}

func newNet(t *testing.T) *net {
	t.Helper()
	key, _ := ipscKey()
	m, err := session.NewMaster(&session.MasterConfig{ID: 1, Auth: true, Key: key, Timeout: 60 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	n := &net{t: t, master: m, events: map[session.Addr][]session.Output{}, deaf: map[session.Addr]bool{}, silence: map[session.Addr]bool{}}
	for i := range 3 {
		addr := session.Addr{IP: [4]byte{10, 0, 0, byte(i + 2)}, Port: 50000}
		p, err := session.NewPeer(&session.PeerConfig{ID: uint32(100 + i), Master: masterAddr, Auth: true, Key: key, MaxMissed: 3})
		if err != nil {
			t.Fatal(err)
		}
		n.nodes = append(n.nodes, &node{addr: addr, peer: p})
	}
	n.run(30 * time.Second)
	if m.Peers() != 3 || count(n.events[masterAddr], session.OutputUp, session.ReasonNone) != 3 {
		t.Fatalf("master peers %d %v", m.Peers(), kinds(n.events[masterAddr]))
	}
	for _, nd := range n.nodes {
		ev := n.events[nd.addr]
		if nd.peer.State() != session.StateRunning || nd.peer.Peers() != 2 || count(ev, session.OutputUp, session.ReasonNone) != 3 {
			t.Fatalf("%v: %v peers %d %v", nd.addr, nd.peer.State(), nd.peer.Peers(), kinds(ev))
		}
		for i := range nd.peer.Peers() {
			if !nd.peer.PeerAt(i).Up {
				t.Fatalf("%v: peer %+v down", nd.addr, nd.peer.PeerAt(i))
			}
		}
	}
	n.events = map[session.Addr][]session.Output{}
	return n
}

func TestLoopback(t *testing.T) {
	t.Parallel()

	n := newNet(t)
	m := n.master
	n.events = map[session.Addr][]session.Output{}
	n.run(10 * time.Minute)
	for a, ev := range n.events {
		if count(ev, session.OutputDown, session.ReasonTimeout) != 0 {
			t.Fatalf("%v: %v", a, kinds(ev))
		}
	}
	a, b := n.nodes[0], n.nodes[1]
	voice := a.peer.Sign([]byte{byte(ipsc.OpGroupVoice), 0, 0, 0, 100, 1, 2, 3})
	out := b.peer.Packet(a.addr, voice, nil)
	if len(out) != 1 || out[0].Kind != session.OutputData || out[0].ID != 100 || len(out[0].Packet) != 8 {
		t.Fatalf("voice %+v", out)
	}
	if out = b.peer.Packet(a.addr, voice[:8], nil); len(out) != 0 {
		t.Fatal("packet without trailer taken")
	}
	if out = m.Packet(n.now, a.addr, voice, nil); len(out) != 1 || out[0].Kind != session.OutputData {
		t.Fatalf("master voice %v", kinds(out))
	}
}

func TestLoopbackSilentPeer(t *testing.T) {
	t.Parallel()

	n := newNet(t)
	m := n.master
	a, b, c := n.nodes[0], n.nodes[1], n.nodes[2]
	n.events = map[session.Addr][]session.Output{}
	n.deaf[c.addr], n.silence[c.addr] = true, true
	n.run(n.now + 2*time.Minute)
	if m.Peers() != 2 || count(n.events[masterAddr], session.OutputDown, session.ReasonTimeout) != 1 {
		t.Fatalf("master after silence %d %v", m.Peers(), kinds(n.events[masterAddr]))
	}
	for _, nd := range []*node{a, b} {
		ev := n.events[nd.addr]
		if count(ev, session.OutputDown, session.ReasonTimeout)+count(ev, session.OutputDown, session.ReasonList) != 1 || nd.peer.Peers() != 1 {
			t.Fatalf("%v after silence: %v peers %d", nd.addr, kinds(ev), nd.peer.Peers())
		}
	}
	ev := n.events[c.addr]
	if count(ev, session.OutputDown, session.ReasonTimeout) != 3 || c.peer.State() != session.StateRegistering || c.peer.Peers() != 2 {
		t.Fatalf("silent peer: %v %v", kinds(ev), c.peer.State())
	}
	n.deaf[c.addr], n.silence[c.addr] = false, false
	n.events = map[session.Addr][]session.Output{}
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
	n.events = map[session.Addr][]session.Output{}
	n.out(b.addr, b.peer.Close(nil))
	n.pump()
	if m.Peers() != 2 || count(n.events[masterAddr], session.OutputDown, session.ReasonClosed) != 1 || b.peer.State() != session.StateIdle {
		t.Fatalf("close: %d %v", m.Peers(), kinds(n.events[masterAddr]))
	}
	if count(n.events[a.addr], session.OutputDown, session.ReasonClosed) != 1 || a.peer.Peers() != 1 {
		t.Fatalf("a after close %v %d", kinds(n.events[a.addr]), a.peer.Peers())
	}
	if _, ok := b.peer.Deadline(); ok {
		t.Fatal("deadline after Close")
	}
	n.events = map[session.Addr][]session.Output{}
	n.out(masterAddr, m.Close(nil))
	n.pump()
	if m.State() != session.StateIdle || count(n.events[a.addr], session.OutputDown, session.ReasonClosed) != 1 || a.peer.State() != session.StateRegistering {
		t.Fatalf("master close %v %v", kinds(n.events[a.addr]), a.peer.State())
	}
	b.peer.Reset()
	m.Reset()
	if m.State() != session.StateRunning || b.peer.State() != session.StateRegistering {
		t.Fatal("reset")
	}
}

func TestMasterKeepaliveRegisters(t *testing.T) {
	t.Parallel()

	alive, _ := ipsc.Control{Opcode: ipsc.OpAliveRequest, ID: 7, Mode: 0x6a}.AppendBinary(nil)
	open, _ := session.NewMaster(&session.MasterConfig{ID: 1})
	if out := open.Packet(0, xprAddr, alive, nil); !isDrop(out, 7, session.ReasonUnregistered) {
		t.Fatal("keepalive of a peer that is not registered taken without Auth")
	}
	key, _ := ipscKey()
	m, _ := session.NewMaster(&session.MasterConfig{ID: 1, Auth: true, Key: key, MaxPeers: 1})
	out := m.Packet(0, xprAddr, m.Sign(append([]byte(nil), alive...)), nil)
	if len(out) != 2 || out[1].Kind != session.OutputUp {
		t.Fatalf("authenticated keepalive %v", kinds(out))
	}
	other, _ := ipsc.Control{Opcode: ipsc.OpRegisterRequest, ID: 8}.AppendBinary(nil)
	if out = m.Packet(0, masterAddr, m.Sign(other), nil); !isDrop(out, 8, session.ReasonFull) {
		t.Fatal("registered beyond MaxPeers")
	}
	dereg, _ := ipsc.Control{Opcode: ipsc.OpDeregisterRequest, ID: 7}.AppendBinary(nil)
	if out = m.Packet(0, masterAddr, m.Sign(dereg), nil); !isDrop(out, 7, session.ReasonAddress) || m.Peers() != 1 {
		t.Fatal("de-registration from another address")
	}
	for _, op := range []ipsc.Opcode{ipsc.OpPeerRegisterRequest, ipsc.OpPeerAliveRequest} {
		c, _ := ipsc.Control{Opcode: op, ID: 7}.AppendBinary(nil)
		if out = m.Packet(time.Second, xprAddr, m.Sign(c), nil); len(out) != 1 || out[0].Kind != session.OutputSend {
			t.Fatalf("%v: %v", op, kinds(out))
		}
	}
	if at, _ := m.Deadline(); at != time.Second+session.TimeoutDefault {
		t.Fatalf("deadline %v", at)
	}
}

func TestConfigErrors(t *testing.T) {
	t.Parallel()

	var ce *session.ConfigError
	for i, cfg := range []*session.PeerConfig{nil, {Alive: -1}, {MaxMissed: -1}, {MaxPeers: -1}, {MaxPeers: 6000}} {
		if _, err := session.NewPeer(cfg); !errors.As(err, &ce) || ce.Error() == "" {
			t.Errorf("peer %d: %v", i, err)
		}
	}
	for i, cfg := range []*session.MasterConfig{nil, {Timeout: -1}, {MaxPeers: 6000}} {
		if _, err := session.NewMaster(cfg); !errors.As(err, &ce) {
			t.Errorf("master %d: %v", i, err)
		}
	}
}

func TestNames(t *testing.T) {
	t.Parallel()

	if session.OutputData.String() != "Data" || session.OutputKind(0).String() != "Unknown" || session.ReasonList.String() != "List" ||
		session.Reason(99).String() != "Unknown" || session.StateRegistering.String() != "Registering" || session.State(9).String() != "Unknown" {
		t.Fatal("names")
	}
	for k := session.OutputKind(0); k <= session.OutputDropped; k++ {
		_ = k.String()
	}
	for r := session.Reason(0); r <= session.ReasonAddress; r++ {
		_ = r.String()
	}
	for s := session.State(0); s <= session.StateRunning; s++ {
		_ = s.String()
	}
}
