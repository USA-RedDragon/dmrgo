package pdu_test

import (
	"errors"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestDataHeaderGroupMatchesBytes(t *testing.T) {
	t.Parallel()

	n := 0
	for _, b := range captureBursts(t) {
		if p, _ := b.Sync(); !isDataSync(p) || b.SlotType().DataType != enums.DataTypeDataHeader {
			continue
		}
		payload, _ := b.BPTCPayload()
		n++
		checkGroup(t, payload)
	}
	if n == 0 {
		t.Fatal("no captured data header")
	}
	for format := range byte(16) {
		for _, b0 := range []byte{format, 0x80 | format, 0x40 | format} {
			checkGroup(t, [12]byte{b0, 0x01, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xBC, 0xDE, 0xF0})
		}
	}
}

func checkGroup(t *testing.T, payload [12]byte) {
	t.Helper()
	h := pdu.DecodeDataHeader(payload)
	hb := pdu.DataHeaderBytes(payload)
	g, ok := h.Group()
	wg, wok := hb.Group()
	if g != wg || ok != wok {
		t.Fatalf("%x: Group %v %v, DataHeaderBytes.Group %v %v", payload, g, ok, wg, wok)
	}
	err := h.SetGroup(!g)
	if werr := hb.SetGroup(!g); (err == nil) != (werr == nil) {
		t.Fatalf("%x: SetGroup %v, DataHeaderBytes.SetGroup %v", payload, err, werr)
	}
	if !ok {
		if !errors.Is(err, pdu.ErrNoGroupFlag) || h.RawPayload != [10]byte(payload[:10]) {
			t.Fatalf("%x: SetGroup on a header without the flag: %v %x", payload, err, h.RawPayload)
		}
		return
	}
	if got, _ := h.Group(); got == g || h.RawPayload != [10]byte(hb[:10]) {
		t.Fatalf("%x: after SetGroup(%v): %x, want %x", payload, !g, h.RawPayload, hb[:10])
	}
}

func TestDataHeaderGroupSMS(t *testing.T) {
	t.Parallel()

	h, err := pdu.UnconfirmedDataHeader{Group: true, SAP: enums.SAPIPBasedPacketData, LLIDDestination: 91, LLIDSource: 3120000}.DataHeader()
	if err != nil {
		t.Fatal(err)
	}
	if g, ok := h.Group(); !g || !ok {
		t.Fatalf("Group %v %v", g, ok)
	}
	if err := h.SetGroup(false); err != nil {
		t.Fatal(err)
	}
	if u, _ := h.AsUnconfirmed(); u.Group || u.LLIDDestination != 91 || u.SAP != enums.SAPIPBasedPacketData {
		t.Fatalf("after SetGroup(false): %+v", u)
	}
	var nilHeader *pdu.DataHeader
	if g, ok := nilHeader.Group(); g || ok {
		t.Fatal("nil Group")
	}
	if err := nilHeader.SetGroup(true); err == nil {
		t.Fatal("nil SetGroup: no error")
	}
	var resp pdu.DataHeader
	if err := resp.SetResponse(pdu.ResponsePacketHeader{LLIDDestination: 1, LLIDSource: 2}); err != nil {
		t.Fatal(err)
	}
	if _, ok := resp.Group(); ok {
		t.Fatal("C_RHEAD has a group flag")
	}
}
