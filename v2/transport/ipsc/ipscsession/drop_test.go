package ipscsession_test

import (
	"sync"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

func dropAddr(id uint32) ipscsession.Addr {
	return ipscsession.Addr{IP: [4]byte{10, 0, 1, byte(id)}, Port: 50000}
}

func registerAt(t *testing.T, m *ipscsession.Master, now time.Duration, id uint32) []ipscsession.Output {
	t.Helper()
	reg, _ := (&ipsc.Control{Opcode: ipsc.OpRegisterRequest, ID: id, Mode: 0x6a}).AppendBinary(nil)
	return m.Packet(now, dropAddr(id), reg, nil, nil)
}

func listIDs(t *testing.T, p []byte) []uint32 {
	t.Helper()
	l, err := ipsc.PeerListOf(p)
	if err != nil {
		t.Fatalf("peer list %x: %v", p, err)
	}
	ids := make([]uint32, l.Len())
	for i := range ids {
		ids[i] = l.Entry(i).ID
	}
	return ids
}

func TestMasterDrop(t *testing.T) {
	t.Parallel()

	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1})
	for _, id := range []uint32{7, 8, 9} {
		if out := registerAt(t, m, 0, id); count(out, ipscsession.OutputUp, ipscsession.ReasonNone) != 1 {
			t.Fatalf("register %d: %v", id, kinds(out))
		}
	}
	out, ok := m.Drop(8, nil)
	if !ok || len(out) != 3 || out[0].Kind != ipscsession.OutputDown || out[0].ID != 8 || out[0].Addr != dropAddr(8) || out[0].Reason != ipscsession.ReasonDropped {
		t.Fatalf("drop: %v %+v", ok, out)
	}
	to := map[ipscsession.Addr]bool{}
	for _, o := range out[1:] {
		if o.Kind != ipscsession.OutputSend || ipsc.Opcode(o.Packet[0]) != ipsc.OpPeerListReply {
			t.Fatalf("drop sends %+v", o)
		}
		for _, id := range listIDs(t, o.Packet) {
			if id == 8 {
				t.Fatalf("list after drop holds 8: %x", o.Packet)
			}
		}
		to[o.Addr] = true
	}
	if !to[dropAddr(7)] || !to[dropAddr(9)] || to[dropAddr(8)] {
		t.Fatalf("list sent to %v", to)
	}
	if m.Peers() != 2 || peerOK(m.Peer(8)) || !peerOK(m.Peer(7)) || !peerOK(m.Peer(9)) {
		t.Fatalf("peers after drop %d", m.Peers())
	}
	voice := []byte{byte(ipsc.OpGroupVoice), 0, 0, 0, 8, 1}
	if out = m.Packet(time.Second, dropAddr(8), voice, nil, nil); !isDrop(out, 8, ipscsession.ReasonUnregistered) {
		t.Fatalf("voice after drop %+v", out)
	}
	alive, _ := (&ipsc.Control{Opcode: ipsc.OpAliveRequest, ID: 8}).AppendBinary(nil)
	if out = m.Packet(time.Second, dropAddr(8), alive, nil, nil); !isDrop(out, 8, ipscsession.ReasonUnregistered) {
		t.Fatalf("keepalive after drop %+v", out)
	}
	if out, ok = m.Drop(8, nil); ok || len(out) != 0 {
		t.Fatalf("second drop %v %+v", ok, out)
	}
	if out = registerAt(t, m, 2*time.Second, 8); count(out, ipscsession.OutputUp, ipscsession.ReasonNone) != 1 || m.Peers() != 3 {
		t.Fatalf("register after drop %v", kinds(out))
	}
	if out = m.Packet(3*time.Second, dropAddr(8), voice, nil, nil); len(out) != 1 || out[0].Kind != ipscsession.OutputData {
		t.Fatalf("voice after register %+v", out)
	}
}

