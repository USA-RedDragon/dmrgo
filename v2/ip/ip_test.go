package ip_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/appcapture"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
)

func unhex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeCapturedIPv4(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		data    string
		port    ip.Port
		id      uint16
		ttl     uint8
		payload string
	}{
		{"TMS", appcapture.TMS, ip.PortTMS, 13, 64, "001ca00081040d000a004d004f0054004f00200054004500530054000000"},
		{"DMR text", appcapture.DMRText, ip.PortText, 12, 1, "000d000a44004d005200200054004500530054000000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			data := unhex(t, tt.data)
			padded := append(append([]byte{}, data...), 0, 0, 0)
			d, err := ip.Decode(enums.SAPIPBasedPacketData, padded)
			if err != nil {
				t.Fatal(err)
			}
			if d.Source != ip.RadioAddress(ip.DefaultNetwork, 3191868) || d.Destination != ip.RadioAddress(ip.DefaultNetwork, 3140598) {
				t.Errorf("addresses %v to %v", d.Source, d.Destination)
			}
			if ip.ID(d.Source) != 3191868 {
				t.Errorf("ID(%v) = %d", d.Source, ip.ID(d.Source))
			}
			if d.SourcePort != tt.port || d.DestinationPort != tt.port || d.ID != tt.id || d.TTL != tt.ttl || d.Compressed {
				t.Errorf("got %+v", d)
			}
			if got := hex.EncodeToString(d.Payload); got != tt.payload {
				t.Errorf("payload %s, want %s", got, tt.payload)
			}
			out, err := ip.AppendIPv4([]byte{0xAA}, &d)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(out[1:], data) || out[0] != 0xAA {
				t.Errorf("AppendIPv4 = %x, want aa%x", out, data)
			}
		})
	}
}

func TestDecodeCapturedCompressed(t *testing.T) {
	t.Parallel()

	data := unhex(t, appcapture.GPS)
	d, err := ip.Decode(enums.SAPUDPIPHeaderCompression, data)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Compressed || d.SourcePort != ip.PortNMEA || d.DestinationPort != ip.PortNMEA || d.ID != 1 || !bytes.HasPrefix(d.Payload, []byte("$GPRMC,")) {
		t.Fatalf("got %+v", d)
	}
	out, err := ip.AppendCompressed(nil, &d)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, data) {
		t.Errorf("AppendCompressed = %x, want %x", out, data)
	}
}

func TestCompressedPortIDs(t *testing.T) {
	t.Parallel()

	d := ip.Datagram{SourcePort: ip.PortText, DestinationPort: ip.PortLocation, ID: 0x1234, Payload: []byte{1, 2}}
	out, err := ip.AppendCompressed(nil, &d)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := hex.EncodeToString(out), "12340001020102"; got != want {
		t.Fatalf("AppendCompressed = %s, want %s", got, want)
	}
	back, err := ip.Decode(enums.SAPUDPIPHeaderCompression, out)
	if err != nil || back.SourcePort != ip.PortText || back.DestinationPort != ip.PortLocation || !bytes.Equal(back.Payload, d.Payload) {
		t.Fatalf("Decode = %+v, %v", back, err)
	}
}

func TestDecodeErrors(t *testing.T) {
	t.Parallel()

	good := unhex(t, appcapture.TMS)
	mod := func(f func(b []byte) []byte) []byte { return f(append([]byte{}, good...)) }
	tests := []struct {
		name string
		sap  enums.ServiceAccessPointID
		data []byte
		want error
	}{
		{"SAP", enums.SAPShortData, good, ip.ErrSAP},
		{"short", enums.SAPIPBasedPacketData, good[:10], ip.ErrShort},
		{"total beyond data", enums.SAPIPBasedPacketData, good[:40], ip.ErrShort},
		{"version", enums.SAPIPBasedPacketData, mod(func(b []byte) []byte { b[0] = 0x65; return b }), ip.ErrVersion},
		{"IHL", enums.SAPIPBasedPacketData, mod(func(b []byte) []byte { b[0] = 0x44; return b }), ip.ErrLength},
		{"protocol", enums.SAPIPBasedPacketData, mod(func(b []byte) []byte { b[9] = 6; return b }), ip.ErrProtocol},
		{"fragment", enums.SAPIPBasedPacketData, mod(func(b []byte) []byte { b[6] = 0x20; return b }), ip.ErrFragment},
		{"header checksum", enums.SAPIPBasedPacketData, mod(func(b []byte) []byte { b[8] = 63; return b }), ip.ErrChecksum},
		{"UDP checksum", enums.SAPIPBasedPacketData, mod(func(b []byte) []byte { b[40] ^= 1; return b }), ip.ErrChecksum},
		{"UDP length", enums.SAPIPBasedPacketData, mod(func(b []byte) []byte { b[25] = 0x30; return b }), ip.ErrShort},
		{"compressed short", enums.SAPUDPIPHeaderCompression, []byte{0, 1, 0, 0}, ip.ErrShort},
		{"compressed extended short", enums.SAPUDPIPHeaderCompression, []byte{0, 1, 0, 0, 0, 0x27}, ip.ErrShort},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ip.Decode(tt.sap, tt.data)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Decode = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestAppendIPv4ZeroChecksum(t *testing.T) {
	t.Parallel()

	d := ip.Datagram{SourcePort: 1, DestinationPort: 2, Payload: []byte{0xFF, 0xFF}}
	for p := range 256 {
		d.Payload[0] = byte(p)
		out, err := ip.AppendIPv4(nil, &d)
		if err != nil {
			t.Fatal(err)
		}
		if out[26] == 0 && out[27] == 0 {
			t.Fatalf("payload %x gave UDP checksum 0", d.Payload)
		}
		if _, err := ip.Decode(enums.SAPIPBasedPacketData, out); err != nil {
			t.Fatalf("Decode(AppendIPv4) = %v", err)
		}
	}
}

func TestAppendIPv4TooLong(t *testing.T) {
	t.Parallel()

	d := ip.Datagram{Payload: make([]byte, 65536)}
	out, err := ip.AppendIPv4([]byte{1}, &d)
	if err == nil || len(out) != 1 {
		t.Fatalf("AppendIPv4 = %d octets, %v", len(out), err)
	}
}

func TestAddresses(t *testing.T) {
	t.Parallel()

	if got := ip.GroupAddress(225, 9); got != [4]byte{225, 0, 0, 9} {
		t.Errorf("GroupAddress = %v", got)
	}
	if got := ip.RadioAddress(13, 0x123456); got != [4]byte{13, 0x12, 0x34, 0x56} || ip.ID(got) != 0x123456 {
		t.Errorf("RadioAddress = %v", got)
	}
}

func TestNames(t *testing.T) {
	t.Parallel()

	if ip.PortTMS.String() != "TMS" || ip.Port(1234).String() != "1234" || ip.ProtocolUDP.String() != "UDP" || ip.Protocol(99).String() != "Protocol(99)" {
		t.Error("names")
	}
}
