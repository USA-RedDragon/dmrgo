package ip_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
)

const hostUnreachable = "45000038000000004001c2330c2febf60c30b43c03018cec000000004500003a000d00004011c2140c30b43c0c2febf60fa70fa70026509e"

func TestICMPMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		m         ip.ICMPMessage
		typ, code uint8
		name      string
	}{
		{ip.ICMPNetworkUnreachable, 3, 0, "Network Unreachable"},
		{ip.ICMPHostUnreachable, 3, 1, "Host Unreachable"},
		{ip.ICMPFragmentationNeeded, 3, 4, "Fragmentation Needed"},
		{ip.ICMPNetworkUnknown, 3, 6, "Destination Network Unknown"},
		{ip.ICMPBadHeader, 12, 0, "IP Header Bad"},
		{ip.ICMPMessage(8 << 8), 8, 0, "ICMPMessage(type 8, code 0)"},
	}
	for _, tt := range tests {
		if tt.m.Type() != tt.typ || tt.m.Code() != tt.code || tt.m.String() != tt.name {
			t.Errorf("%04x: type %d code %d %q", uint16(tt.m), tt.m.Type(), tt.m.Code(), tt.m.String())
		}
	}
}

func TestAppendICMPPacket(t *testing.T) {
	t.Parallel()

	tms := unhex(t, appcapture.TMS)
	c := ip.ICMPPacket{
		Source:      ip.RadioAddress(ip.DefaultNetwork, 3140598),
		Destination: ip.RadioAddress(ip.DefaultNetwork, 3191868),
		Message:     ip.ICMPHostUnreachable,
		Original:    tms,
	}
	out, err := ip.AppendICMPPacket([]byte{0xAA}, &c)
	if err != nil {
		t.Fatal(err)
	}
	if out[0] != 0xAA || hex.EncodeToString(out[1:]) != hostUnreachable {
		t.Fatalf("got %x\nwant %s", out[1:], hostUnreachable)
	}
	back, err := ip.DecodeICMPPacket(append(out[1:], 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if back.Message != c.Message || back.Source != c.Source || back.Destination != c.Destination || back.TTL != 64 ||
		!bytes.Equal(back.Original, tms[:28]) {
		t.Errorf("decoded %+v", back)
	}
	for _, m := range []ip.ICMPPacket{
		{Message: ip.ICMPBadHeader, Pointer: 10, ID: 7, TTL: 3, Original: tms[:20]},
		{Message: ip.ICMPFragmentationNeeded, NextHopMTU: 576, TTL: 64, Original: tms},
		{Message: ip.ICMPNetworkUnknown, TTL: 1, Original: []byte{1, 2, 3}},
	} {
		raw, err := ip.AppendICMPPacket(nil, &m)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ip.DecodeICMPPacket(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got.Message != m.Message || got.Pointer != m.Pointer || got.NextHopMTU != m.NextHopMTU || got.ID != m.ID ||
			got.TTL != m.TTL || len(got.Original) > 28 || !bytes.Equal(got.Original, m.Original[:len(got.Original)]) {
			t.Errorf("round trip %+v gave %+v", m, got)
		}
	}
}

func TestICMPPacketErrors(t *testing.T) {
	t.Parallel()

	good := unhex(t, hostUnreachable)
	mod := func(f func([]byte)) []byte {
		b := append([]byte(nil), good...)
		f(b)
		return b
	}
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"udp", unhex(t, appcapture.TMS), ip.ErrProtocol},
		{"version", mod(func(b []byte) { b[0] = 0x65 }), ip.ErrVersion},
		{"short", good[:30], ip.ErrShort},
		{"icmp short", mod(func(b []byte) { b[3] = 24 }), ip.ErrShort},
		{"ihl", mod(func(b []byte) { b[0] = 0x44 }), ip.ErrLength},
		{"fragment", mod(func(b []byte) { b[6] = 0x20 }), ip.ErrFragment},
		{"ip checksum", mod(func(b []byte) { b[8] = 1 }), ip.ErrChecksum},
		{"icmp checksum", mod(func(b []byte) { b[len(b)-1] ^= 1 }), ip.ErrChecksum},
	}
	for _, tt := range tests {
		if _, err := ip.DecodeICMPPacket(tt.data); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}
	c := ip.ICMPPacket{Message: ip.ICMPHostUnreachable, Original: make([]byte, 65535)}
	if out, err := ip.AppendICMPPacket([]byte{1}, &c); err == nil || len(out) != 1 {
		t.Errorf("long: %d octets, %v", len(out), err)
	}
}

func TestAppendICMPPacketRFC1122(t *testing.T) {
	t.Parallel()

	tms := unhex(t, appcapture.TMS)
	icmp := func(typ uint8) []byte {
		b := append([]byte(nil), unhex(t, hostUnreachable)...)
		b[20] = typ
		return b
	}
	mod := func(f func([]byte)) []byte {
		b := append([]byte(nil), tms...)
		f(b)
		return b
	}
	tests := []struct {
		name string
		orig []byte
		ok   bool
	}{
		{"udp", tms, true},
		{"echo request", icmp(8), true},
		{"echo reply", icmp(0), true},
		{"timestamp", icmp(13), true},
		{"destination unreachable", icmp(3), false},
		{"source quench", icmp(4), false},
		{"redirect", icmp(5), false},
		{"time exceeded", icmp(11), false},
		{"parameter problem", icmp(12), false},
		{"icmp without its type", icmp(8)[:20], false},
		{"first fragment", mod(func(b []byte) { b[6] = 0x20 }), true},
		{"later fragment", mod(func(b []byte) { b[7] = 1 }), false},
		{"later fragment, high bits", mod(func(b []byte) { b[6] = 0x10 }), false},
		{"multicast destination", mod(func(b []byte) { b[16] = 224 }), false},
		{"last multicast destination", mod(func(b []byte) { b[16] = 239 }), false},
		{"broadcast destination", mod(func(b []byte) { copy(b[16:], []byte{255, 255, 255, 255}) }), false},
		{"zero source", mod(func(b []byte) { copy(b[12:], []byte{0, 0, 0, 0}) }), false},
		{"loopback source", mod(func(b []byte) { b[12] = 127 }), false},
		{"multicast source", mod(func(b []byte) { b[12] = 230 }), false},
		{"class E source", mod(func(b []byte) { b[12] = 240 }), false},
		{"broadcast source", mod(func(b []byte) { copy(b[12:], []byte{255, 255, 255, 255}) }), false},
	}
	for _, tt := range tests {
		c := ip.ICMPPacket{Message: ip.ICMPNetworkUnknown, Original: tt.orig}
		out, err := ip.AppendICMPPacket([]byte{1}, &c)
		if tt.ok && (err != nil || len(out) == 1) || !tt.ok && (!errors.Is(err, ip.ErrNoICMP) || len(out) != 1) {
			t.Errorf("%s: %d octets, %v", tt.name, len(out), err)
		}
	}
}

func TestCheckIPv4(t *testing.T) {
	t.Parallel()

	tms := unhex(t, appcapture.TMS)
	mod := func(f func([]byte)) []byte {
		b := append([]byte(nil), tms...)
		f(b)
		return b
	}
	df := mod(func(b []byte) { b[6] = 0x40; b[10], b[11] = 0x82, 0x14 })
	tests := []struct {
		name    string
		data    []byte
		mtu     int
		m       ip.ICMPMessage
		pointer uint8
		ok      bool
	}{
		{"good", tms, 1500, 0, 0, false},
		{"no mtu", df, 0, 0, 0, false},
		{"fits", df, 58, 0, 0, false},
		{"df", df, 57, ip.ICMPFragmentationNeeded, 0, true},
		{"no df", tms, 57, 0, 0, false},
		{"short", tms[:19], 0, ip.ICMPBadHeader, 0, true},
		{"version", mod(func(b []byte) { b[0] = 0x65 }), 0, ip.ICMPBadHeader, 0, true},
		{"ihl", mod(func(b []byte) { b[0] = 0x44 }), 0, ip.ICMPBadHeader, 0, true},
		{"ihl long", mod(func(b []byte) { b[0] = 0x4F })[:40], 0, ip.ICMPBadHeader, 0, true},
		{"length", mod(func(b []byte) { b[3] = 0xFF }), 0, ip.ICMPBadHeader, 2, true},
		{"checksum", mod(func(b []byte) { b[8] = 1 }), 0, ip.ICMPBadHeader, 10, true},
	}
	for _, tt := range tests {
		m, p, ok := ip.CheckIPv4(tt.data, tt.mtu)
		if m != tt.m || p != tt.pointer || ok != tt.ok {
			t.Errorf("%s: %v %d %v", tt.name, m, p, ok)
		}
	}
}
