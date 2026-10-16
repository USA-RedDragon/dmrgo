package layer2_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func headerGroup(t *testing.T, h *pdu.DataHeader) bool {
	t.Helper()
	switch {
	case h.UnconfirmedDataHeader != nil:
		return h.UnconfirmedDataHeader.Group
	case h.ConfirmedDataHeader != nil:
		return h.ConfirmedDataHeader.Group
	case h.DefinedDataHeader != nil:
		return h.DefinedDataHeader.Group
	case h.StatusPrecodedHeader != nil:
		return h.StatusPrecodedHeader.Group
	case h.RawDataHeader != nil:
		return h.RawDataHeader.Group
	case h.UDTHeader != nil:
		return h.UDTHeader.Group
	}
	return false
}

func TestDataHeaderBytes_SMSCaptures(t *testing.T) {
	t.Parallel()

	editable := [12]byte{0x80, 0, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0, 0, 0xFF, 0xFF}
	rng := rand.New(rand.NewSource(5))
	formats := map[pdu.Format]int{}
	for _, name := range []string{"d-sms", "h-sms", "m-sms"} {
		for i, frame := range loadBursts(t, "testdata/"+name+".bin") {
			b, _ := layer2.NewBurstFromBytes(frame)
			dh, isHeader := b.Data.(*pdu.DataHeader)
			if !isHeader || b.FEC.PDU.HasErrors() {
				continue
			}
			r := frame
			p, _ := r.BPTCPayload()
			h := pdu.DataHeaderBytes(p)
			if !h.CRCOK() || h.Format() != dh.Format {
				t.Fatalf("%s burst %d: CRC %t, format %s", name, i, h.CRCOK(), h.Format())
			}
			formats[h.Format()]++
			wantT, wantS, wantOK := dh.Addresses()
			gotT, gotS, gotOK := h.Addresses()
			gotG, _ := h.Group()
			if gotT != wantT || gotS != wantS || gotOK != wantOK || gotG != headerGroup(t, dh) {
				t.Fatalf("%s burst %d: Addresses %d %d %t %t, DataHeader %d %d %t %t", name, i, gotT, gotS, gotG, gotOK, wantT, wantS, headerGroup(t, dh), wantOK)
			}

			newT, newS := uint32(rng.Intn(1<<24)), uint32(rng.Intn(1<<24))
			newG := !gotG
			e := h
			_, hasGroup := h.Group()
			if e.SetAddresses(newT, newS) != nil || e.SetGroup(newG) != hasGroup {
				t.Fatalf("%s burst %d: SetAddresses or SetGroup refused for %s", name, i, h.Format())
			}
			if !hasGroup {
				newG = false
			}
			if e.CRCOK() {
				t.Fatalf("%s burst %d: CRC still passes before Reseal", name, i)
			}
			e.Reseal()
			if !e.CRCOK() || !layer2.MMDVMHostAccepts(enums.DataTypeDataHeader, e) {
				t.Fatalf("%s burst %d: resealed header fails its CRC", name, i)
			}
			for j := range e {
				if (e[j]^h[j])&^editable[j] != 0 {
					t.Fatalf("%s burst %d: octet %d changed %02x -> %02x", name, i, j, h[j], e[j])
				}
			}

			r.SetBPTCPayload(e)
			nb, err := layer2.NewBurstFromBytes(r)
			if err != nil {
				t.Fatalf("%s burst %d: %v", name, i, err)
			}
			nh, ok := nb.Data.(*pdu.DataHeader)
			if !ok || nb.FEC.PDU.HasErrors() {
				t.Fatalf("%s burst %d: edited burst does not decode as a clean data header", name, i)
			}
			if dt, ds, ok := nh.Addresses(); !ok || dt != newT || ds != newS || headerGroup(t, nh) != newG {
				t.Fatalf("%s burst %d: edited header decodes as %d %d %t", name, i, dt, ds, headerGroup(t, nh))
			}
		}
	}
	if len(formats) == 0 {
		t.Fatal("no data headers in the SMS captures")
	}
	t.Logf("formats %v", formats)
}

func TestDataHeaderBytes_Formats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format    pdu.Format
		llids, gi bool
	}{
		{pdu.FormatUnifiedDataTransport, true, true},
		{pdu.FormatResponsePacket, true, false},
		{pdu.FormatUnconfirmed, true, true},
		{pdu.FormatConfirmed, true, true},
		{pdu.FormatShortDataDefined, true, true},
		{pdu.FormatShortDataRawOrStatusPrecoded, true, true},
		{pdu.FormatProprietary, false, false},
		{pdu.Format(4), false, false},
	}
	for _, tt := range tests {
		var h pdu.DataHeaderBytes
		h[0] = 0x80 | byte(tt.format)
		h[2], h[7] = 0x12, 0x34
		h.Reseal()
		orig := h
		tgt, src, ok := h.Addresses()
		group, gok := h.Group()
		if ok != tt.llids || gok != tt.gi || group != tt.gi || (ok && (tgt != 0x120000 || src != 0x34)) {
			t.Errorf("format %d: Addresses %x %x %t, Group %t %t", tt.format, tgt, src, ok, group, gok)
		}
		if h.SetGroup(false) != tt.gi || (!tt.gi && h != orig) {
			t.Errorf("format %d: SetGroup", tt.format)
		}
		h = orig
		if (h.SetAddresses(1, 2) == nil) != tt.llids || h[0] != orig[0] {
			t.Errorf("format %d: SetAddresses", tt.format)
		}
		h = orig
		if h.SetAddresses(1<<24, 0) == nil || h.SetAddresses(0, 1<<24) == nil || h != orig {
			t.Errorf("format %d: SetAddresses accepts 25 bits", tt.format)
		}
	}
}
