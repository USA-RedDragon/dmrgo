package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func dataHeaderGroup(h *pdu.DataHeader) (group, ok bool) {
	switch {
	case h.UnconfirmedDataHeader != nil:
		return h.UnconfirmedDataHeader.Group, true
	case h.ConfirmedDataHeader != nil:
		return h.ConfirmedDataHeader.Group, true
	case h.DefinedDataHeader != nil:
		return h.DefinedDataHeader.Group, true
	case h.RawDataHeader != nil:
		return h.RawDataHeader.Group, true
	case h.StatusPrecodedHeader != nil:
		return h.StatusPrecodedHeader.Group, true
	case h.UDTHeader != nil:
		return h.UDTHeader.Group, true
	}
	return false, false
}

func setDataHeaderGroup(h *pdu.DataHeader, group bool) {
	switch {
	case h.UnconfirmedDataHeader != nil:
		h.UnconfirmedDataHeader.Group = group
	case h.ConfirmedDataHeader != nil:
		h.ConfirmedDataHeader.Group = group
	case h.DefinedDataHeader != nil:
		h.DefinedDataHeader.Group = group
	case h.RawDataHeader != nil:
		h.RawDataHeader.Group = group
	case h.StatusPrecodedHeader != nil:
		h.StatusPrecodedHeader.Group = group
	case h.UDTHeader != nil:
		h.UDTHeader.Group = group
	}
}

func FuzzPDUEditors(f *testing.F) {
	editorSeeds(f, func(kind uint8, data []byte, target, source uint32, group bool, v uint8) {
		f.Add(kind, data, target, source, group, v)
	})
	f.Fuzz(fuzzPDUEditors)
}

func editorSeeds(tb testing.TB, add func(kind uint8, data []byte, target, source uint32, group bool, v uint8)) {
	for _, b := range captureBursts(tb) {
		if p, _ := b.Sync(); !isDataSync(p) {
			continue
		}
		payload, _ := b.BPTCPayload()
		switch b.SlotType().DataType {
		case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC:
			add(uint8(0), payload[:], uint32(9), uint32(3120101), true, uint8(3))
		case enums.DataTypeCSBK:
			add(uint8(1), payload[:], uint32(9), uint32(3120101), false, uint8(0))
		case enums.DataTypeDataHeader:
			add(uint8(2), payload[:], uint32(1<<24), uint32(3120101), true, uint8(0))
		default:
		}
	}
}

func fuzzPDUEditors(t *testing.T, kind uint8, data []byte, target, source uint32, group bool, v uint8) {
	var in [12]byte
	copy(in[:], data)
	switch kind % 3 {
	case 0:
		checkLCBytes(t, pdu.LCBytes(in[:9]), target, source, group, v)
	case 1:
		checkCSBKBytes(t, pdu.CSBKBytes(in), target, source, group)
	case 2:
		checkDataHeaderBytes(t, pdu.DataHeaderBytes(in), target, source, group)
	}
}

func checkLCBytes(t *testing.T, l pdu.LCBytes, target, source uint32, group bool, v uint8) {
	t.Helper()
	dec, err := pdu.DecodeFullLinkControlBytes(l)
	if err != nil {
		t.Fatalf("DecodeFullLinkControlBytes(%x): %v", l, err)
	}
	if dec.FLCO != l.FLCO() || dec.FID != l.FID() || dec.ProtectFlag != l.ProtectFlag() || dec.Reserved != l.Reserved() {
		t.Fatalf("LCBytes %x header fields differ from FullLinkControl %s", l, dec.String())
	}
	wt, ws, wok := dec.Addresses()
	if gt, gs, gok := l.Addresses(); gt != wt || gs != ws || gok != wok {
		t.Fatalf("LCBytes %x Addresses %x %x %t, FullLinkControl %x %x %t", l, gt, gs, gok, wt, ws, wok)
	}
	g, gok := l.Group()
	if gok != wok || gok && g != (l.FLCO() == enums.FLCOGroupVoiceChannelUser) {
		t.Fatalf("LCBytes %x Group %t %t", l, g, gok)
	}

	e, want := l, dec
	errB, errS := e.SetAddresses(target, source), want.SetAddresses(target, source)
	if (errB == nil) != (errS == nil) || (errB == nil) != (wok && target < 1<<24 && source < 1<<24) {
		t.Fatalf("LCBytes %x SetAddresses(%x, %x) = %v, FullLinkControl %v", l, target, source, errB, errS)
	}
	if errB != nil && e != l {
		t.Fatalf("LCBytes %x: a refused SetAddresses changed it to %x", l, e)
	}
	if b := must(want.Bytes()); b != [9]byte(e) {
		t.Fatalf("LCBytes %x SetAddresses gives %x, FullLinkControl %x", l, e, b)
	}
	wantFLCO := enums.FLCOUnitToUnitVoiceChannelUser
	if group {
		wantFLCO = enums.FLCOGroupVoiceChannelUser
	}
	if e.SetGroup(group) != gok || gok && e.FLCO() != wantFLCO {
		t.Fatalf("LCBytes %x SetGroup(%t) gives FLCO %d", l, group, e.FLCO())
	}
	checkLCBytesSetters(t, l, group, v)
}

