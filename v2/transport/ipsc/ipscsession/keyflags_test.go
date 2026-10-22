package ipscsession_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/auth"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscsession"
)

func TestKeyedRepliesCarryAuthFlag(t *testing.T) {
	t.Parallel()

	lines := readXPR(t)
	const xpr = 0x0004ce2e
	key, _ := auth.ParseKey("1234")
	m, _ := ipscsession.NewMaster(&ipscsession.MasterConfig{ID: 1})
	signer := auth.NewKey(key)
	for _, step := range []struct{ in, want ipsc.Opcode }{{ipsc.OpRegisterRequest, ipsc.OpRegisterReply}, {ipsc.OpAliveRequest, ipsc.OpAliveReply}} {
		in, _ := signer.Sign(append([]byte(nil), xprPacket(t, lines, step.in, xpr)...))
		out := m.Packet(0, xprAddr, in, &key, nil)
		want := xprPacket(t, lines, step.want, 1)
		if len(out) == 0 || out[0].Kind != ipscsession.OutputSend || !bytes.Equal(out[0].Packet[:len(out[0].Packet)-auth.TrailerLen], want) {
			t.Fatalf("%v: %x, want %x", step.in, out[0].Packet, want)
		}
	}
	out := m.Packet(0, masterAddr, ctl(ipsc.OpRegisterRequest, 7), nil, nil)
	if c, err := ipsc.DecodeControl(out[0].Packet); err != nil || c.Flags.Has(ipsc.FlagAuth) {
		t.Fatalf("reply to a peer without a key %x", out[0].Packet)
	}
}
