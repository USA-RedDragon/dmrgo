package ip_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestCompressedPorts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		spid     enums.SPID
		dpid     enums.DPID
		octets   int
		src, dst ip.Port
	}{
		{enums.SPIDExtendedHeader, enums.DPIDExtendedHeader, 9, 0x1111, 0x2222},
		{enums.SPIDExtendedHeader, enums.DPIDLocationProtocol, 7, 0x1111, 5017},
		{enums.SPIDTextMessage, enums.DPIDExtendedHeader, 7, 5016, 0x1111},
		{enums.SPIDLocationProtocol, enums.DPIDTextMessage, 5, 5017, 5016},
		{enums.SPID(95), enums.DPID(95), 5, 0, 0},
	}
	for _, tt := range tests {
		h := pdu.UDPIPv4CompressedHeader{SPID: tt.spid, DPID: tt.dpid, ExtendedHeader1: 0x1111, ExtendedHeader2: 0x2222}
		src, dst := ip.CompressedPorts(&h)
		if h.Octets() != tt.octets || src != tt.src || dst != tt.dst {
			t.Errorf("SPID %d DPID %d: %d octets, ports %d %d, want %d octets, ports %d %d",
				tt.spid, tt.dpid, h.Octets(), src, dst, tt.octets, tt.src, tt.dst)
		}
	}
	if src, dst := ip.CompressedPorts(nil); src != 0 || dst != 0 {
		t.Errorf("nil header: ports %d %d", src, dst)
	}
}

func TestPortIDs(t *testing.T) {
	t.Parallel()
	for v := range 128 {
		d, s := enums.DPID(v), enums.SPID(v)
		if p := ip.DPIDPort(d); p != 0 {
			if back, ok := ip.PortDPID(p); !ok || back != d {
				t.Errorf("DPID %d: port %d gives DPID %d %t", v, p, back, ok)
			}
		} else if v == 1 || v == 2 {
			t.Errorf("DPID %d has no port", v)
		}
		if p := ip.SPIDPort(s); p != 0 {
			if back, ok := ip.PortSPID(p); !ok || back != s {
				t.Errorf("SPID %d: port %d gives SPID %d %t", v, p, back, ok)
			}
		} else if v == 1 || v == 2 {
			t.Errorf("SPID %d has no port", v)
		}
	}
	if ip.DPIDPort(enums.DPIDTextMessage) != ip.PortText || ip.SPIDPort(enums.SPIDLocationProtocol) != ip.PortLocation {
		t.Error("named ports")
	}
	if d, ok := ip.PortDPID(ip.PortNMEA); ok || d != enums.DPIDExtendedHeader {
		t.Errorf("PortDPID(PortNMEA) = %d %t", d, ok)
	}
	if s, ok := ip.PortSPID(ip.PortNMEA); ok || s != enums.SPIDExtendedHeader {
		t.Errorf("PortSPID(PortNMEA) = %d %t", s, ok)
	}
}
