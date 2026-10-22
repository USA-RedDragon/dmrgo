package ipsc_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
)

func TestControlPeerListReplyRoundTrip(t *testing.T) {
	t.Parallel()
	c := ipsc.Control{Opcode: ipsc.OpPeerListReply, ID: 0x01020304}
	out, err := c.AppendBinary([]byte{9})
	if err != nil {
		t.Fatal(err)
	}
	back, err := ipsc.DecodeControl(out[1:])
	if err != nil || back != c {
		t.Fatalf("empty list %x: %+v %v", out, back, err)
	}
	if !bytes.Equal(out[1:], ipsc.AppendPeerListReply(nil, c.ID, nil)) {
		t.Fatalf("empty list %x differs from AppendPeerListReply", out)
	}
	c.ListLen = 11
	out, err = c.AppendBinary([]byte{9})
	if err == nil {
		if _, derr := ipsc.DecodeControl(out[1:]); derr != nil {
			t.Fatalf("ListLen 11 appended %x, which DecodeControl rejects: %v", out, derr)
		}
	}
	if !bytes.Equal(out, []byte{9}) {
		t.Fatalf("ListLen 11 with an error appended %x", out)
	}
}