func TestMasterDropUnknown(t *testing.T) {
	t.Parallel()

	var nilMaster *ipscsession.Master
	if out, ok := nilMaster.Drop(7, nil); ok || out != nil {
		t.Fatal("nil Master dropped")
	}
	if out, ok := (&ipscsession.Master{}).Drop(7, nil); ok || out != nil {
		t.Fatal("zero Master dropped")
	}
	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1})
	registerAt(t, m, 0, 7)
	dst := make([]ipscsession.Output, 1, 4)
	if out, ok := m.Drop(99, dst); ok || len(out) != 1 || m.Peers() != 1 {
		t.Fatalf("unknown drop %v %d", ok, len(out))
	}
	m.Close(0, nil)
	if _, ok := m.Drop(7, nil); ok {
		t.Fatal("drop after Close")
	}
}

func TestMasterDropOwnKey(t *testing.T) {
	t.Parallel()

	key, _ := ipscKey()
	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1})
	registerAt(t, m, 0, 7)
	reg, _ := (&ipsc.Control{Opcode: ipsc.OpRegisterRequest, ID: 8}).AppendBinary(nil)
	signer, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: true, Key: key})
	signed := must(signer.Sign(reg))
	if out := m.Packet(0, dropAddr(8), signed, &key, nil); count(out, ipscsession.OutputUp, ipscsession.ReasonNone) != 1 {
		t.Fatalf("keyed register %v", kinds(out))
	}
	out, ok := m.Drop(8, nil)
	if !ok || len(out) != 1 || out[0].Kind != ipscsession.OutputDown || out[0].Reason != ipscsession.ReasonDropped || m.Peers() != 1 {
		t.Fatalf("keyed drop %v %+v", ok, out)
	}
	rotated := key
	rotated[0] ^= 1
	alive, _ := (&ipsc.Control{Opcode: ipsc.OpAliveRequest, ID: 8}).AppendBinary(nil)
	if out = m.Packet(time.Second, dropAddr(8), must(signer.Sign(alive)), &rotated, nil); !isDrop(out, 0, ipscsession.ReasonAuth) || m.Peers() != 1 {
		t.Fatalf("old key after drop %+v", out)
	}
	newSigner, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: true, Key: rotated})
	reg, _ = (&ipsc.Control{Opcode: ipsc.OpRegisterRequest, ID: 8}).AppendBinary(nil)
	if out = m.Packet(2*time.Second, dropAddr(8), must(newSigner.Sign(reg)), &rotated, nil); count(out, ipscsession.OutputUp, ipscsession.ReasonNone) != 1 || m.Peers() != 2 {
		t.Fatalf("new key after drop %v", kinds(out))
	}
}

func TestMasterDropRace(t *testing.T) {
	t.Parallel()

	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1})
	var mu sync.Mutex
	var wg sync.WaitGroup
	start := time.Now()
	ids := []uint32{7, 8, 9, 10}
	regs := make([][]byte, len(ids))
	for i, id := range ids {
		regs[i], _ = (&ipsc.Control{Opcode: ipsc.OpRegisterRequest, ID: id}).AppendBinary(nil)
	}
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := make([]ipscsession.Output, 0, m.MaxOutputs())
			voice := []byte{byte(ipsc.OpGroupVoice), 0, 0, 0, byte(id), 1}
			for range 500 {
				mu.Lock()
				out = m.Packet(time.Since(start), dropAddr(id), regs[i], nil, out[:0])
				out = m.Packet(time.Since(start), dropAddr(id), voice, nil, out[:0])
				for _, o := range out {
					if o.Kind == ipscsession.OutputData && o.ID != id {
						t.Errorf("data of %d from %d", o.ID, id)
					}
				}
				mu.Unlock()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		out := make([]ipscsession.Output, 0, m.MaxOutputs())
		for n := range 2000 {
			mu.Lock()
			var ok bool
			if out, ok = m.Drop(ids[n%len(ids)], out[:0]); ok {
				if _, still := m.Peer(ids[n%len(ids)]); still {
					t.Errorf("peer %d kept after drop", ids[n%len(ids)])
				}
			}
			out = m.Tick(time.Since(start), out[:0])
			mu.Unlock()
		}
	}()
	wg.Wait()
}
