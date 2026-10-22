package sms_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/sms"
)

func TestNetworkAndGroupPrefix(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		network, prefix         uint8
		wantNetwork, wantPrefix uint8
	}{
		{0, 0, ip.DefaultNetwork, 225},
		{13, 239, 13, 239},
	} {
		for _, f := range []sms.Format{sms.FormatTMS, sms.FormatIP, sms.FormatAnyTone} {
			for _, group := range []bool{false, true} {
				in := sms.Text{Format: f, Source: 1234, Destination: 99, Group: group, Network: c.network, GroupPrefix: c.prefix, Body: []byte("hi")}
				var h sms.Header
				out, err := sms.AppendPacket(nil, &in, false, &h)
				if err != nil {
					t.Fatalf("%s: %v", f, err)
				}
				u, ok := h.Data.AsUnconfirmedDataHeader()
				if !ok {
					t.Fatalf("%s: header %v is not a U_HEAD", f, &h.Data)
				}
				d, err := ip.Decode(u.SAP, out)
				if err != nil {
					t.Fatalf("%s: %v", f, err)
				}
				wantDst := ip.RadioAddress(c.wantNetwork, 99)
				if group {
					wantDst = ip.GroupAddress(c.wantPrefix, 99)
				}
				if d.Source != ip.RadioAddress(c.wantNetwork, 1234) || d.Destination != wantDst {
					t.Errorf("%s network %d prefix %d group %v: %v to %v", f, c.network, c.prefix, group, d.Source, d.Destination)
				}
				_, got, err := sms.Decode(nil, &h.Data, out)
				if err != nil {
					t.Fatalf("%s: %v", f, err)
				}
				wantPrefix := uint8(0)
				if group {
					wantPrefix = c.wantPrefix
				}
				if got.Network != c.wantNetwork || got.GroupPrefix != wantPrefix {
					t.Errorf("%s: Decode gives network %d prefix %d, want %d %d", f, got.Network, got.GroupPrefix, c.wantNetwork, wantPrefix)
				}
			}
		}
	}
}

func TestNilArguments(t *testing.T) {
	t.Parallel()

	var h sms.Header
	if out, err := sms.AppendPacket([]byte("x"), nil, false, &h); !errors.Is(err, sms.ErrNil) || string(out) != "x" {
		t.Errorf("nil Text: %q %v", out, err)
	}
	if _, err := sms.AppendPacket(nil, &sms.Text{Format: sms.FormatIP}, false, nil); !errors.Is(err, sms.ErrNil) {
		t.Errorf("nil Header: %v", err)
	}
	if _, _, err := sms.Decode(nil, nil, []byte{1}); !errors.Is(err, sms.ErrNil) {
		t.Errorf("Decode nil header: %v", err)
	}
}
