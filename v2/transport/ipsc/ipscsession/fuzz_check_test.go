package ipscsession_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

const (
	maxOps   = 128
	maxRaw   = 128
	maxSteps = 512

	repeatOctet = 21
)

type fuzzInput struct {
	ops, raw       []byte
	from, to, reps uint8
}

type fuzzPacket struct {
	pk   []byte
	from int
	dt   time.Duration
	raw  bool
}

func fuzzPool() [][]byte {
	ctl := func(op ipsc.Opcode, id uint32) []byte {
		b, _ := (&ipsc.Control{Opcode: op, ID: id, Mode: 0x6a}).AppendBinary(nil)
		return b
	}
	user := func(h string, id uint32) []byte {
		b, _ := hex.DecodeString(h)
		u, _ := ipsc.DecodeUser(b)
		u.ID = id
		b, _ = u.AppendBinary(nil)
		return b
	}
	return [][]byte{
		ctl(ipsc.OpRegisterRequest, 2),
		ctl(ipsc.OpAliveRequest, 2),
		ctl(ipsc.OpPeerListRequest, 2),
		ctl(ipsc.OpDeregisterRequest, 2),
		ctl(ipsc.OpRegisterReply, 1),
		ipsc.AppendPeerListReply(nil, 1, []ipsc.PeerEntry{{ID: 2, IP: [4]byte{10, 0, 0, 2}, Port: 1}, {ID: 3, IP: [4]byte{10, 0, 0, 3}, Port: 1}}),
		ctl(ipsc.OpAliveReply, 1),
		ctl(ipsc.OpPeerRegisterReply, 2),
		ctl(ipsc.OpPeerAliveRequest, 3),
		ctl(ipsc.OpDeregisterRequest, 1),
		user("800004ce2e0330b43c00000902000047de20805d014a30060508000000008a1440541cbd8ee6d793cd54372cbe82e8c4004a2a78", 1),
		user("830004ce2e0130b43c000009010000701360805eeb3337a9eef10000000008c2000d800a009052474f203445535400000000bd3e6a6c444d0078488b", 2),
	}
}

func fuzzSeeds() []fuzzInput {
	voice := fuzzPool()[10]
	return []fuzzInput{
		{ops: []byte{0, 0, 7, 1, 1, 7, 2, 2, 7, 3, 0, 7}},
		{ops: []byte{4, 0, 7, 5, 1, 7, 6, 2, 7, 7, 0, 7, 8, 1, 7}},
		{ops: []byte{4, 0, 7, 5, 1, 7, 0x87, 2, 7, 9, 0, 7}, raw: []byte{0x80, 0, 0, 0, 2, 9, 9}},
		{},
		{ops: []byte{4, 0, 7, 5, 1, 7, 6, 0, 50, 8, 2, 50, 9, 0, 7}, from: 2, to: 4, reps: 40},
		{ops: []byte{4, 0, 7, 5, 0, 7, 0, 1, 7, 10, 0, 1, 11, 1, 1, 9, 0, 7}, from: 3, to: 5, reps: 20},
		{ops: []byte{4, 0, 7, 5, 0, 7, 0x80 | byte(len(voice)), 0, 1, 9, 0, 7}, raw: voice, from: 2, to: 3, reps: 30},
	}
}

func scriptLoop(n int, from, to, reps uint8) (lo, hi, total int) {
	lo = min(int(from), n)
	hi = max(min(int(to), n), lo)
	return lo, hi, min(n+int(reps)*(hi-lo), maxSteps)
}

func scriptIndex(s, lo, hi int, reps uint8) int {
	if s < hi {
		return s
	}
	s -= hi
	w := int(reps) * (hi - lo)
	if s < w {
		return lo + s%(hi-lo)
	}
	return hi + s - w
}

func scriptRepeat(s, lo, hi int, reps uint8) int {
	if s < hi || hi == lo || s-hi >= int(reps)*(hi-lo) {
		return 0
	}
	return (s-hi)/(hi-lo) + 1
}

