package session_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/session"
)

func fuzzSeeds() [][]byte {
	join := func(ps ...[]byte) []byte {
		var out []byte
		for i, p := range ps {
			out = append(out, byte(len(p)>>8), byte(len(p)), byte(i%3), 7)
			out = append(out, p...)
		}
		return out
	}
	ctl := func(op ipsc.Opcode, id uint32) []byte {
		b, _ := ipsc.Control{Opcode: op, ID: id, Mode: 0x6a}.AppendBinary(nil)
		return b
	}
	list := ipsc.AppendPeerListReply(nil, 1, []ipsc.PeerEntry{{ID: 2, IP: [4]byte{10, 0, 0, 2}, Port: 1}, {ID: 3, IP: [4]byte{10, 0, 0, 3}, Port: 1}})
	return [][]byte{
		join(ctl(ipsc.OpRegisterRequest, 2), ctl(ipsc.OpAliveRequest, 2), ctl(ipsc.OpPeerListRequest, 2), ctl(ipsc.OpDeregisterRequest, 2)),
		join(ctl(ipsc.OpRegisterReply, 1), list, ctl(ipsc.OpAliveReply, 1), ctl(ipsc.OpPeerRegisterReply, 2), ctl(ipsc.OpPeerAliveRequest, 3)),
		join(ctl(ipsc.OpRegisterReply, 1), list, []byte{0x80, 0, 0, 0, 2, 9, 9}, ctl(ipsc.OpDeregisterRequest, 1)),
		{},
	}
}

func checkSession(t *testing.T, data []byte) {
	addrs := []session.Addr{masterAddr, {IP: [4]byte{10, 0, 0, 2}, Port: 1}, {IP: [4]byte{10, 0, 0, 3}, Port: 1}}
	for _, useAuth := range []bool{false, true} {
		key, _ := ipscKey()
		p, err := session.NewPeer(&session.PeerConfig{ID: 9, Master: masterAddr, Auth: useAuth, Key: key, MaxPeers: 2, MaxMissed: 2})
		if err != nil {
			t.Fatal(err)
		}
		m, _ := session.NewMaster(&session.MasterConfig{ID: 1, Auth: useAuth, Key: key, MaxPeers: 2})
		out := make([]session.Output, 0, 64)
		now := time.Duration(0)
		check := func(n, limit int) {
			for _, o := range out[n:] {
				if limit == p.MaxOutputs() && o.Kind == session.OutputSend && controlTooLong(o.Packet) {
					t.Fatalf("peer sent %d octets", len(o.Packet))
				}
			}
			if len(out)-n > limit {
				t.Fatalf("%d outputs, limit %d", len(out)-n, limit)
			}
			for _, o := range out[n:] {
				if o.Kind < session.OutputSend || o.Kind > session.OutputDropped {
					t.Fatalf("kind %v", o.Kind)
				}
				if tooLong(o.Packet) {
					t.Fatalf("%d octets", len(o.Packet))
				}
				if o.Kind == session.OutputSend {
					_, err := ipsc.DecodeControl(o.Packet)
					if err != nil && len(o.Packet) > 10 {
						_, err = ipsc.DecodeControl(o.Packet[:len(o.Packet)-10])
					}
					if err != nil {
						t.Fatalf("sent %x: %v", o.Packet, err)
					}
				}
			}
			if len(out) > 48 {
				out = out[:0]
			}
		}
		for len(data) >= 4 {
			size := int(data[0])<<8 | int(data[1])
			from := addrs[int(data[2])%len(addrs)]
			now += time.Duration(data[3]) * 100 * time.Millisecond
			data = data[4:]
			size = min(size, len(data))
			pk := data[:size]
			data = data[size:]
			if useAuth {
				pk = m.Sign(append([]byte(nil), pk...))
			}
			n := len(out)
			out = p.Tick(now, out)
			check(n, p.MaxOutputs())
			n = len(out)
			out = p.Packet(from, pk, out)
			check(n, p.MaxOutputs())
			n = len(out)
			out = m.Tick(now, out)
			check(n, m.MaxOutputs())
			n = len(out)
			out = m.Packet(now, from, pk, out)
			check(n, m.MaxOutputs())
			n = len(out)
			out = m.PacketWithKey(now, from, key, m.SignFor(9, append([]byte(nil), pk...)), out)
			check(n, m.MaxOutputs())
			n = len(out)
			if len(pk) > 4 && pk[4]&1 == 1 {
				out = m.AdoptWithKey(now, from, uint32(pk[4]), ipsc.Mode(pk[0]), ipsc.Flags{}, key, out)
			} else if len(pk) > 4 {
				out = m.Adopt(now, from, uint32(pk[4]), ipsc.Mode(pk[0]), ipsc.Flags{}, out)
			}
			check(n, m.MaxOutputs())
		}
		n := len(out)
		out = p.Close(out)
		check(n, p.MaxOutputs())
		n = len(out)
		out = m.Close(out)
		check(n, m.MaxOutputs())
	}
}