func checkLCBytesSetters(t *testing.T, l pdu.LCBytes, group bool, v uint8) {
	t.Helper()
	e := l
	if e.SetFLCO(enums.FLCO(v)) == nil != (v < 64) {
		t.Fatalf("SetFLCO(%d) range check", v)
	}
	if v < 64 && (e.FLCO() != enums.FLCO(v) || e[0]&0xC0 != l[0]&0xC0 || [8]byte(e[1:]) != [8]byte(l[1:])) || v >= 64 && e != l {
		t.Fatalf("LCBytes %x SetFLCO(%d) gives %x", l, v, e)
	}
	e = l
	e.SetProtectFlag(group)
	e.SetReserved(!group)
	e.SetFID(enums.FeatureSetID(v))
	e.SetServiceOptions(^v)
	if e.ProtectFlag() != group || e.Reserved() == group || e.FID() != enums.FeatureSetID(v) || e.ServiceOptions() != ^v || e.FLCO() != l.FLCO() || [6]byte(e[3:]) != [6]byte(l[3:]) {
		t.Fatalf("LCBytes %x setters give %x", l, e)
	}
	for _, dt := range []enums.DataType{enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC} {
		if cw := must(e.Codeword(dt)); !pdu.FullLCParityOK(cw, dt) || [9]byte(cw[:9]) != [9]byte(e) {
			t.Fatalf("%s: LCBytes %x Codeword %x fails its parity", dt, e, cw)
		}
	}
	if _, err := e.Codeword(enums.DataTypeCSBK); err == nil {
		t.Fatal("LCBytes.Codeword accepts a CSBK")
	}
}

func checkCSBKBytes(t *testing.T, c pdu.CSBKBytes, target, source uint32, group bool) {
	t.Helper()
	c.Reseal()
	if !c.CRCOK() || !pdu.BlockCRCOK(c, enums.DataTypeCSBK) {
		t.Fatalf("resealed CSBK %x fails its CRC", c)
	}
	dec := pdu.DecodeCSBK(c)
	if dec.FEC.HasErrors() || dec.Opcode != c.Opcode() || dec.FID != c.FID() {
		t.Fatalf("CSBKBytes %x differs from CSBK %s", c, dec.String())
	}
	wt, ws, wok := dec.Addresses()
	if gt, gs, gok := c.Addresses(); gt != wt || gs != ws || gok != wok {
		t.Fatalf("CSBKBytes %x Addresses %x %x %t, CSBK %x %x %t", c, gt, gs, gok, wt, ws, wok)
	}
	wg, wgok := csbkGroup(&dec)
	if g, ok := c.Group(); g != wg || ok != wgok {
		t.Fatalf("CSBKBytes %x Group %t %t, CSBK %t %t", c, g, ok, wg, wgok)
	}
	e, want := c, dec
	errB, errS := e.SetAddresses(target, source), want.SetAddresses(target, source)
	if (errB == nil) != (errS == nil) || errB != nil && e != c {
		t.Fatalf("CSBKBytes %x SetAddresses(%x, %x) = %v, CSBK %v", c, target, source, errB, errS)
	}
	if e.SetGroup(group) != wgok {
		t.Fatalf("CSBKBytes %x SetGroup ok differs from Group", c)
	}
	setCSBKGroup(&want, group)
	e.Reseal()
	if b := must(want.Encode()); b != [12]byte(e) {
		t.Fatalf("CSBKBytes %x edit gives %x, CSBK edit %x", c, e, b)
	}
}

func checkDataHeaderBytes(t *testing.T, h pdu.DataHeaderBytes, target, source uint32, group bool) {
	t.Helper()
	h.Reseal()
	if !h.CRCOK() {
		t.Fatalf("resealed data header %x fails its CRC", h)
	}
	dec := pdu.DecodeDataHeader(h)
	if dec.FEC.HasErrors() || dec.Format != h.Format() {
		t.Fatalf("DataHeaderBytes %x differs from DataHeader %s", h, dec.String())
	}
	wt, ws, wok := dec.Addresses()
	if gt, gs, gok := h.Addresses(); gt != wt || gs != ws || gok != wok {
		t.Fatalf("DataHeaderBytes %x Addresses %x %x %t, DataHeader %x %x %t", h, gt, gs, gok, wt, ws, wok)
	}
	wg, wgok := dataHeaderGroup(&dec)
	if g, ok := h.Group(); g != wg || ok != wgok {
		t.Fatalf("DataHeaderBytes %x Group %t %t, DataHeader %t %t", h, g, ok, wg, wgok)
	}
	e, want := h, dec
	errB, errS := e.SetAddresses(target, source), want.SetAddresses(target, source)
	if (errB == nil) != (errS == nil) || errB != nil && e != h {
		t.Fatalf("DataHeaderBytes %x SetAddresses(%x, %x) = %v, DataHeader %v", h, target, source, errB, errS)
	}
	if e.SetGroup(group) != wgok {
		t.Fatalf("DataHeaderBytes %x SetGroup ok differs from Group", h)
	}
	setDataHeaderGroup(&want, group)
	e.Reseal()
	if b := must(want.Encode()); b != [12]byte(e) {
		t.Fatalf("DataHeaderBytes %x edit gives %x, DataHeader edit %x", h, e, b)
	}
}