func fuzzPackets(pool [][]byte, in fuzzInput, f func(pk []byte, from int, dt time.Duration)) {
	ops := in.ops[:min(len(in.ops), maxOps)]
	raw := in.raw[:min(len(in.raw), maxRaw)]
	list := make([]fuzzPacket, 0, len(ops)/3)
	for i := 0; i+2 < len(ops); i += 3 {
		sel := ops[i]
		pk, isRaw := pool[int(sel)%len(pool)], sel&0x80 != 0
		if isRaw {
			n := min(int(sel&0x7F), len(raw))
			pk, raw = raw[:n], raw[n:]
		}
		list = append(list, fuzzPacket{pk: pk, from: int(ops[i+1]), dt: time.Duration(ops[i+2]) * 100 * time.Millisecond, raw: isRaw})
	}
	lo, hi, total := scriptLoop(len(list), in.from, in.to, in.reps)
	for s := range total {
		p := list[scriptIndex(s, lo, hi, in.reps)]
		pk := append([]byte{}, p.pk...)
		if p.raw && len(pk) > repeatOctet {
			pk[repeatOctet] ^= byte(scriptRepeat(s, lo, hi, in.reps))
		}
		f(pk, p.from, p.dt)
	}
}

func checkSession(t *testing.T, pool [][]byte, in fuzzInput) {
	addrs := []ipscsession.Addr{masterAddr, {IP: [4]byte{10, 0, 0, 2}, Port: 1}, {IP: [4]byte{10, 0, 0, 3}, Port: 1}}
	for _, useAuth := range []bool{false, true} {
		key, _ := ipscKey()
		p, err := ipscsession.NewPeer(&ipscsession.PeerConfig{ID: 9, Master: masterAddr, Auth: useAuth, Key: key, MaxPeers: 2, MaxMissed: 2})
		if err != nil {
			t.Fatal(err)
		}
		m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1, Auth: useAuth, Key: key, MaxPeers: 2})
		out := make([]ipscsession.Output, 0, 64)
		now := time.Duration(0)
		check := func(n, limit int) {
			for _, o := range out[n:] {
				if limit == p.MaxOutputs() && o.Kind == ipscsession.OutputSend && controlTooLong(o.Packet) {
					t.Fatalf("peer sent %d octets", len(o.Packet))
				}
			}
			if len(out)-n > limit {
				t.Fatalf("%d outputs, limit %d", len(out)-n, limit)
			}
			for _, o := range out[n:] {
				if o.Kind < ipscsession.OutputSend || o.Kind > ipscsession.OutputDropped {
					t.Fatalf("kind %v", o.Kind)
				}
				if tooLong(o.Packet) {
					t.Fatalf("%d octets", len(o.Packet))
				}
				if o.Kind == ipscsession.OutputSend {
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
		fuzzPackets(pool, in, func(pk []byte, i int, dt time.Duration) {
			from := addrs[i%len(addrs)]
			now += dt
			if useAuth {
				pk = must(m.Sign(append([]byte(nil), pk...)))
			}
			n := len(out)
			out = p.Tick(now, out)
			check(n, p.MaxOutputs())
			n = len(out)
			out = p.Packet(0, from, pk, out)
			check(n, p.MaxOutputs())
			n = len(out)
			out = m.Tick(now, out)
			check(n, m.MaxOutputs())
			n = len(out)
			out = m.Packet(now, from, pk, nil, out)
			check(n, m.MaxOutputs())
			n = len(out)
			out = m.Packet(now, from, must(m.SignFor(9, append([]byte(nil), pk...))), &key, out)
			check(n, m.MaxOutputs())
			n = len(out)
			if len(pk) > 4 && pk[4]&1 == 1 {
				out = m.Adopt(now, from, uint32(pk[4]), &key, out)
			} else if len(pk) > 4 {
				out = m.Adopt(now, from, uint32(pk[4]), nil, out)
			}
			check(n, m.MaxOutputs())
		})
		n := len(out)
		out = p.Close(0, out)
		check(n, p.MaxOutputs())
		n = len(out)
		out = m.Close(0, out)
		check(n, m.MaxOutputs())
	}
}
