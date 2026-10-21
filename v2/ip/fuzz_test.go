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
