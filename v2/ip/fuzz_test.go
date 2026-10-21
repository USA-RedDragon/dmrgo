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

func FuzzDecode(f *testing.F) {
	decodeSeeds(func(sap uint8, data []byte) { f.Add(sap, data) })
	f.Fuzz(fuzzDecode)
}

func decodeSeeds(add func(sap uint8, data []byte)) {
	for _, s := range []string{appcapture.TMS, appcapture.DMRText} {
		b, _ := hex.DecodeString(s)
		add(uint8(enums.SAPIPBasedPacketData), b)
	}
	b, _ := hex.DecodeString(appcapture.GPS)
	add(uint8(enums.SAPUDPIPHeaderCompression), b)
	add(uint8(enums.SAPUDPIPHeaderCompression), []byte{0, 1, 0, 1, 2})
	add(uint8(enums.SAPIPBasedPacketData), []byte{0x46, 0, 0, 24})
}

var decodeErrors = []error{ip.ErrSAP, ip.ErrShort, ip.ErrLength, ip.ErrVersion, ip.ErrProtocol, ip.ErrFragment, ip.ErrChecksum}

func fuzzDecode(t *testing.T, sap uint8, data []byte) {
	s := enums.ServiceAccessPointID(sap & 0xF)
	d, err := ip.Decode(s, data)
	if err != nil {
		known := false
		for _, e := range decodeErrors {
			known = known || errors.Is(err, e)
		}
		if !known {
			t.Fatalf("Decode(%d, %x): unexpected error %v", s, data, err)
		}
		if !errors.Is(err, ip.ErrChecksum) {
			return
		}
	}
	var out []byte
	if d.Compressed {
		out, err = ip.AppendCompressed(nil, &d)
	} else {
		out, err = ip.AppendIPv4(nil, &d)
	}
	if err != nil {
		t.Fatalf("append %+v: %v", d, err)
	}
	back, err := ip.Decode(s, out)
	if err != nil {
		t.Fatalf("Decode(append %+v) = %v", d, err)
	}
	if back.SourcePort != d.SourcePort || back.DestinationPort != d.DestinationPort || back.ID != d.ID ||
		back.Source != d.Source || back.Destination != d.Destination || !bytes.Equal(back.Payload, d.Payload) {
		t.Fatalf("round trip %+v gave %+v", d, back)
	}
}

func FuzzDecodeICMPPacket(f *testing.F) {
	icmpSeeds(func(data []byte) { f.Add(data) })
	f.Fuzz(fuzzDecodeICMPPacket)
}

func icmpSeeds(add func(data []byte)) {
	b, _ := hex.DecodeString(hostUnreachable)
	add(b)
	b, _ = hex.DecodeString(appcapture.TMS)
	add(b)
	add([]byte{0x45, 0, 0, 28, 0, 0, 0, 0, 64, 1})
	add([]byte{3, 1, 0, 0})
}

var icmpErrors = []error{ip.ErrShort, ip.ErrLength, ip.ErrVersion, ip.ErrProtocol, ip.ErrFragment, ip.ErrChecksum}

func fuzzDecodeICMPPacket(t *testing.T, data []byte) {
	ip.CheckIPv4(data, len(data)/2)
	if m, err := ip.DecodeICMP(data); err == nil {
		out, err := m.AppendBinary(nil)
		if err != nil || !bytes.Equal(out, data) {
			t.Fatalf("DecodeICMP(%x) round trip %x, %v", data, out, err)
		}
	} else if !errors.Is(err, ip.ErrShort) {
		t.Fatalf("DecodeICMP(%x): %v", data, err)
	}
	c, err := ip.DecodeICMPPacket(data)
	if err != nil {
		known := false
		for _, e := range icmpErrors {
			known = known || errors.Is(err, e)
		}
		if !known {
			t.Fatalf("DecodeICMPPacket(%x): unexpected error %v", data, err)
		}
		if !errors.Is(err, ip.ErrChecksum) {
			return
		}
	}
	out, err := ip.AppendICMPPacket(nil, &c)
	if errors.Is(err, ip.ErrProtocol) {
		return
	}
	if err != nil {
		t.Fatalf("append %+v: %v", c, err)
	}
	back, err := ip.DecodeICMPPacket(out)
	if err != nil {
		t.Fatalf("DecodeICMPPacket(append %+v) = %v", c, err)
	}
	if back.Message != c.Message || back.Pointer != c.Pointer || back.NextHopMTU != c.NextHopMTU || back.ID != c.ID ||
		back.Source != c.Source || back.Destination != c.Destination || !bytes.Equal(back.Original, c.Original[:len(back.Original)]) {
		t.Fatalf("round trip %+v gave %+v", c, back)
	}
}
